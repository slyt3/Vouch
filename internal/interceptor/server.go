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
	"github.com/slyt3/Vouch/internal/pool"
	"github.com/slyt3/Vouch/internal/proxy"
)

// PolicyAction defines the outcome of a policy check
type PolicyAction string

const (
	ActionAllow  PolicyAction = "allow"
	ActionStall  PolicyAction = "stall"
	ActionRedact PolicyAction = "redact"
)

// Interceptor handles the proxy interception logic
type Interceptor struct {
	Core *core.Engine
}

func NewInterceptor(engine *core.Engine) *Interceptor {
	return &Interceptor{Core: engine}
}

// InterceptRequest intercepts and analyzes incoming requests (Orchestrator)
func (i *Interceptor) InterceptRequest(req *http.Request) {
	if req.Method != http.MethodPost {
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

	// 3. Handle Stall
	if action == ActionStall {
		if err := i.handleStall(taskID, method, mcpReq, matchedRule); err != nil {
			i.SendErrorResponse(req, http.StatusForbidden, -32000, "Stall rejected")
			return
		}
	}

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
	if err := assert.Check(len(body) < 1024*1024, "request body too large", "size", len(body)); err != nil {
		return nil, "", "", err
	}

	var mcpReq mcp.MCPRequest
	if err := json.Unmarshal(body, &mcpReq); err != nil {
		return nil, "", "", fmt.Errorf("invalid JSON-RPC: %w", err)
	}

	if err := assert.Check(mcpReq.Method != "", "method must not be empty"); err != nil {
		return nil, "", "", err
	}
	if err := assert.Check(mcpReq.JSONRPC == "2.0", "invalid JSON-RPC version", "version", mcpReq.JSONRPC); err != nil {
		return nil, "", "", err
	}

	taskID, _ := mcpReq.Params["task_id"].(string)
	return &mcpReq, taskID, mcpReq.Method, nil
}

// evaluatePolicy determines the action for the request
func (i *Interceptor) evaluatePolicy(method string, params map[string]interface{}) (PolicyAction, *proxy.PolicyRule, error) {
	if err := assert.Check(i.Core.Policy != nil, "policy configuration missing"); err != nil {
		return ActionAllow, nil, err
	}
	if err := assert.Check(method != "", "method name is non-empty"); err != nil {
		return ActionAllow, nil, err
	}

	for _, rule := range i.Core.Policy.Policies {
		for _, pattern := range rule.MatchMethods {
			if proxy.MatchPattern(pattern, method) {
				if rule.Conditions != nil && !proxy.CheckConditions(rule.Conditions, params) {
					continue
				}

				action := PolicyAction(rule.Action)
				if action == "" {
					action = ActionAllow
				}
				return action, &rule, nil
			}
		}
	}

	return ActionAllow, nil, nil
}

// handleStall manages the approval workflow
func (i *Interceptor) handleStall(taskID, method string, mcpReq *mcp.MCPRequest, matchedRule *proxy.PolicyRule) error {
	if err := assert.Check(mcpReq != nil, "mcpReq must not be nil"); err != nil {
		return err
	}
	if err := assert.Check(matchedRule != nil, "matchedRule must not be nil"); err != nil {
		return err
	}

	eventID := uuid.New().String()[:8]
	log.Printf("[STALL] Method: %s | Policy: %s | ID: %s", method, matchedRule.ID, eventID)

	event := pool.GetEvent()
	event.ID = eventID
	event.Timestamp = time.Now()
	event.EventType = "blocked"
	event.Method = method
	event.Params = mcpReq.Params
	event.TaskID = taskID
	event.PolicyID = matchedRule.ID
	event.RiskLevel = matchedRule.RiskLevel
	event.WasBlocked = true

	i.Core.Worker.Submit(event)

	approvalChan := make(chan bool, 1)
	i.Core.StallSignals.Store(eventID, approvalChan)

	log.Printf("Waiting for approval (ID: %s)...", eventID)

	select {
	case approved := <-approvalChan:
		if !approved {
			return fmt.Errorf("stall rejected")
		}
		return nil
	case <-time.After(10 * time.Minute):
		return fmt.Errorf("stall timeout")
	}
}

// submitToolCallEvent prepares and sends the tool_call event to the ledger
func (i *Interceptor) submitToolCallEvent(taskID string, mcpReq *mcp.MCPRequest, matchedRule *proxy.PolicyRule) {
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

// SendErrorResponse sends a JSON-RPC error response
func (i *Interceptor) SendErrorResponse(req *http.Request, statusCode int, code int, message string) {
	errorResp := mcp.MCPResponse{
		JSONRPC: "2.0",
		ID:      nil,
		Error: map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}

	respBytes, err := json.Marshal(errorResp)
	if err != nil {
		log.Printf("[CRITICAL] Failed to marshal error response: %v", err)
		return
	}
	log.Printf("[SECURITY] Blocking request: %s (JSON: %s)", message, string(respBytes))
}
