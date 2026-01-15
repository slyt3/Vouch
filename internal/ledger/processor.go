package ledger

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/vouch/internal/assert"
	"github.com/yourname/vouch/internal/crypto"
	"github.com/yourname/vouch/internal/proxy"
)

// EventProcessor handles the logic for hashing, signing, and state tracking
type EventProcessor struct {
	db         *DB
	signer     *crypto.Signer
	runID      string
	taskStates map[string]string
}

func NewEventProcessor(db *DB, signer *crypto.Signer, runID string) *EventProcessor {
	return &EventProcessor{
		db:         db,
		signer:     signer,
		runID:      runID,
		taskStates: make(map[string]string),
	}
}

// ProcessEvent applies the business logic to a single intercepted event
func (p *EventProcessor) ProcessEvent(event *proxy.Event) error {
	if err := p.persistEvent(event); err != nil {
		return err
	}

	// Log to console for visibility
	timestamp := event.Timestamp.Format("15:04:05.000")

	if event.WasBlocked {
		log.Printf("[%s] BLOCKED | %s | Seq: %d | Hash: %s",
			timestamp, event.Method, event.SeqIndex, event.CurrentHash[:16])
	} else if event.EventType == "tool_call" || event.EventType == "tool_response" {
		log.Printf("[%s] %-8s | %s | Seq: %d | Hash: %s",
			timestamp, event.EventType, event.Method, event.SeqIndex, event.CurrentHash[:16])

		if event.TaskID != "" {
			p.trackTaskState(event)
		}
	}

	return nil
}

func (p *EventProcessor) trackTaskState(event *proxy.Event) {
	oldState, exists := p.taskStates[event.TaskID]
	if exists && oldState != event.TaskState {
		log.Printf("  Task %s: %s -> %s", event.TaskID, oldState, event.TaskState)

		if isTerminalState(event.TaskState) {
			p.createTaskCompletionEvent(event.TaskID, event.TaskState)
			delete(p.taskStates, event.TaskID)
			log.Printf(" [CLEANUP] Task %s state purged from memory", event.TaskID)
		}
	}
	if !isTerminalState(event.TaskState) {
		p.taskStates[event.TaskID] = event.TaskState
	}
}

func isTerminalState(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled"
}

// persistEvent prepares, hashes, signs and stores an event in the database
func (p *EventProcessor) persistEvent(event *proxy.Event) error {
	// 1. Assign sequence index
	stats, err := p.db.GetRunStats(p.runID)
	if err != nil {
		return fmt.Errorf("getting run stats: %w", err)
	}
	event.SeqIndex = stats.TotalEvents
	event.RunID = p.runID

	// 2. Get previous hash
	if event.SeqIndex == 0 {
		event.PrevHash = "0000000000000000000000000000000000000000000000000000000000000000"
	} else {
		_, lastHash, err := p.db.GetLastEvent(p.runID)
		if err != nil {
			return fmt.Errorf("getting last event: %w", err)
		}
		if err := assert.Check(lastHash != "", "prev_hash must be non-empty", "seq", event.SeqIndex); err != nil {
			return err
		}
		event.PrevHash = lastHash
	}

	// 3. Hash and Sign
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

	signature, err := p.signer.SignHash(currentHash)
	if err != nil {
		return fmt.Errorf("signing hash: %w", err)
	}
	event.Signature = signature

	// 4. Store
	return insertEvent(p.db, *event)
}

func (p *EventProcessor) createTaskCompletionEvent(taskID string, state string) {
	event := proxy.Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "task_terminal",
		Method:    "vouch:task_state",
		Params: map[string]interface{}{
			"task_id": taskID,
			"state":   state,
		},
		TaskID:    taskID,
		TaskState: state,
	}
	// We don't recurse here to the worker channel, we process it directly
	_ = p.persistEvent(&event)
}
