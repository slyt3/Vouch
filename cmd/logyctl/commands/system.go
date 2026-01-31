package commands

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/slyt3/Logryph/internal/ledger/store"
)

func StatusCommand() {
	// Open database
	db, err := store.NewDB("logryph.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

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
		log.Fatalf("Failed to contact Logryph API: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Failed to close rekey response: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error: Failed to read response body: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(body))
}

// BackupKeyCommand creates a timestamped backup of the Ed25519 signing key.
// Backup is saved as .logryph_key.backup.<timestamp> with restricted permissions (0600).
// This should be run before key rotation to ensure recovery capability.
func BackupKeyCommand() {
	const maxBackupName = 256
	const keyPath = ".logryph_key"

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		fmt.Println("Error: No key file found at .logryph_key")
		os.Exit(1)
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	backupPath := fmt.Sprintf(".logryph_key.backup.%s", timestamp)

	// Check backup name length
	if len(backupPath) > maxBackupName {
		fmt.Println("Error: Backup path exceeds maximum length")
		os.Exit(1)
	}

	// Read original key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Printf("Error: Failed to read key file: %v\n", err)
		os.Exit(1)
	}

	// Write backup with same restricted permissions
	if err := os.WriteFile(backupPath, keyData, 0600); err != nil {
		fmt.Printf("Error: Failed to write backup: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Key backed up to: %s\n", backupPath)
	fmt.Println("Store this backup securely (offline storage recommended)")
}

// RestoreKeyCommand restores a signing key from a backup file.
// Moves current key to .logryph_key.old if it exists, then restores from backup.
// Warning: This operation should only be done when Logryph is stopped.
func RestoreKeyCommand(backupPath string) {
	const maxPath = 512
	const keyPath = ".logryph_key"

	// Check path length
	if len(backupPath) > maxPath {
		fmt.Println("Error: Backup path exceeds maximum length")
		os.Exit(1)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		fmt.Printf("Error: Backup file not found: %s\n", backupPath)
		os.Exit(1)
	}

	// Read backup
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		fmt.Printf("Error: Failed to read backup: %v\n", err)
		os.Exit(1)
	}

	// Move current key if it exists
	if _, err := os.Stat(keyPath); err == nil {
		oldPath := keyPath + ".old"
		if err := os.Rename(keyPath, oldPath); err != nil {
			fmt.Printf("Error: Failed to preserve old key: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Existing key moved to: %s\n", oldPath)
	}

	// Restore backup
	if err := os.WriteFile(keyPath, backupData, 0600); err != nil {
		fmt.Printf("Error: Failed to restore key: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Key restored successfully")
	fmt.Println("Warning: Chain verification will fail for events signed with the old key")
}

// ListBackupsCommand lists all key backup files in the current directory.
// Shows backup files with timestamps and file sizes.
func ListBackupsCommand() {
	const maxFiles = 1000

	files, err := os.ReadDir(".")
	if err != nil {
		fmt.Printf("Error: Failed to read directory: %v\n", err)
		os.Exit(1)
	}

	backups := make([]os.DirEntry, 0, 10)
	fileCount := 0

	// Collect backup files with bounded loop
	for i := 0; i < maxFiles && i < len(files); i++ {
		fileCount++
		file := files[i]
		if filepath.Ext(file.Name()) == "" && len(file.Name()) > 17 && file.Name()[:17] == ".logryph_key.backup" {
			backups = append(backups, file)
		}
	}

	if len(backups) == 0 {
		fmt.Println("No key backups found")
		return
	}

	fmt.Println("Key Backups")
	fmt.Println("===========")

	// Display backups with bounded loop
	for i := 0; i < len(backups); i++ {
		info, err := backups[i].Info()
		if err != nil {
			continue
		}
		fmt.Printf("%s (%d bytes, %s)\n",
			backups[i].Name(),
			info.Size(),
			info.ModTime().Format("2006-01-02 15:04:05"))
	}
}
