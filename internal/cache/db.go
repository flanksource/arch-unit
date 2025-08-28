package cache

import (
	"database/sql"
	"sync"
)

// DB wraps sql.DB with mutex synchronization for write operations
type DB struct {
	conn    *sql.DB
	writeMu sync.Mutex // Protects write operations
}

// NewDB creates a new synchronized database wrapper
func NewDB(driverName, dataSourceName string) (*DB, error) {
	conn, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	// Configure SQLite for better concurrency
	if driverName == "sqlite" {
		// Enable WAL mode for better concurrent access
		if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
			conn.Close()
			return nil, err
		}

		// Set busy timeout to 5 seconds (5000ms)
		if _, err := conn.Exec("PRAGMA busy_timeout=5000"); err != nil {
			conn.Close()
			return nil, err
		}

		// Enable foreign keys
		if _, err := conn.Exec("PRAGMA foreign_keys=ON"); err != nil {
			conn.Close()
			return nil, err
		}

		// Set synchronous to NORMAL for better performance
		if _, err := conn.Exec("PRAGMA synchronous=NORMAL"); err != nil {
			conn.Close()
			return nil, err
		}
	}

	return &DB{conn: conn}, nil
}

// Exec executes a query with mutex protection for writes
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	return db.conn.Exec(query, args...)
}

// Begin starts a transaction with mutex protection
func (db *DB) Begin() (*Tx, error) {
	db.writeMu.Lock()
	tx, err := db.conn.Begin()
	if err != nil {
		db.writeMu.Unlock()
		return nil, err
	}
	return &Tx{tx: tx, db: db}, nil
}

// Query performs read operations (no mutex needed for reads)
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// QueryRow performs single row reads (no mutex needed for reads)
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// Prepare prepares a statement
func (db *DB) Prepare(query string) (*sql.Stmt, error) {
	return db.conn.Prepare(query)
}

// Close closes the database connection
func (db *DB) Close() error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	return db.conn.Close()
}

// Tx wraps sql.Tx to ensure mutex is released on commit/rollback
type Tx struct {
	tx       *sql.Tx
	db       *DB
	finished bool // Track if transaction is already finished
}

// Exec executes a query within the transaction
func (t *Tx) Exec(query string, args ...interface{}) (sql.Result, error) {
	return t.tx.Exec(query, args...)
}

// Prepare prepares a statement within the transaction
func (t *Tx) Prepare(query string) (*sql.Stmt, error) {
	return t.tx.Prepare(query)
}

// Query performs a query within the transaction
func (t *Tx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return t.tx.Query(query, args...)
}

// QueryRow performs a single row query within the transaction
func (t *Tx) QueryRow(query string, args ...interface{}) *sql.Row {
	return t.tx.QueryRow(query, args...)
}

// Commit commits the transaction and releases the write lock
func (t *Tx) Commit() error {
	if t.finished {
		return nil // Already committed or rolled back
	}
	t.finished = true
	defer t.db.writeMu.Unlock()
	return t.tx.Commit()
}

// Rollback rolls back the transaction and releases the write lock
func (t *Tx) Rollback() error {
	if t.finished {
		return nil // Already committed or rolled back
	}
	t.finished = true
	defer t.db.writeMu.Unlock()
	return t.tx.Rollback()
}
