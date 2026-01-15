package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourname/vouch/internal/assert"
	"gopkg.in/yaml.v3"
)

// Config represents the vouch-policy.yaml structure (2026.1 spec)
type Config struct {
	Version  string `yaml:"version"`
	Defaults struct {
		RetentionDays  int    `yaml:"retention_days"`
		SigningEnabled bool   `yaml:"signing_enabled"`
		LogLevel       string `yaml:"log_level"`
	} `yaml:"defaults"`
	Policies []Rule `yaml:"policies"`
}

// Rule represents a single policy rule
type Rule struct {
	ID             string                 `yaml:"id"`
	MatchMethods   []string               `yaml:"match_methods"`
	RiskLevel      string                 `yaml:"risk_level"`
	Action         string                 `yaml:"action"` // "allow" | "stall"
	ProofOfRefusal bool                   `yaml:"proof_of_refusal"`
	LogLevel       string                 `yaml:"log_level,omitempty"`
	Conditions     map[string]interface{} `yaml:"conditions,omitempty"`
}

// Engine handles policy evaluation and enforcement
type Engine struct {
	config *Config
}

// NewEngine creates a new policy engine
func NewEngine(configPath string) (*Engine, error) {
	if err := assert.Check(configPath != "", "config path must not be empty"); err != nil {
		return nil, err
	}
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return &Engine{config: config}, nil
}

// loadConfig loads the vouch-policy.yaml file
func loadConfig(path string) (*Config, error) {
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

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}

	return &config, nil
}

// GetVersion returns the policy version
func (e *Engine) GetVersion() string {
	return e.config.Version
}

// GetRuleCount returns the number of policy rules
func (e *Engine) GetRuleCount() int {
	return len(e.config.Policies)
}

// ShouldStall checks if a method should be stalled based on policy
func (e *Engine) ShouldStall(method string, params map[string]interface{}) (bool, *Rule) {
	if err := assert.Check(method != "", "method name is non-empty"); err != nil {
		return false, nil
	}
	for _, rule := range e.config.Policies {
		if rule.Action != "stall" {
			continue
		}

		// Check method match with wildcard support
		for _, pattern := range rule.MatchMethods {
			if matchPattern(pattern, method) {
				// Check additional conditions if present
				if rule.Conditions != nil {
					if !checkConditions(rule.Conditions, params) {
						continue
					}
				}
				return true, &rule
			}
		}
	}
	return false, nil
}

// matchPattern matches a method against a pattern with wildcard support
func matchPattern(pattern, method string) bool {
	if err := assert.Check(pattern != "", "pattern is non-empty"); err != nil {
		return false
	}
	if err := assert.Check(method != "", "method is non-empty"); err != nil {
		return false
	}
	if pattern == method {
		return true
	}

	// Handle wildcard patterns (e.g., "aws:*", "stripe:*")
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(method, prefix)
	}

	return false
}

// checkConditions evaluates policy conditions against request parameters
func checkConditions(conditions map[string]interface{}, params map[string]interface{}) bool {
	// Check amount_gt condition for financial operations
	if amountGt, ok := conditions["amount_gt"].(int); ok {
		if amount, ok := params["amount"].(float64); ok {
			return amount > float64(amountGt)
		}
	}

	// Default: condition not met
	return true
}
