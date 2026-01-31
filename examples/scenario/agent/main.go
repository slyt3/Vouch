package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	LogryphProxy = "http://localhost:9999"
)

type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
	ID      int                    `json:"id"`
}

func main() {
	client := &http.Client{Timeout: 5 * time.Second}

	fmt.Println("🤖 Rogue Agent starting task: 'Infrastructure Audit'")
	time.Sleep(1 * time.Second)

	// Step 1: List Instances
	fmt.Println("Step 1: Listing compute instances...")
	call(client, "compute:list_instances", map[string]interface{}{"task_id": "audit-2026"})

	time.Sleep(1 * time.Second)

	// Step 2: "Rogue" Action - Attempt to delete production database
	fmt.Println("Step 2: [CRITICAL] Attempting to decommission legacy database 'prod-users-v2'...")
	call(client, "database:delete", map[string]interface{}{
		"name":    "prod-users-v2",
		"task_id": "audit-2026",
	})

	fmt.Println("\n🤖 Task finished (with security failure).")
	fmt.Println("🔍 Investigator: Use 'logryph-cli trace' to see what happened.")
}

func call(client *http.Client, method string, params map[string]interface{}) {
	reqData := MCPRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	b, _ := json.Marshal(reqData)
	req, err := http.NewRequest("POST", LogryphProxy, bytes.NewBuffer(b))
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Connection error: %v (Is Logryph running?)\n", err)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", err)
		}
	}()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("   Response [%d]: %s\n", resp.StatusCode, string(respBody))
}
