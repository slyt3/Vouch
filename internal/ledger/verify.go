package ledger

import (
	"fmt"
	"time"

	"github.com/yourname/vouch/internal/crypto"
	"github.com/yourname/vouch/internal/proxy"
)

// VerificationResult contains the results of chain verification
type VerificationResult struct {
	Valid        bool
	TotalEvents  int
	ErrorMessage string
	FailedAtSeq  int
}

// VerifyChain validates the entire event chain for a given run
func VerifyChain(db *DB, runID string, signer *crypto.Signer) (*VerificationResult, error) {
	result := &VerificationResult{
		Valid: true,
	}

	// Get all events for this run, ordered by sequence
	events, err := db.GetAllEvents(runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	result.TotalEvents = len(events)

	if len(events) == 0 {
		result.Valid = false
		result.ErrorMessage = "No events found for run"
		return result, nil
	}

	// Verify each event
	for i, event := range events {
		if err := VerifyEvent(&event, signer); err != nil {
			result.Valid = false
			result.ErrorMessage = fmt.Sprintf("Event %d (seq %d) failed verification: %v", i, event.SeqIndex, err)
			result.FailedAtSeq = event.SeqIndex
			return result, nil
		}

		// Verify hash chain linkage (except for genesis)
		if i > 0 {
			prevEvent := events[i-1]
			if event.PrevHash != prevEvent.CurrentHash {
				result.Valid = false
				result.ErrorMessage = fmt.Sprintf("Hash chain broken at seq %d: prev_hash mismatch", event.SeqIndex)
				result.FailedAtSeq = event.SeqIndex
				return result, nil
			}
		}
	}

	return result, nil
}

// VerifyEvent validates a single event's hash and signature
func VerifyEvent(event *proxy.Event, signer *crypto.Signer) error {
	// Recalculate the hash
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

	calculatedHash, err := crypto.CalculateEventHash(event.PrevHash, payload)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Verify hash matches
	if calculatedHash != event.CurrentHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", event.CurrentHash, calculatedHash)
	}

	// Verify signature
	if !signer.VerifySignature(calculatedHash, event.Signature) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
