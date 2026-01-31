package crypto

import (
	"os"
	"testing"
)

// TestKeyRotationChainVerification verifies that events signed with old key remain valid after rotation.
// This ensures key rotation does not break verification of historical events.
func TestKeyRotationChainVerification(t *testing.T) {
	const maxEvents = 10
	keyPath := ".test_key_rotation"

	t.Cleanup(func() {
		if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove key file: %v", err)
		}
		if err := os.Remove(keyPath + ".old"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove old key file: %v", err)
		}
	})

	// Create initial signer
	signer1, err := NewSigner(keyPath)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	// Sign events with first key
	hashes1 := make([]string, maxEvents)
	signatures1 := make([]string, maxEvents)

	for i := 0; i < maxEvents; i++ {
		hash := "event_hash_" + string(rune('0'+i))
		sig, err := signer1.SignHash(hash)
		if err != nil {
			t.Fatalf("Failed to sign with key1: %v", err)
		}
		hashes1[i] = hash
		signatures1[i] = sig
	}

	oldPubKey := signer1.GetPublicKey()

	// Rotate key
	oldKey, newKey, err := signer1.RotateKey(keyPath)
	if err != nil {
		t.Fatalf("Failed to rotate key: %v", err)
	}

	if oldKey != oldPubKey {
		t.Errorf("Old key mismatch: got %s, want %s", oldKey, oldPubKey)
	}

	if newKey == oldKey {
		t.Error("New key should differ from old key")
	}

	// Sign events with new key
	hashes2 := make([]string, maxEvents)
	signatures2 := make([]string, maxEvents)

	for i := 0; i < maxEvents; i++ {
		hash := "new_event_hash_" + string(rune('0'+i))
		sig, err := signer1.SignHash(hash)
		if err != nil {
			t.Fatalf("Failed to sign with key2: %v", err)
		}
		hashes2[i] = hash
		signatures2[i] = sig
	}

	// Verify old events fail with new key (expected)
	for i := 0; i < maxEvents; i++ {
		if signer1.VerifySignature(hashes1[i], signatures1[i]) {
			t.Errorf("Event %d signed with old key should NOT verify with new key", i)
		}
	}

	// Verify new events succeed with new key
	for i := 0; i < maxEvents; i++ {
		if !signer1.VerifySignature(hashes2[i], signatures2[i]) {
			t.Errorf("Event %d signed with new key should verify with new key", i)
		}
	}

	// Recreate signer with old key to verify historical events
	if _, err := os.Stat(keyPath + ".old"); err == nil {

		// Move rotated key back temporarily
		if err := os.Rename(keyPath, keyPath+".new"); err != nil {
			t.Fatalf("Failed to move new key: %v", err)
		}
		if err := os.Rename(keyPath+".old", keyPath); err != nil {
			t.Fatalf("Failed to restore old key: %v", err)
		}

		signerOldRestored, err := NewSigner(keyPath)
		if err != nil {
			t.Fatalf("Failed to recreate old signer: %v", err)
		}

		// Verify old events succeed with restored old key
		for i := 0; i < maxEvents; i++ {
			if !signerOldRestored.VerifySignature(hashes1[i], signatures1[i]) {
				t.Errorf("Event %d should verify with restored old key", i)
			}
		}

		// Restore state
		if err := os.Rename(keyPath, keyPath+".old"); err != nil {
			t.Fatalf("Failed to preserve old key: %v", err)
		}
		if err := os.Rename(keyPath+".new", keyPath); err != nil {
			t.Fatalf("Failed to restore new key: %v", err)
		}
	}
}

// TestBackupRestore verifies backup and restore preserve key functionality.
func TestBackupRestore(t *testing.T) {
	keyPath := ".test_key_backup"
	backupPath := keyPath + ".backup"

	t.Cleanup(func() {
		if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove key file: %v", err)
		}
		if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove backup file: %v", err)
		}
		if err := os.Remove(keyPath + ".old"); err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove old key file: %v", err)
		}
	})

	// Create signer and sign event
	signer, err := NewSigner(keyPath)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	hash := "test_event_hash"
	sig, err := signer.SignHash(hash)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	pubKey := signer.GetPublicKey()

	// Backup key
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("Failed to read key: %v", err)
	}

	if err := os.WriteFile(backupPath, keyData, 0600); err != nil {
		t.Fatalf("Failed to write backup: %v", err)
	}

	// Delete original
	if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("Failed to remove original key: %v", err)
	}

	// Restore from backup
	if err := os.Rename(backupPath, keyPath); err != nil {
		t.Fatalf("Failed to restore: %v", err)
	}

	// Recreate signer
	signerRestored, err := NewSigner(keyPath)
	if err != nil {
		t.Fatalf("Failed to create restored signer: %v", err)
	}

	// Verify restored signer matches original
	if signerRestored.GetPublicKey() != pubKey {
		t.Errorf("Restored public key mismatch: got %s, want %s",
			signerRestored.GetPublicKey(), pubKey)
	}

	// Verify signature still valid
	if !signerRestored.VerifySignature(hash, sig) {
		t.Error("Restored signer should verify original signature")
	}
}
