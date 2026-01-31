package ledger

import (
	"os"
	"testing"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/crypto"
	"github.com/slyt3/Logryph/internal/models"
)

// mockEventRepository is a minimal mock for testing
type mockEventRepository struct {
	lastSeq  uint64
	lastHash string
	events   []*models.Event
}

func (m *mockEventRepository) StoreEvent(event *models.Event) error {
	if event == nil {
		return assert.Check(false, "event is nil")
	}
	m.events = append(m.events, event)
	m.lastSeq = event.SeqIndex
	m.lastHash = event.CurrentHash
	return nil
}

func (m *mockEventRepository) InsertRun(id, agent, genesisHash, pubKey string) error {
	return nil
}

func (m *mockEventRepository) GetLastEvent(runID string) (uint64, string, error) {
	if len(m.events) == 0 {
		return 0, "", nil
	}
	last := m.events[len(m.events)-1]
	return last.SeqIndex, last.CurrentHash, nil
}

func (m *mockEventRepository) GetEventByID(eventID string) (*models.Event, error) {
	return nil, nil
}

func (m *mockEventRepository) GetAllEvents(runID string) ([]models.Event, error) {
	result := make([]models.Event, len(m.events))
	for i, e := range m.events {
		result[i] = *e
	}
	return result, nil
}

func (m *mockEventRepository) GetRecentEvents(runID string, limit int) ([]models.Event, error) {
	return nil, nil
}

func (m *mockEventRepository) GetEventsByTaskID(taskID string) ([]models.Event, error) {
	return nil, nil
}

func (m *mockEventRepository) GetRiskEvents() ([]models.Event, error) {
	return nil, nil
}

func (m *mockEventRepository) HasRuns() (bool, error) {
	return len(m.events) > 0, nil
}

func (m *mockEventRepository) GetRunID() (string, error) {
	return "test-run", nil
}

func (m *mockEventRepository) GetRunInfo(runID string) (agent, genesisHash, pubKey string, err error) {
	return "", "", "", nil
}

func (m *mockEventRepository) GetRunStats(runID string) (*RunStats, error) {
	// Use lastSeq if set (for gap testing), otherwise use events length
	totalEvents := uint64(len(m.events))
	if m.lastSeq > 0 {
		totalEvents = m.lastSeq + 1 // lastSeq is 0-based
	}
	return &RunStats{
		RunID:       runID,
		TotalEvents: totalEvents,
	}, nil
}

func (m *mockEventRepository) GetGlobalStats() (*GlobalStats, error) {
	return &GlobalStats{TotalEvents: uint64(len(m.events))}, nil
}

func (m *mockEventRepository) Close() error {
	return nil
}

// TestProcessEvent_NilEvent tests processor behavior with nil event
func TestProcessEvent_NilEvent(t *testing.T) {
	// Disable strict mode and logs to test error returns cleanly
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	signer, err := crypto.NewSigner(".test_key_nil")
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(".test_key_nil"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove test key: %v", err)
		}
	})

	mockDB := &mockEventRepository{}
	processor := NewEventProcessor(mockDB, signer, "test-run-nil")

	err = processor.ProcessEvent(nil)
	if err == nil {
		t.Error("expected error for nil event, got nil")
	}
}

// TestProcessEvent_EmptyTaskID tests processor with empty task_id field
func TestProcessEvent_EmptyTaskID(t *testing.T) {
	signer, err := crypto.NewSigner(".test_key_empty_task")
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(".test_key_empty_task"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove test key: %v", err)
		}
	})

	mockDB := &mockEventRepository{}
	processor := NewEventProcessor(mockDB, signer, "test-run-empty")

	event := &models.Event{
		ID:        "test-1",
		RunID:     "run-1",
		Timestamp: time.Now(),
		EventType: "tool_call",
		Method:    "test:method",
		TaskID:    "", // Empty task ID
		Params:    make(map[string]interface{}),
		Response:  make(map[string]interface{}),
	}

	err = processor.ProcessEvent(event)
	if err != nil {
		t.Errorf("processor should handle empty task_id, got error: %v", err)
	}

	// Verify event was processed
	if event.SeqIndex != 0 {
		t.Errorf("expected seq_index 0, got %d", event.SeqIndex)
	}
	if event.CurrentHash == "" {
		t.Error("expected current_hash to be set")
	}
	if event.Signature == "" {
		t.Error("expected signature to be set")
	}
}

// TestProcessEvent_SequenceGap tests detection of sequence number gaps
func TestProcessEvent_SequenceGap(t *testing.T) {
	// Disable strict mode and logs to test error returns cleanly
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	signer, err := crypto.NewSigner(".test_key_gap")
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(".test_key_gap"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove test key: %v", err)
		}
	})

	mockDB := &mockEventRepository{
		lastSeq:  5, // Simulate existing sequence
		lastHash: "abcd1234",
	}
	processor := NewEventProcessor(mockDB, signer, "test-run-gap")

	// Try to process event with wrong sequence
	event := &models.Event{
		ID:        "test-gap",
		RunID:     "run-1",
		Timestamp: time.Now(),
		EventType: "tool_call",
		Method:    "test:method",
		SeqIndex:  10, // Gap: should be 6 (lastSeq + 1)
		Params:    make(map[string]interface{}),
		Response:  make(map[string]interface{}),
	}

	err = processor.ProcessEvent(event)
	if err == nil {
		t.Error("expected error for sequence gap, got nil")
	}
}

// TestProcessEvent_FirstEvent tests genesis event processing
func TestProcessEvent_FirstEvent(t *testing.T) {
	signer, err := crypto.NewSigner(".test_key_first")
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(".test_key_first"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove test key: %v", err)
		}
	})

	mockDB := &mockEventRepository{}
	processor := NewEventProcessor(mockDB, signer, "test-run-first")

	event := &models.Event{
		ID:        "genesis",
		RunID:     "run-1",
		Timestamp: time.Now(),
		EventType: "genesis",
		Method:    "system:genesis",
		Params:    make(map[string]interface{}),
		Response:  make(map[string]interface{}),
	}

	err = processor.ProcessEvent(event)
	if err != nil {
		t.Fatalf("failed to process first event: %v", err)
	}

	// Verify genesis event properties
	if event.SeqIndex != 0 {
		t.Errorf("genesis event should have seq_index 0, got %d", event.SeqIndex)
	}
	if event.PrevHash != "0000000000000000000000000000000000000000000000000000000000000000" {
		t.Errorf("genesis event should have 64 zero prev_hash, got %s", event.PrevHash)
	}
	if event.CurrentHash == "" {
		t.Error("genesis event should have current_hash set")
	}
	if event.Signature == "" {
		t.Error("genesis event should have signature set")
	}
}

// TestProcessEvent_ChainContinuity tests that chain links properly
func TestProcessEvent_ChainContinuity(t *testing.T) {
	signer, err := crypto.NewSigner(".test_key_chain")
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(".test_key_chain"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove test key: %v", err)
		}
	})

	mockDB := &mockEventRepository{}
	processor := NewEventProcessor(mockDB, signer, "test-run-chain")

	// Process multiple events
	const numEvents = 5
	for i := 0; i < numEvents; i++ {
		event := &models.Event{
			ID:        "event-" + string(rune('a'+i)),
			RunID:     "run-1",
			Timestamp: time.Now(),
			EventType: "tool_call",
			Method:    "test:method",
			Params:    make(map[string]interface{}),
			Response:  make(map[string]interface{}),
		}

		err := processor.ProcessEvent(event)
		if err != nil {
			t.Fatalf("failed to process event %d: %v", i, err)
		}

		// Verify sequence increments
		if event.SeqIndex != uint64(i) {
			t.Errorf("event %d: expected seq_index %d, got %d", i, i, event.SeqIndex)
		}

		// Verify chain linkage (except first event)
		if i > 0 {
			prevEvent := mockDB.events[i-1]
			if event.PrevHash != prevEvent.CurrentHash {
				t.Errorf("event %d: prev_hash mismatch: expected %s, got %s",
					i, prevEvent.CurrentHash, event.PrevHash)
			}
		}
	}

	// Verify total count
	if len(mockDB.events) != numEvents {
		t.Errorf("expected %d events stored, got %d", numEvents, len(mockDB.events))
	}
}

// TestProcessEvent_EmptyFields tests processor with minimal event data
func TestProcessEvent_EmptyFields(t *testing.T) {
	signer, err := crypto.NewSigner(".test_key_empty")
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(".test_key_empty"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove test key: %v", err)
		}
	})

	mockDB := &mockEventRepository{}
	processor := NewEventProcessor(mockDB, signer, "test-run-empty-fields")

	event := &models.Event{
		ID:        "minimal",
		RunID:     "run-1",
		Timestamp: time.Now(),
		EventType: "",  // Empty event type
		Method:    "",  // Empty method
		Params:    nil, // Nil params
		Response:  nil, // Nil response
		TaskID:    "",  // Empty task ID
		ParentID:  "",  // Empty parent ID
		PolicyID:  "",  // Empty policy ID
		RiskLevel: "",  // Empty risk level
	}

	err = processor.ProcessEvent(event)
	if err != nil {
		t.Errorf("processor should handle empty fields, got error: %v", err)
	}

	// Verify cryptographic fields are still set
	if event.CurrentHash == "" {
		t.Error("current_hash should be set even with empty fields")
	}
	if event.Signature == "" {
		t.Error("signature should be set even with empty fields")
	}
}
