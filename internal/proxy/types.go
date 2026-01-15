package proxy

import (
	"time"
)

// Event represents a single intercepted action in the agent's execution
type Event struct {
	ID          string                 `json:"id"`
	RunID       string                 `json:"run_id"`
	SeqIndex    int                    `json:"seq_index"`
	Timestamp   time.Time              `json:"timestamp"`
	Actor       string                 `json:"actor"` // "agent", "user", or "system"
	EventType   string                 `json:"event_type"`
	Method      string                 `json:"method"`
	Params      map[string]interface{} `json:"params"`
	Response    map[string]interface{} `json:"response"`
	TaskID      string                 `json:"task_id,omitempty"`
	TaskState   string                 `json:"task_state,omitempty"` // SEP-1686: working|input_required|completed|failed|cancelled
	ParentID    string                 `json:"parent_id,omitempty"`  // Hierarchy tracking
	PolicyID    string                 `json:"policy_id,omitempty"`
	RiskLevel   string                 `json:"risk_level,omitempty"`
	PrevHash    string                 `json:"prev_hash"`
	CurrentHash string                 `json:"current_hash"`
	Signature   string                 `json:"signature"`
	WasBlocked  bool                   `json:"was_blocked"`
}
