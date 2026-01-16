package ledger

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/ledger/audit"
	"github.com/slyt3/Vouch/internal/pool"
)

// CreateGenesisBlock creates the initial genesis event for a new run
func CreateGenesisBlock(db EventRepository, signer *crypto.Signer, agentName string) (string, error) {
	// Generate run ID (UUIDv7 for time-ordering)
	runID := uuid.New().String()

	// Create genesis event
	genesisEvent := pool.GetEvent()
	genesisEvent.ID = uuid.New().String()
	genesisEvent.RunID = runID
	genesisEvent.SeqIndex = 0
	genesisEvent.Timestamp = time.Now()
	genesisEvent.Actor = "system"
	genesisEvent.EventType = "genesis"
	genesisEvent.Method = "vouch:init"
	genesisEvent.Params["public_key"] = signer.GetPublicKey()
	genesisEvent.Params["agent_name"] = agentName
	genesisEvent.Params["version"] = "1.0.0"
	genesisEvent.PrevHash = "0000000000000000000000000000000000000000000000000000000000000000" // 64 zeros
	genesisEvent.WasBlocked = false

	// Fetch Bitcoin Anchor (Phase 3)
	anchor, err := audit.FetchBitcoinAnchor()
	if err != nil {
		// Passive: proceed without anchor if fetch fails
	} else {
		genesisEvent.Params["anchor_source"] = anchor.Source
		genesisEvent.Params["anchor_height"] = anchor.BlockHeight
		genesisEvent.Params["anchor_hash"] = anchor.BlockHash
		genesisEvent.Params["anchor_time"] = anchor.Timestamp
	}

	// Calculate genesis hash
	payload := map[string]interface{}{
		"id":         genesisEvent.ID,
		"run_id":     genesisEvent.RunID,
		"seq_index":  genesisEvent.SeqIndex,
		"timestamp":  genesisEvent.Timestamp.Format(time.RFC3339Nano),
		"actor":      genesisEvent.Actor,
		"event_type": genesisEvent.EventType,
		"method":     genesisEvent.Method,
		"params":     genesisEvent.Params,
		"response":   genesisEvent.Response,
		"task_id":    genesisEvent.TaskID,
		"task_state": genesisEvent.TaskState,
		"parent_id":  genesisEvent.ParentID,
		"policy_id":  genesisEvent.PolicyID,
		"risk_level": genesisEvent.RiskLevel,
	}

	currentHash, err := crypto.CalculateEventHash(genesisEvent.PrevHash, payload)
	if err != nil {
		return "", fmt.Errorf("calculating genesis hash: %w", err)
	}
	genesisEvent.CurrentHash = currentHash

	// Sign genesis hash
	signature, err := signer.SignHash(currentHash)
	if err != nil {
		return "", fmt.Errorf("signing genesis hash: %w", err)
	}
	genesisEvent.Signature = signature

	// Insert run record
	if err := db.InsertRun(runID, agentName, currentHash, signer.GetPublicKey()); err != nil {
		return "", fmt.Errorf("inserting run: %w", err)
	}

	// Insert genesis event
	if err := db.StoreEvent(genesisEvent); err != nil {
		return "", fmt.Errorf("inserting genesis event: %w", err)
	}

	return runID, nil
}
