package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// Simple mock MCP server for testing
func main() {
	http.HandleFunc("/", handleMCPRequest)
	
	log.Println("[TEST] Mock MCP Server starting on :8080")
	log.Println("[TEST] Simulating real MCP tool server for testing")
	
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Failed to start mock server: %v", err)
	}
}

func handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	method, _ := req["method"].(string)
	id := req["id"]

	// Simulate different responses based on method
	var result map[string]interface{}
	
	switch method {
	case "tools/call":
		params, _ := req["params"].(map[string]interface{})
		toolName, _ := params["name"].(string)
		
		// Simulate async task for certain tools
		if toolName == "long_running_task" {
			result = map[string]interface{}{
				"task_id": "task-12345",
				"state":   "working",
				"message": "Task started",
			}
		} else {
			result = map[string]interface{}{
				"success": true,
				"output":  "Tool executed successfully",
			}
		}
		
	case "tasks/get":
		params, _ := req["params"].(map[string]interface{})
		taskID, _ := params["task_id"].(string)
		
		// Simulate task completion
		result = map[string]interface{}{
			"task_id": taskID,
			"state":   "completed",
			"output":  "Task finished successfully",
		}
		
	case "google_search:query":
		result = map[string]interface{}{
			"results": []string{"Result 1", "Result 2"},
		}
		
	case "aws:ec2:launch":
		result = map[string]interface{}{
			"instance_id": "i-1234567890abcdef0",
			"status":      "pending",
		}
		
	case "stripe:charge":
		params, _ := req["params"].(map[string]interface{})
		result = map[string]interface{}{
			"charge_id": "ch_1234567890",
			"amount":    params["amount"],
			"status":    "succeeded",
		}
		
	default:
		result = map[string]interface{}{
			"success": true,
		}
	}

	// Add small delay to simulate real processing
	time.Sleep(100 * time.Microsecond)

	// Send response
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
