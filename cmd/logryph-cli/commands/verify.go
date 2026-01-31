package commands

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/crypto"
	"github.com/slyt3/Logryph/internal/ledger/audit"
	"github.com/slyt3/Logryph/internal/ledger/store"
)

func VerifyCommand() {
	// Parse flags
	verifyFlags := flag.NewFlagSet("verify", flag.ExitOnError)
	skipLive := verifyFlags.Bool("skip-live", false, "Skip live verification of Bitcoin anchors")
	_ = verifyFlags.Parse(os.Args[2:])

	// Open database
	db, err := store.NewDB("logryph.db")
	if err := assert.Check(err == nil, "failed to open database: %v", err); err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Load signer
	signer, err := crypto.NewSigner(".logryph_key")
	if err := assert.Check(err == nil, "failed to load signer: %v", err); err != nil {
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
	result, err := audit.VerifyChain(db, runID, signer)
	if err != nil {
		log.Fatalf("Verification error: %v", err)
	}

	if result.Valid {
		fmt.Printf("[OK] Chain is valid (%d events verified)\n", result.TotalEvents)
	} else {
		fmt.Print("[FAILED] Chain verification failed\n")
		fmt.Printf("  Error: %s\n", result.ErrorMessage)
		if result.FailedAtSeq > 0 {
			fmt.Printf("  Failed at sequence: %d\n", result.FailedAtSeq)
		}
		os.Exit(1)
	}

	if *skipLive {
		return
	}

	// Verify Bitcoin Anchors (Live)
	fmt.Println("Verifying external Bitcoin anchors...")
	anchorResult, err := audit.VerifyAnchors(db, runID)
	if err != nil {
		fmt.Printf("[WARN] Anchor verification failed: %v\n", err)
	} else if anchorResult.Valid {
		fmt.Printf("[OK] Bitcoin anchors verified against Blockstream API (%d anchors checked)\n", anchorResult.AnchorsChecked)
	} else {
		fmt.Printf("[FAILED] Bitcoin anchor mismatch: %s\n", anchorResult.ErrorMessage)
		os.Exit(1)
	}
}
