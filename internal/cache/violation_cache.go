package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/flanksource/arch-unit/models"
	"gorm.io/gorm"
)

// FileScan represents a file scan record
type FileScan struct {
	FilePath      string `gorm:"primaryKey;column:file_path"`
	LastScanTime  int64  `gorm:"column:last_scan_time;not null"`
	FileModTime   int64  `gorm:"column:file_mod_time;not null"`
	FileHash      string `gorm:"column:file_hash;not null"`
}

// TableName specifies the table name for FileScan
func (FileScan) TableName() string {
	return "file_scans"
}

// ViolationCache manages cached violations using SQLite
type ViolationCache struct {
	db *DB
}

var (
	violationCacheInstance *ViolationCache
	violationCacheOnce     sync.Once
	violationCacheMutex    sync.RWMutex
)

// NewViolationCache creates a new violation cache
func NewViolationCache() (*ViolationCache, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "arch-unit")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "violations.db")
	db, err := NewDB("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	cache := &ViolationCache{db: db}
	// Migrations are handled automatically by the DB class during connection

	return cache, nil
}

// GetViolationCache returns the global violation cache singleton
func GetViolationCache() (*ViolationCache, error) {
	var err error
	violationCacheOnce.Do(func() {
		violationCacheInstance, err = NewViolationCache()
	})
	return violationCacheInstance, err
}

// ResetViolationCache resets the singleton (for testing)
func ResetViolationCache() {
	violationCacheMutex.Lock()
	defer violationCacheMutex.Unlock()
	if violationCacheInstance != nil {
		_ = violationCacheInstance.Close()
		violationCacheInstance = nil
	}
	violationCacheOnce = sync.Once{}
}


// GetFileHash computes SHA256 hash of file contents
func GetFileHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// NeedsRescan checks if a file needs to be rescanned
func (c *ViolationCache) NeedsRescan(filePath string) (bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return true, nil // File doesn't exist, needs scan
	}

	gormDB := c.db.GormDB()
	var fileScan FileScan
	err = gormDB.Where("file_path = ?", filePath).First(&fileScan).Error

	if err == gorm.ErrRecordNotFound {
		return true, nil // Never scanned
	}
	if err != nil {
		return true, err
	}

	// Check if file was modified based on mod time
	if info.ModTime().Unix() > fileScan.FileModTime {
		return true, nil
	}

	// Double-check with hash for accuracy
	currentHash, err := GetFileHash(filePath)
	if err != nil {
		return true, err
	}

	return currentHash != fileScan.FileHash, nil
}

// GetCachedViolations retrieves cached violations for a file
func (c *ViolationCache) GetCachedViolations(filePath string) ([]models.Violation, error) {
	gormDB := c.db.GormDB()
	var violations []models.Violation
	
	err := gormDB.Preload("Caller").Preload("Called").
		Where("file_path = ?", filePath).Find(&violations).Error
	
	return violations, err
}

// GetAllViolations retrieves all violations from the cache
func (c *ViolationCache) GetAllViolations() ([]models.Violation, error) {
	gormDB := c.db.GormDB()
	var violations []models.Violation
	
	err := gormDB.Preload("Caller").Preload("Called").
		Order("file_path, line, column").Find(&violations).Error
	
	return violations, err
}

// GetViolationsBySource retrieves violations filtered by source
func (c *ViolationCache) GetViolationsBySource(source string) ([]models.Violation, error) {
	gormDB := c.db.GormDB()
	var violations []models.Violation
	
	err := gormDB.Preload("Caller").Preload("Called").
		Where("source = ?", source).Order("file_path, line, column").Find(&violations).Error
	
	return violations, err
}

// GetViolationsBySources retrieves violations filtered by multiple sources
func (c *ViolationCache) GetViolationsBySources(sources []string) ([]models.Violation, error) {
	if len(sources) == 0 {
		return []models.Violation{}, nil
	}

	gormDB := c.db.GormDB()
	var violations []models.Violation
	
	err := gormDB.Preload("Caller").Preload("Called").
		Where("source IN ?", sources).Order("file_path, line, column").Find(&violations).Error
	
	return violations, err
}

// StoreViolations stores violations for a file
func (c *ViolationCache) StoreViolations(filePath string, violations []models.Violation) error {
	gormDB := c.db.GormDB()
	
	// Use GORM transaction
	return gormDB.Transaction(func(tx *gorm.DB) error {
		// Get file info
		info, err := os.Stat(filePath)
		if err != nil {
			return err
		}

		hash, err := GetFileHash(filePath)
		if err != nil {
			return err
		}

		// Delete old data
		if err := tx.Where("file_path = ?", filePath).Delete(&models.Violation{}).Error; err != nil {
			return err
		}
		
		if err := tx.Where("file_path = ?", filePath).Delete(&FileScan{}).Error; err != nil {
			return err
		}

		// Insert new scan record
		fileScan := FileScan{
			FilePath:     filePath,
			LastScanTime: time.Now().Unix(),
			FileModTime:  info.ModTime().Unix(),
			FileHash:     hash,
		}
		if err := tx.Create(&fileScan).Error; err != nil {
			return err
		}

		// Insert violations - first create any AST nodes if they don't exist
		for i := range violations {
			v := &violations[i]
			v.File = filePath
			
			// Create or update AST nodes if they exist
			if v.Caller != nil {
				if err := tx.Save(v.Caller).Error; err != nil {
					return err
				}
			}
			if v.Called != nil {
				if err := tx.Save(v.Called).Error; err != nil {
					return err
				}
			}
			
			// Create the violation
			if err := tx.Create(v).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// GetAllCachedFiles returns all files that have cached violations
func (c *ViolationCache) GetAllCachedFiles() ([]string, error) {
	gormDB := c.db.GormDB()
	var fileScans []FileScan
	
	err := gormDB.Select("file_path").Find(&fileScans).Error
	if err != nil {
		return nil, err
	}
	
	files := make([]string, len(fileScans))
	for i, fs := range fileScans {
		files[i] = fs.FilePath
	}
	
	return files, nil
}

// ClearCache removes all cached data
func (c *ViolationCache) ClearCache() error {
	gormDB := c.db.GormDB()
	
	// Use GORM session for batch deletes
	if err := gormDB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.Violation{}).Error; err != nil {
		return err
	}
	
	return gormDB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&FileScan{}).Error
}

// ClearFileCache removes cached data for specific files
func (c *ViolationCache) ClearFileCache(filePaths []string) error {
	gormDB := c.db.GormDB()
	
	return gormDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("file_path IN ?", filePaths).Delete(&models.Violation{}).Error; err != nil {
			return err
		}
		
		return tx.Where("file_path IN ?", filePaths).Delete(&FileScan{}).Error
	})
}

// Close closes the database connection
func (c *ViolationCache) Close() error {
	gormDB := c.db.GormDB()
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// GetStats returns cache statistics
func (c *ViolationCache) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	gormDB := c.db.GormDB()

	var fileCount int64
	if err := gormDB.Model(&FileScan{}).Count(&fileCount).Error; err != nil {
		return nil, err
	}
	stats["cached_files"] = fileCount

	var violationCount int64
	if err := gormDB.Model(&models.Violation{}).Count(&violationCount).Error; err != nil {
		return nil, err
	}
	stats["total_violations"] = violationCount

	// Get cache size using raw SQL for PRAGMA
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}
	
	var pageCount, pageSize int
	if err := sqlDB.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return nil, err
	}
	if err := sqlDB.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return nil, err
	}
	stats["cache_size_bytes"] = pageCount * pageSize

	return stats, nil
}

// ClearViolations clears violations based on filters
func (c *ViolationCache) ClearViolations(olderThan time.Time, pathPattern string) (int64, error) {
	gormDB := c.db.GormDB()
	var deletedCount int64

	err := gormDB.Transaction(func(tx *gorm.DB) error {
		var filesToDelete []string

		// Handle path pattern filtering
		if pathPattern != "" {
			// Get all violations to filter by pattern
			allViolations, err := c.GetAllViolations()
			if err != nil {
				return err
			}

			fileSet := make(map[string]bool)
			for _, v := range allViolations {
				matched := false

				// Use doublestar for proper glob matching with ** support
				if match, err := doublestar.Match(pathPattern, v.File); err == nil && match {
					matched = true
				}

				// Try matching against basename if full path didn't match
				if !matched {
					if match, err := doublestar.Match(pathPattern, filepath.Base(v.File)); err == nil && match {
						matched = true
					}
				}

				// For relative patterns, try matching against relative path
				if !matched && !filepath.IsAbs(pathPattern) {
					if relPath, err := filepath.Rel(filepath.Dir(v.File), v.File); err == nil {
						if match, err := doublestar.Match(pathPattern, relPath); err == nil && match {
							matched = true
						}
					}
				}

				if matched {
					fileSet[v.File] = true
				}
			}

			if len(fileSet) == 0 {
				return nil
			}

			for file := range fileSet {
				filesToDelete = append(filesToDelete, file)
			}
		}

		// Build the query
		query := tx.Model(&models.Violation{})

		// Apply time filter
		if !olderThan.IsZero() {
			query = query.Where("created_at < ?", olderThan)
		}

		// Apply file pattern filter
		if len(filesToDelete) > 0 {
			query = query.Where("file_path IN ?", filesToDelete)
		}

		// Count before deletion
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		deletedCount = count

		// Delete violations
		if !olderThan.IsZero() || len(filesToDelete) > 0 {
			if err := query.Delete(&models.Violation{}).Error; err != nil {
				return err
			}
		} else {
			// Delete all violations
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.Violation{}).Error; err != nil {
				return err
			}
		}

		// Clean up file_scans entries
		if len(filesToDelete) > 0 {
			if err := tx.Where("file_path IN ?", filesToDelete).Delete(&FileScan{}).Error; err != nil {
				return err
			}
		} else if !olderThan.IsZero() {
			if err := tx.Where("last_scan_time < ?", olderThan.Unix()).Delete(&FileScan{}).Error; err != nil {
				return err
			}
		} else {
			// Clear all file_scans if clearing all violations
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&FileScan{}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	return deletedCount, err
}
