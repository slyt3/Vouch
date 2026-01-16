package ledger

import (
	"fmt"
	"log"
	"sync/atomic"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/models"
	"github.com/slyt3/Vouch/internal/pool"
	"github.com/slyt3/Vouch/internal/ring"
)

// Worker processes events asynchronously without blocking the proxy
type Worker struct {
	ringBuffer  *ring.Buffer[*models.Event]
	signalChan  chan struct{} // Signal to wake up processor
	db          EventRepository
	signer      *crypto.Signer
	runID       string
	processor   *EventProcessor
	isUnhealthy atomic.Bool // Health sentinel
}

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

	go w.processEvents()

	return nil
}

// Submit sends an event to the worker for processing (non-blocking)
func (w *Worker) Submit(event *models.Event) {
	// NASA Rule: Check preconditions
	assert.NotNil(event, "event")

	if w.ringBuffer.IsFull() {
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

// Close shuts down the worker and releases resources
func (w *Worker) Close() error {
	close(w.signalChan)
	return w.db.Close()
}

// processEvents is the main worker loop
func (w *Worker) processEvents() {
	for range w.signalChan {
		// Drain buffer
		for !w.ringBuffer.IsEmpty() {
			event, err := w.ringBuffer.Pop()
			if err != nil {
				break
			}
			if err := w.processor.ProcessEvent(event); err != nil {
				log.Printf("[CRITICAL] Event Processing Failure: %v", err)
				w.isUnhealthy.Store(true)
			}
			pool.PutEvent(event)
		}
	}
}
