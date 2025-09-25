package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/flanksource/arch-unit/models"
	commonsLogger "github.com/flanksource/commons/logger"
)

var (
	gormInstance *gorm.DB
	gormOnce     sync.Once
	gormMutex    sync.RWMutex
)

// GetGormDB returns the singleton GORM database instance
func GetGormDB() (*gorm.DB, error) {
	var err error
	gormOnce.Do(func() {
		gormInstance, err = newGormDB()
	})
	if err != nil {
		return nil, err
	}
	return gormInstance, nil
}

// MustGetGormDB returns GORM instance or panics
func MustGetGormDB() *gorm.DB {
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
	
	if gormInstance != nil {
		sqlDB, _ := gormInstance.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		gormInstance = nil
	}
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

// NewGormDBWithPath creates a new GORM database instance in the specified directory
func NewGormDBWithPath(cacheDir string) (*gorm.DB, error) {
	return newGormDBWithPath(cacheDir)
}

// newGormDBWithPath creates a new GORM database instance in the specified directory
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

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

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