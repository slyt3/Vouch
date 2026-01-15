package proxy

import (
	"time"
)

// Event represents a single intercepted action in the agent's execution
type Event struct {
	ID         string                 `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	EventType  string                 `json:"event_type"`
	Method     string                 `json:"method"`
	Params     map[string]interface{} `json:"params"`
	Response   map[string]interface{} `json:"response"`
	TaskID     string                 `json:"task_id,omitempty"`
	TaskState  string                 `json:"task_state,omitempty"` // SEP-1686: working|input_required|completed|failed|cancelled
	WasBlocked bool                   `json:"was_blocked"`
}

// MCPRequest represents a Model Context Protocol JSON-RPC request
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

// MCPResponse represents a Model Context Protocol JSON-RPC response
type MCPResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   map[string]interface{} `json:"error,omitempty"`
}
