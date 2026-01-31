package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/models"
)

const maxEventRows = 100000

// StoreEvent persists a models.Event to the ledger, unpacking it for the SQL query
func (db *DB) StoreEvent(event *models.Event) error {
	paramsBytes, err := json.Marshal(event.Params)
	if err != nil {
		return fmt.Errorf("marshaling params: %w", err)
	}
	responseBytes, err := json.Marshal(event.Response)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}

	return db.InsertEvent(
		event.ID,
		event.RunID,
		event.SeqIndex,
		event.Timestamp.Format(time.RFC3339Nano),
		event.Actor,
		event.EventType,
		event.Method,
		string(paramsBytes),
		string(responseBytes),
		event.TaskID,
		event.TaskState,
		event.ParentID,
		event.PolicyID,
		event.RiskLevel,
		event.PrevHash,
		event.CurrentHash,
		event.Signature,
	)
}

// InsertEvent inserts a new event into the ledger
func (db *DB) InsertEvent(id, runID string, seqIndex uint64, timestamp, actor, eventType, method, params, response, taskID, taskState, parentID, policyID, riskLevel, prevHash, currentHash, signature string) error {
	if err := assert.Check(id != "", "event id must not be empty"); err != nil {
		return err
	}
	if err := assert.Check(runID != "", "run id must not be empty"); err != nil {
		return err
	}
	if err := assert.Check(currentHash != "", "current hash must not be empty"); err != nil {
		return err
	}
	if err := assert.Check(signature != "", "signature must not be empty"); err != nil {
		return err
	}
	query := `
		INSERT INTO events (
			id, run_id, seq_index, timestamp, actor, event_type, method, params, response,
			task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := db.conn.Exec(query,
		id, runID, seqIndex, timestamp, actor, eventType, method, params, response,
		taskID, taskState, parentID, policyID, riskLevel, prevHash, currentHash, signature,
	)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil || rows != 1 {
		return fmt.Errorf("failed to insert event: rows affected = %d", rows)
	}
	return nil
}

// GetLastEvent retrieves the most recent event for a given run
func (db *DB) GetLastEvent(runID string) (seqIndex uint64, currentHash string, err error) {
	if err := assert.Check(runID != "", "runID must not be empty"); err != nil {
		return 0, "", err
	}

	query := `SELECT seq_index, current_hash FROM events WHERE run_id = ? ORDER BY seq_index DESC LIMIT 1`
	err = db.conn.QueryRow(query, runID).Scan(&seqIndex, &currentHash)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("querying last event: %w", err)
	}
	return seqIndex, currentHash, nil
}

// GetAllEvents retrieves all events for a run, ordered by sequence
func (db *DB) GetAllEvents(runID string) (events []models.Event, err error) {
	if err := assert.Check(runID != "", "runID must not be empty"); err != nil {
		return nil, err
	}

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
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing events rows: %w", closeErr)
		}
	}()

	for i := 0; i < maxEventRows; i++ {
		if !rows.Next() {
			break
		}
		var e models.Event
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
			if err := json.Unmarshal([]byte(params), &paramsMap); err != nil {
				log.Printf("Warning: failed to unmarshal params for event %s: %v", e.ID, err)
			} else {
				e.Params = paramsMap
			}
		}

		if response != "" && response != "null" {
			var responseMap map[string]interface{}
			if err := json.Unmarshal([]byte(response), &responseMap); err != nil {
				log.Printf("Warning: failed to unmarshal response for event %s: %v", e.ID, err)
			} else {
				e.Response = responseMap
			}
		}

		events = append(events, e)
	}

	if err := assert.Check(rows.Err() == nil, "get all events rows error: %v", rows.Err()); err != nil {
		return nil, err
	}
	return events, nil
}

// GetRecentEvents retrieves the N most recent events
func (db *DB) GetRecentEvents(runID string, limit int) (events []models.Event, err error) {
	if err := assert.Check(runID != "", "runID must not be empty"); err != nil {
		return nil, err
	}
	if err := assert.Check(limit > 0, "limit must be positive"); err != nil {
		return nil, err
	}

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
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing recent events rows: %w", closeErr)
		}
	}()

	for i := 0; i < maxEventRows; i++ {
		if !rows.Next() {
			break
		}
		var e models.Event
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

	if err := assert.Check(rows.Err() == nil, "recent events rows error: %v", rows.Err()); err != nil {
		return nil, err
	}
	return events, nil
}

// GetEventByID retrieves a specific event by ID
func (db *DB) GetEventByID(eventID string) (*models.Event, error) {
	if err := assert.Check(eventID != "", "eventID must not be empty"); err != nil {
		return nil, err
	}

	query := `
		SELECT id, run_id, seq_index, timestamp, actor, event_type, method, 
		       params, response, task_id, task_state, parent_id, policy_id, risk_level, prev_hash, current_hash, signature
		FROM events 
		WHERE id = ?
	`

	var e models.Event
	var timestamp, params, response, taskID, taskState, parentID, policyID, riskLevel string

	err := db.conn.QueryRow(query, eventID).Scan(
		&e.ID, &e.RunID, &e.SeqIndex, &timestamp, &e.Actor, &e.EventType, &e.Method,
		&params, &response, &taskID, &taskState, &parentID, &policyID, &riskLevel, &e.PrevHash, &e.CurrentHash, &e.Signature,
	)
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
func (db *DB) GetEventsByTaskID(taskID string) (events []models.Event, err error) {
	if err := assert.Check(taskID != "", "taskID must not be empty"); err != nil {
		return nil, err
	}
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
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing task events rows: %w", closeErr)
		}
	}()

	for i := 0; i < maxEventRows; i++ {
		if !rows.Next() {
			break
		}
		var e models.Event
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
	if err := assert.Check(rows.Err() == nil, "task events rows error: %v", rows.Err()); err != nil {
		return nil, err
	}
	return events, nil
}

// GetRiskEvents returns events with high or critical risk
func (db *DB) GetRiskEvents() (events []models.Event, err error) {
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
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing risk events rows: %w", closeErr)
		}
	}()

	for i := 0; i < maxEventRows; i++ {
		if !rows.Next() {
			break
		}
		var e models.Event
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

	if err := assert.Check(rows.Err() == nil, "risk events rows error: %v", rows.Err()); err != nil {
		return nil, err
	}
	return events, nil
}

// GetUniqueTasks returns all unique task IDs in the ledger
func (db *DB) GetUniqueTasks() (tasks []string, err error) {
	query := `SELECT DISTINCT task_id FROM events WHERE task_id != ''`
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing unique tasks rows: %w", closeErr)
		}
	}()

	for i := 0; i < maxEventRows; i++ {
		if !rows.Next() {
			break
		}
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := assert.Check(rows.Err() == nil, "unique tasks rows error: %v", rows.Err()); err != nil {
		return nil, err
	}
	return tasks, nil
}
