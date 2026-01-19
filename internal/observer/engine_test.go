package observer

import (
	"os"
	"testing"
	"time"
)

func TestObserverEngine_Reload(t *testing.T) {
	tmpFile := "test-policy.yaml"
	initialYaml := `
version: "1.0"
defaults:
  retention_days: 7
  signing_enabled: true
  log_level: "metadata_only"
policies:
  - id: "rule-1"
    match_methods: ["test:method"]
    risk_level: "low"
`
	err := os.WriteFile(tmpFile, []byte(initialYaml), 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	engine, err := NewObserverEngine(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	if engine.GetRuleCount() != 1 {
		t.Errorf("Expected 1 rule, got %d", engine.GetRuleCount())
	}

	// Modify file
	updatedYaml := `
version: "1.1"
defaults:
  retention_days: 7
  signing_enabled: true
  log_level: "metadata_only"
policies:
  - id: "rule-1"
    match_methods: ["test:method"]
    risk_level: "low"
  - id: "rule-2"
    match_methods: ["test:admin"]
    risk_level: "critical"
`
	err = os.WriteFile(tmpFile, []byte(updatedYaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Trigger reload
	err = engine.Reload()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if engine.GetRuleCount() != 2 {
		t.Errorf("Expected 2 rules after reload, got %d", engine.GetRuleCount())
	}
	if engine.GetVersion() != "1.1" {
		t.Errorf("Expected version 1.1, got %s", engine.GetVersion())
	}
}

func TestObserverEngine_Watch(t *testing.T) {
	// Skip on CI if it's too slow/flaky, but for manual verification it's good.
	tmpFile := "test-watch-policy.yaml"
	initialYaml := `
version: "1.0"
defaults:
  retention_days: 7
  signing_enabled: true
  log_level: "metadata_only"
policies: []
`
	err := os.WriteFile(tmpFile, []byte(initialYaml), 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	engine, err := NewObserverEngine(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	engine.Watch()

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Update file
	updatedYaml := `
version: "2.0"
defaults:
  retention_days: 7
  signing_enabled: true
  log_level: "metadata_only"
policies:
  - id: "new-rule"
    match_methods: ["*"]
    risk_level: "high"
`
	// Ensure mod time changes
	time.Sleep(1 * time.Second)
	err = os.WriteFile(tmpFile, []byte(updatedYaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Polling is 5s, so we wait longer or we could reduce it for testing.
	// For the test, let's reduce the interval or just test Reload() mostly.
	// But let's try a short wait and see.

	// Actually, let's just test that it eventually reloads.
	timeout := time.After(7 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timed out waiting for policy reload")
		case <-ticker.C:
			if engine.GetVersion() == "2.0" {
				return // Success
			}
		}
	}
}
