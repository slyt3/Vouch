package observer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/logging"
	"gopkg.in/yaml.v3"
)

// Config represents the logryph-policy.yaml structure (2026.1 spec).
// Includes version, defaults section (retention, signing, log level), and policies list.
type Config struct {
	Version  string `yaml:"version"`
	Defaults struct {
		RetentionDays  int    `yaml:"retention_days"`
		SigningEnabled bool   `yaml:"signing_enabled"`
		LogLevel       string `yaml:"log_level"`
	} `yaml:"defaults"`
	Policies []Rule `yaml:"policies"`
}

// Rule represents a single policy rule with method patterns, conditions, and redaction keys.
// MatchMethods supports wildcards (e.g., "aws:*"). Redact lists parameter keys to scrub.
type Rule struct {
	ID              string              `yaml:"id"`
	MatchMethods    []string            `yaml:"match_methods"`
	RiskLevel       string              `yaml:"risk_level"`
	LogLevel        string              `yaml:"log_level,omitempty"`
	MatchConditions []map[string]string `yaml:"conditions,omitempty"`
	Redact          []string            `yaml:"redact,omitempty"` // List of param keys to redact
}

// ObserverEngine handles policy evaluation and hot-reload from logryph-policy.yaml.
// Reloads config every 5 seconds when Watch() is running.
// Thread-safe for concurrent policy lookups.
type ObserverEngine struct {
	mu         sync.RWMutex
	config     *Config
	configPath string
	stopChan   chan struct{}
	stopOnce   sync.Once
}

// NewObserverEngine creates a new observer engine and loads the initial policy file.
// Returns an error if configPath is empty or policy file cannot be loaded/parsed.
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
		stopChan:   make(chan struct{}),
	}, nil
}

// loadConfig loads the logryph-policy.yaml file
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

// Reload reloads the policy configuration from disk.
// Returns an error if the file cannot be read or parsed.
// Logs "policy_reloaded" event on success.
func (e *ObserverEngine) Reload() error {
	newConfig, err := loadConfig(e.configPath)
	if err != nil {
		return err
	}

	e.mu.Lock()
	e.config = newConfig
	e.mu.Unlock()

	logging.Info("policy_reloaded", logging.Fields{Component: "observer"})
	return nil
}

// Watch starts a background goroutine that checks for policy file changes every 5 seconds.
// Automatically reloads config when modification time changes. Call Stop() to terminate.
func (e *ObserverEngine) Watch() {
	if err := assert.NotNil(e, "engine"); err != nil {
		return
	}
	if err := assert.NotNil(e.stopChan, "stop channel"); err != nil {
		return
	}
	go func() {
		var lastMod time.Time
		if stat, err := os.Stat(e.configPath); err == nil {
			lastMod = stat.ModTime()
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		const maxWatchTicks = 1 << 30
		for i := 0; i < maxWatchTicks; i++ {
			select {
			case <-ticker.C:
				stat, err := os.Stat(e.configPath)
				if err != nil {
					continue
				}

				if stat.ModTime().After(lastMod) {
					if err := e.Reload(); err == nil {
						lastMod = stat.ModTime()
					}
				}
			case <-e.stopChan:
				return
			}
		}
		if err := assert.Check(false, "watch loop exceeded max ticks"); err != nil {
			return
		}
	}()
}

// Stop signals the watcher goroutine to terminate. Safe to call multiple times (idempotent).
func (e *ObserverEngine) Stop() error {
	if err := assert.NotNil(e, "engine"); err != nil {
		return err
	}
	if err := assert.NotNil(e.stopChan, "stop channel"); err != nil {
		return err
	}
	e.stopOnce.Do(func() {
		close(e.stopChan)
	})
	return nil
}

// GetVersion returns the policy version string from the loaded config.
func (e *ObserverEngine) GetVersion() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Version
}

// GetRuleCount returns the number of policy rules currently loaded.
func (e *ObserverEngine) GetRuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.config.Policies)
}

// GetPolicies returns a copy of all policy rules for interceptor evaluation.
// Thread-safe for concurrent access.
func (e *ObserverEngine) GetPolicies() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Policies
}

// MatchPattern checks if a method matches a policy pattern.
// Supports exact match and wildcard patterns (e.g., "aws:*" matches "aws:CreateBucket").
// Returns false if either pattern or method is empty.
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

// CheckConditions evaluates policy conditions against request parameters.
// Supports operators: eq, gt, lt, gte, lte. Returns true if all conditions pass.
// Returns true if conditions list is empty. Returns false if params is nil.
func CheckConditions(conditions []map[string]string, params map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}
	if err := assert.Check(params != nil, "params must not be nil"); err != nil {
		return false
	}
	const maxConditions = 64
	if err := assert.Check(len(conditions) <= maxConditions, "conditions exceed max: %d", len(conditions)); err != nil {
		return false
	}

	for i := 0; i < maxConditions; i++ {
		if i >= len(conditions) {
			break
		}
		cond := conditions[i]
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
		case "gt", "lt", "gte", "lte":
			fVal, ok1 := toFloat(val)
			fTarget, err2 := strconv.ParseFloat(value, 64)
			if !ok1 || err2 != nil {
				return false
			}

			switch operator {
			case "gt":
				if !(fVal > fTarget) {
					return false
				}
			case "lt":
				if !(fVal < fTarget) {
					return false
				}
			case "gte":
				if !(fVal >= fTarget) {
					return false
				}
			case "lte":
				if !(fVal <= fTarget) {
					return false
				}
			}
		default:
			// Unknown operator, skip
			continue
		}
	}

	return true
}

func toFloat(v interface{}) (float64, bool) {
	switch i := v.(type) {
	case float64:
		return i, true
	case float32:
		return float64(i), true
	case int:
		return float64(i), true
	case int64:
		return float64(i), true
	case string:
		f, err := strconv.ParseFloat(i, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
