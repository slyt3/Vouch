package ledger

import (
	"fmt"
	"time"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/proxy"
)

// VerificationResult contains the results of chain verification
type VerificationResult struct {
	Valid        bool
	TotalEvents  int
	ErrorMessage string
	FailedAtSeq  uint64
}

// VerifyChain validates the entire event chain for a given run
func VerifyChain(db *DB, runID string, signer *crypto.Signer) (*VerificationResult, error) {
	if err := assert.Check(runID != "", "runID must not be empty"); err != nil {
		return nil, err
	}
	if err := assert.Check(db != nil, "database connection missing"); err != nil {
		return nil, err
	}
	if err := assert.Check(signer != nil, "signer is nil"); err != nil {
		return nil, err
	}
	result := &VerificationResult{
		Valid: true,
	}

	// Get all events for this run, ordered by sequence
	events, err := db.GetAllEvents(runID)
	if err := assert.Check(err == nil, "database query failed: %v", err); err != nil {
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
	// Safety Assertion: Check signature before hash verification
	if err := assert.Check(event.Signature != "", "event signature must not be empty: id=%s", event.ID); err != nil {
		return err
	}
	if err := assert.Check(event.CurrentHash != "", "event current hash is missing: id=%s", event.ID); err != nil {
		return err
	}
	// 4. Calculate hash using normalized payload and JCS
	tsStr := event.Timestamp.Format(time.RFC3339Nano)
	// Recalculate the hash
	payload := map[string]interface{}{
		"id":         event.ID,
		"run_id":     event.RunID,
		"seq_index":  event.SeqIndex,
		"timestamp":  tsStr, // Use the formatted string
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
	isValid := signer.VerifySignature(calculatedHash, event.Signature)
	if err := assert.Check(isValid, "signature verification failed: id=%s hash=%s", event.ID, calculatedHash); err != nil {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}
