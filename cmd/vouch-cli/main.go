package main

import (
	"fmt"
	"os"

	"github.com/yourname/vouch/internal/cli/cmd"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "verify":
		cmd.VerifyCommand()
	case "status":
		cmd.StatusCommand()
	case "events":
		cmd.EventsCommand()
	case "approve":
		cmd.ApproveCommand()
	case "reject":
		cmd.RejectCommand()
	case "stats":
		cmd.StatsCommand()
	case "risk":
		cmd.RiskCommand()
	case "export":
		cmd.ExportCommand()
	case "topology":
		cmd.TopologyCommand()
	case "rekey":
		cmd.RekeyCommand()
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
