package ledger

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
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
