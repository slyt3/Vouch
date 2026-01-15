package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yourname/vouch/internal/proxy"
)

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
}

// NewDB creates a new database connection and initializes the schema
func NewDB(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	// Open database connection
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// Execute schema
	schemaPath := "schema.sql"
	if !filepath.IsAbs(schemaPath) {
		wd, err := os.Getwd()
		if err == nil {
			schemaPath = filepath.Join(wd, schemaPath)
		}
	}

	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading schema file: %w", err)
	}

	if _, err := conn.Exec(string(schemaSQL)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("executing schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// InsertRun creates a new run record
func (db *DB) InsertRun(id, agentName, genesisHash, ledgerPubKey string) error {
	query := `INSERT INTO runs (id, agent_name, genesis_hash, ledger_pub_key) VALUES (?, ?, ?, ?)`
	_, err := db.conn.Exec(query, id, agentName, genesisHash, ledgerPubKey)
	if err != nil {
		return fmt.Errorf("inserting run: %w", err)
	}
	return nil
}

// InsertEvent inserts a new event into the ledger
func (db *DB) InsertEvent(id, runID string, seqIndex int, timestamp, actor, eventType, method, params, response, taskID, taskState, parentID, policyID, riskLevel, prevHash, currentHash, signature string) error {
	query := `
		INSERT INTO events (
			id, run_id, seq_index, timestamp, actor, event_type, method, params, response,
			task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.conn.Exec(query,
		id, runID, seqIndex, timestamp, actor, eventType, method, params, response,
		taskID, taskState, parentID, policyID, riskLevel, prevHash, currentHash, signature,
	)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}
	return nil
}

// GetLastEvent retrieves the most recent event for a given run
func (db *DB) GetLastEvent(runID string) (seqIndex int, currentHash string, err error) {
	query := `SELECT seq_index, current_hash FROM events WHERE run_id = ? ORDER BY seq_index DESC LIMIT 1`
	err = db.conn.QueryRow(query, runID).Scan(&seqIndex, &currentHash)
	if err == sql.ErrNoRows {
		// No events yet, return defaults
		return -1, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("querying last event: %w", err)
	}
	return seqIndex, currentHash, nil
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

// GetAllEvents retrieves all events for a run, ordered by sequence
func (db *DB) GetAllEvents(runID string) ([]proxy.Event, error) {
	query := `
		SELECT id, run_id, seq_index, timestamp, actor, event_type, method, 
		       params, response, task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		FROM events 
		WHERE run_id = ? 
		ORDER BY seq_index ASC
	`

	rows, err := db.conn.Query(query, runID)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []proxy.Event
	for rows.Next() {
		var e proxy.Event
		var timestamp, params, response, taskID, taskState, parentID, policyID, riskLevel string

		err := rows.Scan(
			&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
			&params, &response, &taskID, &taskState, &parentID, &policyID, &riskLevel, &e.PrevHash, &e.CurrentHash, &e.Signature,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}

		e.ParentID = parentID
		e.PolicyID = policyID
		e.RiskLevel = riskLevel

		// Parse timestamp
		t, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("parsing timestamp: %w", err)
		}
		e.Timestamp = t
		e.TaskID = taskID
		e.TaskState = taskState

		// Parse JSON fields
		if params != "" && params != "null" {
			var paramsMap map[string]interface{}
			if err := json.Unmarshal([]byte(params), &paramsMap); err != nil {
				return nil, fmt.Errorf("parsing params JSON: %w", err)
			}
			e.Params = paramsMap
		}

		if response != "" && response != "null" {
			var responseMap map[string]interface{}
			if err := json.Unmarshal([]byte(response), &responseMap); err != nil {
				return nil, fmt.Errorf("parsing response JSON: %w", err)
			}
			e.Response = responseMap
		}

		events = append(events, e)
	}

	return events, nil
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

// GetRecentEvents retrieves the N most recent events
func (db *DB) GetRecentEvents(runID string, limit int) ([]proxy.Event, error) {
	query := `
		SELECT id, run_id, seq_index, timestamp, actor, event_type, method, 
		       params, response, task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		FROM events 
		WHERE run_id = ? 
		ORDER BY seq_index DESC 
		LIMIT ?
	`

	rows, err := db.conn.Query(query, runID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent events: %w", err)
	}
	defer rows.Close()

	var events []proxy.Event
	for rows.Next() {
		var e proxy.Event
		var timestamp, params, response, taskID, taskState, parentID, policyID, riskLevel string

		err := rows.Scan(
			&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
			&params, &response, &taskID, &taskState, &parentID, &policyID, &riskLevel, &e.PrevHash, &e.CurrentHash, &e.Signature,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}

		e.TaskID = taskID
		e.TaskState = taskState
		e.ParentID = parentID
		e.PolicyID = policyID
		e.RiskLevel = riskLevel

		events = append(events, e)
	}

	return events, nil
}

// GetEventByID retrieves a specific event by ID
func (db *DB) GetEventByID(eventID string) (*proxy.Event, error) {
	query := `
		SELECT id, run_id, seq_index, timestamp, actor, event_type, method, 
		       params, response, task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		FROM events 
		WHERE id = ?
	`

	var e proxy.Event
	var timestamp, params, response, taskID, taskState, parentID, policyID, riskLevel string

	err := db.conn.QueryRow(query, eventID).Scan(
		&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
		&params, &response, &taskID, &taskState, &parentID, &policyID, &riskLevel, &e.PrevHash, &e.CurrentHash, &e.Signature,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event not found: %s", eventID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying event: %w", err)
	}

	e.TaskID = taskID
	e.TaskState = taskState
	e.ParentID = parentID
	e.PolicyID = policyID
	e.RiskLevel = riskLevel

	return &e, nil
}

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

// GetRiskEvents returns events with high or critical risk
func (db *DB) GetRiskEvents() ([]proxy.Event, error) {
	query := `
		SELECT id, run_id, seq_index, timestamp, actor, event_type, method, params, response,
		       task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		FROM events 
		WHERE risk_level IN ('high', 'critical')
		ORDER BY timestamp DESC
	`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []proxy.Event
	for rows.Next() {
		var e proxy.Event
		var timestamp, params, response, taskID, taskState, parentID, policyID, riskLevel string

		err := rows.Scan(
			&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
			&params, &response, &taskID, &taskState, &parentID, &policyID, &riskLevel, &e.PrevHash, &e.CurrentHash, &e.Signature,
		)
		if err != nil {
			return nil, err
		}

		e.ParentID = parentID
		e.PolicyID = policyID
		e.RiskLevel = riskLevel
		// Minimal parsing for reporting
		events = append(events, e)
	}

	return events, nil
}

// GetEventsByTaskID retrieves all events for a specific task
func (db *DB) GetEventsByTaskID(taskID string) ([]proxy.Event, error) {
	query := `
		SELECT id, run_id, seq_index, timestamp, actor, event_type, method, 
		       params, response, task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		FROM events 
		WHERE task_id = ? 
		ORDER BY seq_index ASC
	`
	rows, err := db.conn.Query(query, taskID)
	if err != nil {
		return nil, fmt.Errorf("querying task events: %w", err)
	}
	defer rows.Close()

	var events []proxy.Event
	for rows.Next() {
		var e proxy.Event
		var timestamp, params, response, tID, tState, parentID, policyID, riskLevel string

		err := rows.Scan(
			&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
			&params, &response, &tID, &tState, &parentID, &policyID, &riskLevel, &e.PrevHash, &e.CurrentHash, &e.Signature,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}

		e.TaskID = tID
		e.TaskState = tState
		e.ParentID = parentID
		e.PolicyID = policyID
		e.RiskLevel = riskLevel
		events = append(events, e)
	}
	return events, nil
}

// GetTaskFailureCount returns the number of failed or cancelled states for a task
func (db *DB) GetTaskFailureCount(taskID string) (int, error) {
	var count int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM events 
		WHERE task_id = ? AND task_state IN ('failed', 'cancelled')
	`, taskID).Scan(&count)
	return count, err
}
