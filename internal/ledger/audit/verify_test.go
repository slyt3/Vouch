package audit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/crypto"
	"github.com/slyt3/Logryph/internal/ledger"
	"github.com/slyt3/Logryph/internal/ledger/audit"
	"github.com/slyt3/Logryph/internal/ledger/store"
	"github.com/slyt3/Logryph/internal/models"
)

func TestVerifyChain(t *testing.T) {
	// Disable strict assertions for this test as we test failure conditions
	assert.StrictMode = false
	defer func() { assert.StrictMode = true }()

	// Setup
	tmpDir, _ := os.MkdirTemp("", "logryph-verify-test-*")
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("failed to remove temp dir: %v", err)
		}
	})

	db, err := store.NewDB(filepath.Join(tmpDir, "logryph.db"))
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	signer, _ := crypto.NewSigner(".logryph_key")

	// Create valid chain
	agentName := "test-agent"

	// Genesis
	genesisID, err := ledger.CreateGenesisBlock(db, signer, agentName)
	if err != nil {
		t.Fatalf("CreateGenesisBlock failed: %v", err)
	}
	if genesisID == "" {
		t.Fatal("CreateGenesisBlock returned empty ID")
	}

	// Add some events
	var prevHash string
	_, prevHash, err = db.GetLastEvent(genesisID)
	if err != nil {
		t.Fatalf("GetLastEvent failed: %v", err)
	}

	event1 := models.Event{
		ID:        "event-1",
		RunID:     genesisID,
		SeqIndex:  1,
		Timestamp: time.Now(),
		Actor:     "agent",
		EventType: "call",
		Method:    "test",
		Params:    map[string]interface{}{"foo": "bar"},
		Response:  map[string]interface{}{},
		PrevHash:  prevHash,
	}

	payload1 := map[string]interface{}{
		"id":         event1.ID,
		"run_id":     event1.RunID,
		"seq_index":  event1.SeqIndex,
		"timestamp":  event1.Timestamp.Format(time.RFC3339Nano),
		"actor":      event1.Actor,
		"event_type": event1.EventType,
		"method":     event1.Method,
		"params":     event1.Params,
		"response":   event1.Response,
		"task_id":    "",
		"task_state": "",
		"parent_id":  "",
		"policy_id":  "",
		"risk_level": "",
	}

	h1, _ := crypto.CalculateEventHash(event1.PrevHash, payload1)
	event1.CurrentHash = h1
	s1, _ := signer.SignHash(h1)
	event1.Signature = s1

	err = db.InsertEvent(
		event1.ID, event1.RunID, event1.SeqIndex, event1.Timestamp.Format(time.RFC3339Nano),
		event1.Actor, event1.EventType, event1.Method, `{"foo":"bar"}`, "{}", "", "", "", "", "",
		event1.PrevHash, event1.CurrentHash, event1.Signature,
	)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	// Verify valid chain
	result, err := audit.VerifyChain(db, genesisID, signer)
	if err != nil {
		t.Fatalf("VerifyChain returned error: %v", err)
	}
	if !result.Valid {
		t.Errorf("Chain should be valid, but failed: %s", result.ErrorMessage)
	}
	if result.TotalEvents != 2 {
		t.Errorf("Expected 2 events verified, got %d", result.TotalEvents)
	}

	// Test Tampering
	// Helper to modify an event in database via SQL since db is store.DB which has unexported conn,
	// BUT wait, db.conn is unexported. I should maybe export it or use a raw connection.
	// Actually, for tests, I'll just open a raw connection to the same file.

	// Test signature tampering
	err = db.InsertEvent(
		"event-2", genesisID, 2, time.Now().Format(time.RFC3339Nano),
		"agent", "call", "test2", "{}", "{}", "", "", "", "", "",
		event1.CurrentHash, "hash2", "invalid_sig",
	)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	result, _ = audit.VerifyChain(db, genesisID, signer)
	if result.Valid {
		t.Error("Chain should be invalid after signature tampering")
	}
}
