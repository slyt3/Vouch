package commands

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/slyt3/Vouch/internal/ledger/store"
	"github.com/slyt3/Vouch/internal/models"
)

func TraceCommand() {
	db, err := store.NewDB("vouch.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if len(os.Args) < 3 {
		tasks, err := db.GetUniqueTasks()
		if err != nil {
			log.Fatalf("Failed to get tasks: %v", err)
		}
		if len(tasks) == 0 {
			fmt.Println("No recorded tasks found in ledger.")
			return
		}
		fmt.Println("Available Tasks for Forensic Trace:")
		fmt.Println(strings.Repeat("-", 30))
		for _, t := range tasks {
			if t == "" {
				continue
			}
			fmt.Printf("• %s\n", t)
		}
		fmt.Println("\nUsage: vouch trace <task-id>")
		return
	}
	taskID := os.Args[2]
	htmlOutput := ""
	if len(os.Args) >= 5 && os.Args[3] == "--html" {
		htmlOutput = os.Args[4]
	}

	events, err := db.GetEventsByTaskID(taskID)
	if err != nil {
		log.Fatalf("Failed to get events: %v", err)
	}

	if len(events) == 0 {
		fmt.Printf("No events found for task %s\n", taskID)
		return
	}

	if htmlOutput != "" {
		err := generateHTMLReport(taskID, events, htmlOutput)
		if err != nil {
			log.Fatalf("Failed to generate HTML report: %v", err)
		}
		fmt.Printf("✓ Forensic HTML report generated: %s\n", htmlOutput)
		return
	}

	fmt.Printf("Forensic Timeline Trace: %s\n", taskID)
	fmt.Printf("Run ID: %s\n", events[0].RunID[:8])
	fmt.Printf("Start:  %s\n", events[0].Timestamp.Format(time.RFC3339))
	fmt.Println(strings.Repeat("=", 60))

	// Reconstruct Hierarchy
	roots, childrenMap := buildTree(events)

	// Visualize
	for i, root := range roots {
		printTraceNode(root, childrenMap, "", i == len(roots)-1, events[0].Timestamp)
	}

	// Footer Summary
	duration := events[len(events)-1].Timestamp.Sub(events[0].Timestamp)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Summary: %d events | Total Duration: %v\n", len(events), duration.Truncate(time.Millisecond))
}

func buildTree(events []models.Event) ([]models.Event, map[string][]models.Event) {
	childrenMap := make(map[string][]models.Event)
	var roots []models.Event

	for _, e := range events {
		if e.ParentID == "" {
			roots = append(roots, e)
		} else {
			childrenMap[e.ParentID] = append(childrenMap[e.ParentID], e)
		}
	}
	return roots, childrenMap
}

func printTraceNode(e models.Event, childrenMap map[string][]models.Event, prefix string, isLast bool, startTime time.Time) {
	// Marker symbols
	marker := "├── "
	if isLast {
		marker = "└── "
	}

	// Status icon
	statusSym := "○" // Default: Call
	if e.EventType == "tool_response" {
		statusSym = "●" // Response
	}
	if e.WasBlocked {
		statusSym = "×" // Blocked
	}
	if e.RiskLevel == "critical" {
		statusSym = "‼" // Critical
	}

	// Calculate delta from start
	delta := e.Timestamp.Sub(startTime)

	fmt.Printf("%s%s%s %-15s [%s] (+%v)\n", prefix, marker, statusSym, e.Method, e.ID[:6], delta.Truncate(time.Millisecond))

	// New Prefix for children
	newPrefix := prefix
	if isLast {
		newPrefix += "    "
	} else {
		newPrefix += "│   "
	}

	children := childrenMap[e.ID]
	for i, child := range children {
		printTraceNode(child, childrenMap, newPrefix, i == len(children)-1, startTime)
	}
}
func generateHTMLReport(taskID string, events []models.Event, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	title := fmt.Sprintf("Vouch Forensic Report - Task %s", taskID)
	fmt.Fprintf(f, `<!DOCTYPE html>
<html>
<head>
    <title>%s</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 1000px; margin: 0 auto; padding: 20px; background: #f9f9f9; }
        .header { background: #1a1a1a; color: white; padding: 20px; border-radius: 8px; margin-bottom: 30px; }
        .event { background: white; border: 1px solid #ddd; padding: 15px; border-radius: 6px; margin-bottom: 10px; border-left: 5px solid #eee; }
        .event-call { border-left-color: #007bff; }
        .event-response { border-left-color: #28a745; }
        .event-risk-high { border-left-color: #ffc107; background: #fffdf5; }
        .event-risk-critical { border-left-color: #dc3545; background: #fff5f5; }
        .meta { font-size: 0.85em; color: #666; margin-bottom: 5px; }
        .payload { background: #f1f1f1; padding: 10px; border-radius: 4px; font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace; font-size: 0.9em; white-space: pre-wrap; overflow-x: auto; margin-top: 10px; }
        .id { color: #007bff; font-weight: bold; }
        .risk-badge { display: inline-block; padding: 2px 8px; border-radius: 12px; font-size: 0.8em; font-weight: bold; text-transform: uppercase; }
        .risk-low { background: #e2fcd4; color: #2e7d32; }
        .risk-high { background: #fff3cd; color: #856404; }
        .risk-critical { background: #f8d7da; color: #721c24; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Forensic Evidence Report</h1>
        <p><strong>Task ID:</strong> %s</p>
        <p><strong>Run ID:</strong> %s</p>
        <p><strong>Generated:</strong> %s</p>
    </div>
`, title, taskID, events[0].RunID, time.Now().Format(time.RFC1123))

	for _, e := range events {
		riskClass := ""
		if e.RiskLevel == "high" {
			riskClass = "event-risk-high"
		} else if e.RiskLevel == "critical" {
			riskClass = "event-risk-critical"
		} else if e.EventType == "tool_call" {
			riskClass = "event-call"
		} else {
			riskClass = "event-response"
		}

		fmt.Fprintf(f, `
    <div class="event %s">
        <div class="meta">
            <span class="id">[%s]</span> %s &bull; <strong>%s</strong> &bull; %s
            %s
        </div>
        <div><strong>Method:</strong> %s</div>
`, riskClass, e.ID[:8], e.Timestamp.Format("15:04:05.000"), e.Actor, e.EventType, formatRiskBadge(e.RiskLevel), e.Method)

		if len(e.Params) > 0 {
			fmt.Fprintf(f, `        <div class="payload"><strong>Params:</strong> %v</div>`, formatPayload(e.Params))
		}
		if len(e.Response) > 0 {
			fmt.Fprintf(f, `        <div class="payload"><strong>Response:</strong> %v</div>`, formatPayload(e.Response))
		}

		fmt.Fprintf(f, `    </div>`)
	}

	fmt.Fprintf(f, `
</body>
</html>
`)
	return nil
}

func formatRiskBadge(risk string) string {
	if risk == "" || risk == "low" {
		return `<span class="risk-badge risk-low">Low Risk</span>`
	}
	if risk == "high" {
		return `<span class="risk-badge risk-high">High Risk</span>`
	}
	return `<span class="risk-badge risk-critical">Critical Risk</span>`
}

func formatPayload(p map[string]interface{}) string {
	// Simple string representation for now
	return fmt.Sprintf("%+v", p)
}
