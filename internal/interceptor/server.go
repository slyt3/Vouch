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
	"github.com/yourname/vouch/internal/assert"
	"github.com/yourname/vouch/internal/core"
	"github.com/yourname/vouch/internal/mcp"
	"github.com/yourname/vouch/internal/proxy"
)

// Interceptor handles the proxy interception logic
type Interceptor struct {
	Core *core.Engine
}

func NewInterceptor(engine *core.Engine) *Interceptor {
	return &Interceptor{Core: engine}
}

// InterceptRequest intercepts and analyzes incoming requests
func (i *Interceptor) InterceptRequest(req *http.Request) {
	if req.Method != http.MethodPost {
		return
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		return
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 1. Extract Metadata
	mcpReq, taskID, taskState, err := i.extractTaskMetadata(bodyBytes)
	if err != nil {
		i.SendErrorResponse(req, http.StatusBadRequest, -32000, err.Error())
		return
	}

	// 2. Health Check
	if !i.Core.Worker.IsHealthy() {
		i.SendErrorResponse(req, http.StatusServiceUnavailable, -32000, "Ledger Storage Failure")
		return
	}

	// 3. Policy Evaluation
	shouldStall, matchedRule, err := i.evaluatePolicy(mcpReq.Method, mcpReq.Params)
	if err != nil {
		i.SendErrorResponse(req, http.StatusBadRequest, -32000, "Policy violation")
		return
	}

	// 4. Handle Stall (Human-in-the-loop)
	if shouldStall {
		if err := i.handleStall(taskID, taskState, mcpReq, matchedRule); err != nil {
			i.SendErrorResponse(req, http.StatusForbidden, -32000, "Stall rejected or failed")
			return
		}
	}

	// 5. Finalize Event & Submit
	i.submitToolCallEvent(taskID, taskState, mcpReq, matchedRule)
}

// extractTaskMetadata parses and validates the request
func (i *Interceptor) extractTaskMetadata(body []byte) (*mcp.MCPRequest, string, string, error) {
	if err := assert.Check(len(body) > 0, "request body is empty"); err != nil {
		return nil, "", "", err
	}

	var mcpReq mcp.MCPRequest
	if err := json.Unmarshal(body, &mcpReq); err != nil {
		return nil, "", "", fmt.Errorf("invalid JSON-RPC: %w", err)
	}

	if err := assert.Check(mcpReq.Method != "", "method must not be empty"); err != nil {
		return nil, "", "", err
	}

	taskID, _ := mcpReq.Params["task_id"].(string)
	taskState := "working"

	if taskID != "" {
		if err := assert.Check(len(taskID) <= 64, "task_id too long", "id", taskID); err != nil {
			return nil, "", "", err
		}
	}

	return &mcpReq, taskID, taskState, nil
}

// evaluatePolicy determines the action for the request
func (i *Interceptor) evaluatePolicy(method string, params map[string]interface{}) (bool, *proxy.PolicyRule, error) {
	if err := assert.Check(i.Core.Policy != nil, "policy configuration missing"); err != nil {
		return false, nil, err
	}

	// shouldStallMethod logic
	for _, rule := range i.Core.Policy.Policies {
		if rule.Action != "stall" {
			continue
		}

		for _, pattern := range rule.MatchMethods {
			if proxy.MatchPattern(pattern, method) {
				if rule.Conditions != nil {
					if !proxy.CheckConditions(rule.Conditions, params) {
						continue
					}
				}
				return true, &rule, nil
			}
		}
	}

	return false, nil, nil
}

// handleStall manages the approval workflow
func (i *Interceptor) handleStall(taskID, taskState string, mcpReq *mcp.MCPRequest, matchedRule *proxy.PolicyRule) error {
	if err := assert.Check(mcpReq != nil, "mcpReq must not be nil"); err != nil {
		return err
	}

	eventID := uuid.New().String()[:8]
	log.Printf("[STALL] Method: %s | Policy: %s | ID: %s", mcpReq.Method, matchedRule.ID, eventID)

	event := proxy.Event{
		ID:         eventID,
		Timestamp:  time.Now(),
		EventType:  "blocked",
		Method:     mcpReq.Method,
		Params:     mcpReq.Params,
		TaskID:     taskID,
		TaskState:  taskState,
		PolicyID:   matchedRule.ID,
		RiskLevel:  matchedRule.RiskLevel,
		WasBlocked: true,
	}
	i.Core.Worker.Submit(event)

	approvalChan := make(chan bool, 1)
	i.Core.StallSignals.Store(eventID, approvalChan)

	log.Printf("Waiting for approval (ID: %s)...", eventID)

	if !<-approvalChan {
		return fmt.Errorf("stall rejected")
	}

	return nil
}

// submitToolCallEvent prepares and sends the tool_call event to the ledger
func (i *Interceptor) submitToolCallEvent(taskID, taskState string, mcpReq *mcp.MCPRequest, matchedRule *proxy.PolicyRule) {
	event := proxy.Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_call",
		Method:    mcpReq.Method,
		Params:    mcpReq.Params,
		TaskID:    taskID,
		TaskState: taskState,
	}

	if matchedRule != nil {
		event.PolicyID = matchedRule.ID
		event.RiskLevel = matchedRule.RiskLevel
		if len(matchedRule.Redact) > 0 {
			event.Params = i.RedactSensitiveData(mcpReq.Params, matchedRule.Redact)
		}
	}

	if taskID != "" {
		if parentID, ok := i.Core.LastEventByTask.Load(taskID); ok {
			event.ParentID = parentID.(string)
		}
		i.Core.LastEventByTask.Store(taskID, event.ID)
		i.Core.ActiveTasks.Store(taskID, taskState)
	}

	i.Core.Worker.Submit(event)
}

// RedactSensitiveData scrubs PII based on policy
func (i *Interceptor) RedactSensitiveData(params map[string]interface{}, keys []string) map[string]interface{} {
	redacted := make(map[string]interface{})
	for k, v := range params {
		shouldRedact := false
		for _, key := range keys {
			if k == key {
				shouldRedact = true
				break
			}
		}
		if shouldRedact {
			redacted[k] = "[REDACTED]"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// InterceptResponse intercepts and analyzes responses
func (i *Interceptor) InterceptResponse(resp *http.Response) error {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
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

	event := proxy.Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_response",
		Response:  mcpResp.Result,
		TaskID:    taskID,
		TaskState: taskState,
	}

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

	respBytes, _ := json.Marshal(errorResp)
	log.Printf("[SECURITY] Blocking request: %s (JSON: %s)", message, string(respBytes))
}
