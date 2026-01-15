package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"net/http"
	"time"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/ledger"
	"github.com/slyt3/Vouch/internal/pool"
	"github.com/slyt3/Vouch/internal/proxy"
)

func EventsCommand() {
	// Parse flags
	eventsFlags := flag.NewFlagSet("events", flag.ExitOnError)
	limit := eventsFlags.Int("limit", 10, "Number of events to show")
	_ = eventsFlags.Parse(os.Args[2:])

	// Open database
	db, err := ledger.NewDB("vouch.db")
	if err := assert.Check(err == nil, "failed to open database", "err", err); err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	if err := assert.Check(db != nil, "database handle is nil"); err != nil {
		log.Fatalf("Database handle is nil")
	}
	defer db.Close()

	// Get current run ID
	runID, err := db.GetRunID()
	if err := assert.Check(err == nil, "failed to get run ID", "err", err); err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}

	if runID == "" {
		fmt.Println("No runs found in database")
		return
	}

	// Get recent events
	events, err := db.GetRecentEvents(runID, *limit)
	if err := assert.Check(err == nil, "failed to get events", "err", err); err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}

	fmt.Printf("Recent Events (showing %d)\n", len(events))
	fmt.Println("===========================")
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		fmt.Printf("[%d] %s | %s | %s\n", e.SeqIndex, e.ID[:8], e.EventType, e.Method)
		if e.WasBlocked {
			fmt.Print("    BLOCKED\n")
		}
	}
}

func StatsCommand() {
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
	if err := assert.Check(err == nil, "failed to get stats", "err", err); err != nil {
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

	// Fetch Memory Pool Metrics from API
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:9998/api/metrics")
	if err == nil {
		defer resp.Body.Close()
		var m pool.Metrics
		if err := json.NewDecoder(resp.Body).Decode(&m); err == nil {
			fmt.Println("\nMemory Infrastructure (Zero-Allocation Pools)")
			fmt.Println("--------------------------------------------")
			printPoolMetric("Event Pool", m.EventHits, m.EventMisses)
			printPoolMetric("Buffer Pool", m.BufferHits, m.BufferMisses)
		}
	}
}

func printPoolMetric(name string, hits, misses uint64) {
	total := hits + misses
	rate := 0.0
	if total > 0 {
		rate = (float64(hits) / float64(total)) * 100
	}
	fmt.Printf("%-12s | Hits: %-5d | Misses: %-5d | Efficiency: %.1f%%\n", name, hits, misses, rate)
}

func RiskCommand() {
	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	risky, err := db.GetRiskEvents()
	if err := assert.Check(err == nil, "failed to get risky events", "err", err); err != nil {
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

func ExportCommand() {
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
		"exported_at":  os.Args[0], // Using placeholder for exported time to avoid time dependency if not needed
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

func TopologyCommand() {
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
