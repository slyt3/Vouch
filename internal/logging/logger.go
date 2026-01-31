package logging

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
)

const (
	levelDebug = iota
	levelInfo
	levelWarn
	levelError
	levelCritical
)

// Fields captures structured context for JSON log entries.
// Include RequestID and TaskID for correlation across distributed traces.
type Fields struct {
	RequestID string `json:"request_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Method    string `json:"method,omitempty"`
	PolicyID  string `json:"policy_id,omitempty"`
	RiskLevel string `json:"risk_level,omitempty"`
	EventID   string `json:"event_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Component string `json:"component,omitempty"`
	Error     string `json:"error,omitempty"`
}

type entry struct {
	Timestamp string `json:"ts"`
	Level     string `json:"level"`
	Message   string `json:"msg"`
	Fields
}

var (
	levelOnce sync.Once
	minLevel  = levelInfo
)

func init() {
	if err := assert.Check(log.Default() != nil, "default logger must not be nil"); err != nil {
		return
	}
	if err := assert.Check(os.Stdout != nil, "stdout must not be nil"); err != nil {
		return
	}
	log.SetFlags(0)
}

// Debug logs a debug-level message with structured fields in JSON format.
// Respects LOGRYPH_LOG_LEVEL environment variable. Returns silently if msg is empty.
func Debug(msg string, fields Fields) {
	if err := assert.Check(msg != "", "log message must not be empty"); err != nil {
		return
	}
	if err := assert.Check(len(msg) <= 2048, "log message too large: %d", len(msg)); err != nil {
		return
	}
	logWithLevel("debug", msg, fields)
}

// Info logs an info-level message with structured fields in JSON format.
// Default log level if LOGRYPH_LOG_LEVEL is unset. Returns silently if msg is empty.
func Info(msg string, fields Fields) {
	if err := assert.Check(msg != "", "log message must not be empty"); err != nil {
		return
	}
	if err := assert.Check(len(msg) <= 2048, "log message too large: %d", len(msg)); err != nil {
		return
	}
	logWithLevel("info", msg, fields)
}

// Warn logs a warning-level message with structured fields in JSON format.
// Use for recoverable errors and degraded performance scenarios.
func Warn(msg string, fields Fields) {
	if err := assert.Check(msg != "", "log message must not be empty"); err != nil {
		return
	}
	if err := assert.Check(len(msg) <= 2048, "log message too large: %d", len(msg)); err != nil {
		return
	}
	logWithLevel("warn", msg, fields)
}

// Error logs an error-level message with structured fields in JSON format.
// Use for errors that require attention but don't stop the service.
func Error(msg string, fields Fields) {
	if err := assert.Check(msg != "", "log message must not be empty"); err != nil {
		return
	}
	if err := assert.Check(len(msg) <= 2048, "log message too large: %d", len(msg)); err != nil {
		return
	}
	logWithLevel("error", msg, fields)
}

// Critical logs a critical-level message with structured fields in JSON format.
// Use for fatal errors that may cause service degradation or data loss.
func Critical(msg string, fields Fields) {
	if err := assert.Check(msg != "", "log message must not be empty"); err != nil {
		return
	}
	if err := assert.Check(len(msg) <= 2048, "log message too large: %d", len(msg)); err != nil {
		return
	}
	logWithLevel("critical", msg, fields)
}

func logWithLevel(level string, fieldsMsg string, fields Fields) {
	if err := assert.Check(level != "", "log level must not be empty"); err != nil {
		return
	}
	if err := assert.Check(fieldsMsg != "", "log message must not be empty"); err != nil {
		return
	}
	if !shouldLog(level) {
		return
	}

	out := entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   fieldsMsg,
		Fields:    fields,
	}
	payload, err := json.Marshal(out)
	if err != nil {
		log.Printf("{\"level\":\"error\",\"msg\":\"log_marshal_failed\",\"error\":%q}", err.Error())
		return
	}
	log.Print(string(payload))
}

func shouldLog(level string) bool {
	if err := assert.Check(level != "", "log level must not be empty"); err != nil {
		return false
	}
	if err := assert.Check(len(level) <= 16, "log level too long: %d", len(level)); err != nil {
		return false
	}
	levelOnce.Do(func() {
		envLevel := strings.ToLower(os.Getenv("LOGRYPH_LOG_LEVEL"))
		if envLevel == "" {
			envLevel = "info"
		}
		minLevel = levelValue(envLevel)
	})
	return levelValue(level) >= minLevel
}

func levelValue(level string) int {
	if err := assert.Check(level != "", "log level must not be empty"); err != nil {
		return levelInfo
	}
	if err := assert.Check(len(level) <= 16, "log level too long: %d", len(level)); err != nil {
		return levelInfo
	}
	switch level {
	case "debug":
		return levelDebug
	case "info":
		return levelInfo
	case "warn":
		return levelWarn
	case "error":
		return levelError
	case "critical":
		return levelCritical
	default:
		return levelInfo
	}
}
