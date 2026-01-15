package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/vouch/internal/ledger"
	"github.com/yourname/vouch/internal/proxy"
)

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

// VouchProxy is the main proxy server
type VouchProxy struct {
	proxy           *httputil.ReverseProxy
	worker          *ledger.Worker
	activeTasks     *sync.Map // task_id -> state
	policy          *proxy.PolicyConfig
	stallSignals    *sync.Map // Maps event ID to approval channel
	lastEventByTask *sync.Map // task_id -> last_event_id
}

func main() {
	log.Println("Vouch (Agent Analytics & Safety) - The Interceptor")

	// Load policy
	policy, err := proxy.LoadPolicy("vouch-policy.yaml")
	if err != nil {
		log.Fatalf("Failed to load policy: %v", err)
	}
	log.Printf("Loaded policy version %s with %d rules", policy.Version, len(policy.Policies))

	// Create target URL
	targetURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	// Create proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Initialize ledger worker with database and crypto
	worker, err := ledger.NewWorker(1000, "vouch.db", ".vouch_key")
	if err != nil {
		log.Fatalf("Failed to initialize worker: %v", err)
	}

	// Start worker (creates genesis block if needed)
	if err := worker.Start(); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}

	// Create Vouch proxy
	vouchProxy := &VouchProxy{
		proxy:           proxy,
		worker:          worker,
		activeTasks:     &sync.Map{}, // task_id -> state
		policy:          policy,
		stallSignals:    &sync.Map{}, // event_id -> chan struct{}
		lastEventByTask: &sync.Map{}, // task_id -> last_event_id
	}

	// Custom director to intercept requests
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		vouchProxy.interceptRequest(req)
	}

	// Custom response modifier
	proxy.ModifyResponse = vouchProxy.interceptResponse

	// Start API server for CLI commands (approval/rejection)
	go func() {
		apiMux := http.NewServeMux()
		apiMux.HandleFunc("/api/approve/", vouchProxy.handleApprove)
		apiMux.HandleFunc("/api/reject/", vouchProxy.handleReject)
		apiMux.HandleFunc("/api/rekey", vouchProxy.handleRekey)

		apiAddr := ":9998"
		log.Printf("API server listening on %s", apiAddr)
		if err := http.ListenAndServe(apiAddr, apiMux); err != nil {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	// Start proxy server
	listenAddr := ":9999"
	log.Printf("Proxying :9999 -> :8080")
	log.Printf("Event buffer size: 1000")
	log.Printf("Policy engine: ACTIVE")
	log.Printf("Ready to intercept MCP traffic on %s\n", listenAddr)

	if err := http.ListenAndServe(listenAddr, proxy); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// interceptRequest intercepts and analyzes incoming requests
func (v *VouchProxy) interceptRequest(req *http.Request) {
	// Only intercept POST requests (MCP uses JSON-RPC over HTTP POST)
	if req.Method != http.MethodPost {
		return
	}

	// Read body
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		return
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Try to parse as MCP request
	var mcpReq MCPRequest
	if err := json.Unmarshal(bodyBytes, &mcpReq); err != nil {
		// Not a JSON-RPC request, skip
		return
	}

	// Extract task_id if present (SEP-1686)
	var taskID string
	var taskState string
	if params, ok := mcpReq.Params["task_id"].(string); ok {
		taskID = params
		taskState = "working" // Default state
	}

	// Check if method should be stalled (Policy Guard)
	shouldStall, matchedRule := v.shouldStallMethod(mcpReq.Method, mcpReq.Params)

	if shouldStall {
		// Create event ID
		eventID := uuid.New().String()[:8]

		// Log the stall
		log.Printf("STALL DETECTED")
		log.Printf("Method: %s", mcpReq.Method)
		log.Printf("Policy: %s (Risk: %s)", matchedRule.ID, matchedRule.RiskLevel)
		log.Printf("Event ID: %s", eventID)

		// Create blocked event
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
		v.worker.Submit(event)

		// Create approval channel
		approvalChan := make(chan bool, 1)
		v.stallSignals.Store(eventID, approvalChan)

		// Stall Intelligence: Check for previous failures in this task
		if taskID != "" {
			failCount, err := v.worker.GetDB().GetTaskFailureCount(taskID)
			if err == nil && failCount > 0 {
				log.Printf("⚠️ STALL INTELLIGENCE WARNING: Task %s has failed %d times in previous events.", taskID, failCount)
			}
		}

		log.Printf("Waiting for approval... (Type 'vouch approve %s' or press Enter to continue)", eventID)

		// For demo purposes, we'll wait for a simple stdin signal
		// In production, this would be handled by the CLI tool
		go func() {
			var input string
			_, _ = fmt.Scanln(&input)
			if _, ok := v.stallSignals.Load(eventID); ok {
				approvalChan <- true
			}
		}()

		// Wait for approval
		<-approvalChan
		log.Printf("Event %s approved, continuing...", eventID)
	}

	// Create event
	event := proxy.Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_call",
		Method:    mcpReq.Method,
		Params:    mcpReq.Params,
		TaskID:    taskID,
		TaskState: taskState,
	}

	// If a rule matched but didn't stall, we still want the metadata
	if matchedRule != nil {
		event.PolicyID = matchedRule.ID
		event.RiskLevel = matchedRule.RiskLevel

		// Apply redaction if policy specifies it
		if len(matchedRule.Redact) > 0 {
			event.Params = redactParams(mcpReq.Params, matchedRule.Redact)
		}
	}

	// Hierarchy tracking: if this task has a previous event, link it
	if taskID != "" {
		if parentID, ok := v.lastEventByTask.Load(taskID); ok {
			event.ParentID = parentID.(string)
		}
		v.lastEventByTask.Store(taskID, event.ID)
	}

	// Track task if present
	if taskID != "" {
		v.activeTasks.Store(taskID, taskState)
	}

	// Send to async worker
	v.worker.Submit(event)
}

// interceptResponse intercepts and analyzes responses
func (v *VouchProxy) interceptResponse(resp *http.Response) error {
	// Read body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Try to parse as MCP response
	var mcpResp MCPResponse
	if err := json.Unmarshal(bodyBytes, &mcpResp); err != nil {
		// Not a JSON-RPC response, skip
		return nil
	}

	// Check for task information in response
	var taskID string
	var taskState string

	if result := mcpResp.Result; result != nil {
		if tid, ok := result["task_id"].(string); ok {
			taskID = tid
		}
		if state, ok := result["state"].(string); ok {
			taskState = state
			// Update active tasks map
			if taskID != "" {
				v.activeTasks.Store(taskID, taskState)
			}
		}
	}

	// Create response event
	event := proxy.Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_response",
		Response:  mcpResp.Result,
		TaskID:    taskID,
		TaskState: taskState,
	}

	// Send to async worker
	v.worker.Submit(event)

	return nil
}

// shouldStallMethod checks if a method should be stalled based on policy
func (v *VouchProxy) shouldStallMethod(method string, params map[string]interface{}) (bool, *proxy.PolicyRule) {
	for _, rule := range v.policy.Policies {
		if rule.Action != "stall" {
			continue
		}

		// Check method match with wildcard support
		for _, pattern := range rule.MatchMethods {
			if proxy.MatchPattern(pattern, method) {
				// Check additional conditions if present
				if rule.Conditions != nil {
					if !proxy.CheckConditions(rule.Conditions, params) {
						continue
					}
				}
				return true, &rule
			}
		}
	}
	return false, nil
}

// redactParams removes sensitive keys from parameters
func redactParams(params map[string]interface{}, keys []string) map[string]interface{} {
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

// handleRekey handles key rotation requests
func (v *VouchProxy) handleRekey(w http.ResponseWriter, r *http.Request) {
	oldPubKey, newPubKey, err := v.worker.GetSigner().RotateKey(".vouch_key")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to rotate key: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("KEY ROTATION SUCCESSFUL")
	log.Printf("Old Public Key: %s", oldPubKey)
	log.Printf("New Public Key: %s", newPubKey)

	_, _ = fmt.Fprintf(w, "Key rotated successfully\nOld: %s\nNew: %s", oldPubKey, newPubKey)
}

// handleApprove handles approval requests from the CLI
func (v *VouchProxy) handleApprove(w http.ResponseWriter, r *http.Request) {
	// Extract event ID from URL path
	eventID := strings.TrimPrefix(r.URL.Path, "/api/approve/")

	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	// Look up the approval channel
	val, ok := v.stallSignals.Load(eventID)
	if !ok {
		http.Error(w, "Event not found or already processed", http.StatusNotFound)
		return
	}

	approvalChan := val.(chan bool)

	// Send approval signal
	select {
	case approvalChan <- true:
		log.Printf("Event %s approved via CLI", eventID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Event approved\n"))
	default:
		http.Error(w, "Event already processed", http.StatusConflict)
	}
}

// handleReject handles rejection requests from the CLI
func (v *VouchProxy) handleReject(w http.ResponseWriter, r *http.Request) {
	// Extract event ID from URL path
	eventID := strings.TrimPrefix(r.URL.Path, "/api/reject/")

	if eventID == "" {
		http.Error(w, "Event ID required", http.StatusBadRequest)
		return
	}

	// Look up the approval channel
	val, ok := v.stallSignals.Load(eventID)
	if !ok {
		http.Error(w, "Event not found or already processed", http.StatusNotFound)
		return
	}

	approvalChan := val.(chan bool)

	// Send rejection signal (false)
	select {
	case approvalChan <- false:
		log.Printf("Event %s rejected via CLI", eventID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Event rejected\n"))
	default:
		http.Error(w, "Event already processed", http.StatusConflict)
	}
}
