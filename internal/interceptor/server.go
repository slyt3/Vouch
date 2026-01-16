package interceptor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/core"
	"github.com/slyt3/Vouch/internal/mcp"
	"github.com/slyt3/Vouch/internal/observer"
	"github.com/slyt3/Vouch/internal/pool"
)

// PolicyAction defines the outcome of a policy check
type PolicyAction string

const (
	ActionAllow  PolicyAction = "allow"
	ActionTag    PolicyAction = "tag"    // Renamed from Redact/Stall
	ActionRedact PolicyAction = "redact" // Keep for hygiene, but not blocking
)

// Interceptor handles the proxy interception logic
type Interceptor struct {
	Core *core.Engine
}

func NewInterceptor(engine *core.Engine) *Interceptor {
	return &Interceptor{Core: engine}
}

func (i *Interceptor) InterceptRequest(req *http.Request) {
	if req.Method != http.MethodPost {
		return
	}

	if req.Body == nil {
		return
	}

	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)

	if _, err := buf.ReadFrom(req.Body); err != nil {
		log.Printf("Failed to read request body: %v", err)
		return
	}
	bodyBytes := buf.Bytes()
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 1. Extract Metadata
	mcpReq, taskID, method, err := i.extractTaskMetadata(bodyBytes)
	if err != nil {
		i.SendErrorResponse(req, http.StatusBadRequest, -32000, err.Error())
		return
	}

	// 2. Policy Evaluation
	action, matchedRule, err := i.evaluatePolicy(method, mcpReq.Params)
	if err != nil {
		i.SendErrorResponse(req, http.StatusBadRequest, -32000, "Policy violation")
		return
	}

	// 3. Handle Stall (REMOVED - Phase 2 Lobotomy)
	// We no longer block traffic. We only observe.
	// if action == ActionStall { ... }

	// 4. Redaction (if needed)
	if action == ActionRedact && matchedRule != nil {
		scrubbedBody, err := i.redactSensitiveData(bodyBytes, matchedRule.Redact)
		if err != nil {
			log.Printf("Redaction failed: %v", err)
			i.SendErrorResponse(req, http.StatusInternalServerError, -32000, "Redaction failed")
			return
		}
		bodyBytes = scrubbedBody
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// 5. Submit Event & Forward
	i.submitToolCallEvent(taskID, mcpReq, matchedRule)
}

// extractTaskMetadata parses and validates the request
func (i *Interceptor) extractTaskMetadata(body []byte) (*mcp.MCPRequest, string, string, error) {
	if err := assert.Check(len(body) > 0, "request body is empty"); err != nil {
		return nil, "", "", err
	}
	if err := assert.Check(len(body) < 1024*1024, "request body too large: size=%d", len(body)); err != nil {
		return nil, "", "", err
	}

	var mcpReq mcp.MCPRequest
	if err := json.Unmarshal(body, &mcpReq); err != nil {
		return nil, "", "", fmt.Errorf("invalid JSON-RPC: %w", err)
	}

	if err := assert.Check(mcpReq.Method != "", "method must not be empty"); err != nil {
		return nil, "", "", err
	}
	if err := assert.Check(mcpReq.JSONRPC == "2.0", "invalid JSON-RPC version: %s", mcpReq.JSONRPC); err != nil {
		return nil, "", "", err
	}

	taskID, _ := mcpReq.Params["task_id"].(string)
	return &mcpReq, taskID, mcpReq.Method, nil
}

// evaluatePolicy determines the action for the request
func (i *Interceptor) evaluatePolicy(method string, params map[string]interface{}) (PolicyAction, *observer.Rule, error) {
	if err := assert.Check(i.Core.Observer != nil, "observer engine missing"); err != nil {
		return ActionAllow, nil, err
	}
	if err := assert.Check(method != "", "method name is non-empty"); err != nil {
		return ActionAllow, nil, err
	}

	// Use the new Observer engine
	for _, rule := range i.Core.Observer.GetPolicies() {
		for _, pattern := range rule.MatchMethods {
			if observer.MatchPattern(pattern, method) {
				if len(rule.MatchConditions) > 0 && !observer.CheckConditions(rule.MatchConditions, params) {
					continue
				}

				action := ActionTag
				if len(rule.Redact) > 0 {
					action = ActionRedact
				}

				return action, &rule, nil
			}
		}
	}

	return ActionAllow, nil, nil
}

// handleStall was removed in Phase 2 (Lobotomy).
//func (i *Interceptor) handleStall(...) error { ... }

// submitToolCallEvent prepares and sends the tool_call event to the ledger
func (i *Interceptor) submitToolCallEvent(taskID string, mcpReq *mcp.MCPRequest, matchedRule *observer.Rule) {
	if err := assert.Check(mcpReq != nil, "mcpReq must not be nil"); err != nil {
		return
	}
	if err := assert.Check(i.Core.Worker != nil, "worker must be initialized"); err != nil {
		return
	}

	event := pool.GetEvent()
	event.ID = uuid.New().String()[:8]
	event.Timestamp = time.Now()
	event.EventType = "tool_call"
	event.Method = mcpReq.Method
	event.Params = mcpReq.Params
	event.TaskID = taskID

	if matchedRule != nil {
		event.PolicyID = matchedRule.ID
		event.RiskLevel = matchedRule.RiskLevel
	}

	if taskID != "" {
		if parentID, ok := i.Core.LastEventByTask.Load(taskID); ok {
			event.ParentID = parentID.(string)
		}
		i.Core.LastEventByTask.Store(taskID, event.ID)
	}

	i.Core.Worker.Submit(event)
}

// redactSensitiveData scrubs PII based on policy (accepts and returns bytes)
func (i *Interceptor) redactSensitiveData(body []byte, keys []string) ([]byte, error) {
	if err := assert.Check(len(body) > 0, "body must not be empty"); err != nil {
		return nil, err
	}
	if err := assert.Check(len(keys) > 0, "redaction keys must be defined"); err != nil {
		return body, nil
	}

	var mcpReq mcp.MCPRequest
	if err := json.Unmarshal(body, &mcpReq); err != nil {
		return nil, err
	}

	// 4. Bound map iterations
	if err := assert.Check(len(mcpReq.Params) < 100, "excessive parameters in request"); err != nil {
		return nil, err
	}
	for k := range mcpReq.Params {
		if err := assert.Check(len(keys) < 1000, "excessive key redaction list"); err != nil {
			break
		}
		for _, key := range keys {
			if k == key {
				mcpReq.Params[k] = "[REDACTED]"
			}
		}
	}

	return json.Marshal(mcpReq)
}

// InterceptResponse intercepts and analyzes responses
func (i *Interceptor) InterceptResponse(resp *http.Response) error {
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)

	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return err
	}
	bodyBytes := buf.Bytes()
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var mcpResp mcp.MCPResponse
	if err := json.Unmarshal(bodyBytes, &mcpResp); err != nil {
		return nil
	}

	if !i.Core.Worker.IsHealthy() {
		return nil
	}

	var taskID string
	var taskState string

	if result := mcpResp.Result; result != nil {
		if tid, ok := result["task_id"].(string); ok {
			taskID = tid
		}
		if state, ok := result["state"].(string); ok {
			taskState = state
			if taskID != "" {
				i.Core.ActiveTasks.Store(taskID, taskState)
			}
		}
	}

	event := pool.GetEvent()
	event.ID = uuid.New().String()[:8]
	event.Timestamp = time.Now()
	event.EventType = "tool_response"
	event.Response = mcpResp.Result
	event.TaskID = taskID
	event.TaskState = taskState

	i.Core.Worker.Submit(event)
	return nil
}

// SendErrorResponse was used to block requests (Phase 1).
// In Phase 2 (Lobotomy), we are a passive recorder and do not block traffic.
func (i *Interceptor) SendErrorResponse(req *http.Request, statusCode int, code int, message string) {
	// Passive: We do not block. We just log the failure to record if needed.
}
