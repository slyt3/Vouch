package observer

import (
	"testing"

	"github.com/slyt3/Logryph/internal/assert"
)

// FuzzMatchPattern tests pattern matching with random inputs
func FuzzMatchPattern(f *testing.F) {
	// Disable strict mode and logs to test error returns cleanly
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	// Seed corpus with known cases
	f.Add("aws:*", "aws:CreateBucket")
	f.Add("stripe:*", "stripe:CreateCharge")
	f.Add("exact:match", "exact:match")
	f.Add("", "")
	f.Add("*", "anything")
	f.Add("test", "test")

	f.Fuzz(func(t *testing.T, pattern, method string) {
		// Should never panic
		result := MatchPattern(pattern, method)

		// Validate basic invariants
		if pattern == "" || method == "" {
			if result {
				t.Errorf("empty pattern/method should not match: pattern=%q method=%q", pattern, method)
			}
			return 
		}

		if pattern == method {
			if !result {
				t.Errorf("exact match should succeed: pattern=%q method=%q", pattern, method)
			}
		}
	})
}

// FuzzCheckConditions tests condition evaluation with random inputs
func FuzzCheckConditions(f *testing.F) {
	// Disable strict mode and logs to test error returns cleanly
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	// Seed corpus with known cases
	f.Add("key1", "eq", "value1", "value1")
	f.Add("amount", "gt", "100", "150")
	f.Add("price", "lt", "50", "25")

	f.Fuzz(func(t *testing.T, key, operator, value, actualValue string) {
		// Build condition and params
		conditions := []map[string]string{
			{
				"key":      key,
				"operator": operator,
				"value":    value,
			},
		}
		params := map[string]interface{}{
			key: actualValue,
		}

		// Should never panic
		_ = CheckConditions(conditions, params)

		// Validate nil params handling
		nilResult := CheckConditions(conditions, nil)
		if nilResult {
			t.Errorf("nil params should return false")
		}
	})
}
