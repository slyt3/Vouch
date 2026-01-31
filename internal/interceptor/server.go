package interceptor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/core"
	"github.com/slyt3/Logryph/internal/logging"
	"github.com/slyt3/Logryph/internal/mcp"
	"github.com/slyt3/Logryph/internal/observer"
	"github.com/slyt3/Logryph/internal/pool"
)

// PolicyAction defines the outcome of a policy check
type PolicyAction string

const (
	ActionAllow  PolicyAction = "allow"
	ActionTag    PolicyAction = "tag"    // Renamed from Redact/Stall
	ActionRedact PolicyAction = "redact" // Keep for hygiene, but not blocking
)

const (
	maxPolicies   = 256
	maxPatterns   = 128
	maxConditions = 64
	maxRedactKeys = 128
	maxParams     = 256
)

// Interceptor handles HTTP proxy interception and MCP JSON-RPC request/response capture.
// It evaluates policies, applies redaction rules, and submits events to the ledger
// without blocking agent traffic (fail-open behavior).
type Interceptor struct {
	Core *core.Engine
}

func NewInterceptor(engine *core.Engine) *Interceptor {
	return &Interceptor{Core: engine}
}

// InterceptRequest captures HTTP POST requests, extracts MCP metadata, evaluates policies,
// applies redaction rules, and submits events to the async worker.
// Returns immediately without blocking proxy traffic. Drops events on backpressure.
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
		logging.Error("request_body_read_failed", logging.Fields{Component: "interceptor", Error: err.Error()})
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
	requestID := ""
	if mcpReq.ID != nil {
		requestID = fmt.Sprint(mcpReq.ID)
	}

	// 2. Policy Evaluation
	action, matchedRule, err := i.evaluatePolicy(method, mcpReq.Params)
	if err != nil {
		logging.Warn("policy_evaluation_failed", logging.Fields{Component: "interceptor", RequestID: requestID, TaskID: taskID, Method: method, Error: err.Error()})
		i.SendErrorResponse(req, http.StatusBadRequest, -32000, "Policy violation")
		return
	}

	// 3. Handle Stall (REMOVED - Phase 2 Lobotomy)
	// We no longer block traffic. We only observe.
	// if action == ActionStall { ... }

	// 4. Apply Redaction & Submit Event
	if err := i.applyRedactionAndSubmit(req, action, matchedRule, bodyBytes, requestID, taskID, method, mcpReq); err != nil {
		return
	}
}

// applyRedactionAndSubmit handles redaction and event submission
func (i *Interceptor) applyRedactionAndSubmit(req *http.Request, action PolicyAction, matchedRule *observer.Rule, bodyBytes []byte, requestID, taskID, method string, mcpReq *mcp.MCPRequest) error {
	if err := assert.Check(mcpReq != nil, "mcpReq must not be nil"); err != nil {
		return err
	}
	if err := assert.Check(len(method) > 0, "method must not be empty"); err != nil {
		return err
	}

	// Redaction (if needed)
	if action == ActionRedact && matchedRule != nil {
		scrubbedBody, err := i.redactSensitiveData(bodyBytes, matchedRule.Redact)
		if err != nil {
			logging.Error("redaction_failed", logging.Fields{Component: "interceptor", RequestID: requestID, TaskID: taskID, Method: method, PolicyID: matchedRule.ID, RiskLevel: matchedRule.RiskLevel, Error: err.Error()})
			i.SendErrorResponse(req, http.StatusInternalServerError, -32000, "Redaction failed")
			return err
		}
		bodyBytes = scrubbedBody
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	logging.Info("request_observed", logging.Fields{Component: "interceptor", RequestID: requestID, TaskID: taskID, Method: method, PolicyID: policyIDOrEmpty(matchedRule), RiskLevel: riskLevelOrEmpty(matchedRule)})

	// Submit Event & Forward
	i.submitToolCallEvent(taskID, mcpReq, matchedRule)
	return nil
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
	policies := i.Core.Observer.GetPolicies()
	if err := assert.Check(len(policies) <= maxPolicies, "policy count exceeds max: %d", len(policies)); err != nil {
		return ActionAllow, nil, err
	}

	// Use the new Observer engine
	for i := 0; i < maxPolicies; i++ {
		if i >= len(policies) {
			break
		}
		rule := &policies[i]
		if err := assert.Check(len(rule.MatchMethods) <= maxPatterns, "match_methods exceeds max in rule=%s", rule.ID); err != nil {
			return ActionAllow, nil, err
		}
		if err := assert.Check(len(rule.MatchConditions) <= maxConditions, "conditions exceeds max in rule=%s", rule.ID); err != nil {
			return ActionAllow, nil, err
		}
		for j := 0; j < maxPatterns; j++ {
			if j >= len(rule.MatchMethods) {
				break
			}
			pattern := rule.MatchMethods[j]
			if observer.MatchPattern(pattern, method) {
				if len(rule.MatchConditions) > 0 && !observer.CheckConditions(rule.MatchConditions, params) {
					continue
				}

				action := ActionTag
				if len(rule.Redact) > 0 {
					action = ActionRedact
				}

				return action, rule, nil
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
			if pid, ok := parentID.(string); ok {
				event.ParentID = pid
			} else {
				if err := assert.Check(false, "parentID has unexpected type for taskID=%s", taskID); err != nil {
					logging.Warn("parent_id_type_mismatch", logging.Fields{Component: "interceptor", TaskID: taskID})
				}
			}
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
	if err := assert.Check(len(keys) <= maxRedactKeys, "redaction keys exceed max: %d", len(keys)); err != nil {
		return nil, err
	}

	var mcpReq mcp.MCPRequest
	if err := json.Unmarshal(body, &mcpReq); err != nil {
		return nil, err
	}

	if err := assert.Check(len(mcpReq.Params) <= maxParams, "excessive parameters in request: %d", len(mcpReq.Params)); err != nil {
		return nil, err
	}
	for i := 0; i < maxRedactKeys; i++ {
		if i >= len(keys) {
			break
		}
		key := keys[i]
		if _, ok := mcpReq.Params[key]; ok {
			mcpReq.Params[key] = "[REDACTED]"
		}
	}

	return json.Marshal(mcpReq)
}

// InterceptResponse captures HTTP responses, extracts task_id and state from MCP results,
// and submits tool_response events to the ledger. Returns nil on JSON parse errors
// to maintain fail-open behavior.
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

	requestID := ""
	if mcpResp.ID != nil {
		requestID = fmt.Sprint(mcpResp.ID)
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

	logging.Info("response_observed", logging.Fields{Component: "interceptor", RequestID: requestID, TaskID: taskID})

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

// SendErrorResponse is a no-op stub retained for backwards compatibility.
// Phase 2 (Lobotomy) removed request blocking - Logryph now operates as a passive recorder.
// Does not send HTTP errors or block traffic.
func (i *Interceptor) SendErrorResponse(req *http.Request, statusCode int, code int, message string) {
	// Passive: We do not block. We just log the failure to record if needed.
}

func policyIDOrEmpty(rule *observer.Rule) string {
	if rule == nil {
		return ""
	}
	if err := assert.Check(rule.ID != "", "policy ID missing"); err != nil {
		return ""
	}
	if err := assert.Check(len(rule.ID) <= 128, "policy ID too long: %d", len(rule.ID)); err != nil {
		return ""
	}
	return rule.ID
}

func riskLevelOrEmpty(rule *observer.Rule) string {
	if rule == nil {
		return ""
	}
	if err := assert.Check(rule.RiskLevel != "", "risk level missing"); err != nil {
		return ""
	}
	if err := assert.Check(len(rule.RiskLevel) <= 32, "risk level too long: %d", len(rule.RiskLevel)); err != nil {
		return ""
	}
	return rule.RiskLevel
}
