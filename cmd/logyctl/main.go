package main

import (
	"fmt"
	"os"

	"github.com/slyt3/Logryph/cmd/logyctl/commands"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "verify":
		commands.VerifyCommand()
	case "status":
		commands.StatusCommand()
	case "events":
		commands.EventsCommand()
	case "stats":
		commands.StatsCommand()
	case "risk":
		commands.RiskCommand()
	case "export":
		commands.ExportCommand()

	case "rekey":
		commands.RekeyCommand()
	case "backup-key":
		commands.BackupKeyCommand()
	case "restore-key":
		if len(os.Args) < 3 {
			fmt.Println("Error: restore-key requires backup file path")
			fmt.Println("Usage: logyctl restore-key <backup-file>")
			os.Exit(1)
		}
		commands.RestoreKeyCommand(os.Args[2])
	case "list-backups":
		commands.ListBackupsCommand()
	case "trace":
		commands.TraceCommand()
	case "replay":
		commands.ReplayCommand()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Logryph CLI - Associated Evidence Ledger (AEL) Tool tool")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  logyctl verify                    Validate the entire hash chain")
	fmt.Println("  logyctl status                    Show current run information")
	fmt.Println("  logyctl events [--limit N]        List recent events (default: 10)")
	fmt.Println("  logyctl stats                     Show detailed run and global statistics")
	fmt.Println("  logyctl risk                      List all high-risk events")
	fmt.Println("  logyctl export <file.zip>         Export the current run as an Evidence Bag (ZIP)")
	fmt.Println("  logyctl trace <task-id>           Visualize the forensic timeline of a task")
	fmt.Println("  logyctl replay <id>               Re-execute a tool call to reproduce an incident")
	fmt.Println()
	fmt.Println("Key Management:")
	fmt.Println("  logyctl rekey                     Rotate the Ed25519 signing keys")
	fmt.Println("  logyctl backup-key                Create timestamped backup of signing key")
	fmt.Println("  logyctl restore-key <file>        Restore signing key from backup")
	fmt.Println("  logyctl list-backups              List available key backups")
}
