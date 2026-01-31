package ledger

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/crypto"
	"github.com/slyt3/Logryph/internal/models"
	"github.com/slyt3/Logryph/internal/pool"
)

// EventProcessor handles the logic for hashing, signing, and state tracking
type EventProcessor struct {
	db         EventRepository
	signer     *crypto.Signer
	runID      string
	taskStates map[string]string
}

func NewEventProcessor(db EventRepository, signer *crypto.Signer, runID string) *EventProcessor {
	return &EventProcessor{
		db:         db,
		signer:     signer,
		runID:      runID,
		taskStates: make(map[string]string),
	}
}

// ProcessEvent applies the business logic to a single intercepted event
func (p *EventProcessor) ProcessEvent(event *models.Event) error {
	if err := assert.Check(event != nil, "event must not be nil"); err != nil {
		return err
	}
	if err := assert.Check(p.db != nil, "database must be initialized"); err != nil {
		return err
	}
	if err := p.persistEvent(event); err != nil {
		return err
	}

	// Track task state if applicable
	if (event.EventType == "tool_call" || event.EventType == "tool_response") && event.TaskID != "" {
		p.trackTaskState(event)
	}

	return nil
}

func (p *EventProcessor) trackTaskState(event *models.Event) {
	if err := assert.Check(event.TaskID != "", "taskID must not be empty"); err != nil {
		return
	}
	oldState, exists := p.taskStates[event.TaskID]
	if exists && oldState != event.TaskState {
		if isTerminalState(event.TaskState) {
			p.createTaskCompletionEvent(event.TaskID, event.TaskState)
			delete(p.taskStates, event.TaskID)
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
func (p *EventProcessor) persistEvent(event *models.Event) error {
	if err := assert.Check(event != nil, "event must not be nil"); err != nil {
		return err
	}

	// 1. Assign sequence index and validate chain
	if err := p.assignSequenceAndPrevHash(event); err != nil {
		return err
	}

	// 2. Hash and sign the event
	if err := p.hashAndSignEvent(event); err != nil {
		return err
	}

	// 3. Store in database
	return p.db.StoreEvent(event)
}

// assignSequenceAndPrevHash assigns the sequence index and previous hash
func (p *EventProcessor) assignSequenceAndPrevHash(event *models.Event) error {
	if err := assert.Check(event != nil, "event must not be nil"); err != nil {
		return err
	}

	stats, err := p.db.GetRunStats(p.runID)
	if err := assert.Check(err == nil, "failed to get run stats: %v", err); err != nil {
		return fmt.Errorf("getting run stats: %w", err)
	}
	event.SeqIndex = stats.TotalEvents
	event.RunID = p.runID

	lastIndex, lastHash, err := p.db.GetLastEvent(p.runID)
	if err != nil {
		return fmt.Errorf("getting last event: %w", err)
	}

	if event.SeqIndex == 0 {
		if err := assert.Check(lastHash == "", "expected empty last hash for seq 0"); err != nil {
			return err
		}
		event.PrevHash = "0000000000000000000000000000000000000000000000000000000000000000"
	} else {
		if err := assert.Check(lastHash != "", "prev_hash must be non-empty: seq=%d", event.SeqIndex); err != nil {
			return err
		}
		if err := assert.Check(event.SeqIndex == lastIndex+1, "sequence gap detected: prev=%d, curr=%d", lastIndex, event.SeqIndex); err != nil {
			return err
		}
		event.PrevHash = lastHash
	}

	return nil
}

// hashAndSignEvent calculates the hash and signature for the event
func (p *EventProcessor) hashAndSignEvent(event *models.Event) error {
	if err := assert.Check(event != nil, "event must not be nil"); err != nil {
		return err
	}
	if err := assert.Check(event.PrevHash != "", "prev_hash must be set"); err != nil {
		return err
	}

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

	return nil
}

func (p *EventProcessor) createTaskCompletionEvent(taskID string, state string) {
	if err := assert.Check(taskID != "", "taskID must not be empty"); err != nil {
		return
	}
	if err := assert.Check(state != "", "state must not be empty"); err != nil {
		return
	}
	event := pool.GetEvent()
	event.ID = uuid.New().String()[:8]
	event.Timestamp = time.Now()
	event.EventType = "task_terminal"
	event.Method = "logryph:task_state"
	if event.Params == nil {
		event.Params = make(map[string]interface{})
	}
	event.Params["task_id"] = taskID
	event.Params["state"] = state
	event.TaskID = taskID
	event.TaskState = state

	// Direct call to persist
	if err := p.persistEvent(event); err != nil {
		if err2 := assert.Check(false, "persist task completion event failed: %v", err); err2 != nil {
			pool.PutEvent(event)
			return
		}
		pool.PutEvent(event)
		return
	}
	// We don't PutEvent here because this is often called inside Worker loop which will Put it,
	// BUT wait, this is NOT coming from the channel.
	// Actually, persistEvent just reads it. So we can PutEvent here.
	pool.PutEvent(event)
}
