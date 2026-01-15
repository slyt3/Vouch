package ledger

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/vouch/internal/crypto"
	"github.com/yourname/vouch/internal/proxy"
)

// Worker processes events asynchronously without blocking the proxy
type Worker struct {
	eventChannel chan proxy.Event
	db           *DB
	signer       *crypto.Signer
	runID        string
	taskStates   map[string]string // Track task state changes
}

// NewWorker creates a new async ledger worker with a buffered channel
func NewWorker(bufferSize int, dbPath, keyPath string) (*Worker, error) {
	// Initialize database
	db, err := NewDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// Initialize signer (loads or generates keypair)
	signer, err := crypto.NewSigner(keyPath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing signer: %w", err)
	}

	return &Worker{
		eventChannel: make(chan proxy.Event, bufferSize),
		db:           db,
		signer:       signer,
		taskStates:   make(map[string]string),
	}, nil
}

// GetDB returns the database instance
func (w *Worker) GetDB() *DB {
	return w.db
}

// GetSigner returns the signer instance
func (w *Worker) GetSigner() *crypto.Signer {
	return w.signer
}

// Start begins processing events in the background
func (w *Worker) Start() error {
	// Check if we need to create genesis block
	hasRuns, err := w.db.HasRuns()
	if err != nil {
		return fmt.Errorf("checking for existing runs: %w", err)
	}

	if !hasRuns {
		// Create genesis block
		runID, err := CreateGenesisBlock(w.db, w.signer, "Vouch-Agent")
		if err != nil {
			return fmt.Errorf("creating genesis block: %w", err)
		}
		w.runID = runID
		log.Printf("Genesis block created (Run ID: %s)", runID[:8])
		log.Printf("Public key: %s", w.signer.GetPublicKey()[:32]+"...")
	} else {
		// Load existing run ID
		runID, err := w.db.GetRunID()
		if err != nil {
			return fmt.Errorf("loading run ID: %w", err)
		}
		w.runID = runID
		log.Printf("Loaded existing run (Run ID: %s)", runID[:8])
	}

	// Start async worker
	go w.processEvents()
	log.Println("Async worker started with database persistence")

	return nil
}

// Submit sends an event to the worker for processing (non-blocking)
func (w *Worker) Submit(event proxy.Event) {
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
		if err := w.persistEvent(&event); err != nil {
			log.Printf("Error persisting event: %v", err)
			continue
		}

		// Log to console for visibility
		timestamp := event.Timestamp.Format("15:04:05.000")

		if event.WasBlocked {
			log.Printf("[%s] BLOCKED | %s | Seq: %d | Hash: %s",
				timestamp, event.Method, event.SeqIndex, event.CurrentHash[:16])
		} else if event.EventType == "tool_call" {
			log.Printf("[%s] CALL    | %s | Seq: %d | Hash: %s",
				timestamp, event.Method, event.SeqIndex, event.CurrentHash[:16])
			if event.TaskID != "" {
				log.Printf("  Task ID: %s (State: %s)", event.TaskID, event.TaskState)
			}
		} else if event.EventType == "tool_response" {
			log.Printf("[%s] RESPONSE| %s | Seq: %d | Hash: %s",
				timestamp, event.Method, event.SeqIndex, event.CurrentHash[:16])
			if event.TaskID != "" {
				// Check for task state change
				oldState, exists := w.taskStates[event.TaskID]
				if exists && oldState != event.TaskState {
					log.Printf("  Task %s: %s -> %s", event.TaskID, oldState, event.TaskState)

					// If task completed, create a task_completed event
					if event.TaskState == "completed" || event.TaskState == "failed" || event.TaskState == "cancelled" {
						w.createTaskCompletionEvent(event.TaskID, event.TaskState)
					}
				}
				w.taskStates[event.TaskID] = event.TaskState
			}
		}
	}
}

// persistEvent saves an event to the database with hash-chaining and signing
func (w *Worker) persistEvent(event *proxy.Event) error {
	// Set run ID
	event.RunID = w.runID

	// Set actor if not set
	if event.Actor == "" {
		event.Actor = "agent"
	}

	// Get previous event's hash and seq_index
	lastSeqIndex, lastHash, err := w.db.GetLastEvent(w.runID)
	if err != nil {
		return fmt.Errorf("getting last event: %w", err)
	}

	// Set sequence index and prev_hash
	event.SeqIndex = lastSeqIndex + 1
	event.PrevHash = lastHash

	// Calculate current hash using canonical JSON
	payload := map[string]interface{}{
		"id":         event.ID,
		"run_id":     event.RunID,
		"seq_index":  event.SeqIndex,
		"timestamp":  event.Timestamp.Format(time.RFC3339Nano),
		"actor":      event.Actor,
		"event_type": event.EventType,
		"method":     event.Method,
		"params":     event.Params,
		"response":   event.Response,
		"task_id":    event.TaskID,
		"task_state": event.TaskState,
		"parent_id":  event.ParentID,
		"policy_id":  event.PolicyID,
		"risk_level": event.RiskLevel,
	}

	currentHash, err := crypto.CalculateEventHash(event.PrevHash, payload)
	if err != nil {
		return fmt.Errorf("calculating hash: %w", err)
	}
	event.CurrentHash = currentHash

	// Sign the hash
	signature, err := w.signer.SignHash(currentHash)
	if err != nil {
		return fmt.Errorf("signing hash: %w", err)
	}
	event.Signature = signature

	// Insert into database
	return insertEvent(w.db, *event)
}

// createTaskCompletionEvent creates a task_completed event when a task finishes
func (w *Worker) createTaskCompletionEvent(taskID, finalState string) {
	completionEvent := proxy.Event{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		EventType: "task_completed",
		Method:    "vouch:task_completed",
		Params: map[string]interface{}{
			"task_id":     taskID,
			"final_state": finalState,
		},
		Response:  map[string]interface{}{},
		TaskID:    taskID,
		TaskState: finalState,
	}

	// Submit back to the channel for processing
	w.eventChannel <- completionEvent
}
