package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yourname/ael/internal/proxy"
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
func (db *DB) InsertEvent(
	id, runID string,
	seqIndex int,
	timestamp, actor, eventType, method, params, response, taskID, taskState, prevHash, currentHash, signature string,
) error {
	query := `
		INSERT INTO events (
			id, run_id, seq_index, timestamp, actor, event_type, method, params, response,
			task_id, task_state, prev_hash, current_hash, signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.conn.Exec(query,
		id, runID, seqIndex, timestamp, actor, eventType, method, params, response,
		taskID, taskState, prevHash, currentHash, signature,
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
		       params, response, task_id, task_state, prev_hash, current_hash, signature
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
		var timestamp, params, response, taskID, taskState string

		err := rows.Scan(
			&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
			&params, &response, &taskID, &taskState, &e.PrevHash, &e.CurrentHash, &e.Signature,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}

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
		       params, response, task_id, task_state, prev_hash, current_hash, signature
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
		var timestamp, params, response, taskID, taskState string

		err := rows.Scan(
			&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
			&params, &response, &taskID, &taskState, &e.PrevHash, &e.CurrentHash, &e.Signature,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}

		e.TaskID = taskID
		e.TaskState = taskState

		events = append(events, e)
	}

	return events, nil
}

// GetEventByID retrieves a specific event by ID
func (db *DB) GetEventByID(eventID string) (*proxy.Event, error) {
	query := `
		SELECT id, run_id, seq_index, timestamp, actor, event_type, method, 
		       params, response, task_id, task_state, prev_hash, current_hash, signature
		FROM events 
		WHERE id = ?
	`

	var e proxy.Event
	var timestamp, params, response, taskID, taskState string

	err := db.conn.QueryRow(query, eventID).Scan(
		&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
		&params, &response, &taskID, &taskState, &e.PrevHash, &e.CurrentHash, &e.Signature,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event not found: %s", eventID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying event: %w", err)
	}

	e.TaskID = taskID
	e.TaskState = taskState

	return &e, nil
}
