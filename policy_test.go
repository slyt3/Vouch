package main

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
		if got := matchPattern(tt.pattern, tt.method); got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v; want %v", tt.pattern, tt.method, got, tt.want)
		}
	}
}

func TestCheckConditions(t *testing.T) {
	proxy := &AELProxy{}

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
		{map[string]interface{}{"other": 500.0}, true}, // amount missing, condition check might be skipped or return true depending on implementation
	}

	for _, tt := range tests {
		if got := proxy.checkConditions(conditions, tt.params); got != tt.want {
			t.Errorf("checkConditions(%v, %v) = %v; want %v", conditions, tt.params, got, tt.want)
		}
	}
}

func TestShouldStallMethod(t *testing.T) {
	config := &PolicyConfig{
		Policies: []PolicyRule{
			{
				ID:           "critical-infra",
				Action:       "stall",
				MatchMethods: []string{"aws:*"},
				RiskLevel:    "critical",
			},
			{
				ID:           "payments",
				Action:       "stall",
				MatchMethods: []string{"stripe:payment"},
				RiskLevel:    "high",
				Conditions: map[string]interface{}{
					"amount_gt": 100,
				},
			},
		},
	}

	proxy := &AELProxy{policy: config}

	tests := []struct {
		method string
		params map[string]interface{}
		want   bool
	}{
		{"aws:ec2:delete", nil, true},
		{"stripe:payment", map[string]interface{}{"amount": 500.0}, true},
		{"stripe:payment", map[string]interface{}{"amount": 50.0}, false},
		{"mcp:list_tools", nil, false},
	}

	for _, tt := range tests {
		got, _ := proxy.shouldStallMethod(tt.method, tt.params)
		if got != tt.want {
			t.Errorf("shouldStallMethod(%q, %v) = %v; want %v", tt.method, tt.params, got, tt.want)
		}
	}
}
