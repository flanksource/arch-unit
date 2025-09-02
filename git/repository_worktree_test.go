package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeContainsData(t *testing.T) {
	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping worktree test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "worktree-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Logf("Using temp cache directory: %s", tempDir)

	// Create git manager
	gitManager := NewGitRepositoryManager(tempDir)
	defer gitManager.Close()

	// Use a small, stable repository for testing
	testGitURL := "https://github.com/flanksource/mission-control-chart"
	testVersion := "HEAD"

	t.Logf("Testing worktree creation for %s@%s", testGitURL, testVersion)

	// Get worktree path - this should create the clone
	worktreePath, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
	require.NoError(t, err, "GetWorktreePath should succeed")
	require.NotEmpty(t, worktreePath, "Worktree path should not be empty")

	t.Logf("Expected worktree path: %s", worktreePath)

	// Check if worktree exists at expected location
	expectedExists := false
	if _, err := os.Stat(worktreePath); err == nil {
		expectedExists = true
		t.Logf("✓ Worktree exists at expected location")
	}

	// Check for nested cache directory structure (the bug we discovered)
	nestedPath := filepath.Join(tempDir, "github.com", "flanksource", "mission-control-chart", 
		tempDir, "github.com", "flanksource", "mission-control-chart", "worktrees", testVersion)
	nestedExists := false
	if _, err := os.Stat(nestedPath); err == nil {
		nestedExists = true
		t.Logf("⚠ Worktree found at nested location: %s", nestedPath)
	}

	// At least one should exist
	require.True(t, expectedExists || nestedExists, 
		"Worktree should exist either at expected location or nested location")

	// Use the actual worktree path
	actualWorktreePath := worktreePath
	if !expectedExists && nestedExists {
		actualWorktreePath = nestedPath
		t.Errorf("NESTED CACHE BUG DETECTED: Worktree created at nested path instead of expected path")
		t.Errorf("Expected: %s", worktreePath)
		t.Errorf("Actual: %s", nestedPath)
	}

	// Verify worktree contains actual repository data
	t.Logf("Verifying worktree contains data at: %s", actualWorktreePath)

	// Check that worktree directory exists and is not empty
	entries, err := os.ReadDir(actualWorktreePath)
	require.NoError(t, err, "Should be able to read worktree directory")
	require.NotEmpty(t, entries, "Worktree should not be empty")

	t.Logf("Worktree contains %d entries", len(entries))

	// Verify essential files exist
	expectedFiles := []string{
		".git",        // Git metadata
		"README.md",   // Should have a README
		"chart",       // Should have the chart directory (this is mission-control-chart repo)
	}

	for _, expectedFile := range expectedFiles {
		filePath := filepath.Join(actualWorktreePath, expectedFile)
		_, err := os.Stat(filePath)
		assert.NoError(t, err, "Expected file/directory should exist: %s", expectedFile)
		if err == nil {
			t.Logf("✓ Found expected file: %s", expectedFile)
		}
	}

	// Verify at least one file has actual content (not empty)
	readmePath := filepath.Join(actualWorktreePath, "README.md")
	if content, err := os.ReadFile(readmePath); err == nil {
		assert.Greater(t, len(content), 0, "README.md should have content")
		assert.True(t, len(strings.TrimSpace(string(content))) > 10, 
			"README.md should have meaningful content")
		t.Logf("✓ README.md has %d bytes of content", len(content))
	}

	// Check chart directory has content too
	chartPath := filepath.Join(actualWorktreePath, "chart")
	if chartEntries, err := os.ReadDir(chartPath); err == nil {
		assert.NotEmpty(t, chartEntries, "chart directory should not be empty")
		t.Logf("✓ Chart directory has %d entries", len(chartEntries))
		
		// Look for Chart.yaml specifically
		chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
		if _, err := os.Stat(chartYamlPath); err == nil {
			t.Logf("✓ Found Chart.yaml in chart directory")
		}
	}

	t.Logf("✓ Worktree validation completed successfully")
}

func TestWorktreePathIsCorrect(t *testing.T) {
	// Test that worktree paths don't have nested cache directory structure

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "worktree-path-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create git manager
	gitManager := NewGitRepositoryManager(tempDir)
	defer gitManager.Close()

	testGitURL := "https://github.com/flanksource/mission-control-chart"
	testVersion := "HEAD"

	// Get worktree path (don't actually create it, just get the path)
	worktreePath, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
	require.NoError(t, err)

	t.Logf("Worktree path: %s", worktreePath)
	t.Logf("Temp dir: %s", tempDir)

	// The worktree path should:
	// 1. Start with the temp directory
	// 2. Not contain the temp directory path twice (nested)
	
	// Check it starts with temp dir
	assert.True(t, strings.HasPrefix(worktreePath, tempDir), 
		"Worktree path should start with cache directory")

	// Check for nested cache directory (the bug)
	// If the path contains the temp directory twice, it's the nested bug
	relativePath := strings.TrimPrefix(worktreePath, tempDir)
	hasNestedCache := strings.Contains(relativePath, tempDir)
	
	if hasNestedCache {
		t.Errorf("NESTED CACHE BUG: Worktree path contains nested cache directory")
		t.Errorf("Path: %s", worktreePath)
		t.Errorf("Temp dir appears multiple times in path")
	} else {
		t.Logf("✓ Worktree path is correctly structured (no nested cache)")
	}

	// Expected pattern: tempDir/github.com/org/repo/clones/version-depth1
	expectedPattern := filepath.Join(tempDir, "github.com", "flanksource", "mission-control-chart", "clones", testVersion+"-depth1")
	assert.Equal(t, expectedPattern, worktreePath, 
		"Worktree path should follow expected pattern")
}