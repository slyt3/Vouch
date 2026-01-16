package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/slyt3/Vouch/internal/ledger"
)

type EvidenceManifest struct {
	Version       string                 `json:"version"`
	RunID         string                 `json:"run_id"`
	ExportTime    time.Time              `json:"export_time"`
	RunStats      *ledger.RunStats       `json:"run_stats"`
	GenesisAnchor map[string]interface{} `json:"genesis_anchor"`
	LastHash      string                 `json:"last_hash"`
}

func ExportCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: vouch export <output-file.zip> [run-id]")
		os.Exit(1)
	}
	outputFile := os.Args[2]

	// Default to current run if not specified
	targetRunID := ""
	if len(os.Args) > 3 {
		targetRunID = os.Args[3]
	}

	if err := ExportEvidenceBag(outputFile, targetRunID); err != nil {
		log.Fatalf("Export failed: %v", err)
	}
	fmt.Printf("✓ Evidence bag created: %s\n", outputFile)
}

func ExportEvidenceBag(zipPath, targetRunID string) error {
	// 1. Open DB
	db, err := ledger.NewDB("vouch.db")
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	defer db.Close()

	// 2. Identify Run ID
	runID := targetRunID
	if runID == "" {
		runID, err = db.GetRunID()
		if err != nil {
			return fmt.Errorf("getting run id: %w", err)
		}
	}
	if runID == "" {
		return fmt.Errorf("no runs found")
	}

	// 3. Gather Data
	stats, err := db.GetRunStats(runID)
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	_, lastHash, err := db.GetLastEvent(runID)
	if err != nil {
		return fmt.Errorf("getting last hash: %w", err)
	}

	manifest := EvidenceManifest{
		Version:    "1.0 (Vouch 2026.1)",
		RunID:      runID,
		ExportTime: time.Now(),
		RunStats:   stats,
		LastHash:   lastHash,
	}

	// 4. Create Zip
	f, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("creating zip file: %w", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// 5. Add Manifest
	manFile, err := w.Create("manifest.json")
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(manFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return err
	}

	// 6. Add DB (Raw)
	// We allow reading the DB even if locked by WAL, usually.
	dbFile, err := os.Open("vouch.db")
	if err != nil {
		// Try to read generic way if locked
		return fmt.Errorf("opening vouch.db: %w", err)
	}
	defer dbFile.Close()

	destFile, err := w.Create("vouch.db")
	if err != nil {
		return err
	}
	if _, err := io.Copy(destFile, dbFile); err != nil {
		return err
	}

	return nil
}
