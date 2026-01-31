package ledger

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/crypto"
	"github.com/slyt3/Logryph/internal/ledger/audit"
	"github.com/slyt3/Logryph/internal/logging"
	"github.com/slyt3/Logryph/internal/models"
	"github.com/slyt3/Logryph/internal/pool"
	"github.com/slyt3/Logryph/internal/ring"
)

// BackpressureMode defines how the worker handles full ring buffer scenarios.
type BackpressureMode int

const (
	// BackpressureDrop drops events when buffer is full (fail-open, default).
	BackpressureDrop BackpressureMode = iota
	// BackpressureBlock blocks Submit() until space is available (fail-closed).
	BackpressureBlock
)

// Worker processes events asynchronously via a ring buffer and background goroutine.
// Submissions are non-blocking by default - if the buffer is full, events are dropped
// and metrics are incremented. This ensures agent traffic is never blocked (fail-open).
// Set backpressureMode to BackpressureBlock for fail-closed behavior.
type Worker struct {
	ringBuffer       *ring.Buffer[*models.Event]
	signalChan       chan struct{} // Signal to wake up processor
	quitChan         chan struct{} // Signal to stop background loops
	db               EventRepository
	signer           *crypto.Signer
	runID            string
	processor        *EventProcessor
	backpressureMode BackpressureMode
	isUnhealthy      atomic.Bool   // Health sentinel
	processedEvents  atomic.Uint64 // Metrics
	droppedEvents    atomic.Uint64 // Metrics
	blockedSubmits   atomic.Uint64 // Count of blocked Submit() calls
	latencySumNs     atomic.Uint64 // Latency sum (ns)
	latencyCount     atomic.Uint64 // Latency count
	latencyBuckets   [maxLatencyBuckets]atomic.Uint64
	closing          atomic.Bool // Shutdown sentinel
	wg               sync.WaitGroup
	shutdownOnce     sync.Once
}

const (
	maxAnchorTicks    = 1 << 30
	maxSignalBatches  = 1 << 30
	maxDrainEvents    = 1 << 20
	maxShutdownTicks  = 1 << 12
	maxLatencyBuckets = 7
)

var latencyBucketUpperNs = [maxLatencyBuckets]uint64{
	1 * uint64(time.Millisecond),
	5 * uint64(time.Millisecond),
	10 * uint64(time.Millisecond),
	25 * uint64(time.Millisecond),
	50 * uint64(time.Millisecond),
	100 * uint64(time.Millisecond),
	^uint64(0),
}

// LatencySnapshot captures event processing latency metrics with histogram buckets.
// BoundsNs defines upper bounds in nanoseconds, Counts tracks events per bucket,
// SumNs is total latency, Count is total number of events processed.
type LatencySnapshot struct {
	BoundsNs [maxLatencyBuckets]uint64
	Counts   [maxLatencyBuckets]uint64
	SumNs    uint64
	Count    uint64
}

// NewWorker creates a new async ledger worker with the specified ring buffer size.
// Returns an error if bufferSize <= 0, db is nil, or keyPath is empty.
// The worker must be started with Start() and stopped with Shutdown() for graceful cleanup.
// Uses dependency injection for storage layer to enable testing with mock repositories.
// Default backpressure mode is BackpressureDrop (fail-open).
func NewWorker(bufferSize int, db EventRepository, keyPath string) (*Worker, error) {
	// NASA Rule: Check all parameters
	if err := assert.Check(bufferSize > 0, "buffer size must be positive"); err != nil {
		return nil, err
	}
	if err := assert.Check(db != nil, "database repository missing"); err != nil {
		return nil, err
	}
	if err := assert.Check(keyPath != "", "key path must not be empty"); err != nil {
		return nil, err
	}

	signer, err := crypto.NewSigner(keyPath)
	if err != nil {
		return nil, fmt.Errorf("initializing signer: %w", err)
	}

	rb, err := ring.New[*models.Event](bufferSize)
	if err != nil {
		return nil, err
	}

	return &Worker{
		ringBuffer:       rb,
		signalChan:       make(chan struct{}, 1),
		quitChan:         make(chan struct{}),
		db:               db,
		signer:           signer,
		backpressureMode: BackpressureDrop, // Default: fail-open
	}, nil
}

// SetBackpressureMode configures how Submit() handles full ring buffer.
// Must be called before Start(). Returns error if mode is invalid.
func (w *Worker) SetBackpressureMode(mode BackpressureMode) error {
	if err := assert.NotNil(w, "worker"); err != nil {
		return err
	}
	if err := assert.Check(mode == BackpressureDrop || mode == BackpressureBlock, "invalid backpressure mode"); err != nil {
		return err
	}
	w.backpressureMode = mode
	modeLabel := "drop"
	if mode == BackpressureBlock {
		modeLabel = "block"
	}
	logging.Info("backpressure_mode_set", logging.Fields{
		Component: "worker",
		Method:    "backpressure_mode",
		RiskLevel: modeLabel,
	})
	return nil
}

// BackpressureMode returns the current backpressure handling mode.
func (w *Worker) BackpressureMode() BackpressureMode {
	if err := assert.NotNil(w, "worker"); err != nil {
		return BackpressureDrop
	}
	mode := w.backpressureMode
	if err := assert.Check(mode == BackpressureDrop || mode == BackpressureBlock, "invalid backpressure mode"); err != nil {
		return BackpressureDrop
	}
	return mode
}

// BlockedSubmits returns the number of times Submit() had to wait due to backpressure.
func (w *Worker) BlockedSubmits() uint64 {
	if err := assert.NotNil(w, "worker"); err != nil {
		return 0
	}
	count := w.blockedSubmits.Load()
	return count
}

func (w *Worker) GetDB() EventRepository {
	return w.db
}

func (w *Worker) IsHealthy() bool {
	if err := assert.Check(w != nil, "worker handle is nil"); err != nil {
		return false
	}
	return !w.isUnhealthy.Load()
}

func (w *Worker) GetSigner() *crypto.Signer {
	return w.signer
}

// Start initializes the worker, loads existing runs or creates a genesis block, and starts event processing.
func (w *Worker) Start() error {
	if err := assert.NotNil(w, "worker"); err != nil {
		return err
	}
	if err := assert.NotNil(w.db, "database"); err != nil {
		return err
	}
	hasRuns, err := w.db.HasRuns()
	if err != nil {
		return fmt.Errorf("checking for existing runs: %w", err)
	}

	if !hasRuns {
		runID, err := CreateGenesisBlock(w.db, w.signer, "Logryph-Agent")
		if err != nil {
			return fmt.Errorf("creating genesis block: %w", err)
		}
		w.runID = runID
		logging.Info("genesis_created", logging.Fields{Component: "worker", RunID: runID})
	} else {
		runID, err := w.db.GetRunID()
		if err != nil {
			return fmt.Errorf("loading run ID: %w", err)
		}
		w.runID = runID
		logging.Info("run_loaded", logging.Fields{Component: "worker", RunID: runID})
	}

	w.processor = NewEventProcessor(w.db, w.signer, w.runID)
	w.closing.Store(false)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.processEvents()
	}()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.anchorLoop()
	}()

	return nil
}

// Submit sends an event to the worker for processing (non-blocking)
func (w *Worker) Submit(event *models.Event) {
	// Check preconditions
	if err := assert.NotNil(event, "event"); err != nil {
		return
	}
	if err := assert.NotNil(w.ringBuffer, "ring buffer"); err != nil {
		return
	}
	if w.closing.Load() {
		w.droppedEvents.Add(1)
		logging.Warn("event_dropped_shutdown", logging.Fields{Component: "worker", EventID: event.ID, TaskID: event.TaskID})
		return
	}

	// Backpressure handling based on configured mode
	if w.backpressureMode == BackpressureBlock {
		// Blocking mode: wait until space is available
		const maxBlockAttempts = 1000
		for i := 0; i < maxBlockAttempts; i++ {
			if !w.ringBuffer.IsFull() {
				break
			}
			if w.closing.Load() {
				w.droppedEvents.Add(1)
				logging.Warn("event_dropped_shutdown_blocking", logging.Fields{Component: "worker", EventID: event.ID})
				return
			}
			w.blockedSubmits.Add(1)
			time.Sleep(1 * time.Millisecond)
		}
		if w.ringBuffer.IsFull() {
			w.droppedEvents.Add(1)
			logging.Error("event_dropped_block_timeout", logging.Fields{Component: "worker", EventID: event.ID})
			return
		}
	} else {
		// Drop mode: fail-open, drop event if buffer full
		if w.ringBuffer.IsFull() {
			w.droppedEvents.Add(1)
			logging.Warn("event_dropped_backpressure", logging.Fields{Component: "worker", EventID: event.ID, TaskID: event.TaskID})
			return
		}
	}

	if err := w.ringBuffer.Push(event); err != nil {
		logging.Error("ring_buffer_push_failed", logging.Fields{Component: "worker", Error: err.Error()})
		return
	}

	// Notify worker (non-blocking send)
	select {
	case w.signalChan <- struct{}{}:
	default:
		// Already signaled
	}
}

// Stats returns worker performance metrics
func (w *Worker) Stats() (processed, dropped uint64) {
	if err := assert.NotNil(w, "worker"); err != nil {
		return 0, 0
	}
	return w.processedEvents.Load(), w.droppedEvents.Load()
}

// QueueDepth returns the current queue depth and capacity.
func (w *Worker) QueueDepth() (int, int) {
	if err := assert.NotNil(w, "worker"); err != nil {
		return 0, 0
	}
	if err := assert.NotNil(w.ringBuffer, "ring buffer"); err != nil {
		return 0, 0
	}

	return w.ringBuffer.Len(), w.ringBuffer.Cap()
}

// LatencyMetrics returns a snapshot of latency histogram data.
func (w *Worker) LatencyMetrics() LatencySnapshot {
	if err := assert.NotNil(w, "worker"); err != nil {
		return LatencySnapshot{}
	}
	if err := assert.Check(maxLatencyBuckets > 0, "latency buckets must be positive"); err != nil {
		return LatencySnapshot{}
	}

	var snap LatencySnapshot
	for i := 0; i < maxLatencyBuckets; i++ {
		snap.BoundsNs[i] = latencyBucketUpperNs[i]
		snap.Counts[i] = w.latencyBuckets[i].Load()
	}
	snap.SumNs = w.latencySumNs.Load()
	snap.Count = w.latencyCount.Load()
	return snap
}

// Close shuts down the worker and releases resources
func (w *Worker) Close() error {
	if err := assert.NotNil(w, "worker"); err != nil {
		return err
	}
	if err := assert.NotNil(w.db, "database"); err != nil {
		return err
	}
	return w.Shutdown(5 * time.Second)
}

// Shutdown drains pending events, stops background loops, and closes the database.
func (w *Worker) Shutdown(timeout time.Duration) error {
	if err := assert.NotNil(w, "worker"); err != nil {
		return err
	}
	if err := assert.Check(timeout > 0, "timeout must be positive"); err != nil {
		return err
	}

	w.closing.Store(true)
	w.shutdownOnce.Do(func() {
		close(w.quitChan)
		close(w.signalChan)
	})

	if err := w.waitForStop(timeout); err != nil {
		logging.Warn("shutdown_wait_timeout", logging.Fields{Component: "worker", Error: err.Error()})
	}
	if err := w.drainBuffer(); err != nil {
		return err
	}

	return w.db.Close()
}

func (w *Worker) waitForStop(timeout time.Duration) error {
	if err := assert.NotNil(w, "worker"); err != nil {
		return err
	}
	if err := assert.Check(timeout > 0, "timeout must be positive"); err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	step := timeout / maxShutdownTicks
	if step == 0 {
		step = time.Millisecond
	}
	if err := assert.Check(step > 0, "shutdown step must be positive"); err != nil {
		return err
	}

	ticker := time.NewTicker(step)
	defer ticker.Stop()

	for i := 0; i < maxShutdownTicks; i++ {
		select {
		case <-done:
			return nil
		case <-ticker.C:
		}
	}
	return fmt.Errorf("worker shutdown wait exceeded timeout")
}

func (w *Worker) drainBuffer() error {
	if err := assert.NotNil(w, "worker"); err != nil {
		return err
	}
	if err := assert.NotNil(w.processor, "processor"); err != nil {
		return err
	}
	if err := assert.NotNil(w.ringBuffer, "ring buffer"); err != nil {
		return err
	}

	if err := assert.Check(w.ringBuffer.Cap() <= maxDrainEvents, "ring buffer cap exceeds max: %d", w.ringBuffer.Cap()); err != nil {
		w.isUnhealthy.Store(true)
		return err
	}

	for j := 0; j < maxDrainEvents; j++ {
		if w.ringBuffer.IsEmpty() {
			break
		}
		event, err := w.ringBuffer.Pop()
		if err != nil {
			break
		}
		start := time.Now()
		if err := w.processor.ProcessEvent(event); err != nil {
			logging.Critical("event_processing_failed", logging.Fields{Component: "worker", EventID: event.ID, TaskID: event.TaskID, Error: err.Error()})
			w.isUnhealthy.Store(true)
		}
		w.recordLatency(time.Since(start))
		w.processedEvents.Add(1)
		pool.PutEvent(event)
	}
	return nil
}

func (w *Worker) anchorLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for i := 0; i < maxAnchorTicks; i++ {
		select {
		case <-ticker.C:
			anchor, err := audit.FetchBitcoinAnchor()
			if err != nil {
				logging.Warn("anchor_fetch_failed", logging.Fields{Component: "worker", Error: err.Error()})
				continue
			}

			event := pool.GetEvent()
			event.ID = uuid.New().String()[:8]
			event.Timestamp = time.Now()
			event.EventType = "anchor"
			event.Method = "logryph:anchor"
			event.Actor = "system"
			if event.Params == nil {
				event.Params = make(map[string]interface{})
			}
			event.Params["anchor_source"] = anchor.Source
			event.Params["anchor_height"] = anchor.BlockHeight
			event.Params["anchor_hash"] = anchor.BlockHash
			event.Params["anchor_time"] = anchor.Timestamp

			w.Submit(event)
		case <-w.quitChan:
			return
		}
	}
	if err := assert.Check(false, "anchor loop exceeded max ticks"); err != nil {
		return
	}
}

// processEvents is the main worker loop
func (w *Worker) processEvents() {
	for i := 0; i < maxSignalBatches; i++ {
		_, ok := <-w.signalChan
		if !ok {
			return
		}
		// Drain buffer
		if err := assert.Check(w.ringBuffer.Cap() <= maxDrainEvents, "ring buffer cap exceeds max: %d", w.ringBuffer.Cap()); err != nil {
			w.isUnhealthy.Store(true)
			return
		}
		for j := 0; j < maxDrainEvents; j++ {
			if w.ringBuffer.IsEmpty() {
				break
			}
			event, err := w.ringBuffer.Pop()
			if err != nil {
				break
			}
			start := time.Now()
			if err := w.processor.ProcessEvent(event); err != nil {
				logging.Critical("event_processing_failed", logging.Fields{Component: "worker", EventID: event.ID, TaskID: event.TaskID, Error: err.Error()})
				w.isUnhealthy.Store(true)
			}
			w.recordLatency(time.Since(start))
			w.processedEvents.Add(1)
			pool.PutEvent(event)
		}
	}
	if err := assert.Check(false, "processEvents exceeded max signal batches"); err != nil {
		return
	}
}

func (w *Worker) recordLatency(d time.Duration) {
	if err := assert.NotNil(w, "worker"); err != nil {
		return
	}
	if err := assert.Check(d >= 0, "latency duration must be non-negative"); err != nil {
		return
	}

	if err := assert.Check(maxLatencyBuckets > 0, "latency buckets must be positive"); err != nil {
		return
	}

	if d == 0 {
		w.latencySumNs.Add(0)
		w.latencyCount.Add(1)
		w.latencyBuckets[0].Add(1)
		return
	}

	latencyNs := uint64(d.Nanoseconds())
	for i := 0; i < maxLatencyBuckets; i++ {
		if latencyNs <= latencyBucketUpperNs[i] {
			w.latencyBuckets[i].Add(1)
			break
		}
	}
	w.latencySumNs.Add(latencyNs)
	w.latencyCount.Add(1)
}
