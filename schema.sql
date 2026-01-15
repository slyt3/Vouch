-- schema.sql
CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    agent_name TEXT,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    genesis_hash TEXT,
    ledger_pub_key TEXT
);

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY, -- UUIDv7
    run_id TEXT,
    seq_index INTEGER,
    timestamp TEXT,
    actor TEXT,          -- agent | user | system
    event_type TEXT,     -- tool_call | tool_response | task_started | task_completed | blocked | genesis
    method TEXT,
    params TEXT,         -- JSON string
    response TEXT,       -- JSON string
    task_id TEXT,        -- MCP SEP-1686
    task_state TEXT,     -- working | input_required | completed | failed | cancelled
    prev_hash TEXT,
    current_hash TEXT,
    signature TEXT,
    FOREIGN KEY(run_id) REFERENCES runs(id)
);

CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id);
