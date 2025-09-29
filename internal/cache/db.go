package cache

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/flanksource/arch-unit/models"
	commonsLogger "github.com/flanksource/commons/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB wraps GORM DB with mutex synchronization for write operations
type DB struct {
	conn    *gorm.DB
	writeMu sync.Mutex // Protects write operations
}

// NewDB creates a new synchronized GORM database wrapper
func NewDB(driverName, dataSourceName string) (*DB, error) {
	var gormDB *gorm.DB
	var err error

	if driverName == "sqlite" {
		// Configure GORM with SQLite
		// Enable SQL logging if verbosity level is 3 or higher (-vvv)
		var logMode logger.LogLevel = logger.Silent
		if commonsLogger.IsLevelEnabled(3) {
			logMode = logger.Info // Enable SQL query logging for high verbosity
		}

		config := &gorm.Config{
			Logger: logger.Default.LogMode(logMode),
		}
		
		gormDB, err = gorm.Open(sqlite.Open(dataSourceName), config)
		if err != nil {
			return nil, err
		}

		// Get underlying sql.DB to configure SQLite pragmas
		sqlDB, err := gormDB.DB()
		if err != nil {
			return nil, err
		}

		// Configure SQLite for better concurrency
		// Enable WAL mode for better concurrent access
		if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
			return nil, err
		}

		// Set busy timeout to 5 seconds (5000ms)
		if _, err := sqlDB.Exec("PRAGMA busy_timeout=5000"); err != nil {
			return nil, err
		}

		// Enable foreign keys
		if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
			return nil, err
		}

		// Set synchronous to NORMAL for better performance
		if _, err := sqlDB.Exec("PRAGMA synchronous=NORMAL"); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unsupported database driver: %s", driverName)
	}

	db := &DB{conn: gormDB}
	
	// Automatically run migrations for all models
	if err := db.Migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	
	return db, nil
}

// GormDB returns the underlying GORM database instance
func (db *DB) GormDB() *gorm.DB {
	return db.conn
}

// Exec executes a query with mutex protection for writes
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	sqlDB, err := db.conn.DB()
	if err != nil {
		return nil, err
	}
	return sqlDB.Exec(query, args...)
}

// Begin starts a transaction with mutex protection
func (db *DB) Begin() (*Tx, error) {
	db.writeMu.Lock()
	sqlDB, err := db.conn.DB()
	if err != nil {
		db.writeMu.Unlock()
		return nil, err
	}
	tx, err := sqlDB.Begin()
	if err != nil {
		db.writeMu.Unlock()
		return nil, err
	}
	return &Tx{tx: tx, db: db}, nil
}

// Query performs read operations (no mutex needed for reads)
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	sqlDB, err := db.conn.DB()
	if err != nil {
		return nil, err
	}
	return sqlDB.Query(query, args...)
}

// QueryRow performs single row reads (no mutex needed for reads)
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	sqlDB, _ := db.conn.DB()
	return sqlDB.QueryRow(query, args...)
}

// Prepare prepares a statement
func (db *DB) Prepare(query string) (*sql.Stmt, error) {
	sqlDB, err := db.conn.DB()
	if err != nil {
		return nil, err
	}
	return sqlDB.Prepare(query)
}

// Close closes the database connection
func (db *DB) Close() error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()
	sqlDB, err := db.conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Migrate runs automatic migrations for all models
func (db *DB) Migrate() error {
	return db.conn.AutoMigrate(
		&models.Violation{},
		&models.ASTNode{},
		&FileScan{},
	)
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
