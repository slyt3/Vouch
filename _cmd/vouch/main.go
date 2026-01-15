package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/vouch/internal/ledger"
	"github.com/yourname/vouch/internal/policy"
	"github.com/yourname/vouch/internal/proxy"
)

// VouchProxy is the main proxy server
type VouchProxy struct {
	reverseProxy *httputil.ReverseProxy
	ledgerWorker *ledger.Worker
	policyEngine *policy.Engine
	activeTasks  *sync.Map // SEP-1686 async task tracking
	stallSignals *sync.Map // Maps event ID to approval channel
}

func main() {
	log.Println("Vouch (Agent Analytics & Safety) - Phase 1: The Interceptor")
	log.Println("========================================================")

	// Initialize policy engine
	policyEngine, err := policy.NewEngine("vouch-policy.yaml")
	if err != nil {
		log.Fatalf("[ERROR] Failed to load policy: %v", err)
	}
	log.Printf("[INFO] Loaded policy version %s with %d rules", policyEngine.GetVersion(), policyEngine.GetRuleCount())

	// Initialize ledger worker (Phase 1: console logging, Phase 2: SQLite)
	ledgerWorker := ledger.NewWorker(1000) // 1000-event buffer
	ledgerWorker.Start()

	// Create target URL
	targetURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	// Create reverse proxy
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Create Vouch proxy
	vouchProxy := &VouchProxy{
		reverseProxy: reverseProxy,
		ledgerWorker: ledgerWorker,
		policyEngine: policyEngine,
		activeTasks:  &sync.Map{},
		stallSignals: &sync.Map{},
	}

	// Custom director to intercept requests
	originalDirector := reverseProxy.Director
	reverseProxy.Director = func(req *http.Request) {
		originalDirector(req)
		vouchProxy.interceptRequest(req)
	}

	// Custom response modifier
	reverseProxy.ModifyResponse = vouchProxy.interceptResponse

	// Start server
	listenAddr := ":9999"
	log.Printf("[INFO] Proxying :9999 -> :8080")
	log.Printf("[INFO] Event buffer size: 1000")
	log.Printf("[INFO] Policy engine: ACTIVE")
	log.Println("========================================================")
	log.Printf("[INFO] Ready to intercept MCP traffic on %s\n", listenAddr)

	if err := http.ListenAndServe(listenAddr, reverseProxy); err != nil {
		log.Fatalf("[ERROR] Server failed: %v", err)
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
	var mcpReq proxy.MCPRequest
	if err := json.Unmarshal(bodyBytes, &mcpReq); err != nil {
		// Not a JSON-RPC request, skip
		return
	}

	// Check if method should be stalled (Policy Guard)
	shouldStall, matchedRule := v.policyEngine.ShouldStall(mcpReq.Method, mcpReq.Params)

	if shouldStall {
		// Create event ID
		eventID := uuid.New().String()[:8]

		// Log the stall
		log.Println("========================================================")
		log.Printf("[STALL] Policy violation detected")
		log.Printf("[STALL] Method: %s", mcpReq.Method)
		log.Printf("[STALL] Policy: %s (Risk: %s)", matchedRule.ID, matchedRule.RiskLevel)
		log.Printf("[STALL] Event ID: %s", eventID)
		log.Println("========================================================")

		// Create blocked event (Article 12 "Proof of Refusal")
		event := proxy.Event{
			ID:         eventID,
			Timestamp:  time.Now(),
			EventType:  "blocked",
			Method:     mcpReq.Method,
			Params:     mcpReq.Params,
			WasBlocked: true,
		}
		v.ledgerWorker.Submit(event)

		// Create approval channel
		approvalChan := make(chan bool, 1)
		v.stallSignals.Store(eventID, approvalChan)

		log.Printf("[STALL] Waiting for approval (Event ID: %s)", eventID)

		// For demo/testing: auto-approve after 2 seconds
		// In production, this would be handled by the CLI tool
		go func() {
			time.Sleep(2 * time.Second)
			approvalChan <- true
		}()

		// Wait for approval
		<-approvalChan
		log.Printf("[APPROVED] Event %s approved, continuing", eventID)
	}

	// Extract task_id if present (SEP-1686)
	var taskID string
	var taskState string
	if params, ok := mcpReq.Params["task_id"].(string); ok {
		taskID = params
		taskState = "working" // Default state
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

	// Track task if present (SEP-1686 state machine)
	if taskID != "" {
		v.activeTasks.Store(taskID, taskState)
	}

	// Submit to async worker (non-blocking)
	v.ledgerWorker.Submit(event)
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
	var mcpResp proxy.MCPResponse
	if err := json.Unmarshal(bodyBytes, &mcpResp); err != nil {
		// Not a JSON-RPC response, skip
		return nil
	}

	// Check for task information in response (SEP-1686)
	var taskID string
	var taskState string

	if result := mcpResp.Result; result != nil {
		if tid, ok := result["task_id"].(string); ok {
			taskID = tid
		}
		if state, ok := result["state"].(string); ok {
			taskState = state
			// Update active tasks map with new state
			// States: working | input_required | completed | failed | cancelled
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

	// Submit to async worker (non-blocking)
	v.ledgerWorker.Submit(event)

	return nil
}
