package store

import (
	"fmt"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/ledger"
)

// GetRunStats returns statistics for a specific run
func (db *DB) GetRunStats(runID string) (stats *ledger.RunStats, err error) {
	if err := assert.Check(runID != "", "runID must not be empty"); err != nil {
		return nil, err
	}
	stats = &ledger.RunStats{
		RunID:         runID,
		RiskBreakdown: make(map[string]int),
	}

	// Total and Blocked counts
	err = db.conn.QueryRow(`
		SELECT COUNT(*), 
		       COALESCE(SUM(CASE WHEN event_type = 'blocked' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN event_type = 'tool_call' THEN 1 ELSE 0 END), 0)
		FROM events WHERE run_id = ?`, runID).Scan(&stats.TotalEvents, &stats.BlockedCount, &stats.CallCount)
	if err != nil {
		return nil, err
	}

	// Risk breakdown
	rows, err := db.conn.Query(`
		SELECT risk_level, COUNT(*) FROM events 
		WHERE run_id = ? AND risk_level != '' 
		GROUP BY risk_level`, runID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing risk breakdown rows: %w", closeErr)
		}
	}()

	const maxRiskLevels = 32
	for i := 0; i < maxRiskLevels; i++ {
		if !rows.Next() {
			break
		}
		var risk string
		var count int
		if err := rows.Scan(&risk, &count); err == nil {
			stats.RiskBreakdown[risk] = count
		}
	}
	if err := assert.Check(rows.Err() == nil, "risk breakdown rows error: %v", rows.Err()); err != nil {
		return nil, err
	}

	return stats, nil
}

// GetGlobalStats returns overall statistics
func (db *DB) GetGlobalStats() (*ledger.GlobalStats, error) {
	stats := &ledger.GlobalStats{}

	err := db.conn.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&stats.TotalRuns)
	if err != nil {
		return nil, err
	}

	err = db.conn.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&stats.TotalEvents)
	if err != nil {
		return nil, err
	}

	err = db.conn.QueryRow(`SELECT COUNT(*) FROM events WHERE risk_level = 'critical'`).Scan(&stats.CriticalCount)
	if err != nil {
		return nil, err
	}

	return stats, nil
}
