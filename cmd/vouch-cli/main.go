package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"encoding/json"

	"github.com/yourname/vouch/internal/crypto"
	"github.com/yourname/vouch/internal/ledger"
	"github.com/yourname/vouch/internal/proxy"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "verify":
		verifyCommand()
	case "status":
		statusCommand()
	case "events":
		eventsCommand()
	case "approve":
		approveCommand()
	case "reject":
		rejectCommand()
	case "stats":
		statsCommand()
	case "risk":
		riskCommand()
	case "export":
		exportCommand()
	case "topology":
		topologyCommand()
	case "rekey":
		rekeyCommand()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Vouch CLI - Agent Analytics & Safety Command Line Tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  vouch verify              Validate the entire hash chain")
	fmt.Println("  vouch status              Show current run information")
	fmt.Println("  vouch events [--limit N]  List recent events (default: 10)")
	fmt.Println("  vouch approve <event-id>  Approve a stalled action")
	fmt.Println("  vouch reject <event-id>   Reject a stalled action")
	fmt.Println("  vouch stats               Show detailed run and global statistics")
	fmt.Println("  vouch risk                List all high-risk events")
	fmt.Println("  vouch export <file.json>  Export the current run to a JSON file")
	fmt.Println("  vouch topology <task-id>  Show the hierarchy of tools within a task")
	fmt.Println("  vouch rekey               Rotate the Ed25519 signing keys")
}

func verifyCommand() {
	// Open database
	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Load signer
	signer, err := crypto.NewSigner(".vouch_key")
	if err != nil {
		log.Fatalf("Failed to load signer: %v", err)
	}

	// Get current run ID
	runID, err := db.GetRunID()
	if err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}

	if runID == "" {
		fmt.Println("No runs found in database")
		return
	}

	fmt.Printf("Verifying chain for run: %s\n", runID[:8])

	// Verify chain
	result, err := ledger.VerifyChain(db, runID, signer)
	if err != nil {
		log.Fatalf("Verification error: %v", err)
	}

	if result.Valid {
		fmt.Printf("✓ Chain is valid (%d events verified)\n", result.TotalEvents)
	} else {
		fmt.Printf("✗ Chain verification failed\n")
		fmt.Printf("  Error: %s\n", result.ErrorMessage)
		if result.FailedAtSeq > 0 {
			fmt.Printf("  Failed at sequence: %d\n", result.FailedAtSeq)
		}
		os.Exit(1)
	}
}

func statusCommand() {
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

func eventsCommand() {
	// Parse flags
	eventsFlags := flag.NewFlagSet("events", flag.ExitOnError)
	limit := eventsFlags.Int("limit", 10, "Number of events to show")
	_ = eventsFlags.Parse(os.Args[2:])

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

	// Get recent events
	events, err := db.GetRecentEvents(runID, *limit)
	if err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}

	fmt.Printf("Recent Events (showing %d)\n", len(events))
	fmt.Println("===========================")
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		fmt.Printf("[%d] %s | %s | %s\n", e.SeqIndex, e.ID[:8], e.EventType, e.Method)
		if e.WasBlocked {
			fmt.Printf("    BLOCKED\n")
		}
	}
}

func approveCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch approve <event-id>")
		os.Exit(1)
	}

	eventID := os.Args[2]

	// Send HTTP request to proxy API
	url := fmt.Sprintf("http://localhost:9998/api/approve/%s", eventID)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("Error: Failed to connect to Vouch proxy: %v\n", err)
		fmt.Println("Make sure the proxy is running on port 9998")
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("✓ Event %s approved successfully\n", eventID)
	} else {
		fmt.Printf("✗ Failed to approve event: %s\n", string(body))
		os.Exit(1)
	}
}

func rejectCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch reject <event-id>")
		os.Exit(1)
	}

	eventID := os.Args[2]

	// Send HTTP request to proxy API
	url := fmt.Sprintf("http://localhost:9998/api/reject/%s", eventID)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("Error: Failed to connect to Vouch proxy: %v\n", err)
		fmt.Println("Make sure the proxy is running on port 9998")
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("✓ Event %s rejected successfully\n", eventID)
	} else {
		fmt.Printf("✗ Failed to reject event: %s\n", string(body))
		os.Exit(1)
	}
}

func statsCommand() {
	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	runID, _ := db.GetRunID()
	if runID == "" {
		fmt.Println("No runs found")
		return
	}

	stats, err := db.GetRunStats(runID)
	if err != nil {
		log.Fatalf("Failed to get stats: %v", err)
	}

	gStats, _ := db.GetGlobalStats()

	fmt.Printf("Run Statistics (%s)\n", runID[:8])
	fmt.Println("=======================")
	fmt.Printf("Total Events:    %d\n", stats.TotalEvents)
	fmt.Printf("Tool Calls:      %d\n", stats.CallCount)
	fmt.Printf("Blocked Calls:   %d\n", stats.BlockedCount)
	fmt.Println("\nRisk Breakdown:")
	if len(stats.RiskBreakdown) == 0 {
		fmt.Println("  None")
	} else {
		for risk, count := range stats.RiskBreakdown {
			fmt.Printf("  %-10s: %d\n", risk, count)
		}
	}

	if gStats != nil {
		fmt.Println("\nGlobal Context")
		fmt.Println("--------------")
		fmt.Printf("Total Runs:      %d\n", gStats.TotalRuns)
		fmt.Printf("Total Events:    %d\n", gStats.TotalEvents)
		fmt.Printf("Critical Alerts: %d\n", gStats.CriticalCount)
	}
}

func riskCommand() {
	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	risky, err := db.GetRiskEvents()
	if err != nil {
		log.Fatalf("Failed to get risky events: %v", err)
	}

	if len(risky) == 0 {
		fmt.Println("✓ No high-risk events detected")
		return
	}

	fmt.Printf("High-Risk Events Found: %d\n", len(risky))
	fmt.Println("==========================")
	for _, e := range risky {
		fmt.Printf("[%s] %-8s | %-10s | %s\n", e.RiskLevel, e.ID[:8], e.EventType, e.Method)
		if e.PolicyID != "" {
			fmt.Printf("    Policy: %s\n", e.PolicyID)
		}
	}
}

func exportCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch export <output-file.json>")
		os.Exit(1)
	}
	outputFile := os.Args[2]

	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	runID, _ := db.GetRunID()
	if runID == "" {
		fmt.Println("No runs found")
		return
	}

	events, err := db.GetAllEvents(runID)
	if err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}

	agent, genesisHash, pubKey, _ := db.GetRunInfo(runID)

	exportData := map[string]interface{}{
		"run_id":       runID,
		"agent":        agent,
		"genesis_hash": genesisHash,
		"public_key":   pubKey,
		"events":       events,
		"exported_at":  time.Now(),
	}

	data, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal export data: %v", err)
	}

	err = os.WriteFile(outputFile, data, 0644)
	if err != nil {
		log.Fatalf("Failed to write to file: %v", err)
	}

	fmt.Printf("✓ Exported %d events to %s\n", len(events), outputFile)
}

func topologyCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch topology <task-id>")
		os.Exit(1)
	}
	taskID := os.Args[2]

	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	events, err := db.GetEventsByTaskID(taskID)
	if err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}

	if len(events) == 0 {
		fmt.Printf("No events found for task %s\n", taskID)
		return
	}

	fmt.Printf("Task Topology: %s\n", taskID)
	fmt.Println("======================================")

	// Build the tree
	byParent := make(map[string][]proxy.Event)
	var roots []proxy.Event

	for _, e := range events {
		if e.ParentID == "" {
			roots = append(roots, e)
		} else {
			byParent[e.ParentID] = append(byParent[e.ParentID], e)
		}
	}

	var printNode func(e proxy.Event, indent string)
	printNode = func(e proxy.Event, indent string) {
		status := ""
		if e.WasBlocked {
			status = " [BLOCKED]"
		}
		fmt.Printf("%s└─ %s (%s)%s\n", indent, e.Method, e.ID[:8], status)
		children := byParent[e.ID]
		for _, child := range children {
			printNode(child, indent+"   ")
		}
	}

	for _, root := range roots {
		printNode(root, "")
	}
}

func rekeyCommand() {
	resp, err := http.Post("http://localhost:9998/api/rekey", "application/json", nil)
	if err != nil {
		log.Fatalf("Failed to contact Vouch API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Rekey failed: %s", string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
