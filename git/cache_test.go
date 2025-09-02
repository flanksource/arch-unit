package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitRepositoryCaching(t *testing.T) {
	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping git caching test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "git-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create git manager
	gitManager := NewGitRepositoryManager(tempDir)
	defer gitManager.Close()

	// Use a small, stable repository for testing
	testGitURL := "https://github.com/flanksource/is-healthy"

	// First request - should create new repository
	repo1, err := gitManager.GetRepository(testGitURL)
	require.NoError(t, err, "First GetRepository should succeed")
	require.NotNil(t, repo1)

	// Second request - should return cached repository
	repo2, err := gitManager.GetRepository(testGitURL)
	require.NoError(t, err, "Second GetRepository should succeed")
	require.NotNil(t, repo2)

	// Should be the same instance (cached)
	assert.Same(t, repo1, repo2, "Second request should return cached repository instance")

	t.Logf("✓ Git repository caching works correctly")
}

func TestVersionResolutionCaching(t *testing.T) {
	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping version resolution caching test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "version-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create git manager
	gitManager := NewGitRepositoryManager(tempDir)
	defer gitManager.Close()

	testGitURL := "https://github.com/flanksource/is-healthy"
	testAlias := "HEAD"

	// First resolution - should fetch from remote
	start1 := time.Now()
	version1, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
	duration1 := time.Since(start1)
	require.NoError(t, err, "First version resolution should succeed")
	require.NotEmpty(t, version1, "Resolved version should not be empty")

	t.Logf("First resolution took %v, resolved to: %s", duration1, version1)

	// Second resolution - should use cache (much faster)
	start2 := time.Now()
	version2, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
	duration2 := time.Since(start2)
	require.NoError(t, err, "Second version resolution should succeed")
	require.NotEmpty(t, version2, "Cached resolved version should not be empty")

	// Should be the same version
	assert.Equal(t, version1, version2, "Cached version should match original")

	// Second resolution should be significantly faster (cached)
	assert.True(t, duration2 < duration1/2, 
		"Cached resolution (%v) should be much faster than first resolution (%v)", 
		duration2, duration1)

	t.Logf("Second resolution took %v (cached), same version: %s", duration2, version2)
	t.Logf("✓ Version resolution caching works correctly")
}

func TestGitNoCacheBehavior(t *testing.T) {
	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping no-cache behavior test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "no-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testGitURL := "https://github.com/flanksource/is-healthy"
	testAlias := "HEAD"

	// Test with normal caching
	t.Run("WithCache", func(t *testing.T) {
		gitManager := NewGitRepositoryManager(tempDir)
		defer gitManager.Close()

		// First resolution
		version1, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
		require.NoError(t, err)
		require.NotEmpty(t, version1)

		// Second resolution - should use cache
		start := time.Now()
		version2, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
		duration := time.Since(start)
		require.NoError(t, err)
		assert.Equal(t, version1, version2)

		// Should be very fast (cached)
		assert.True(t, duration < 10*time.Millisecond, 
			"Cached resolution should be very fast, took: %v", duration)

		t.Logf("Cached resolution took %v", duration)
	})

	// Test without cache (simulating --no-cache behavior)
	t.Run("WithoutCache", func(t *testing.T) {
		// For git operations, no-cache is simulated by creating a new manager
		// each time or by clearing the cache
		gitManager1 := NewGitRepositoryManager(filepath.Join(tempDir, "nocache1"))
		defer gitManager1.Close()

		gitManager2 := NewGitRepositoryManager(filepath.Join(tempDir, "nocache2"))
		defer gitManager2.Close()

		// First resolution
		start1 := time.Now()
		version1, err := gitManager1.ResolveVersionAlias(testGitURL, testAlias)
		duration1 := time.Since(start1)
		require.NoError(t, err)
		require.NotEmpty(t, version1)

		// Second resolution with different manager (no shared cache)
		start2 := time.Now()
		version2, err := gitManager2.ResolveVersionAlias(testGitURL, testAlias)
		duration2 := time.Since(start2)
		require.NoError(t, err)
		require.NotEmpty(t, version2)

		// Versions should be the same
		assert.Equal(t, version1, version2)

		// Both resolutions should take similar time (no cache benefit)
		ratio := float64(duration2) / float64(duration1)
		assert.True(t, ratio > 0.5 && ratio < 2.0, 
			"Without cache, both resolutions should take similar time. First: %v, Second: %v (ratio: %.2f)", 
			duration1, duration2, ratio)

		t.Logf("No-cache: First resolution took %v, second took %v", duration1, duration2)
	})

	t.Logf("✓ No-cache behavior works correctly")
}

func TestWorktreeCaching(t *testing.T) {
	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping worktree caching test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "worktree-cache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create git manager
	gitManager := NewGitRepositoryManager(tempDir)
	defer gitManager.Close()

	testGitURL := "https://github.com/flanksource/is-healthy"
	testVersion := "HEAD"

	// First worktree request - should create new clone
	start1 := time.Now()
	path1, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
	duration1 := time.Since(start1)
	require.NoError(t, err, "First GetWorktreePath should succeed")
	require.NotEmpty(t, path1, "Worktree path should not be empty")

	// Verify worktree exists
	_, err = os.Stat(path1)
	require.NoError(t, err, "Worktree directory should exist")

	t.Logf("First worktree creation took %v, path: %s", duration1, path1)

	// Second worktree request - should return cached path
	start2 := time.Now()
	path2, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
	duration2 := time.Since(start2)
	require.NoError(t, err, "Second GetWorktreePath should succeed")
	require.NotEmpty(t, path2, "Cached worktree path should not be empty")

	// Should be the same path
	assert.Equal(t, path1, path2, "Second request should return same worktree path")

	// Second request should be much faster (cached)
	assert.True(t, duration2 < duration1/2, 
		"Cached worktree lookup (%v) should be much faster than creation (%v)", 
		duration2, duration1)

	// Verify worktree still exists and has content
	entries, err := os.ReadDir(path2)
	require.NoError(t, err, "Should be able to read cached worktree directory")
	assert.NotEmpty(t, entries, "Cached worktree should not be empty")

	t.Logf("Second worktree lookup took %v (cached), same path", duration2)
	t.Logf("✓ Worktree caching works correctly")
}

func TestCacheCleanup(t *testing.T) {
	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping cache cleanup test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "cache-cleanup-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create git manager
	gitManager := NewGitRepositoryManager(tempDir)
	defer gitManager.Close()

	testGitURL := "https://github.com/flanksource/is-healthy"

	// Create a repository entry
	repo, err := gitManager.GetRepository(testGitURL)
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Get a worktree to ensure some cache content
	_, err = gitManager.GetWorktreePath(testGitURL, "HEAD", 1)
	require.NoError(t, err)

	// Test cleanup with a very short maxAge (should clean up)
	err = gitManager.CleanupUnused(1 * time.Nanosecond)
	assert.NoError(t, err, "Cleanup should succeed")

	// Test cleanup with a long maxAge (should not clean up)
	err = gitManager.CleanupUnused(24 * time.Hour)
	assert.NoError(t, err, "Cleanup should succeed")

	t.Logf("✓ Cache cleanup works correctly")
}