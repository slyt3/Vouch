package ledger

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/ledger/audit"
	"github.com/slyt3/Vouch/internal/models"
	"github.com/slyt3/Vouch/internal/pool"
	"github.com/slyt3/Vouch/internal/ring"
)

// Worker processes events asynchronously without blocking the proxy
type Worker struct {
	ringBuffer      *ring.Buffer[*models.Event]
	signalChan      chan struct{} // Signal to wake up processor
	quitChan        chan struct{} // Signal to stop background loops
	db              EventRepository
	signer          *crypto.Signer
	runID           string
	processor       *EventProcessor
	isUnhealthy     atomic.Bool   // Health sentinel
	processedEvents atomic.Uint64 // Metrics
	droppedEvents   atomic.Uint64 // Metrics
	closing         atomic.Bool   // Shutdown sentinel
	wg              sync.WaitGroup
	shutdownOnce    sync.Once
}

const (
	maxAnchorTicks   = 1 << 30
	maxSignalBatches = 1 << 30
	maxDrainEvents   = 1 << 20
	maxShutdownTicks = 1 << 12
)

// NewWorker creates a new async ledger worker with a buffered channel.
// It uses dependency injection for the storage layer.
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
		ringBuffer: rb,
		signalChan: make(chan struct{}, 1),
		quitChan:   make(chan struct{}),
		db:         db,
		signer:     signer,
	}, nil
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
		runID, err := CreateGenesisBlock(w.db, w.signer, "Vouch-Agent")
		if err != nil {
			return fmt.Errorf("creating genesis block: %w", err)
		}
		w.runID = runID
		log.Printf("Genesis block created (Run ID: %s)", runID[:8])
	} else {
		runID, err := w.db.GetRunID()
		if err != nil {
			return fmt.Errorf("loading run ID: %w", err)
		}
		w.runID = runID
		log.Printf("Loaded existing run (Run ID: %s)", runID[:8])
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
	// NASA Rule: Check preconditions
	if err := assert.NotNil(event, "event"); err != nil {
		return
	}
	if err := assert.NotNil(w.ringBuffer, "ring buffer"); err != nil {
		return
	}
	if w.closing.Load() {
		w.droppedEvents.Add(1)
		log.Printf("[WARN] Worker shutting down, dropping event %s", event.ID)
		return
	}

	if w.ringBuffer.IsFull() {
		w.droppedEvents.Add(1)
		log.Printf("[BACKPRESSURE] Ring buffer full, dropping event %s", event.ID)
		// Option: In Strict Mode, we would block here.
		// For MVP Asyn Mode, we drop.
		return
	}

	if err := w.ringBuffer.Push(event); err != nil {
		log.Printf("[ERROR] Failed to push to ring buffer: %v", err)
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
	return w.processedEvents.Load(), w.droppedEvents.Load()
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
		log.Printf("[WARN] worker shutdown wait: %v", err)
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
		if err := w.processor.ProcessEvent(event); err != nil {
			log.Printf("[CRITICAL] Event Processing Failure: %v", err)
			w.isUnhealthy.Store(true)
		}
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
				log.Printf("[WARN] Periodic anchor fetch failed: %v", err)
				continue
			}

			event := pool.GetEvent()
			event.ID = uuid.New().String()[:8]
			event.Timestamp = time.Now()
			event.EventType = "anchor"
			event.Method = "vouch:anchor"
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
			if err := w.processor.ProcessEvent(event); err != nil {
				log.Printf("[CRITICAL] Event Processing Failure: %v", err)
				w.isUnhealthy.Store(true)
			}
			w.processedEvents.Add(1)
			pool.PutEvent(event)
		}
	}
	if err := assert.Check(false, "processEvents exceeded max signal batches"); err != nil {
		return
	}
}
