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
	t.Cleanup(func() {
		if err := os.Remove(tmpFile); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	})

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
	t.Cleanup(func() {
		if err := os.Remove(tmpFile); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	})

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

	const maxPolls = 20
	for i := 0; i < maxPolls; i++ {
		select {
		case <-timeout:
			t.Fatal("Timed out waiting for policy reload")
			return
		case <-ticker.C:
			if engine.GetVersion() == "2.0" {
				return // Success
			}
		}
	}
	t.Fatal("Exceeded max polls waiting for policy reload")
}

func TestCheckConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions []map[string]string
		params     map[string]interface{}
		want       bool
	}{
		{
			name: "eq success",
			conditions: []map[string]string{
				{"key": "amount", "operator": "eq", "value": "100"},
			},
			params: map[string]interface{}{"amount": 100},
			want:   true,
		},
		{
			name: "gt success",
			conditions: []map[string]string{
				{"key": "amount", "operator": "gt", "value": "1000"},
			},
			params: map[string]interface{}{"amount": 1500},
			want:   true,
		},
		{
			name: "gt fail",
			conditions: []map[string]string{
				{"key": "amount", "operator": "gt", "value": "1000"},
			},
			params: map[string]interface{}{"amount": 500},
			want:   false,
		},
		{
			name: "lt success",
			conditions: []map[string]string{
				{"key": "age", "operator": "lt", "value": "18"},
			},
			params: map[string]interface{}{"age": 17},
			want:   true,
		},
		{
			name: "gte success",
			conditions: []map[string]string{
				{"key": "score", "operator": "gte", "value": "50"},
			},
			params: map[string]interface{}{"score": 50},
			want:   true,
		},
		{
			name: "lte success",
			conditions: []map[string]string{
				{"key": "score", "operator": "lte", "value": "50"},
			},
			params: map[string]interface{}{"score": "50"}, // string param
			want:   true,
		},
		{
			name: "multi condition success",
			conditions: []map[string]string{
				{"key": "amount", "operator": "gt", "value": "100"},
				{"key": "mode", "operator": "eq", "value": "live"},
			},
			params: map[string]interface{}{"amount": 200, "mode": "live"},
			want:   true,
		},
		{
			name: "multi condition fail",
			conditions: []map[string]string{
				{"key": "amount", "operator": "gt", "value": "100"},
				{"key": "mode", "operator": "eq", "value": "live"},
			},
			params: map[string]interface{}{"amount": 50, "mode": "live"},
			want:   false,
		},
	}

	const maxTests = 32
	for i := 0; i < maxTests; i++ {
		if i >= len(tests) {
			break
		}
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckConditions(tt.conditions, tt.params); got != tt.want {
				t.Errorf("CheckConditions() = %v, want %v", got, tt.want)
			}
		})
	}
}
