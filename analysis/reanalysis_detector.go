package analysis

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ReanalysisDetector determines when sources need to be reanalyzed
type ReanalysisDetector struct {
	httpClient      *http.Client
	hashCache       map[string]string
	timestampCache  map[string]time.Time
	mutex           sync.RWMutex
}

// NewReanalysisDetector creates a new reanalysis detector
func NewReanalysisDetector() *ReanalysisDetector {
	return &ReanalysisDetector{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		hashCache:      make(map[string]string),
		timestampCache: make(map[string]time.Time),
	}
}

// NeedsReanalysis determines if a file needs to be reanalyzed based on path and optional URL
func (r *ReanalysisDetector) NeedsReanalysis(filePath, url string) (bool, error) {
	if url != "" {
		return r.checkURLChange(url)
	}

	if filePath != "" {
		return r.checkFileChange(filePath)
	}

	// If neither file nor URL provided, assume needs analysis
	return true, nil
}

// NeedsReanalysisForConnection determines if a database connection needs reanalysis
func (r *ReanalysisDetector) NeedsReanalysisForConnection(connectionString string) (bool, error) {
	// For database connections, we'll use a simple time-based approach
	// In a real implementation, this would check actual schema changes
	r.mutex.RLock()
	lastCheck, exists := r.timestampCache[connectionString]
	r.mutex.RUnlock()

	if !exists {
		// First time checking this connection
		return true, nil
	}

	// Consider schema stale after 1 hour (in practice, this would be configurable)
	return time.Since(lastCheck) > time.Hour, nil
}

// checkFileChange checks if a file has changed since last analysis
func (r *ReanalysisDetector) checkFileChange(filePath string) (bool, error) {
	// Get absolute path for consistent caching
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return true, fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
	}

	// Check if file exists
	_, err = os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, remove from cache if present
			r.mutex.Lock()
			delete(r.hashCache, absPath)
			delete(r.timestampCache, absPath)
			r.mutex.Unlock()
			return false, fmt.Errorf("file does not exist: %s", absPath)
		}
		return true, fmt.Errorf("failed to stat file %s: %w", absPath, err)
	}

	// Calculate current file hash
	currentHash, err := r.calculateFileHash(absPath)
	if err != nil {
		// If we can't calculate hash, assume file changed
		return true, nil
	}

	// Check cached hash
	r.mutex.RLock()
	cachedHash, exists := r.hashCache[absPath]
	r.mutex.RUnlock()

	if !exists {
		// First time analyzing this file
		return true, nil
	}

	// Compare hashes and modification times
	return currentHash != cachedHash, nil
}

// checkURLChange checks if content at a URL has changed
func (r *ReanalysisDetector) checkURLChange(url string) (bool, error) {
	// Calculate current content hash from URL
	currentHash, err := r.calculateURLContentHash(url)
	if err != nil {
		// If we can't fetch URL, assume it changed
		return true, err
	}

	// Check cached hash
	r.mutex.RLock()
	cachedHash, exists := r.hashCache[url]
	r.mutex.RUnlock()

	if !exists {
		// First time analyzing this URL
		return true, nil
	}

	// Compare hashes
	return currentHash != cachedHash, nil
}

// RecordAnalysis records that analysis was performed for a file or URL
func (r *ReanalysisDetector) RecordAnalysis(filePath, url string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()

	if url != "" {
		// Record URL analysis
		hash, err := r.calculateURLContentHash(url)
		if err != nil {
			return fmt.Errorf("failed to calculate URL hash: %w", err)
		}
		r.hashCache[url] = hash
		r.timestampCache[url] = now
		return nil
	}

	if filePath != "" {
		// Record file analysis
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}

		hash, err := r.calculateFileHash(absPath)
		if err != nil {
			return fmt.Errorf("failed to calculate file hash: %w", err)
		}

		r.hashCache[absPath] = hash
		r.timestampCache[absPath] = now
		return nil
	}

	return fmt.Errorf("either filePath or url must be provided")
}

// RecordConnectionAnalysis records that analysis was performed for a database connection
func (r *ReanalysisDetector) RecordConnectionAnalysis(connectionString, schemaHash string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.hashCache[connectionString] = schemaHash
	r.timestampCache[connectionString] = time.Now()
	return nil
}

// ClearAnalysisRecord clears the cached analysis record for a source
func (r *ReanalysisDetector) ClearAnalysisRecord(filePath, url string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if url != "" {
		delete(r.hashCache, url)
		delete(r.timestampCache, url)
		return nil
	}

	if filePath != "" {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		delete(r.hashCache, absPath)
		delete(r.timestampCache, absPath)
		return nil
	}

	return fmt.Errorf("either filePath or url must be provided")
}

// ClearAllRecords clears all cached analysis records
func (r *ReanalysisDetector) ClearAllRecords() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.hashCache = make(map[string]string)
	r.timestampCache = make(map[string]time.Time)
	return nil
}

// CalculateContentHash calculates MD5 hash of content
func (r *ReanalysisDetector) CalculateContentHash(content []byte) string {
	hash := md5.Sum(content)
	return fmt.Sprintf("%x", hash)
}

// calculateFileHash calculates MD5 hash of a file
func (r *ReanalysisDetector) calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// calculateURLContentHash calculates hash of content from a URL
func (r *ReanalysisDetector) calculateURLContentHash(url string) (string, error) {
	// Make HTTP request
	resp, err := r.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error %d for URL %s", resp.StatusCode, url)
	}

	// Calculate hash of response body
	hash := md5.New()
	if _, err := io.Copy(hash, resp.Body); err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}