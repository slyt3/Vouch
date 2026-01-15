package ledger

import (
	"database/sql"
	"fmt"
)

// InsertRun creates a new run record
func (db *DB) InsertRun(id, agentName, genesisHash, ledgerPubKey string) error {
	query := `INSERT INTO runs (id, agent_name, genesis_hash, ledger_pub_key) VALUES (?, ?, ?, ?)`
	_, err := db.conn.Exec(query, id, agentName, genesisHash, ledgerPubKey)
	if err != nil {
		return fmt.Errorf("inserting run: %w", err)
	}
	return nil
}

// HasRuns checks if any runs exist in the database
func (db *DB) HasRuns() (bool, error) {
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM runs").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking runs: %w", err)
	}
	return count > 0, nil
}

// GetRunID retrieves the most recent run ID
func (db *DB) GetRunID() (string, error) {
	var runID string
	err := db.conn.QueryRow("SELECT id FROM runs ORDER BY started_at DESC LIMIT 1").Scan(&runID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying run ID: %w", err)
	}
	return runID, nil
}

// GetRunInfo retrieves run metadata
func (db *DB) GetRunInfo(runID string) (agentName, genesisHash, pubKey string, err error) {
	query := `SELECT agent_name, genesis_hash, ledger_pub_key FROM runs WHERE id = ?`
	err = db.conn.QueryRow(query, runID).Scan(&agentName, &genesisHash, &pubKey)
	if err != nil {
		return "", "", "", fmt.Errorf("querying run info: %w", err)
	}
	return agentName, genesisHash, pubKey, nil
}
