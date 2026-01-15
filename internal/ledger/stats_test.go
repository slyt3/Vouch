package ledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStats(t *testing.T) {
	// Setup temporary database
	tmpDir, _ := os.MkdirTemp("", "vouch-stats-test-*")
	defer os.RemoveAll(tmpDir)

	schemaContent, _ := os.ReadFile("../../schema.sql")
	_ = os.WriteFile(filepath.Join(tmpDir, "schema.sql"), schemaContent, 0644)

	oldWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldWd) }()

	db, _ := NewDB("vouch.db")
	defer db.Close()

	runID := "run-stats-1"
	_ = db.InsertRun(runID, "agent-1", "gen-hash", "pub-key")

	now := time.Now().Format(time.RFC3339Nano)

	// Create some events
	// 1. genesis (seq 0) - auto created by InsertRun in production but here we test InsertEvent
	_ = db.InsertEvent("e0", runID, 0, now, "system", "genesis", "vouch:init", "{}", "{}", "", "", "", "", "", "000", "h0", "s0")
	// 2. tool call (low risk)
	_ = db.InsertEvent("e1", runID, 1, now, "agent", "tool_call", "mcp:list_tools", "{}", "{}", "", "", "", "p1", "low", "h0", "h1", "s1")
	// 3. tool call (high risk)
	_ = db.InsertEvent("e2", runID, 2, now, "agent", "tool_call", "aws:ec2:terminate", "{}", "{}", "", "", "", "p2", "high", "h1", "h2", "s2")
	// 4. blocked event
	_ = db.InsertEvent("e3", runID, 3, now, "agent", "blocked", "aws:ec2:terminate", "{}", "{}", "", "", "", "p2", "high", "h2", "h3", "s3")

	// Test GetRunStats
	stats, err := db.GetRunStats(runID)
	if err != nil {
		t.Fatalf("GetRunStats failed: %v", err)
	}

	if stats.TotalEvents != 4 {
		t.Errorf("Expected 4 total events, got %d", stats.TotalEvents)
	}
	if stats.BlockedCount != 1 {
		t.Errorf("Expected 1 blocked event, got %d", stats.BlockedCount)
	}
	if stats.RiskBreakdown["high"] != 2 { // e2 and e3
		t.Errorf("Expected 2 high risk events, got %d", stats.RiskBreakdown["high"])
	}

	// Test GetGlobalStats
	gStats, err := db.GetGlobalStats()
	if err != nil {
		t.Fatalf("GetGlobalStats failed: %v", err)
	}
	if gStats.TotalRuns != 1 {
		t.Errorf("Expected 1 total run, got %d", gStats.TotalRuns)
	}

	// Test GetRiskEvents
	risky, err := db.GetRiskEvents()
	if err != nil {
		t.Fatalf("GetRiskEvents failed: %v", err)
	}
	if len(risky) != 2 { // e2 and e3 are high
		t.Errorf("Expected 2 risky events, got %d", len(risky))
	}
}
