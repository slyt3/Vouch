package crypto

import (
	"os"
	"testing"
)

func TestCalculateEventHash(t *testing.T) {
	prevHash := "0000000000000000000000000000000000000000000000000000000000000000"
	payload1 := map[string]interface{}{
		"id":   "test-1",
		"data": "hello",
	}
	payload2 := map[string]interface{}{
		"data": "hello",
		"id":   "test-1",
	}

	hash1, err := CalculateEventHash(prevHash, payload1)
	if err != nil {
		t.Fatalf("Failed to calculate hash 1: %v", err)
	}

	hash2, err := CalculateEventHash(prevHash, payload2)
	if err != nil {
		t.Fatalf("Failed to calculate hash 2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("Hashes should be identical regardless of key order: %s != %s", hash1, hash2)
	}
}

func TestSigner(t *testing.T) {
	keyPath := ".test_key"
	defer func() {
		if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
			t.Logf("Failed to remove test key: %v", err)
		}
	}()

	signer, err := NewSigner(keyPath)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	hash := "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e"
	sig, err := signer.SignHash(hash)
	if err != nil {
		t.Fatalf("Failed to sign hash: %v", err)
	}

	if !signer.VerifySignature(hash, sig) {
		t.Errorf("Signature verification failed")
	}

	// Test with wrong hash
	if signer.VerifySignature("wrong-hash", sig) {
		t.Errorf("Signature should not verify with wrong hash")
	}

	// Test persistence
	signer2, err := NewSigner(keyPath)
	if err != nil {
		t.Fatalf("Failed to reload signer: %v", err)
	}

	if signer.GetPublicKey() != signer2.GetPublicKey() {
		t.Errorf("Public keys should match after reload: %s != %s", signer.GetPublicKey(), signer2.GetPublicKey())
	}

	if !signer2.VerifySignature(hash, sig) {
		t.Errorf("Signature should verify with reloaded signer")
	}
}
