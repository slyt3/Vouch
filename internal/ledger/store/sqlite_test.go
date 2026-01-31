package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDB(t *testing.T) {
	// Setup temporary database
	tmpDir, err := os.MkdirTemp("", "logryph-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp dir: %v", err)
		}
	})

	// Create database
	db, err := NewDB(filepath.Join(tmpDir, "logryph.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Failed to close database: %v", err)
		}
	})

	// Test HasRuns (initial)
	hasRuns, err := db.HasRuns()
	if err != nil {
		t.Fatalf("HasRuns failed: %v", err)
	}
	if hasRuns {
		t.Error("HasRuns should be false initially")
	}

	// Test InsertRun
	runID := "test-run-1"
	err = db.InsertRun(runID, "test-agent", "genesis-hash", "pub-key")
	if err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	hasRuns, _ = db.HasRuns()
	if !hasRuns {
		t.Error("HasRuns should be true after insert")
	}

	// Test GetRunID
	gotRunID, err := db.GetRunID()
	if err != nil {
		t.Fatalf("GetRunID failed: %v", err)
	}
	if gotRunID != runID {
		t.Errorf("Expected run ID %s, got %s", runID, gotRunID)
	}

	// Test InsertEvent
	eventID := "event-1"
	timestamp := time.Now().Format(time.RFC3339Nano)
	err = db.InsertEvent(
		"event-1", "run-1", 1, timestamp,
		"agent", "tool_call", "mcp:list_tools", `{"foo":"bar"}`, "{}", "", "", "", "", "",
		"genesis-hash", "hash-1", "sig-1",
	)
	if err != nil {
		t.Fatalf("InsertEvent failed: %v", err)
	}

	// Test GetLastEvent
	seq, hash, err := db.GetLastEvent("run-1")
	if err != nil {
		t.Fatalf("GetLastEvent failed: %v", err)
	}
	if seq != 1 || hash != "hash-1" {
		t.Errorf("Expected seq 1, hash hash-1; got %d, %s", seq, hash)
	}

	// Test GetAllEvents
	events, err := db.GetAllEvents("run-1")
	if err != nil {
		t.Fatalf("GetAllEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].ID != "event-1" {
		t.Errorf("Expected event ID event-1, got %s", events[0].ID)
	}
	// Params check (InsertEvent used `{"foo":"bar"}`)
	if events[0].Params["foo"] != "bar" {
		t.Errorf("Expected param foo=bar, got %v", events[0].Params["foo"])
	}

	// Test GetRunInfo
	agent, genHash, pubKey, err := db.GetRunInfo(runID)
	if err != nil {
		t.Fatalf("GetRunInfo failed: %v", err)
	}
	if agent != "test-agent" || genHash != "genesis-hash" || pubKey != "pub-key" {
		t.Errorf("Run info mismatch")
	}

	// Test GetRecentEvents
	recent, err := db.GetRecentEvents("run-1", 10)
	if err != nil {
		t.Fatalf("GetRecentEvents failed: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("Expected 1 recent event, got %d", len(recent))
	}

	// Test GetEventByID
	event, err := db.GetEventByID(eventID)
	if err != nil {
		t.Fatalf("GetEventByID failed: %v", err)
	}
	if event.ID != eventID {
		t.Errorf("Expected event ID %s, got %s", eventID, event.ID)
	}
}
