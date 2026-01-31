package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
	ID      interface{}            `json:"id"`
}

type Instance struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

var instances = []Instance{
	{ID: "i-0123456", Status: "running", Type: "t3.medium"},
	{ID: "i-0abcdef", Status: "running", Type: "m5.large"},
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON-RPC", http.StatusBadRequest)
			return
		}

		log.Printf("Received MCP Call: %s", req.Method)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "compute:list_instances":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"result":  instances,
				"id":      req.ID,
			})
		case "database:delete":
			name, _ := req.Params["name"].(string)
			if name == "prod-users-v2" {
				w.WriteHeader(http.StatusForbidden)
				if err := json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"error":   map[string]interface{}{"code": -32000, "message": "Unauthorized: Production database deletion requires MFA"},
					"id":      req.ID,
				}); err != nil {
					log.Printf("Failed to encode error response: %v", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"result":  map[string]interface{}{"status": "deleted", "database": name},
				"id":      req.ID,
			}); err != nil {
				log.Printf("Failed to encode response: %v", err)
			}
		default:
			http.Error(w, "Method not found", http.StatusNotFound)
		}
	})

	log.Println("Mock Cloud API (MCP) started on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
