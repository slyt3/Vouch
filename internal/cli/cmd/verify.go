package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/crypto"
	"github.com/slyt3/Vouch/internal/ledger"
)

func VerifyCommand() {
	// Open database
	db, err := ledger.NewDB("vouch.db")
	if err := assert.Check(err == nil, "failed to open database", "err", err); err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Load signer
	signer, err := crypto.NewSigner(".vouch_key")
	if err := assert.Check(err == nil, "failed to load signer", "err", err); err != nil {
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
