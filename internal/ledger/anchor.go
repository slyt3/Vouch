package ledger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Anchor represents an external trust anchor
type Anchor struct {
	Source      string    `json:"source"`
	BlockHeight uint64    `json:"block_height"`
	BlockHash   string    `json:"block_hash"`
	Timestamp   time.Time `json:"timestamp"`
}

// FetchBitcoinAnchor retrieves the latest Bitcoin block hash
func FetchBitcoinAnchor() (*Anchor, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	// Use explicit blockstream API for reliability
	resp, err := client.Get("https://blockstream.info/api/blocks/tip/height")
	if err != nil {
		return nil, fmt.Errorf("fetching tip height: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("blockstream api error: %d", resp.StatusCode)
	}

	var height uint64
	if err := json.NewDecoder(resp.Body).Decode(&height); err != nil {
		return nil, fmt.Errorf("decoding height: %w", err)
	}

	// Fetch Hash
	resp2, err := client.Get(fmt.Sprintf("https://blockstream.info/api/block-height/%d", height))
	if err != nil {
		return nil, fmt.Errorf("fetching block hash: %w", err)
	}
	defer resp2.Body.Close()

	var hash string
	if _, err := fmt.Fscan(resp2.Body, &hash); err != nil {
		return nil, err
	}

	return &Anchor{
		Source:      "bitcoin-mainnet",
		BlockHeight: height,
		BlockHash:   hash,
		Timestamp:   time.Now(),
	}, nil
}
