package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PolicyConfig represents the vouch-policy.yaml structure
type PolicyConfig struct {
	Version  string `yaml:"version"`
	Defaults struct {
		RetentionDays  int    `yaml:"retention_days"`
		SigningEnabled bool   `yaml:"signing_enabled"`
		LogLevel       string `yaml:"log_level"`
	} `yaml:"defaults"`
	Policies []PolicyRule `yaml:"policies"`
}

// PolicyRule represents a single policy rule
type PolicyRule struct {
	ID             string                 `yaml:"id"`
	MatchMethods   []string               `yaml:"match_methods"`
	RiskLevel      string                 `yaml:"risk_level"`
	Action         string                 `yaml:"action"`
	ProofOfRefusal bool                   `yaml:"proof_of_refusal"`
	LogLevel       string                 `yaml:"log_level,omitempty"`
	Conditions     map[string]interface{} `yaml:"conditions,omitempty"`
	Redact         []string               `yaml:"redact,omitempty"` // List of param keys to redact
}

// LoadPolicy loads the vouch-policy.yaml file
func LoadPolicy(path string) (*PolicyConfig, error) {
	// Try absolute path first, then relative
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err == nil {
			path = filepath.Join(wd, path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	var config PolicyConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}

	return &config, nil
}

// MatchPattern matches a method against a pattern with wildcard support
func MatchPattern(pattern, method string) bool {
	if pattern == method {
		return true
	}

	// Handle wildcard patterns (e.g., "aws:*")
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(method, prefix)
	}

	return false
}

// CheckConditions evaluates policy conditions against request parameters
func CheckConditions(conditions map[string]interface{}, params map[string]interface{}) bool {
	// Check amount_gt condition for financial operations
	if amountGt, ok := conditions["amount_gt"].(int); ok {
		if amount, ok := params["amount"].(float64); ok {
			return amount > float64(amountGt)
		}
	}

	// Default: condition not met
	return true
}
