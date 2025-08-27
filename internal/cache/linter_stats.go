package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// LinterStats tracks execution metrics for intelligent debouncing
type LinterStats struct {
	db *DB
}

// ExecutionStats contains metrics for a specific linter
type ExecutionStats struct {
	LinterName     string
	WorkDir        string
	LastRun        time.Time
	LastDuration   time.Duration
	RunCount       int64
	AvgDuration    time.Duration
	ViolationCount int64
	SuccessRate    float64
}

// NewLinterStats creates a new linter statistics tracker
func NewLinterStats() (*LinterStats, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "arch-unit-stats.db")
	db, err := NewDB("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open stats database: %w", err)
	}

	ls := &LinterStats{db: db}
	if err := ls.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return ls, nil
}

// initSchema creates the necessary tables
func (ls *LinterStats) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS linter_executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		linter_name TEXT NOT NULL,
		work_dir TEXT NOT NULL,
		executed_at DATETIME NOT NULL,
		duration_ms INTEGER NOT NULL,
		violation_count INTEGER NOT NULL,
		success BOOLEAN NOT NULL,
		UNIQUE(linter_name, work_dir, executed_at)
	);

	CREATE INDEX IF NOT EXISTS idx_linter_workdir ON linter_executions(linter_name, work_dir);
	CREATE INDEX IF NOT EXISTS idx_executed_at ON linter_executions(executed_at);

	-- Table for intelligent debounce metadata
	CREATE TABLE IF NOT EXISTS debounce_metadata (
		linter_name TEXT NOT NULL,
		work_dir TEXT NOT NULL,
		last_debounce_used_ms INTEGER,
		consecutive_no_violations INTEGER DEFAULT 0,
		consecutive_violations INTEGER DEFAULT 0,
		adaptation_factor REAL DEFAULT 1.0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY(linter_name, work_dir)
	);
	`

	if _, err := ls.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// RecordExecution records a linter execution
func (ls *LinterStats) RecordExecution(linterName, workDir string, duration time.Duration, violations int, success bool) error {
	tx, err := ls.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Record the execution
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO linter_executions 
		(linter_name, work_dir, executed_at, duration_ms, violation_count, success)
		VALUES (?, ?, ?, ?, ?, ?)`,
		linterName, workDir, time.Now(), duration.Milliseconds(), violations, success)
	if err != nil {
		return err
	}

	// Update debounce metadata
	err = ls.updateDebounceMetadata(tx, linterName, workDir, violations)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// updateDebounceMetadata updates the adaptation factors based on violation patterns
func (ls *LinterStats) updateDebounceMetadata(tx *Tx, linterName, workDir string, violations int) error {
	// Get current metadata
	var consecutiveNoViolations, consecutiveViolations int
	var adaptationFactor float64
	
	err := tx.QueryRow(`
		SELECT consecutive_no_violations, consecutive_violations, adaptation_factor
		FROM debounce_metadata 
		WHERE linter_name = ? AND work_dir = ?`,
		linterName, workDir).Scan(&consecutiveNoViolations, &consecutiveViolations, &adaptationFactor)
	
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// Update consecutive counters
	if violations == 0 {
		consecutiveNoViolations++
		consecutiveViolations = 0
	} else {
		consecutiveNoViolations = 0
		consecutiveViolations++
	}

	// Adjust adaptation factor based on patterns
	// More violations = reduce debounce (increase responsiveness)
	// Fewer violations = increase debounce (reduce overhead)
	if consecutiveNoViolations >= 5 {
		adaptationFactor = minFloat(2.0, adaptationFactor*1.1) // Increase debounce
	} else if consecutiveViolations >= 3 {
		adaptationFactor = maxFloat(0.5, adaptationFactor*0.9) // Decrease debounce
	}

	// Insert or update metadata
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO debounce_metadata 
		(linter_name, work_dir, consecutive_no_violations, consecutive_violations, adaptation_factor, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		linterName, workDir, consecutiveNoViolations, consecutiveViolations, adaptationFactor, time.Now())

	return err
}

// GetIntelligentDebounce calculates the optimal debounce duration for a linter
func (ls *LinterStats) GetIntelligentDebounce(linterName, workDir string) (time.Duration, error) {
	// Get recent execution statistics
	var lastDuration, avgDuration time.Duration
	var runCount int64
	
	err := ls.db.QueryRow(`
		SELECT 
			COALESCE(MAX(duration_ms), 0) as last_duration,
			COALESCE(AVG(duration_ms), 1000) as avg_duration,
			COUNT(*) as run_count
		FROM linter_executions 
		WHERE linter_name = ? AND work_dir = ? 
		AND executed_at > datetime('now', '-7 days')`,
		linterName, workDir).Scan(&lastDuration, &avgDuration, &runCount)
	
	if err != nil {
		// Default for new linters
		return 5 * time.Minute, nil
	}

	// lastDurationTime := time.Duration(lastDuration) * time.Millisecond // Currently unused but kept for future use
	avgDurationTime := time.Duration(avgDuration) * time.Millisecond

	// Get adaptation factor
	var adaptationFactor float64 = 1.0
	ls.db.QueryRow(`
		SELECT adaptation_factor 
		FROM debounce_metadata 
		WHERE linter_name = ? AND work_dir = ?`,
		linterName, workDir).Scan(&adaptationFactor)

	// Calculate base debounce using new thresholds
	// Use the average duration as the primary metric
	var baseDebounce time.Duration
	
	if runCount == 0 {
		// New linter - default to 5 minutes
		baseDebounce = 5 * time.Minute
	} else if avgDurationTime < 100*time.Millisecond {
		// < 100ms: no debounce
		baseDebounce = 0
	} else if avgDurationTime < 1*time.Second {
		// < 1s: 5 second debounce
		baseDebounce = 5 * time.Second
	} else if avgDurationTime < 30*time.Second {
		// < 30s: 5 minute debounce
		baseDebounce = 5 * time.Minute
	} else if avgDurationTime < 5*time.Minute {
		// 1-5 minutes: 1 hour debounce
		baseDebounce = 1 * time.Hour
	} else if avgDurationTime < 15*time.Minute {
		// 5-15 minutes: 3 hour debounce
		baseDebounce = 3 * time.Hour
	} else {
		// 15+ minutes: 8 hour debounce
		baseDebounce = 8 * time.Hour
	}
	
	// Apply adaptation factor to fine-tune based on violation patterns
	if baseDebounce > 0 {
		baseDebounce = time.Duration(float64(baseDebounce) * adaptationFactor)
	}
	
	// Ensure reasonable bounds even with adaptation
	// Don't let adaptation factor push debounce too extreme
	if baseDebounce > 24*time.Hour {
		baseDebounce = 24 * time.Hour
	}

	return baseDebounce, nil
}

// ShouldSkipLinter determines if a linter should be skipped due to debounce
func (ls *LinterStats) ShouldSkipLinter(linterName, workDir string, configDebounce string) (bool, time.Duration, error) {
	var effectiveDebounce time.Duration
	var err error

	// Priority: explicit config > intelligent debounce
	if configDebounce != "" && configDebounce != "auto" {
		effectiveDebounce, err = time.ParseDuration(configDebounce)
		if err != nil {
			return false, 0, fmt.Errorf("invalid debounce duration: %w", err)
		}
	} else {
		// Use intelligent debounce
		effectiveDebounce, err = ls.GetIntelligentDebounce(linterName, workDir)
		if err != nil {
			return false, 0, err
		}
	}

	// Get last execution time
	var lastRun time.Time
	err = ls.db.QueryRow(`
		SELECT MAX(executed_at) 
		FROM linter_executions 
		WHERE linter_name = ? AND work_dir = ?`,
		linterName, workDir).Scan(&lastRun)

	if err != nil || lastRun.IsZero() {
		// No previous runs - don't skip
		return false, effectiveDebounce, nil
	}

	elapsed := time.Since(lastRun)
	shouldSkip := elapsed < effectiveDebounce

	return shouldSkip, effectiveDebounce, nil
}

// GetStats returns comprehensive statistics for a linter
func (ls *LinterStats) GetStats(linterName, workDir string) (*ExecutionStats, error) {
	var stats ExecutionStats
	var lastRunStr string
	var lastDurationMs int64
	var avgDurationMs float64
	
	// First get the basic stats
	err := ls.db.QueryRow(`
		SELECT 
			COUNT(*) as run_count,
			COALESCE(AVG(duration_ms), 0) as avg_duration,
			COALESCE(SUM(violation_count), 0) as total_violations,
			COALESCE(AVG(CASE WHEN success THEN 1.0 ELSE 0.0 END), 0) as success_rate
		FROM linter_executions 
		WHERE linter_name = ? AND work_dir = ?`,
		linterName, workDir).Scan(
		&stats.RunCount, &avgDurationMs, &stats.ViolationCount, &stats.SuccessRate)

	if err != nil {
		return nil, err
	}

	// Get the last run info separately if there are any runs
	if stats.RunCount > 0 {
		err = ls.db.QueryRow(`
			SELECT executed_at, duration_ms
			FROM linter_executions 
			WHERE linter_name = ? AND work_dir = ? 
			ORDER BY executed_at DESC
			LIMIT 1`,
			linterName, workDir).Scan(&lastRunStr, &lastDurationMs)
		
		if err != nil {
			return nil, err
		}
		
		// Parse the time string
		stats.LastRun, err = time.Parse("2006-01-02 15:04:05", lastRunStr)
		if err != nil {
			// Try parsing as RFC3339
			stats.LastRun, err = time.Parse(time.RFC3339, lastRunStr)
			if err != nil {
				// Fallback to zero time
				stats.LastRun = time.Time{}
			}
		}
		
		stats.LastDuration = time.Duration(lastDurationMs) * time.Millisecond
	} else {
		stats.LastRun = time.Time{}
		stats.LastDuration = 0
	}

	stats.LinterName = linterName
	stats.WorkDir = workDir
	stats.AvgDuration = time.Duration(int64(avgDurationMs)) * time.Millisecond

	return &stats, nil
}

// GetLinterHistory returns all linters that have execution history
func (ls *LinterStats) GetLinterHistory(workDir string) ([]string, error) {
	rows, err := ls.db.Query(`
		SELECT DISTINCT linter_name 
		FROM linter_executions 
		WHERE work_dir = ? 
		ORDER BY linter_name`,
		workDir)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var linters []string
	for rows.Next() {
		var linter string
		if err := rows.Scan(&linter); err != nil {
			return nil, err
		}
		linters = append(linters, linter)
	}

	return linters, rows.Err()
}

// Close closes the database connection
func (ls *LinterStats) Close() error {
	if ls.db != nil {
		return ls.db.Close()
	}
	return nil
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}