package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCache_GetSetLastRun(t *testing.T) {
	// Use a temporary directory for testing
	tempDir := t.TempDir()
	cache := &Cache{baseDir: tempDir}

	testPath := "/tmp/test-project"
	testTime := time.Now().Truncate(time.Second)

	// Test setting last run time
	err := cache.SetLastRun(testPath, testTime)
	if err != nil {
		t.Fatalf("Failed to set last run time: %v", err)
	}

	// Test getting last run time
	entry, err := cache.GetLastRun(testPath)
	if err != nil {
		t.Fatalf("Failed to get last run time: %v", err)
	}

	if entry == nil {
		t.Fatal("Expected cache entry but got nil")
	}

	if !entry.LastRun.Equal(testTime) {
		t.Errorf("Expected last run time %v, got %v", testTime, entry.LastRun)
	}

	if entry.Path != testPath {
		t.Errorf("Expected path %s, got %s", testPath, entry.Path)
	}
}

func TestCache_ShouldSkip(t *testing.T) {
	tempDir := t.TempDir()
	cache := &Cache{baseDir: tempDir}

	testPath := "/tmp/test-project"

	tests := []struct {
		name         string
		setupLastRun *time.Time
		debounceDur  time.Duration
		expectedSkip bool
	}{
		{
			name:         "no_previous_run",
			setupLastRun: nil,
			debounceDur:  30 * time.Second,
			expectedSkip: false,
		},
		{
			name:         "zero_debounce_duration",
			setupLastRun: timePtr(time.Now().Add(-10 * time.Second)),
			debounceDur:  0,
			expectedSkip: false,
		},
		{
			name:         "within_debounce_period",
			setupLastRun: timePtr(time.Now().Add(-10 * time.Second)),
			debounceDur:  30 * time.Second,
			expectedSkip: true,
		},
		{
			name:         "outside_debounce_period",
			setupLastRun: timePtr(time.Now().Add(-60 * time.Second)),
			debounceDur:  30 * time.Second,
			expectedSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupLastRun != nil {
				err := cache.SetLastRun(testPath, *tt.setupLastRun)
				if err != nil {
					t.Fatalf("Failed to setup last run time: %v", err)
				}
			} else {
				// Clean up any existing cache entry
				cacheFile := cache.getCacheFilePath(testPath)
				os.Remove(cacheFile)
			}

			// Test
			shouldSkip, err := cache.ShouldSkip(testPath, tt.debounceDur)
			if err != nil {
				t.Fatalf("ShouldSkip returned error: %v", err)
			}

			if shouldSkip != tt.expectedSkip {
				t.Errorf("Expected shouldSkip=%v, got %v", tt.expectedSkip, shouldSkip)
			}
		})
	}
}

func TestCache_RecordRun(t *testing.T) {
	tempDir := t.TempDir()
	cache := &Cache{baseDir: tempDir}

	testPath := "/tmp/test-project"
	beforeRun := time.Now()

	err := cache.RecordRun(testPath)
	if err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	afterRun := time.Now()

	// Verify the run was recorded
	entry, err := cache.GetLastRun(testPath)
	if err != nil {
		t.Fatalf("Failed to get recorded run: %v", err)
	}

	if entry == nil {
		t.Fatal("Expected cache entry but got nil")
	}

	// Check that the recorded time is approximately now
	if entry.LastRun.Before(beforeRun) || entry.LastRun.After(afterRun) {
		t.Errorf("Recorded run time %v is not between %v and %v", entry.LastRun, beforeRun, afterRun)
	}
}

func TestCache_CleanOldEntries(t *testing.T) {
	tempDir := t.TempDir()
	cache := &Cache{baseDir: tempDir}

	// Create some cache entries with different ages
	oldTime := time.Now().Add(-2 * time.Hour)
	recentTime := time.Now().Add(-10 * time.Minute)

	err := cache.SetLastRun("/tmp/old-project", oldTime)
	if err != nil {
		t.Fatalf("Failed to create old cache entry: %v", err)
	}

	err = cache.SetLastRun("/tmp/recent-project", recentTime)
	if err != nil {
		t.Fatalf("Failed to create recent cache entry: %v", err)
	}

	// Clean entries older than 1 hour
	err = cache.CleanOldEntries(1 * time.Hour)
	if err != nil {
		t.Fatalf("Failed to clean old entries: %v", err)
	}

	// Check that old entry is gone
	oldEntry, err := cache.GetLastRun("/tmp/old-project")
	if err != nil {
		t.Fatalf("Failed to get old entry: %v", err)
	}
	if oldEntry != nil {
		t.Error("Expected old entry to be cleaned but it still exists")
	}

	// Check that recent entry still exists
	recentEntry, err := cache.GetLastRun("/tmp/recent-project")
	if err != nil {
		t.Fatalf("Failed to get recent entry: %v", err)
	}
	if recentEntry == nil {
		t.Error("Expected recent entry to remain but it was cleaned")
	}
}

func TestNewCache(t *testing.T) {
	cache, err := NewCache()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	if cache.baseDir == "" {
		t.Error("Expected non-empty base directory")
	}

	// Check that the cache directory was created
	if _, err := os.Stat(cache.baseDir); os.IsNotExist(err) {
		t.Errorf("Cache directory %s was not created", cache.baseDir)
	}

	// Verify it's in the expected location
	homeDir, _ := os.UserHomeDir()
	expectedDir := filepath.Join(homeDir, ".cache", "arch-unit")
	if cache.baseDir != expectedDir {
		t.Errorf("Expected cache directory %s, got %s", expectedDir, cache.baseDir)
	}
}

// Helper function to create time pointers
func timePtr(t time.Time) *time.Time {
	return &t
}
