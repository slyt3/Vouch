package commands

import (
	"fmt"
	"log"
	"os"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/ledger/audit"
	"github.com/slyt3/Vouch/internal/ledger/store"
)

func VerifyCommand() {
	// Open database
	db, err := store.NewDB("vouch.db")
	if err := assert.Check(err == nil, "failed to open database: %v", err); err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Load signer
	signer, err := crypto.NewSigner(".vouch_key")
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
		fmt.Printf("✓ Chain is valid (%d events verified)\n", result.TotalEvents)
	} else {
		fmt.Print("✗ Chain verification failed\n")
		fmt.Printf("  Error: %s\n", result.ErrorMessage)
		if result.FailedAtSeq > 0 {
			fmt.Printf("  Failed at sequence: %d\n", result.FailedAtSeq)
		}
		os.Exit(1)
	}

	// Verify Bitcoin Anchors (Live)
	fmt.Println("Verifying external Bitcoin anchors...")
	anchorResult, err := audit.VerifyAnchors(db, runID)
	if err != nil {
		fmt.Printf("! Anchor verification failed: %v\n", err)
	} else if anchorResult.Valid {
		fmt.Printf("✓ Bitcoin anchors verified against Blockstream API (%d anchors checked)\n", anchorResult.AnchorsChecked)
	} else {
		fmt.Printf("✗ Bitcoin anchor mismatch: %s\n", anchorResult.ErrorMessage)
		os.Exit(1)
	}
}
