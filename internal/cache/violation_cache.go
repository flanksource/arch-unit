package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/flanksource/arch-unit/models"
	_ "modernc.org/sqlite"
)

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
	if err := cache.init(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

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
		violationCacheInstance.Close()
		violationCacheInstance = nil
	}
	violationCacheOnce = sync.Once{}
}

// init creates the necessary tables
func (c *ViolationCache) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS file_scans (
		file_path TEXT PRIMARY KEY,
		last_scan_time INTEGER NOT NULL,
		file_mod_time INTEGER NOT NULL,
		file_hash TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS violations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL,
		line INTEGER NOT NULL,
		column INTEGER NOT NULL,
		source TEXT NOT NULL,
		message TEXT,
		rule_json TEXT,
		caller_package TEXT,
		caller_method TEXT,
		called_package TEXT,
		called_method TEXT,
		fixable INTEGER DEFAULT 0,
		fix_applicability TEXT DEFAULT '',
		FOREIGN KEY (file_path) REFERENCES file_scans(file_path) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_violations_file ON violations(file_path);
	CREATE INDEX IF NOT EXISTS idx_violations_source ON violations(source);
	`

	_, err := c.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migrate existing violations table to add stored_at column
	return c.migrateSchema()
}

// migrateSchema handles schema migrations
func (c *ViolationCache) migrateSchema() error {
	// Check if stored_at column exists
	rows, err := c.db.Query("PRAGMA table_info(violations)")
	if err != nil {
		return err
	}
	defer rows.Close()

	hasStoredAt := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString
		
		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			continue
		}
		
		if name == "stored_at" {
			hasStoredAt = true
			break
		}
	}
	rows.Close()

	// Add stored_at column if it doesn't exist
	if !hasStoredAt {
		// First add column with default 0
		_, err = c.db.Exec("ALTER TABLE violations ADD COLUMN stored_at INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return fmt.Errorf("failed to add stored_at column: %w", err)
		}
		
		// Update all existing records to current timestamp
		_, err = c.db.Exec("UPDATE violations SET stored_at = strftime('%s', 'now') WHERE stored_at = 0")
		if err != nil {
			return fmt.Errorf("failed to update stored_at values: %w", err)
		}
		
		// Create index
		_, err = c.db.Exec("CREATE INDEX IF NOT EXISTS idx_violations_stored_at ON violations(stored_at)")
		if err != nil {
			return fmt.Errorf("failed to create stored_at index: %w", err)
		}
	}

	return nil
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

	var lastScan, fileModTime int64
	var storedHash string
	err = c.db.QueryRow(`
		SELECT last_scan_time, file_mod_time, file_hash 
		FROM file_scans 
		WHERE file_path = ?
	`, filePath).Scan(&lastScan, &fileModTime, &storedHash)

	if err == sql.ErrNoRows {
		return true, nil // Never scanned
	}
	if err != nil {
		return true, err
	}

	// Check if file was modified based on mod time
	if info.ModTime().Unix() > fileModTime {
		return true, nil
	}

	// Double-check with hash for accuracy
	currentHash, err := GetFileHash(filePath)
	if err != nil {
		return true, err
	}

	return currentHash != storedHash, nil
}

// GetCachedViolations retrieves cached violations for a file
func (c *ViolationCache) GetCachedViolations(filePath string) ([]models.Violation, error) {
	rows, err := c.db.Query(`
		SELECT line, column, source, message, rule_json, 
		       caller_package, caller_method, called_package, called_method
		FROM violations
		WHERE file_path = ?
	`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []models.Violation
	for rows.Next() {
		var v models.Violation
		var ruleJSON sql.NullString
		var message sql.NullString

		err := rows.Scan(
			&v.Line, &v.Column, &v.Source, &message, &ruleJSON,
			&v.CallerPackage, &v.CallerMethod, &v.CalledPackage, &v.CalledMethod,
		)
		if err != nil {
			return nil, err
		}

		v.File = filePath
		if message.Valid {
			v.Message = message.String
		}

		if ruleJSON.Valid && ruleJSON.String != "" {
			var rule models.Rule
			if err := json.Unmarshal([]byte(ruleJSON.String), &rule); err == nil {
				v.Rule = &rule
			}
		}

		violations = append(violations, v)
	}

	return violations, rows.Err()
}

// GetAllViolations retrieves all violations from the cache
func (c *ViolationCache) GetAllViolations() ([]models.Violation, error) {
	rows, err := c.db.Query(`
		SELECT file_path, line, column, source, message, rule_json, 
		       caller_package, caller_method, called_package, called_method,
		       fixable, fix_applicability, stored_at
		FROM violations
		ORDER BY file_path, line, column
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []models.Violation
	for rows.Next() {
		var v models.Violation
		var ruleJSON sql.NullString
		var message sql.NullString
		var storedAtUnix int64

		err := rows.Scan(
			&v.File, &v.Line, &v.Column, &v.Source, &message, &ruleJSON,
			&v.CallerPackage, &v.CallerMethod, &v.CalledPackage, &v.CalledMethod,
			&v.Fixable, &v.FixApplicability, &storedAtUnix,
		)
		if err != nil {
			return nil, err
		}

		if message.Valid {
			v.Message = message.String
		}

		if ruleJSON.Valid && ruleJSON.String != "" {
			var rule models.Rule
			if err := json.Unmarshal([]byte(ruleJSON.String), &rule); err == nil {
				v.Rule = &rule
			}
		}

		// Convert Unix timestamp to time.Time
		v.CreatedAt = time.Unix(storedAtUnix, 0)

		violations = append(violations, v)
	}

	return violations, rows.Err()
}

// GetViolationsBySource retrieves violations filtered by source
func (c *ViolationCache) GetViolationsBySource(source string) ([]models.Violation, error) {
	rows, err := c.db.Query(`
		SELECT file_path, line, column, source, message, rule_json, 
		       caller_package, caller_method, called_package, called_method,
		       fixable, fix_applicability
		FROM violations
		WHERE source = ?
		ORDER BY file_path, line, column
	`, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []models.Violation
	for rows.Next() {
		var v models.Violation
		var ruleJSON sql.NullString
		var message sql.NullString

		err := rows.Scan(
			&v.File, &v.Line, &v.Column, &v.Source, &message, &ruleJSON,
			&v.CallerPackage, &v.CallerMethod, &v.CalledPackage, &v.CalledMethod,
			&v.Fixable, &v.FixApplicability,
		)
		if err != nil {
			return nil, err
		}

		if message.Valid {
			v.Message = message.String
		}

		if ruleJSON.Valid && ruleJSON.String != "" {
			var rule models.Rule
			if err := json.Unmarshal([]byte(ruleJSON.String), &rule); err == nil {
				v.Rule = &rule
			}
		}

		violations = append(violations, v)
	}

	return violations, rows.Err()
}

// GetViolationsBySources retrieves violations filtered by multiple sources
func (c *ViolationCache) GetViolationsBySources(sources []string) ([]models.Violation, error) {
	if len(sources) == 0 {
		return []models.Violation{}, nil
	}

	// Build placeholders for SQL IN clause
	placeholders := make([]string, len(sources))
	args := make([]interface{}, len(sources))
	for i, source := range sources {
		placeholders[i] = "?"
		args[i] = source
	}

	query := fmt.Sprintf(`
		SELECT file_path, line, column, source, message, rule_json, 
		       caller_package, caller_method, called_package, called_method,
		       fixable, fix_applicability, stored_at
		FROM violations
		WHERE source IN (%s)
		ORDER BY file_path, line, column
	`, strings.Join(placeholders, ", "))

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []models.Violation
	for rows.Next() {
		var v models.Violation
		var ruleJSON sql.NullString
		var message sql.NullString
		var storedAt int64

		err := rows.Scan(
			&v.File, &v.Line, &v.Column, &v.Source, &message, &ruleJSON,
			&v.CallerPackage, &v.CallerMethod, &v.CalledPackage, &v.CalledMethod,
			&v.Fixable, &v.FixApplicability, &storedAt,
		)
		if err != nil {
			return nil, err
		}

		if message.Valid {
			v.Message = message.String
		}

		if ruleJSON.Valid && ruleJSON.String != "" {
			var rule models.Rule
			if err := json.Unmarshal([]byte(ruleJSON.String), &rule); err == nil {
				v.Rule = &rule
			}
		}

		violations = append(violations, v)
	}

	return violations, rows.Err()
}

// StoreViolations stores violations for a file
func (c *ViolationCache) StoreViolations(filePath string, violations []models.Violation) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

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
	_, err = tx.Exec("DELETE FROM violations WHERE file_path = ?", filePath)
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM file_scans WHERE file_path = ?", filePath)
	if err != nil {
		return err
	}

	// Insert new scan record
	_, err = tx.Exec(`
		INSERT INTO file_scans (file_path, last_scan_time, file_mod_time, file_hash)
		VALUES (?, ?, ?, ?)
	`, filePath, time.Now().Unix(), info.ModTime().Unix(), hash)
	if err != nil {
		return err
	}

	// Insert violations
	stmt, err := tx.Prepare(`
		INSERT INTO violations (
			file_path, line, column, source, message, rule_json,
			caller_package, caller_method, called_package, called_method, 
			fixable, fix_applicability, stored_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, v := range violations {
		var ruleJSON sql.NullString
		if v.Rule != nil {
			data, err := json.Marshal(v.Rule)
			if err == nil {
				ruleJSON = sql.NullString{String: string(data), Valid: true}
			}
		}

		message := sql.NullString{String: v.Message, Valid: v.Message != ""}

		_, err = stmt.Exec(
			filePath, v.Line, v.Column, v.Source, message, ruleJSON,
			v.CallerPackage, v.CallerMethod, v.CalledPackage, v.CalledMethod,
			v.Fixable, v.FixApplicability, time.Now().Unix(),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetAllCachedFiles returns all files that have cached violations
func (c *ViolationCache) GetAllCachedFiles() ([]string, error) {
	rows, err := c.db.Query("SELECT file_path FROM file_scans")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var file string
		if err := rows.Scan(&file); err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// ClearCache removes all cached data
func (c *ViolationCache) ClearCache() error {
	_, err := c.db.Exec("DELETE FROM violations; DELETE FROM file_scans")
	return err
}

// ClearFileCache removes cached data for specific files
func (c *ViolationCache) ClearFileCache(filePaths []string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("DELETE FROM violations WHERE file_path = ?; DELETE FROM file_scans WHERE file_path = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, path := range filePaths {
		if _, err := stmt.Exec(path, path); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Close closes the database connection
func (c *ViolationCache) Close() error {
	return c.db.Close()
}

// GetStats returns cache statistics
func (c *ViolationCache) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var fileCount int
	err := c.db.QueryRow("SELECT COUNT(*) FROM file_scans").Scan(&fileCount)
	if err != nil {
		return nil, err
	}
	stats["cached_files"] = fileCount

	var violationCount int
	err = c.db.QueryRow("SELECT COUNT(*) FROM violations").Scan(&violationCount)
	if err != nil {
		return nil, err
	}
	stats["total_violations"] = violationCount

	// Get cache size
	var pageCount, pageSize int
	err = c.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	if err != nil {
		return nil, err
	}
	err = c.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if err != nil {
		return nil, err
	}
	stats["cache_size_bytes"] = pageCount * pageSize

	return stats, nil
}

// ClearViolations clears violations based on filters
func (c *ViolationCache) ClearViolations(olderThan time.Time, pathPattern string) (int64, error) {
	var query string
	var args []interface{}
	conditions := []string{}
	
	// Build conditions based on filters
	if !olderThan.IsZero() {
		conditions = append(conditions, "stored_at < ?")
		args = append(args, olderThan.Unix())
	}
	
	if pathPattern != "" {
		// For glob patterns, we'll filter in Go after fetching
		// This is simpler than implementing glob matching in SQL
		// First, get all files that match the pattern
		allViolations, err := c.GetAllViolations()
		if err != nil {
			return 0, err
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
			return 0, nil
		}
		
		var filesToDelete []string
		for file := range fileSet {
			filesToDelete = append(filesToDelete, file)
		}
		
		if len(filesToDelete) == 0 {
			return 0, nil
		}
		
		// Build IN clause for file paths
		placeholders := make([]string, len(filesToDelete))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		conditions = append(conditions, fmt.Sprintf("file_path IN (%s)", strings.Join(placeholders, ",")))
		for _, file := range filesToDelete {
			args = append(args, file)
		}
	}
	
	// Build the DELETE query
	if len(conditions) > 0 {
		query = "DELETE FROM violations WHERE " + strings.Join(conditions, " AND ")
	} else {
		// Clear all violations
		query = "DELETE FROM violations"
	}
	
	// Execute the deletion
	result, err := c.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	
	// Also clean up file_scans entries for deleted violations
	if pathPattern != "" && len(conditions) > 0 {
		// Clean up file_scans for matching files
		fileQuery := "DELETE FROM file_scans WHERE file_path IN (SELECT DISTINCT file_path FROM violations WHERE " + strings.Join(conditions, " AND ") + ")"
		c.db.Exec(fileQuery, args...)
	} else if !olderThan.IsZero() {
		// Clean up old file_scans entries
		c.db.Exec("DELETE FROM file_scans WHERE last_scan_time < ?", olderThan.Unix())
	} else {
		// Clear all file_scans if clearing all violations
		c.db.Exec("DELETE FROM file_scans")
	}
	
	return result.RowsAffected()
}