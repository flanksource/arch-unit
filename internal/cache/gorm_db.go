package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/flanksource/arch-unit/models"
	commonsLogger "github.com/flanksource/commons/logger"
)

// DualPoolGormDB provides separate read and write connection pools for optimal SQLite concurrency
type DualPoolGormDB struct {
	readDB  *gorm.DB
	writeDB *gorm.DB
}


// DualPoolGormDB Methods - Simplified interface
func (d *DualPoolGormDB) GetReadDB() *gorm.DB {
	return d.readDB
}

func (d *DualPoolGormDB) GetWriteDB() *gorm.DB {
	return d.writeDB
}

// Legacy compatibility methods - route to read pool by default
func (d *DualPoolGormDB) Where(query interface{}, args ...interface{}) *gorm.DB {
	return d.readDB.Where(query, args...)
}

func (d *DualPoolGormDB) Model(value interface{}) *gorm.DB {
	return d.readDB.Model(value)
}

func (d *DualPoolGormDB) Raw(sql string, values ...interface{}) *gorm.DB {
	// Route based on SQL verb - simple heuristic
	sqlLower := strings.ToLower(strings.TrimSpace(sql))
	isWrite := strings.HasPrefix(sqlLower, "insert") ||
			   strings.HasPrefix(sqlLower, "update") ||
			   strings.HasPrefix(sqlLower, "delete") ||
			   strings.HasPrefix(sqlLower, "create") ||
			   strings.HasPrefix(sqlLower, "drop") ||
			   strings.HasPrefix(sqlLower, "alter")

	if isWrite {
		return d.writeDB.Raw(sql, values...)
	}
	return d.readDB.Raw(sql, values...)
}

// Direct write operations - route to write pool
func (d *DualPoolGormDB) Create(value interface{}) error {
	return d.writeDB.Create(value).Error
}

func (d *DualPoolGormDB) Save(value interface{}) error {
	return d.writeDB.Save(value).Error
}

func (d *DualPoolGormDB) First(dest interface{}, conds ...interface{}) error {
	return d.readDB.First(dest, conds...).Error
}

// Transaction operations - always use write pool
func (d *DualPoolGormDB) Transaction(fc func(*gorm.DB) error) error {
	return d.writeDB.Transaction(fc)
}

func (d *DualPoolGormDB) Exec(sql string, values ...interface{}) error {
	return d.writeDB.Exec(sql, values...).Error
}

// AutoMigrate with write pool
func (d *DualPoolGormDB) AutoMigrate(dst ...interface{}) error {
	return d.writeDB.AutoMigrate(dst...)
}

// DB returns the underlying sql.DB from read pool (for connection info)
func (d *DualPoolGormDB) DB() (*sql.DB, error) {
	return d.readDB.DB()
}

// getRawDB returns the read database (internal use only)
func (d *DualPoolGormDB) getRawDB() *gorm.DB {
	return d.readDB
}

// Legacy ProtectedGormDB wraps GORM with read-write mutex for thread-safe database operations
// Deprecated: Use DualPoolGormDB for better SQLite concurrency
type ProtectedGormDB struct {
	db      *gorm.DB
	rwMutex sync.RWMutex
}

// ProtectedQuery represents a query builder with lock protection
// Deprecated: Use direct GORM access through GetReadDB/GetWriteDB for better SQLite concurrency
type ProtectedQuery struct {
	db      *gorm.DB
	rwMutex *sync.RWMutex
	isRead  bool
	locked  bool
}

// NewDualPoolGormDB creates a new dual-pool GORM database wrapper with separate read/write pools
func NewDualPoolGormDB(readDB, writeDB *gorm.DB) *DualPoolGormDB {
	return &DualPoolGormDB{
		readDB:  readDB,
		writeDB: writeDB,
	}
}

// NewProtectedGormDB creates a new protected GORM database wrapper
// Deprecated: Use NewDualPoolGormDB for better SQLite concurrency
func NewProtectedGormDB(db *gorm.DB) *ProtectedGormDB {
	return &ProtectedGormDB{
		db:      db,
		rwMutex: sync.RWMutex{},
	}
}

// GetReadDB returns the database for read operations (with protection)
func (p *ProtectedGormDB) GetReadDB() *gorm.DB {
	// For legacy compatibility, return the raw DB
	// The RWMutex protection is handled in the individual methods
	return p.db
}

// GetWriteDB returns the database for write operations (with protection)
func (p *ProtectedGormDB) GetWriteDB() *gorm.DB {
	// For legacy compatibility, return the raw DB
	// The RWMutex protection is handled in the individual methods
	return p.db
}

// Legacy compatibility methods
func (p *ProtectedGormDB) Where(query interface{}, args ...interface{}) *gorm.DB {
	return p.db.Where(query, args...)
}

func (p *ProtectedGormDB) Model(value interface{}) *gorm.DB {
	return p.db.Model(value)
}

func (p *ProtectedGormDB) Raw(sql string, values ...interface{}) *gorm.DB {
	return p.db.Raw(sql, values...)
}

// WithReadLock creates a query builder with read lock protection
// Deprecated: Use GetReadDB() for direct access
func (p *ProtectedGormDB) WithReadLock() *ProtectedQuery {
	p.rwMutex.RLock()
	return &ProtectedQuery{
		db:      p.db,
		rwMutex: &p.rwMutex,
		isRead:  true,
		locked:  true,
	}
}

// WithWriteLock creates a query builder with write lock protection
// Deprecated: Use GetWriteDB() for direct access
func (p *ProtectedGormDB) WithWriteLock() *ProtectedQuery {
	p.rwMutex.Lock()
	return &ProtectedQuery{
		db:      p.db,
		rwMutex: &p.rwMutex,
		isRead:  false,
		locked:  true,
	}
}

// Unlock releases the lock held by this query
func (pq *ProtectedQuery) Unlock() {
	if pq.locked {
		if pq.isRead {
			pq.rwMutex.RUnlock()
		} else {
			pq.rwMutex.Unlock()
		}
		pq.locked = false
	}
}

// GORM Query Builder Methods for ProtectedQuery
func (pq *ProtectedQuery) Where(query interface{}, args ...interface{}) *ProtectedQuery {
	pq.db = pq.db.Where(query, args...)
	return pq
}

func (pq *ProtectedQuery) First(dest interface{}, conds ...interface{}) error {
	defer pq.Unlock()
	return pq.db.First(dest, conds...).Error
}

func (pq *ProtectedQuery) Find(dest interface{}, conds ...interface{}) error {
	defer pq.Unlock()
	return pq.db.Find(dest, conds...).Error
}

func (pq *ProtectedQuery) Count(count *int64) error {
	defer pq.Unlock()
	return pq.db.Count(count).Error
}

func (pq *ProtectedQuery) Create(value interface{}) error {
	defer pq.Unlock()
	return pq.db.Create(value).Error
}

func (pq *ProtectedQuery) Save(value interface{}) error {
	defer pq.Unlock()
	return pq.db.Save(value).Error
}

func (pq *ProtectedQuery) Delete(value interface{}, conds ...interface{}) error {
	defer pq.Unlock()
	return pq.db.Delete(value, conds...).Error
}

func (pq *ProtectedQuery) Order(value interface{}) *ProtectedQuery {
	pq.db = pq.db.Order(value)
	return pq
}

func (pq *ProtectedQuery) Limit(limit int) *ProtectedQuery {
	pq.db = pq.db.Limit(limit)
	return pq
}

func (pq *ProtectedQuery) Offset(offset int) *ProtectedQuery {
	pq.db = pq.db.Offset(offset)
	return pq
}

func (pq *ProtectedQuery) Model(value interface{}) *ProtectedQuery {
	pq.db = pq.db.Model(value)
	return pq
}

func (pq *ProtectedQuery) Pluck(column string, dest interface{}) error {
	defer pq.Unlock()
	return pq.db.Pluck(column, dest).Error
}

func (pq *ProtectedQuery) Raw(sql string, values ...interface{}) *ProtectedQuery {
	pq.db = pq.db.Raw(sql, values...)
	return pq
}

func (pq *ProtectedQuery) Scan(dest interface{}) error {
	defer pq.Unlock()
	return pq.db.Scan(dest).Error
}

func (pq *ProtectedQuery) Clauses(conds ...clause.Expression) *ProtectedQuery {
	pq.db = pq.db.Clauses(conds...)
	return pq
}

func (pq *ProtectedQuery) Select(query interface{}, args ...interface{}) *ProtectedQuery {
	pq.db = pq.db.Select(query, args...)
	return pq
}

func (pq *ProtectedQuery) Joins(query string, args ...interface{}) *ProtectedQuery {
	pq.db = pq.db.Joins(query, args...)
	return pq
}

func (pq *ProtectedQuery) Preload(query string, args ...interface{}) *ProtectedQuery {
	pq.db = pq.db.Preload(query, args...)
	return pq
}

// Direct protected methods for common operations
func (p *ProtectedGormDB) Transaction(fc func(*gorm.DB) error) error {
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	return p.db.Transaction(fc)
}

func (p *ProtectedGormDB) Exec(sql string, values ...interface{}) error {
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	return p.db.Exec(sql, values...).Error
}

// AutoMigrate with write lock protection
func (p *ProtectedGormDB) AutoMigrate(dst ...interface{}) error {
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	return p.db.AutoMigrate(dst...)
}

// DB returns the underlying sql.DB with read lock protection
func (p *ProtectedGormDB) DB() (*sql.DB, error) {
	p.rwMutex.RLock()
	defer p.rwMutex.RUnlock()
	return p.db.DB()
}

// getRawDB returns the underlying GORM DB (internal use only)
func (p *ProtectedGormDB) getRawDB() *gorm.DB {
	return p.db
}

// Direct GORM methods with appropriate locking



func (p *ProtectedGormDB) Save(value interface{}) error {
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	return p.db.Save(value).Error
}

func (p *ProtectedGormDB) First(dest interface{}, conds ...interface{}) error {
	p.rwMutex.RLock()
	defer p.rwMutex.RUnlock()
	return p.db.First(dest, conds...).Error
}

func (p *ProtectedGormDB) Create(value interface{}) error {
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	return p.db.Create(value).Error
}


// DBInterface defines a simplified interface for database operations
type DBInterface interface {
	// Transaction operations
	Transaction(fc func(*gorm.DB) error) error

	// Schema operations
	AutoMigrate(dst ...interface{}) error

	// Connection info
	DB() (*sql.DB, error)

	// Direct operations (backward compatibility)
	Create(value interface{}) error
	Save(value interface{}) error
	First(dest interface{}, conds ...interface{}) error
	Exec(sql string, values ...interface{}) error

	// Query builders for compatibility (returns GORM DB for fluent API)
	GetReadDB() *gorm.DB
	GetWriteDB() *gorm.DB

	// Legacy compatibility methods
	Where(query interface{}, args ...interface{}) *gorm.DB
	Model(value interface{}) *gorm.DB
	Raw(sql string, values ...interface{}) *gorm.DB
}

var (
	dualPoolGormInstance  *DualPoolGormDB
	protectedGormInstance *ProtectedGormDB
	gormInstance          *gorm.DB
	gormOnce              sync.Once
	gormMutex             sync.RWMutex
	useDualPool           = true // Feature flag to switch between implementations
)

// GetDualPoolGormDB returns the singleton dual-pool GORM database instance
func GetDualPoolGormDB() (*DualPoolGormDB, error) {
	var err error
	gormOnce.Do(func() {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			err = fmt.Errorf("failed to get home directory: %w", homeErr)
			return
		}
		cacheDir := filepath.Join(homeDir, ".cache", "arch-unit")
		dualPoolGormInstance, err = newDualPoolGormDBWithPath(cacheDir)
	})
	if err != nil {
		return nil, err
	}
	return dualPoolGormInstance, nil
}

// MustGetDualPoolGormDB returns dual-pool GORM instance or panics
func MustGetDualPoolGormDB() *DualPoolGormDB {
	db, err := GetDualPoolGormDB()
	if err != nil {
		panic(err)
	}
	return db
}

// GetProtectedGormDB returns the singleton protected GORM database instance
// Deprecated: Use GetDualPoolGormDB for better SQLite concurrency
func GetProtectedGormDB() (*ProtectedGormDB, error) {
	if useDualPool {
		// For backward compatibility, wrap DualPoolGormDB
		dual, err := GetDualPoolGormDB()
		if err != nil {
			return nil, err
		}
		// Return a compatible wrapper - we'll need to implement this
		return &ProtectedGormDB{db: dual.getRawDB()}, nil
	}

	var err error
	gormOnce.Do(func() {
		gormInstance, err = newGormDB()
		if err == nil {
			protectedGormInstance = NewProtectedGormDB(gormInstance)
		}
	})
	if err != nil {
		return nil, err
	}
	return protectedGormInstance, nil
}

// MustGetProtectedGormDB returns protected GORM instance or panics
// Deprecated: Use MustGetDualPoolGormDB for better SQLite concurrency
func MustGetProtectedGormDB() *ProtectedGormDB {
	db, err := GetProtectedGormDB()
	if err != nil {
		panic(err)
	}
	return db
}

// GetGormDB returns the singleton GORM database instance (now dual-pool by default)
func GetGormDB() (DBInterface, error) {
	if useDualPool {
		return GetDualPoolGormDB()
	}
	return GetProtectedGormDB()
}

// MustGetGormDB returns GORM instance or panics (now dual-pool by default)
func MustGetGormDB() DBInterface {
	db, err := GetGormDB()
	if err != nil {
		panic(err)
	}
	return db
}

// ResetGormDB resets the singleton instance (mainly for testing)
func ResetGormDB() {
	gormMutex.Lock()
	defer gormMutex.Unlock()

	if dualPoolGormInstance != nil {
		if readSqlDB, err := dualPoolGormInstance.readDB.DB(); err == nil && readSqlDB != nil {
			_ = readSqlDB.Close()
		}
		if writeSqlDB, err := dualPoolGormInstance.writeDB.DB(); err == nil && writeSqlDB != nil {
			_ = writeSqlDB.Close()
		}
		dualPoolGormInstance = nil
	}

	if gormInstance != nil {
		sqlDB, _ := gormInstance.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		gormInstance = nil
	}
	protectedGormInstance = nil
	gormOnce = sync.Once{}
}

// newGormDB creates a new GORM database instance
func newGormDB() (*gorm.DB, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "arch-unit")
	return newGormDBWithPath(cacheDir)
}

// NewGormDBWithPath creates a new protected GORM database instance in the specified directory
func NewGormDBWithPath(cacheDir string) (*ProtectedGormDB, error) {
	rawDB, err := newGormDBWithPath(cacheDir)
	if err != nil {
		return nil, err
	}
	return NewProtectedGormDB(rawDB), nil
}

// newDualPoolGormDBWithPath creates dual GORM database pools in the specified directory
func newDualPoolGormDBWithPath(cacheDir string) (*DualPoolGormDB, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "ast.db")

	// Create read-only connection string with file: prefix and SQLite parameters
	readConnStr := fmt.Sprintf("file:%s?mode=ro&_journal_mode=wal&_busy_timeout=5000&_foreign_keys=on&_synchronous=normal&_cache_size=10000&_temp_store=memory", dbPath)

	// Create read-write connection string with file: prefix, SQLite parameters, and BEGIN IMMEDIATE
	writeConnStr := fmt.Sprintf("file:%s?mode=rw&_journal_mode=wal&_txlock=immediate&_busy_timeout=5000&_foreign_keys=on&_synchronous=normal&_cache_size=10000&_temp_store=memory", dbPath)

	// Configure GORM
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Reduce log noise
	}

	// Create write database first (needed for migrations)
	writeDB, err := gorm.Open(sqlite.Open(writeConnStr), config)
	if err != nil {
		return nil, fmt.Errorf("failed to open write database with GORM: %w", err)
	}

	// Configure write database connection pool (single connection for SQLite)
	writeSqlDB, err := writeDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying write sql.DB: %w", err)
	}
	writeSqlDB.SetMaxIdleConns(1)  // Single connection for writes
	writeSqlDB.SetMaxOpenConns(1)  // SQLite single writer constraint

	// Auto-migrate all models using write database
	if err := autoMigrateModels(writeDB); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate models: %w", err)
	}

	// Create read-only database
	readDB, err := gorm.Open(sqlite.Open(readConnStr), config)
	if err != nil {
		return nil, fmt.Errorf("failed to open read database with GORM: %w", err)
	}

	// Configure read database connection pool (multiple connections for concurrent reads)
	readSqlDB, err := readDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying read sql.DB: %w", err)
	}
	readSqlDB.SetMaxIdleConns(5)   // Multiple connections for concurrent reads
	readSqlDB.SetMaxOpenConns(10)  // Allow concurrent reads

	return NewDualPoolGormDB(readDB, writeDB), nil
}

// newGormDBWithPath creates a new GORM database instance in the specified directory
// Deprecated: Use newDualPoolGormDBWithPath for better SQLite concurrency
func newGormDBWithPath(cacheDir string) (*gorm.DB, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "ast.db")

	// Configure GORM
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Reduce log noise
	}

	// Open database with GORM
	db, err := gorm.Open(sqlite.Open(dbPath), config)
	if err != nil {
		return nil, fmt.Errorf("failed to open database with GORM: %w", err)
	}

	// Configure underlying SQL database
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Configure SQLite for better concurrency (same as original)
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return nil, fmt.Errorf("failed to set synchronous mode: %w", err)
	}

	// Additional performance optimizations for concurrent access
	if _, err := sqlDB.Exec("PRAGMA cache_size=10000"); err != nil {
		return nil, fmt.Errorf("failed to set cache size: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA temp_store=memory"); err != nil {
		return nil, fmt.Errorf("failed to set temp store: %w", err)
	}

	// Set connection pool settings optimized for SQLite's locking model
	sqlDB.SetMaxIdleConns(5)   // Reduced from 10 - fewer idle connections
	sqlDB.SetMaxOpenConns(20)  // Reduced from 100 - SQLite works better with fewer connections

	// Auto-migrate all models
	if err := autoMigrateModels(db); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate models: %w", err)
	}

	return db, nil
}

// autoMigrateModels performs auto-migration for all models
func autoMigrateModels(db *gorm.DB) error {
	// Migrate all models in proper order (dependencies first)
	modelsToMigrate := []interface{}{
		&models.FileMetadata{},
		&models.ASTNode{},
		&models.ASTRelationship{},
		&models.LibraryNode{},
		&models.LibraryRelationship{},
		&models.DependencyAlias{},
		&models.FileScan{},
		&models.Violation{},
	}

	for _, model := range modelsToMigrate {
		if err := db.AutoMigrate(model); err != nil {
			// If we get a foreign key constraint error, try to truncate data and retry
			if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
				commonsLogger.Warnf("Foreign key constraint error during migration, truncating data and retrying")
				
				// Truncate all tables in reverse order to avoid FK constraints
				tablesToTruncate := []interface{}{
					&models.Violation{},
					&models.FileScan{},
					&models.DependencyAlias{},
					&models.LibraryRelationship{},
					&models.LibraryNode{},
					&models.ASTRelationship{},
					&models.ASTNode{},
					&models.FileMetadata{},
				}
				
				// Disable foreign keys temporarily
				db.Exec("PRAGMA foreign_keys = OFF")
				
				for _, table := range tablesToTruncate {
					if truncErr := db.Unscoped().Where("1 = 1").Delete(table).Error; truncErr != nil {
						commonsLogger.Warnf("Failed to truncate table %T: %v", table, truncErr)
					}
				}
				
				// Re-enable foreign keys
				db.Exec("PRAGMA foreign_keys = ON")
				
				// Retry migration
				if retryErr := db.AutoMigrate(model); retryErr != nil {
					return fmt.Errorf("failed to migrate model %T after truncation: %w", model, retryErr)
				}
			} else {
				return fmt.Errorf("failed to migrate model %T: %w", model, err)
			}
		}
	}

	return nil
}

// ClearAllGormData removes all data from GORM tables (useful for testing)
func ClearAllGormData() error {
	db, err := GetGormDB()
	if err != nil {
		return err
	}

	// Clear all tables in proper order (relationships first)
	tables := []interface{}{
		&models.ASTRelationship{},
		&models.LibraryRelationship{},
		&models.ASTNode{},
		&models.LibraryNode{},
		&models.FileMetadata{},
		&models.FileScan{},
		&models.DependencyAlias{},
		&models.Violation{},
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range tables {
			if err := tx.Unscoped().Delete(table, "1 = 1").Error; err != nil {
				return fmt.Errorf("failed to clear table %T: %w", table, err)
			}
		}
		return nil
	})
}

// TestWriteAccess performs a no-op SQL update to verify database write permissions
func TestWriteAccess() error {
	db, err := GetGormDB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	// Perform no-op update to test write access
	// This updates file_metadata rows with id = -1 (which should match 0 rows)
	// but tests that we have write permissions on the database
	if err := db.Exec("UPDATE file_metadata SET last_analyzed = last_analyzed WHERE id = ?", -1); err != nil {
		return formatWriteAccessError(err)
	}

	return nil
}

// formatWriteAccessError provides user-friendly error messages for common database write issues
func formatWriteAccessError(err error) error {
	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "permission denied") || strings.Contains(errStr, "access denied"):
		return fmt.Errorf("insufficient file permissions to write to database: %w\nTry: chmod 755 ~/.cache/arch-unit/ && chmod 644 ~/.cache/arch-unit/*.db", err)
	case strings.Contains(errStr, "database is locked") || strings.Contains(errStr, "locked"):
		return fmt.Errorf("database is locked by another process: %w\nAnother arch-unit process may be running, or the database wasn't properly closed", err)
	case strings.Contains(errStr, "no space left") || strings.Contains(errStr, "disk full"):
		return fmt.Errorf("insufficient disk space to write to database: %w\nFree up space in ~/.cache/arch-unit/ directory", err)
	case strings.Contains(errStr, "read-only") || strings.Contains(errStr, "readonly"):
		return fmt.Errorf("database is mounted read-only: %w\nCheck filesystem mount options", err)
	default:
		return fmt.Errorf("database write access test failed: %w\nEnsure ~/.cache/arch-unit/ exists and is writable", err)
	}
}