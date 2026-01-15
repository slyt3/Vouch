package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/ucarion/jcs"
	"github.com/yourname/vouch/internal/assert"
)

// CalculateEventHash ensures deterministic hashing across any model/platform
// Uses RFC 8785 (JSON Canonicalization Scheme) for consistent serialization
func CalculateEventHash(prevHash string, payload interface{}) (string, error) {
	// Safety Assertion: Verify prev_hash is non-empty (except genesis)
	// Genesis block has 64 zeros as prev_hash
	if err := assert.Check(prevHash != "", "prev_hash must be non-empty"); err != nil {
		return "", err
	}
	if err := assert.Check(strings.Count(prevHash, "0") != 64 || prevHash == strings.Repeat("0", 64), "invalid prev_hash format"); err != nil {
		return "", err
	}

	if err := assert.Check(payload != nil, "payload must not be nil"); err != nil {
		return "", err
	}
	// 1. First marshal to JSON to normalize the data structure
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// 2. Unmarshal to a clean interface{} for JCS
	var normalized interface{}
	if err := json.Unmarshal(jsonBytes, &normalized); err != nil {
		return "", err
	}

	// 3. Canonicalize using JCS (RFC 8785)
	// This ensures identical output regardless of key order
	canonicalJSON, err := jcs.Format(normalized)
	if err != nil {
		return "", err
	}

	// 4. Hash(Prev + Current)
	hasher := sha256.New()
	hasher.Write([]byte(prevHash))
	hasher.Write([]byte(canonicalJSON))

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
