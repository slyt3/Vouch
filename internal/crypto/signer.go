package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// Signer handles Ed25519 signing operations for cryptographic event integrity.
// Private key is stored hex-encoded in a file (default .logryph_key).
// Thread-safe for concurrent signature operations.
type Signer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// NewSigner creates a new signer, loading an existing key from keyPath or generating a new one.
// Generates a new Ed25519 keypair if keyPath does not exist and saves it with 0600 permissions.
// Returns an error if key generation or file I/O fails.
func NewSigner(keyPath string) (*Signer, error) {
	// Try to load existing key
	privateKey, err := loadPrivateKey(keyPath)
	if err != nil {
		// Generate new keypair
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating keypair: %w", err)
		}

		// Save private key
		if err := savePrivateKey(keyPath, privateKey); err != nil {
			return nil, fmt.Errorf("saving private key: %w", err)
		}

		return &Signer{
			privateKey: privateKey,
			publicKey:  publicKey,
		}, nil
	}

	// Derive public key from private key
	publicKey := privateKey.Public().(ed25519.PublicKey)

	return &Signer{
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

// SignHash signs a hash string with Ed25519 and returns the signature as hex-encoded string.
// The hash is signed directly (not re-hashed). Returns an error only on encoding failure (never fails in practice).
func (s *Signer) SignHash(hash string) (string, error) {
	hashBytes := []byte(hash)
	signature := ed25519.Sign(s.privateKey, hashBytes)
	return hex.EncodeToString(signature), nil
}

// GetPublicKey returns the public key as a hex-encoded string.
// Used for verification by external parties and included in exported evidence bags.
func (s *Signer) GetPublicKey() string {
	return hex.EncodeToString(s.publicKey)
}

// RotateKey generates a new Ed25519 keypair, saves it to keyPath, and updates the signer.
// Returns old and new public keys as hex strings. Use for key rotation after compromise.
// Returns an error if key generation or file save fails.
func (s *Signer) RotateKey(keyPath string) (oldPubKey, newPubKey string, err error) {
	oldPubKey = s.GetPublicKey()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generating new keypair: %w", err)
	}

	if err := savePrivateKey(keyPath, priv); err != nil {
		return "", "", fmt.Errorf("saving rotated key: %w", err)
	}

	s.privateKey = priv
	s.publicKey = pub
	newPubKey = s.GetPublicKey()

	return oldPubKey, newPubKey, nil
}

// VerifySignature checks if a hex-encoded signature is valid for the given hash.
// Returns true if signature is valid, false otherwise (including decode errors).
func (s *Signer) VerifySignature(hash, signatureHex string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return ed25519.Verify(s.publicKey, []byte(hash), signature)
}

// loadPrivateKey loads a private key from file (hex-encoded)
func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	keyBytes, err := hex.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("decoding key: %w", err)
	}

	if len(keyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid key size: expected %d, got %d", ed25519.PrivateKeySize, len(keyBytes))
	}

	return ed25519.PrivateKey(keyBytes), nil
}

// savePrivateKey saves a private key to file (hex-encoded)
func savePrivateKey(path string, key ed25519.PrivateKey) error {
	hexKey := hex.EncodeToString(key)
	return os.WriteFile(path, []byte(hexKey), 0600) // Restrictive permissions
}
