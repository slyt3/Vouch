package audit

import (
	"fmt"
	"net/http"
	"time"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/models"
)

// EventReader defines the subset of ledger operations needed for verification.
type EventReader interface {
	GetAllEvents(runID string) ([]models.Event, error)
}

// VerificationResult contains the results of chain verification
type VerificationResult struct {
	Valid        bool
	TotalEvents  int
	ErrorMessage string
	FailedAtSeq  uint64
}

// VerifyChain validates the entire event chain for a given run
func VerifyChain(db EventReader, runID string, signer *crypto.Signer) (*VerificationResult, error) {
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
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	result.TotalEvents = len(events)

	if len(events) == 0 {
		result.Valid = false
		result.ErrorMessage = ErrNoEvents.Error()
		return result, nil
	}

	// Verify each event
	for i, event := range events {
		// Verify hash chain linkage (except for genesis)
		if i > 0 {
			prevEvent := events[i-1]
			if event.PrevHash != prevEvent.CurrentHash {
				result.Valid = false
				result.ErrorMessage = ErrChainTampered.Error()
				result.FailedAtSeq = event.SeqIndex
				return result, nil
			}
		}

		if err := VerifyEvent(&event, signer); err != nil {
			result.Valid = false
			result.ErrorMessage = fmt.Sprintf("Event %d (seq %d) failed verification: %v", i, event.SeqIndex, err)
			result.FailedAtSeq = event.SeqIndex
			return result, nil
		}
	}

	return result, nil
}

// VerifyEvent validates a single event's hash and signature
func VerifyEvent(event *models.Event, signer *crypto.Signer) error {
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
		return ErrHashMismatch
	}

	// Verify signature
	isValid := signer.VerifySignature(calculatedHash, event.Signature)
	if !isValid {
		return ErrInvalidSignature
	}

	return nil
}

// AnchorVerificationResult contains details about anchor validation
type AnchorVerificationResult struct {
	Valid          bool
	AnchorsChecked int
	ErrorMessage   string
}

// VerifyAnchors validates all anchor events in the ledger against the Bitcoin blockchain
func VerifyAnchors(db EventReader, runID string) (*AnchorVerificationResult, error) {
	events, err := db.GetAllEvents(runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get events for anchor verification: %w", err)
	}

	result := &AnchorVerificationResult{Valid: true}

	for _, event := range events {
		if event.EventType != "genesis" && event.EventType != "anchor" {
			continue
		}

		anchorHash, okHash := event.Params["anchor_hash"].(string)
		anchorHeight, okHeight := event.Params["anchor_height"].(float64) // JSON numbers are float64

		if !okHash || !okHeight {
			continue
		}

		result.AnchorsChecked++

		// Verify against live API
		liveAnchor, err := FetchBitcoinAnchorAtHeight(uint64(anchorHeight))
		if err != nil {
			result.Valid = false
			result.ErrorMessage = fmt.Sprintf("failed to verify anchor at height %d: %v", uint64(anchorHeight), err)
			return result, nil
		}

		if liveAnchor.BlockHash != anchorHash {
			result.Valid = false
			result.ErrorMessage = fmt.Sprintf("anchor mismatch at height %d: ledger=%s, live=%s", uint64(anchorHeight), anchorHash, liveAnchor.BlockHash)
			return result, nil
		}
	}

	return result, nil
}

// FetchBitcoinAnchorAtHeight retrieves the block hash for a specific height
func FetchBitcoinAnchorAtHeight(height uint64) (*Anchor, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://blockstream.info/api/block-height/%d", height))
	if err != nil {
		return nil, fmt.Errorf("fetching block hash: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("blockstream api error: %d", resp.StatusCode)
	}

	var hash string
	if _, err := fmt.Fscan(resp.Body, &hash); err != nil {
		return nil, err
	}

	return &Anchor{
		Source:      "bitcoin-mainnet",
		BlockHeight: height,
		BlockHash:   hash,
		Timestamp:   time.Now(),
	}, nil
}
