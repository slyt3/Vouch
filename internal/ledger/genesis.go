package ledger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/ael/internal/crypto"
	"github.com/yourname/ael/internal/proxy"
)

// CreateGenesisBlock creates the initial genesis event for a new run
func CreateGenesisBlock(db *DB, signer *crypto.Signer, agentName string) (string, error) {
	// Generate run ID (UUIDv7 for time-ordering)
	runID := uuid.New().String()

	// Create genesis event
	genesisEvent := proxy.Event{
		ID:        uuid.New().String(),
		RunID:     runID,
		SeqIndex:  0,
		Timestamp: time.Now(),
		Actor:     "system",
		EventType: "genesis",
		Method:    "ael:init",
		Params: map[string]interface{}{
			"public_key": signer.GetPublicKey(),
			"agent_name": agentName,
			"version":    "1.0.0",
		},
		Response:   map[string]interface{}{},
		PrevHash:   "0000000000000000000000000000000000000000000000000000000000000000", // 64 zeros
		WasBlocked: false,
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
func insertEvent(db *DB, event proxy.Event) error {
	paramsBytes, _ := json.Marshal(event.Params)
	responseBytes, _ := json.Marshal(event.Response)
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
		event.PrevHash,
		event.CurrentHash,
		event.Signature,
	)
}
