package ledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/proxy"
)

func TestVerifyChain(t *testing.T) {
	// Setup
	tmpDir, _ := os.MkdirTemp("", "vouch-verify-test-*")
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldWd) }()

	schemaContent, _ := os.ReadFile(filepath.Join(oldWd, "../../schema.sql"))
	_ = os.WriteFile("schema.sql", schemaContent, 0644)

	db, _ := NewDB("vouch.db")
	defer db.Close()

	signer, _ := crypto.NewSigner(".vouch_key")

	// Create valid chain
	agentName := "test-agent"

	// Genesis
	genesisID, _ := CreateGenesisBlock(db, signer, agentName)

	// Add some events
	var prevHash string
	_, prevHash, _ = db.GetLastEvent(genesisID)

	event1 := proxy.Event{
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

	err := db.InsertEvent(
		event1.ID, event1.RunID, event1.SeqIndex, event1.Timestamp.Format(time.RFC3339Nano),
		event1.Actor, event1.EventType, event1.Method, `{"foo":"bar"}`, "{}", "", "", "", "", "",
		event1.PrevHash, event1.CurrentHash, event1.Signature,
	)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	// Verify valid chain
	result, err := VerifyChain(db, genesisID, signer)
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
	// 1. Modify an event in database
	_, err = db.conn.Exec("UPDATE events SET method = 'tampered' WHERE id = 'event-1'")
	if err != nil {
		t.Fatalf("Failed to tamper with database: %v", err)
	}

	result, _ = VerifyChain(db, genesisID, signer)
	if result.Valid {
		t.Error("Chain should be invalid after tampering with method")
	}

	// 2. Fix it back
	_, _ = db.conn.Exec("UPDATE events SET method = 'test' WHERE id = 'event-1'")
	result, _ = VerifyChain(db, genesisID, signer)
	if !result.Valid {
		t.Error("Chain should be valid again after fixing tampering")
	}

	// 3. Signature tampering
	_, _ = db.conn.Exec("UPDATE events SET signature = 'invalid' WHERE id = 'event-1'")
	result, _ = VerifyChain(db, genesisID, signer)
	if result.Valid {
		t.Error("Chain should be invalid after signature tampering")
	}
}
