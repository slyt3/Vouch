package ledger

// Stats related structs
type RunStats struct {
	RunID         string         `json:"run_id"`
	TotalEvents   int            `json:"total_events"`
	CallCount     int            `json:"call_count"`
	BlockedCount  int            `json:"blocked_count"`
	RiskBreakdown map[string]int `json:"risk_breakdown"`
}

type GlobalStats struct {
	TotalRuns     int `json:"total_runs"`
	TotalEvents   int `json:"total_events"`
	CriticalCount int `json:"critical_count"`
}

// GetRunStats returns statistics for a specific run
func (db *DB) GetRunStats(runID string) (*RunStats, error) {
	stats := &RunStats{
		RunID:         runID,
		RiskBreakdown: make(map[string]int),
	}

	// Total and Blocked counts
	err := db.conn.QueryRow(`
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
	defer rows.Close()

	for rows.Next() {
		var risk string
		var count int
		if err := rows.Scan(&risk, &count); err == nil {
			stats.RiskBreakdown[risk] = count
		}
	}

	return stats, nil
}

// GetGlobalStats returns overall statistics
func (db *DB) GetGlobalStats() (*GlobalStats, error) {
	stats := &GlobalStats{}

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
