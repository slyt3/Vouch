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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
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
	TaskState  string                 `json:"task_state,omitempty"`
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

// PolicyConfig represents the ael-policy.yaml structure
type PolicyConfig struct {
	Version  string `yaml:"version"`
	Defaults struct {
		RetentionDays  int    `yaml:"retention_days"`
		SigningEnabled bool   `yaml:"signing_enabled"`
		LogLevel       string `yaml:"log_level"`
	} `yaml:"defaults"`
	Policies []PolicyRule `yaml:"policies"`
}

// PolicyRule represents a single policy rule
type PolicyRule struct {
	ID               string                 `yaml:"id"`
	MatchMethods     []string               `yaml:"match_methods"`
	RiskLevel        string                 `yaml:"risk_level"`
	Action           string                 `yaml:"action"`
	ProofOfRefusal   bool                   `yaml:"proof_of_refusal"`
	LogLevel         string                 `yaml:"log_level,omitempty"`
	Conditions       map[string]interface{} `yaml:"conditions,omitempty"`
}

// AELProxy is the main proxy server
type AELProxy struct {
	proxy        *httputil.ReverseProxy
	eventChannel chan Event
	activeTasks  *sync.Map
	policy       *PolicyConfig
	stallSignals *sync.Map // Maps event ID to approval channel
}

func main() {
	log.Println("🚀 AEL (Agent Execution Ledger) - Phase 1: The Interceptor")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Load policy
	policy, err := loadPolicy("ael-policy.yaml")
	if err != nil {
		log.Fatalf("❌ Failed to load policy: %v", err)
	}
	log.Printf("✅ Loaded policy version %s with %d rules", policy.Version, len(policy.Policies))

	// Create target URL
	targetURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatalf("❌ Failed to parse target URL: %v", err)
	}

	// Create proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	
	// Create AEL proxy with async worker
	aelProxy := &AELProxy{
		proxy:        proxy,
		eventChannel: make(chan Event, 1000), // Buffered channel for async processing
		activeTasks:  &sync.Map{},
		policy:       policy,
		stallSignals: &sync.Map{},
	}

	// Start async worker
	go aelProxy.asyncWorker()

	// Custom director to intercept requests
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		aelProxy.interceptRequest(req)
	}

	// Custom response modifier
	proxy.ModifyResponse = aelProxy.interceptResponse

	// Start server
	listenAddr := ":9999"
	log.Printf("🎯 Proxying :9999 → :8080")
	log.Printf("📊 Event buffer size: 1000")
	log.Printf("🔍 Policy engine: ACTIVE")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("✨ Ready to intercept MCP traffic on %s\n", listenAddr)

	if err := http.ListenAndServe(listenAddr, proxy); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}

// loadPolicy loads the ael-policy.yaml file
func loadPolicy(path string) (*PolicyConfig, error) {
	// Try absolute path first, then relative
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err == nil {
			path = filepath.Join(wd, path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	var config PolicyConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}

	return &config, nil
}

// asyncWorker processes events asynchronously
func (a *AELProxy) asyncWorker() {
	log.Println("🔧 Async worker started")
	for event := range a.eventChannel {
		// Phase 1: Just log to console
		// Phase 2: Will write to SQLite with hash chaining
		timestamp := event.Timestamp.Format("15:04:05.000")
		
		if event.WasBlocked {
			log.Printf("🚫 [%s] BLOCKED | %s | ID: %s", timestamp, event.Method, event.ID)
			log.Printf("   └─ Proof of Refusal recorded for audit trail")
		} else if event.EventType == "tool_call" {
			log.Printf("📤 [%s] CALL    | %s | ID: %s", timestamp, event.Method, event.ID)
			if event.TaskID != "" {
				log.Printf("   └─ Task ID: %s (State: %s)", event.TaskID, event.TaskState)
			}
		} else if event.EventType == "tool_response" {
			log.Printf("📥 [%s] RESPONSE| %s | ID: %s", timestamp, event.Method, event.ID)
			if event.TaskID != "" {
				log.Printf("   └─ Task ID: %s (State: %s)", event.TaskID, event.TaskState)
			}
		}
	}
}

// interceptRequest intercepts and analyzes incoming requests
func (a *AELProxy) interceptRequest(req *http.Request) {
	// Only intercept POST requests (MCP uses JSON-RPC over HTTP POST)
	if req.Method != http.MethodPost {
		return
	}

	// Read body
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("⚠️  Failed to read request body: %v", err)
		return
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Try to parse as MCP request
	var mcpReq MCPRequest
	if err := json.Unmarshal(bodyBytes, &mcpReq); err != nil {
		// Not a JSON-RPC request, skip
		return
	}

	// Check if method should be stalled
	shouldStall, matchedRule := a.shouldStallMethod(mcpReq.Method, mcpReq.Params)
	
	if shouldStall {
		// Create event ID
		eventID := uuid.New().String()[:8]
		
		// Log the stall
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("🛑 STALL DETECTED")
		log.Printf("   Method: %s", mcpReq.Method)
		log.Printf("   Policy: %s (Risk: %s)", matchedRule.ID, matchedRule.RiskLevel)
		log.Printf("   Event ID: %s", eventID)
		log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		
		// Create blocked event (Proof of Refusal)
		event := Event{
			ID:         eventID,
			Timestamp:  time.Now(),
			EventType:  "blocked",
			Method:     mcpReq.Method,
			Params:     mcpReq.Params,
			WasBlocked: true,
		}
		a.eventChannel <- event
		
		// Create approval channel
		approvalChan := make(chan bool, 1)
		a.stallSignals.Store(eventID, approvalChan)
		
		log.Printf("⏸️  Waiting for approval... (Type 'ael approve %s' or press Enter to continue)", eventID)
		
		// For demo purposes, we'll wait for a simple stdin signal
		// In production, this would be handled by the CLI tool
		go func() {
			var input string
			fmt.Scanln(&input)
			approvalChan <- true
		}()
		
		// Wait for approval
		<-approvalChan
		log.Printf("✅ Event %s approved, continuing...", eventID)
	}

	// Extract task_id if present (SEP-1686)
	var taskID string
	var taskState string
	if params, ok := mcpReq.Params["task_id"].(string); ok {
		taskID = params
		taskState = "working" // Default state
	}

	// Create event
	event := Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_call",
		Method:    mcpReq.Method,
		Params:    mcpReq.Params,
		TaskID:    taskID,
		TaskState: taskState,
	}

	// Track task if present
	if taskID != "" {
		a.activeTasks.Store(taskID, taskState)
	}

	// Send to async worker
	a.eventChannel <- event
}

// interceptResponse intercepts and analyzes responses
func (a *AELProxy) interceptResponse(resp *http.Response) error {
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
				a.activeTasks.Store(taskID, taskState)
			}
		}
	}

	// Create response event
	event := Event{
		ID:        uuid.New().String()[:8],
		Timestamp: time.Now(),
		EventType: "tool_response",
		Response:  mcpResp.Result,
		TaskID:    taskID,
		TaskState: taskState,
	}

	// Send to async worker
	a.eventChannel <- event

	return nil
}

// shouldStallMethod checks if a method should be stalled based on policy
func (a *AELProxy) shouldStallMethod(method string, params map[string]interface{}) (bool, *PolicyRule) {
	for _, rule := range a.policy.Policies {
		if rule.Action != "stall" {
			continue
		}

		// Check method match with wildcard support
		for _, pattern := range rule.MatchMethods {
			if matchPattern(pattern, method) {
				// Check additional conditions if present
				if rule.Conditions != nil {
					if !a.checkConditions(rule.Conditions, params) {
						continue
					}
				}
				return true, &rule
			}
		}
	}
	return false, nil
}

// matchPattern matches a method against a pattern with wildcard support
func matchPattern(pattern, method string) bool {
	if pattern == method {
		return true
	}
	
	// Handle wildcard patterns (e.g., "aws:*")
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(method, prefix)
	}
	
	return false
}

// checkConditions evaluates policy conditions against request parameters
func (a *AELProxy) checkConditions(conditions map[string]interface{}, params map[string]interface{}) bool {
	// Check amount_gt condition for financial operations
	if amountGt, ok := conditions["amount_gt"].(int); ok {
		if amount, ok := params["amount"].(float64); ok {
			return amount > float64(amountGt)
		}
	}
	
	// Default: condition not met
	return true
}
