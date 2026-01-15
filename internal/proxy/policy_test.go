package proxy

import (
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		method  string
		want    bool
	}{
		{"aws:ec2:launch", "aws:ec2:launch", true},
		{"aws:ec2:launch", "aws:s3:list", false},
		{"aws:*", "aws:ec2:launch", true},
		{"aws:*", "aws:s3:list", true},
		{"aws:*", "stripe:payment", false},
		{"*", "anything", true},
	}

	for _, tt := range tests {
		if got := MatchPattern(tt.pattern, tt.method); got != tt.want {
			t.Errorf("MatchPattern(%q, %q) = %v; want %v", tt.pattern, tt.method, got, tt.want)
		}
	}
}

func TestCheckConditions(t *testing.T) {
	conditions := map[string]interface{}{
		"amount_gt": 1000,
	}

	tests := []struct {
		params map[string]interface{}
		want   bool
	}{
		{map[string]interface{}{"amount": 2000.0}, true},
		{map[string]interface{}{"amount": 500.0}, false},
		{map[string]interface{}{"amount": 1000.0}, false},
		{map[string]interface{}{"other": 500.0}, true},
	}

	for _, tt := range tests {
		if got := CheckConditions(conditions, tt.params); got != tt.want {
			t.Errorf("CheckConditions(%v, %v) = %v; want %v", conditions, tt.params, got, tt.want)
		}
	}
}
