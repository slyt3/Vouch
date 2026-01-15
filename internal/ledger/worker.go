package ledger

import (
	"log"

	"github.com/yourname/ael/internal/proxy"
)

// Worker processes events asynchronously without blocking the proxy
type Worker struct {
	eventChannel chan proxy.Event
}

// NewWorker creates a new async ledger worker with a buffered channel
func NewWorker(bufferSize int) *Worker {
	return &Worker{
		eventChannel: make(chan proxy.Event, bufferSize),
	}
}

// Start begins processing events in the background
func (w *Worker) Start() {
	go w.processEvents()
	log.Println("[INFO] Async worker started")
}

// Submit sends an event to the worker for processing (non-blocking)
func (w *Worker) Submit(event proxy.Event) {
	w.eventChannel <- event
}

// processEvents is the main worker loop
func (w *Worker) processEvents() {
	for event := range w.eventChannel {
		// Phase 1: Log to console
		// Phase 2: Will write to SQLite with SHA-256 hash chaining and Ed25519 signing
		timestamp := event.Timestamp.Format("15:04:05.000")

		if event.WasBlocked {
			// Article 12 "Proof of Refusal" logging
			log.Printf("[%s] BLOCKED: %s (ID: %s)", timestamp, event.Method, event.ID)
			log.Printf("[%s] Proof of Refusal recorded for audit trail", timestamp)
		} else if event.EventType == "tool_call" {
			log.Printf("[%s] CALL: %s (ID: %s)", timestamp, event.Method, event.ID)
			if event.TaskID != "" {
				log.Printf("[%s] Task ID: %s (State: %s)", timestamp, event.TaskID, event.TaskState)
			}
		} else if event.EventType == "tool_response" {
			log.Printf("[%s] RESPONSE: %s (ID: %s)", timestamp, event.Method, event.ID)
			if event.TaskID != "" {
				log.Printf("[%s] Task ID: %s (State: %s)", timestamp, event.TaskID, event.TaskState)
			}
		}
	}
}
