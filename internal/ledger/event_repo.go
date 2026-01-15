package ledger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yourname/vouch/internal/proxy"
)

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
		if err == nil {
			e.Timestamp = t
		}
		e.TaskID = taskID
		e.TaskState = taskState

		// Parse JSON fields
		if params != "" && params != "null" {
			var paramsMap map[string]interface{}
			if err := json.Unmarshal([]byte(params), &paramsMap); err == nil {
				e.Params = paramsMap
			}
		}

		if response != "" && response != "null" {
			var responseMap map[string]interface{}
			if err := json.Unmarshal([]byte(response), &responseMap); err == nil {
				e.Response = responseMap
			}
		}

		events = append(events, e)
	}

	return events, nil
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
		events = append(events, e)
	}

	return events, nil
}
