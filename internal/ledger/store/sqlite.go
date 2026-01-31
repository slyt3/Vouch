package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

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
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("enabling WAL mode: %v; closing database: %w", err, closeErr)
		}
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// Execute embedded schema
	if _, err := conn.Exec(schemaSQL); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("executing schema: %v; closing database: %w", err, closeErr)
		}
		return nil, fmt.Errorf("executing schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}
