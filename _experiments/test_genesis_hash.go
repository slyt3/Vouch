package main

import (
	"fmt"
	"log"

	"github.com/yourname/ael/internal/crypto"
)

func main() {
	// Test the exact payload structure from genesis
	payload := map[string]interface{}{
		"id":         "test-id",
		"run_id":     "test-run",
		"seq_index":  float64(0),
		"timestamp":  "2026-01-15T16:55:00Z",
		"actor":      "system",
		"event_type": "genesis",
		"method":     "ael:init",
		"params": map[string]interface{}{
			"public_key": "abc123",
			"agent_name": "AEL-Agent",
			"version":    "1.0.0",
		},
	}

	hash, err := crypto.CalculateEventHash("0000000000000000000000000000000000000000000000000000000000000000", payload)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Success! Hash: %s\n", hash)
}
