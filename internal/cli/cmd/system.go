package cmd

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/yourname/vouch/internal/ledger"
)

func StatusCommand() {
	// Open database
	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Get current run ID
	runID, err := db.GetRunID()
	if err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}

	if runID == "" {
		fmt.Println("No runs found in database")
		return
	}

	// Get run info
	agentName, genesisHash, pubKey, err := db.GetRunInfo(runID)
	if err != nil {
		log.Fatalf("Failed to get run info: %v", err)
	}

	fmt.Println("Current Run Status")
	fmt.Println("==================")
	fmt.Printf("Run ID:       %s\n", runID[:8])
	fmt.Printf("Agent:        %s\n", agentName)
	fmt.Printf("Genesis Hash: %s\n", genesisHash[:16]+"...")
	fmt.Printf("Public Key:   %s\n", pubKey[:32]+"...")
}

func RekeyCommand() {
	resp, err := http.Post("http://localhost:9998/api/rekey", "application/json", nil)
	if err != nil {
		log.Fatalf("Failed to contact Vouch API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error: Failed to read response body: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(body))
}
