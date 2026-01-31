package audit

import (
	"encoding/json"
	"fmt"
	"io"
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

	closeBody := func(body io.Closer, context string) error {
		if err := body.Close(); err != nil {
			return fmt.Errorf("closing %s: %w", context, err)
		}
		return nil
	}

	// Use explicit blockstream API for reliability
	resp, err := client.Get("https://blockstream.info/api/blocks/tip/height")
	if err != nil {
		return nil, fmt.Errorf("fetching tip height: %w", err)
	}

	if resp.StatusCode != 200 {
		if closeErr := closeBody(resp.Body, "tip height response"); closeErr != nil {
			return nil, fmt.Errorf("blockstream api error: %d; %w", resp.StatusCode, closeErr)
		}
		return nil, fmt.Errorf("blockstream api error: %d", resp.StatusCode)
	}

	var height uint64
	if err := json.NewDecoder(resp.Body).Decode(&height); err != nil {
		if closeErr := closeBody(resp.Body, "tip height response"); closeErr != nil {
			return nil, fmt.Errorf("decoding height: %v; %w", err, closeErr)
		}
		return nil, fmt.Errorf("decoding height: %w", err)
	}
	if err := closeBody(resp.Body, "tip height response"); err != nil {
		return nil, err
	}

	// Fetch Hash
	resp2, err := client.Get(fmt.Sprintf("https://blockstream.info/api/block-height/%d", height))
	if err != nil {
		return nil, fmt.Errorf("fetching block hash: %w", err)
	}

	var hash string
	if _, err := fmt.Fscan(resp2.Body, &hash); err != nil {
		if closeErr := closeBody(resp2.Body, "block hash response"); closeErr != nil {
			return nil, fmt.Errorf("reading block hash: %v; %w", err, closeErr)
		}
		return nil, err
	}
	if err := closeBody(resp2.Body, "block hash response"); err != nil {
		return nil, err
	}

	return &Anchor{
		Source:      "bitcoin-mainnet",
		BlockHeight: height,
		BlockHash:   hash,
		Timestamp:   time.Now(),
	}, nil
}
