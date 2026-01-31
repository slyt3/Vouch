package ledger

import "github.com/slyt3/Logryph/internal/models"

// Stats related structs
type RunStats struct {
	RunID         string         `json:"run_id"`
	TotalEvents   uint64         `json:"total_events"`
	CallCount     uint64         `json:"call_count"`
	BlockedCount  uint64         `json:"blocked_count"`
	RiskBreakdown map[string]int `json:"risk_breakdown"`
}

type GlobalStats struct {
	TotalRuns     int    `json:"total_runs"`
	TotalEvents   uint64 `json:"total_events"`
	CriticalCount int    `json:"critical_count"`
}

// EventRepository defines the storage interface for the Logryph ledger.
// This allows swapping SQLite for Postgres/dqlite in the future without changing core logic.
type EventRepository interface {
	// Writer
	StoreEvent(event *models.Event) error
	InsertRun(id, agent, genesisHash, pubKey string) error

	// Reader
	GetLastEvent(runID string) (uint64, string, error)
	GetEventByID(eventID string) (*models.Event, error)
	GetAllEvents(runID string) ([]models.Event, error)
	GetRecentEvents(runID string, limit int) ([]models.Event, error)
	GetEventsByTaskID(taskID string) ([]models.Event, error)
	GetRiskEvents() ([]models.Event, error)

	// Meta
	HasRuns() (bool, error)
	GetRunID() (string, error)
	GetRunInfo(runID string) (agent, genesisHash, pubKey string, err error)

	// Stats
	GetRunStats(runID string) (*RunStats, error)
	GetGlobalStats() (*GlobalStats, error)

	// Lifecycle
	Close() error
}
