package ledger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/vouch/internal/crypto"
	"github.com/yourname/vouch/internal/pool"
	"github.com/yourname/vouch/internal/proxy"
)

// CreateGenesisBlock creates the initial genesis event for a new run
func CreateGenesisBlock(db *DB, signer *crypto.Signer, agentName string) (string, error) {
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
	if err := insertEvent(db, genesisEvent); err != nil {
		return "", fmt.Errorf("inserting genesis event: %w", err)
	}

	return runID, nil
}

// insertEvent is a helper to insert an event into the database
func insertEvent(db *DB, event *proxy.Event) error {
	paramsBytes, err := json.Marshal(event.Params)
	if err != nil {
		return fmt.Errorf("marshaling params: %w", err)
	}
	responseBytes, err := json.Marshal(event.Response)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}
	paramsJSON := string(paramsBytes)
	responseJSON := string(responseBytes)

	return db.InsertEvent(
		event.ID,
		event.RunID,
		event.SeqIndex,
		event.Timestamp.Format(time.RFC3339Nano),
		event.Actor,
		event.EventType,
		event.Method,
		paramsJSON,
		responseJSON,
		event.TaskID,
		event.TaskState,
		event.ParentID,
		event.PolicyID,
		event.RiskLevel,
		event.PrevHash,
		event.CurrentHash,
		event.Signature,
	)
}
