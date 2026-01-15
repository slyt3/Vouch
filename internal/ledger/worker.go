package ledger

import (
	"fmt"
	"log"
	"sync/atomic"

	"github.com/yourname/vouch/internal/crypto"
	"github.com/yourname/vouch/internal/proxy"
)

// Worker processes events asynchronously without blocking the proxy
type Worker struct {
	eventChannel chan proxy.Event
	db           *DB
	signer       *crypto.Signer
	runID        string
	processor    *EventProcessor
	isUnhealthy  atomic.Bool // Health sentinel
}

// NewWorker creates a new async ledger worker with a buffered channel
func NewWorker(bufferSize int, dbPath, keyPath string) (*Worker, error) {
	db, err := NewDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	signer, err := crypto.NewSigner(keyPath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing signer: %w", err)
	}

	return &Worker{
		eventChannel: make(chan proxy.Event, bufferSize),
		db:           db,
		signer:       signer,
	}, nil
}

func (w *Worker) GetDB() *DB {
	return w.db
}

func (w *Worker) IsHealthy() bool {
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
	log.Println("Async worker started with decoupled EventProcessor")

	return nil
}

// Submit sends an event to the worker for processing (non-blocking)
func (w *Worker) Submit(event proxy.Event) {
	capacity := cap(w.eventChannel)
	current := len(w.eventChannel)
	if capacity > 0 && float64(current)/float64(capacity) >= 0.8 {
		log.Printf("[BACKPRESSURE] Ledger buffer at %d/%d (>=80%%) capacity", current, capacity)
	}

	w.eventChannel <- event
}

// Close shuts down the worker and closes the database
func (w *Worker) Close() error {
	close(w.eventChannel)
	return w.db.Close()
}

// processEvents is the main worker loop
func (w *Worker) processEvents() {
	for event := range w.eventChannel {
		if err := w.processor.ProcessEvent(&event); err != nil {
			log.Printf("[CRITICAL] Event Processing Failure: %v", err)
			w.isUnhealthy.Store(true)
			continue
		}
	}
}
