package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"net/http"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/ledger/store"
	"github.com/slyt3/Logryph/internal/pool"
)

func EventsCommand() {
	// Parse flags
	eventsFlags := flag.NewFlagSet("events", flag.ExitOnError)
	limit := eventsFlags.Int("limit", 10, "Number of events to show")
	_ = eventsFlags.Parse(os.Args[2:])

	// Open database
	db, err := store.NewDB("logryph.db")
	if err := assert.Check(err == nil, "failed to open database: %v", err); err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	if err := assert.Check(db != nil, "database handle is nil"); err != nil {
		log.Fatalf("Database handle is nil")
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Get current run ID
	runID, err := db.GetRunID()
	if err := assert.Check(err == nil, "failed to get run ID: %v", err); err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}

	if runID == "" {
		fmt.Println("No runs found in database")
		return
	}

	// Get recent events
	events, err := db.GetRecentEvents(runID, *limit)
	if err := assert.Check(err == nil, "failed to get events: %v", err); err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}
	const maxRecentEvents = 10000
	if err := assert.Check(len(events) <= maxRecentEvents, "recent events exceed max: %d", len(events)); err != nil {
		log.Fatalf("Recent events exceed max: %v", err)
	}

	fmt.Printf("Recent Events (showing %d)\n", len(events))
	fmt.Println("===========================")
	for i := 0; i < maxRecentEvents; i++ {
		idx := len(events) - 1 - i
		if idx < 0 {
			break
		}
		e := events[idx]
		fmt.Printf("[%d] %s | %s | %s\n", e.SeqIndex, e.ID[:8], e.EventType, e.Method)
		if e.WasBlocked {
			fmt.Print("    BLOCKED\n")
		}
	}
}

func StatsCommand() {
	db, err := store.NewDB("logryph.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	runID, _ := db.GetRunID()
	if runID == "" {
		fmt.Println("No runs found")
		return
	}

	stats, err := db.GetRunStats(runID)
	if err := assert.Check(err == nil, "failed to get stats: %v", err); err != nil {
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
		const maxRiskLevels = 32
		if err := assert.Check(len(stats.RiskBreakdown) <= maxRiskLevels, "risk breakdown exceeds max: %d", len(stats.RiskBreakdown)); err != nil {
			log.Fatalf("Risk breakdown exceeds max: %v", err)
		}
		for i := 0; i < maxRiskLevels; i++ {
			key := ""
			found := false
			for risk := range stats.RiskBreakdown {
				key = risk
				found = true
				break
			}
			if !found {
				break
			}
			count := stats.RiskBreakdown[key]
			fmt.Printf("  %-10s: %d\n", key, count)
			delete(stats.RiskBreakdown, key)
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
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Failed to close metrics response: %v", err)
			}
		}()
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
	db, err := store.NewDB("logryph.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	risky, err := db.GetRiskEvents()
	if err := assert.Check(err == nil, "failed to get risky events: %v", err); err != nil {
		log.Fatalf("Failed to get risky events: %v", err)
	}

	if len(risky) == 0 {
		fmt.Println("[OK] No high-risk events detected")
		return
	}
	const maxRiskEvents = 10000
	if err := assert.Check(len(risky) <= maxRiskEvents, "risky events exceed max: %d", len(risky)); err != nil {
		log.Fatalf("Risky events exceed max: %v", err)
	}

	fmt.Printf("High-Risk Events Found: %d\n", len(risky))
	fmt.Println("==========================")
	for i := 0; i < maxRiskEvents; i++ {
		if i >= len(risky) {
			break
		}
		e := risky[i]
		fmt.Printf("[%s] %-8s | %-10s | %s\n", e.RiskLevel, e.ID[:8], e.EventType, e.Method)
		if e.PolicyID != "" {
			fmt.Printf("    Policy: %s\n", e.PolicyID)
		}
	}
}
