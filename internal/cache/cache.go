package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type CacheEntry struct {
	Path      string    `json:"path"`
	LastRun   time.Time `json:"last_run"`
	Directory bool      `json:"directory"`
}

type Cache struct {
	baseDir string
}

func NewCache() (*Cache, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "arch-unit")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Cache{
		baseDir: cacheDir,
	}, nil
}

func (c *Cache) getCacheFilePath(path string) string {
	// Create a hash of the path to use as filename
	hash := sha256.Sum256([]byte(path))
	filename := hex.EncodeToString(hash[:])
	return filepath.Join(c.baseDir, filename+".json")
}

func (c *Cache) GetLastRun(path string) (*CacheEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	cacheFile := c.getCacheFilePath(absPath)
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		return nil, nil // No cache entry exists
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache entry: %w", err)
	}

	return &entry, nil
}

func (c *Cache) SetLastRun(path string, runTime time.Time) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if path is a directory, default to true if path doesn't exist
	isDirectory := true
	if stat, err := os.Stat(absPath); err == nil {
		isDirectory = stat.IsDir()
	}

	entry := CacheEntry{
		Path:      absPath,
		LastRun:   runTime,
		Directory: isDirectory,
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	cacheFile := c.getCacheFilePath(absPath)
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// ShouldSkip checks if analysis should be skipped based on debounce duration
func (c *Cache) ShouldSkip(path string, debounceDuration time.Duration) (bool, error) {
	if debounceDuration == 0 {
		return false, nil
	}

	entry, err := c.GetLastRun(path)
	if err != nil {
		return false, err
	}

	if entry == nil {
		return false, nil // No previous run, don't skip
	}

	timeSinceLastRun := time.Since(entry.LastRun)
	return timeSinceLastRun < debounceDuration, nil
}

// RecordRun records the current time as the last run time for the given path
func (c *Cache) RecordRun(path string) error {
	return c.SetLastRun(path, time.Now())
}

// CleanOldEntries removes cache entries older than the specified duration
func (c *Cache) CleanOldEntries(maxAge time.Duration) error {
	entries, err := os.ReadDir(c.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		cachePath := filepath.Join(c.baseDir, entry.Name())

		// Read the cache entry to check its LastRun time
		data, err := os.ReadFile(cachePath)
		if err != nil {
			continue
		}

		var cacheEntry CacheEntry
		if err := json.Unmarshal(data, &cacheEntry); err != nil {
			continue
		}

		if cacheEntry.LastRun.Before(cutoff) {
			os.Remove(cachePath) // Ignore errors
		}
	}

	return nil
}
