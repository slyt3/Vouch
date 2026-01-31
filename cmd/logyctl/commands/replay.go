package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/slyt3/Logryph/internal/ledger/store"
)

func ReplayCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: logyctl replay <event-id> [--target http://localhost:8080]")
		os.Exit(1)
	}
	eventID := os.Args[2]
	targetURL := "http://localhost:8080" // Default for many MCP setups

	if len(os.Args) >= 5 && os.Args[3] == "--target" {
		targetURL = os.Args[4]
	}

	db, err := store.NewDB("logryph.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	event, err := db.GetEventByID(eventID)
	if err != nil {
		log.Fatalf("Failed to find event: %v", err)
	}

	if event.EventType != "tool_call" {
		log.Fatalf("Can only replay events of type 'tool_call' (found: %s)", event.EventType)
	}

	fmt.Printf("Replaying Event: %s (%s)\n", event.ID, event.Method)
	fmt.Printf("Target URL:     %s\n", targetURL)
	fmt.Println("------------------------------------------------------------")

	// 1. Prepare Request
	reqBody, _ := json.Marshal(event.Params)
	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(reqBody))
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Logryph-Replay", event.ID)

	// 2. Execute
	client := &http.Client{}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Replay failed: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close replay response: %v", err)
		}
	}()
	duration := time.Since(start)

	body, _ := io.ReadAll(resp.Body)
	var newResp map[string]interface{}
	if err := json.Unmarshal(body, &newResp); err != nil {
		log.Fatalf("Failed to decode replay response: %v", err)
	}

	// 3. Compare and Show
	fmt.Printf("Status:   %s\n", resp.Status)
	fmt.Printf("Duration: %v\n", duration)
	fmt.Println()

	fmt.Println("Original Response:")
	origPretty, _ := json.MarshalIndent(event.Response, "", "  ")
	fmt.Println(string(origPretty))
	fmt.Println()

	fmt.Println("Replay Response:")
	newPretty, _ := json.MarshalIndent(newResp, "", "  ")
	fmt.Println(string(newPretty))

	// 4. Verification Check
	if resp.StatusCode < 300 {
		fmt.Println("\n[OK] Replay successful (HTTP 2xx)")
	} else {
		fmt.Printf("\n[FAILED] Replay failed (HTTP %d)\n", resp.StatusCode)
	}
}
