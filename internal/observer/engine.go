package observer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/slyt3/Vouch/internal/assert"
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
	ID              string              `yaml:"id"`
	MatchMethods    []string            `yaml:"match_methods"`
	RiskLevel       string              `yaml:"risk_level"`
	LogLevel        string              `yaml:"log_level,omitempty"`
	MatchConditions []map[string]string `yaml:"conditions,omitempty"`
	Redact          []string            `yaml:"redact,omitempty"` // List of param keys to redact
}

// ObserverEngine handles policy evaluation and enforcement
type ObserverEngine struct {
	mu         sync.RWMutex
	config     *Config
	configPath string
}

// NewObserverEngine creates a new observer engine
func NewObserverEngine(configPath string) (*ObserverEngine, error) {
	if err := assert.Check(configPath != "", "config path must not be empty"); err != nil {
		return nil, err
	}

	// Resolve path once
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		absPath = configPath
	}

	config, err := loadConfig(absPath)
	if err != nil {
		return nil, err
	}
	return &ObserverEngine{
		config:     config,
		configPath: absPath,
	}, nil
}

// loadConfig loads the vouch-policy.yaml file
func loadConfig(path string) (*Config, error) {
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

// Reload reloads the configuration from disk
func (e *ObserverEngine) Reload() error {
	newConfig, err := loadConfig(e.configPath)
	if err != nil {
		return err
	}

	e.mu.Lock()
	e.config = newConfig
	e.mu.Unlock()

	log.Printf("[INFO] Policy reloaded from %s", e.configPath)
	return nil
}

// Watch starts a background goroutine to watch for policy file changes
func (e *ObserverEngine) Watch() {
	go func() {
		var lastMod time.Time
		if stat, err := os.Stat(e.configPath); err == nil {
			lastMod = stat.ModTime()
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			stat, err := os.Stat(e.configPath)
			if err != nil {
				continue
			}

			if stat.ModTime().After(lastMod) {
				if err := e.Reload(); err == nil {
					lastMod = stat.ModTime()
				}
			}
		}
	}()
}

// GetVersion returns the policy version
func (e *ObserverEngine) GetVersion() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Version
}

// GetRuleCount returns the number of policy rules
func (e *ObserverEngine) GetRuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.config.Policies)
}

// GetPolicies returns the full list of rules (for interceptor)
func (e *ObserverEngine) GetPolicies() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Policies
}

// MatchPattern matches a method against a pattern with wildcard support
func MatchPattern(pattern, method string) bool {
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

// CheckConditions evaluates policy conditions against request parameters
func CheckConditions(conditions []map[string]string, params map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}

	for _, cond := range conditions {
		key := cond["key"]
		operator := cond["operator"]
		value := cond["value"]

		val, ok := params[key]
		if !ok {
			return false
		}

		switch operator {
		case "eq":
			if fmt.Sprintf("%v", val) != value {
				return false
			}
		default:
			// Unknown operator, skip
			continue
		}
	}

	return true
}
