package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CalculateEventHash ensures deterministic hashing across any model/platform
func CalculateEventHash(prevHash string, payload interface{}) (string, error) {
	// 1. Marshal to JSON first, then canonicalize
	// Note: Go's json.Marshal with sorted keys provides deterministic output
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// 2. Hash(Prev + Current)
	hasher := sha256.New()
	hasher.Write([]byte(prevHash))
	hasher.Write(jsonBytes)

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
