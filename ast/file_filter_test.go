package ast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldIncludeFile(t *testing.T) {
	workDir := "/project"

	tests := []struct {
		name            string
		filePath        string
		includePatterns []string
		excludePatterns []string
		expected        bool
	}{
		{
			name:            "No patterns - include all",
			filePath:        "/project/src/main.go",
			includePatterns: []string{},
			excludePatterns: []string{},
			expected:        true,
		},
		{
			name:            "Include pattern matches",
			filePath:        "/project/src/main.go",
			includePatterns: []string{"*.go"},
			excludePatterns: []string{},
			expected:        true,
		},
		{
			name:            "Include pattern doesn't match",
			filePath:        "/project/src/main.py",
			includePatterns: []string{"*.go"},
			excludePatterns: []string{},
			expected:        false,
		},
		{
			name:            "Multiple include patterns - one matches",
			filePath:        "/project/src/main.py",
			includePatterns: []string{"*.go", "*.py"},
			excludePatterns: []string{},
			expected:        true,
		},
		{
			name:            "Exclude pattern matches",
			filePath:        "/project/src/main_test.go",
			includePatterns: []string{},
			excludePatterns: []string{"*_test.go"},
			expected:        false,
		},
		{
			name:            "Include matches but exclude also matches",
			filePath:        "/project/src/main_test.go",
			includePatterns: []string{"*.go"},
			excludePatterns: []string{"*_test.go"},
			expected:        false,
		},
		{
			name:            "Directory pattern with doublestar",
			filePath:        "/project/vendor/lib/main.go",
			includePatterns: []string{},
			excludePatterns: []string{"vendor/**"},
			expected:        false,
		},
		{
			name:            "Nested directory inclusion",
			filePath:        "/project/src/internal/service/user.go",
			includePatterns: []string{"src/**/*.go"},
			excludePatterns: []string{},
			expected:        true,
		},
		{
			name:            "Complex filtering",
			filePath:        "/project/src/handlers/user_handler.go",
			includePatterns: []string{"src/**/*.go"},
			excludePatterns: []string{"**/*_test.go", "**/vendor/**"},
			expected:        true,
		},
		{
			name:            "Complex filtering - excluded",
			filePath:        "/project/src/handlers/user_handler_test.go",
			includePatterns: []string{"src/**/*.go"},
			excludePatterns: []string{"**/*_test.go", "**/vendor/**"},
			expected:        false,
		},
		{
			name:            "Basename matching for includes",
			filePath:        "/project/deep/nested/main.go",
			includePatterns: []string{"main.go"},
			excludePatterns: []string{},
			expected:        true,
		},
		{
			name:            "Basename matching for excludes",
			filePath:        "/project/deep/nested/config.json",
			includePatterns: []string{},
			excludePatterns: []string{"config.json"},
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldIncludeFile(tt.filePath, workDir, tt.includePatterns, tt.excludePatterns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnalyzeFilesWithFilter(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"main.go",
		"main_test.go",
		"service.py",
		"config.json",
		"src/handler.go",
		"src/handler_test.go",
		"test/utils.go",
		"vendor/lib.go",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		dir := filepath.Dir(fullPath)

		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		// Create file with minimal Go/Python content
		var content string
		if filepath.Ext(file) == ".go" {
			content = "package main\n\nfunc main() {}\n"
		} else if filepath.Ext(file) == ".py" {
			content = "def main():\n    pass\n"
		} else {
			content = "{}\n"
		}

		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create AST cache
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	analyzer := NewAnalyzer(astCache, tempDir)

	tests := []struct {
		name            string
		includePatterns []string
		excludePatterns []string
		description     string
	}{
		{
			name:            "Include only Go files",
			includePatterns: []string{"*.go"},
			excludePatterns: []string{},
			description:     "Should analyze only Go files",
		},
		{
			name:            "Exclude test files",
			includePatterns: []string{},
			excludePatterns: []string{"*_test.go"},
			description:     "Should exclude test files",
		},
		{
			name:            "Include Go, exclude tests",
			includePatterns: []string{"*.go"},
			excludePatterns: []string{"*_test.go"},
			description:     "Should include Go files but exclude tests",
		},
		{
			name:            "Exclude vendor directory",
			includePatterns: []string{},
			excludePatterns: []string{"vendor/**"},
			description:     "Should exclude vendor directory",
		},
		{
			name:            "Include src directory only",
			includePatterns: []string{"src/**/*.go"},
			excludePatterns: []string{},
			description:     "Should only include Go files in src directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache before each test
			astCache.ClearCache()

			err := analyzer.AnalyzeFilesWithFilter(tt.includePatterns, tt.excludePatterns)

			// Should not error (files might not be valid Go/Python but that's ok for this test)
			// We're mainly testing the filtering logic
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestFileFilteringIntegration(t *testing.T) {
	// Create a temporary Go project structure
	tempDir := t.TempDir()

	// Create a realistic Go project structure
	structure := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello World")
}`,
		"main_test.go": `package main

import "testing"

func TestMain(t *testing.T) {
	// test code
}`,
		"internal/service/user.go": `package service

type UserService struct {}

func (s *UserService) GetUser(id string) error {
	return nil
}`,
		"internal/service/user_test.go": `package service

import "testing"

func TestUserService_GetUser(t *testing.T) {
	// test code
}`,
		"vendor/external/lib.go": `package external

func ExternalFunc() {}`,
	}

	for filePath, content := range structure {
		fullPath := filepath.Join(tempDir, filePath)
		dir := filepath.Dir(fullPath)

		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create AST cache
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	analyzer := NewAnalyzer(astCache, tempDir)

	// Test 1: Include only internal directory Go files, exclude tests
	t.Run("Filter internal Go files excluding tests", func(t *testing.T) {
		astCache.ClearCache()

		err := analyzer.AnalyzeFilesWithFilter(
			[]string{"internal/**/*.go"},
			[]string{"*_test.go"},
		)
		require.NoError(t, err)

		// Query for all nodes to see what was analyzed
		nodes, err := analyzer.QueryPattern("*")
		require.NoError(t, err)

		// Should find nodes from internal/service/user.go but not from test files
		foundMainUserService := false
		foundTestCode := false

		for _, node := range nodes {
			if node.FilePath == filepath.Join(tempDir, "internal/service/user.go") {
				foundMainUserService = true
			}
			if node.FilePath == filepath.Join(tempDir, "internal/service/user_test.go") ||
				node.FilePath == filepath.Join(tempDir, "main_test.go") {
				foundTestCode = true
			}
		}

		assert.True(t, foundMainUserService, "Should find code from internal/service/user.go")
		assert.False(t, foundTestCode, "Should not find any test code")
	})
}

// Test for the bug fix: queries with filtering should work correctly
func TestQueryWithFiltering(t *testing.T) {
	// Create a temporary directory with mixed file types
	tempDir := t.TempDir()

	// Create test files
	structure := map[string]string{
		"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello World")
	// This function has more than 3 lines
	// Adding more lines to test filtering
	var x int = 1
	var y int = 2
	fmt.Printf("Sum: %d", x+y)
}`,
		"README.md": `# Test Project

This is a test project with more than 3 lines of content.
It should be filtered out when using --exclude "*.md".
This markdown file has enough lines to match a lines(*) > 3 query.

## Section
More content here to ensure it has enough lines.
Even more content.
And more.
Final line.`,
		"service.py": `def main():
    print("Hello from Python")
    # This has more than 3 lines
    x = 1
    y = 2
    print(f"Sum: {x+y}")`,
	}

	for filePath, content := range structure {
		fullPath := filepath.Join(tempDir, filePath)
		dir := filepath.Dir(fullPath)

		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create AST cache
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	analyzer := NewAnalyzer(astCache, tempDir)

	// First, analyze all files normally to populate cache
	err = analyzer.AnalyzeFiles()
	require.NoError(t, err)

	// Verify all files are in cache (should include README.md)
	allNodes, err := analyzer.QueryPattern("*")
	require.NoError(t, err)

	hasMarkdownNodes := false
	for _, node := range allNodes {
		if strings.HasSuffix(node.FilePath, ".md") {
			hasMarkdownNodes = true
			break
		}
	}
	assert.True(t, hasMarkdownNodes, "Should have markdown nodes in full analysis")

	// Now clear cache and analyze with filtering
	err = astCache.ClearCache()
	require.NoError(t, err)

	err = analyzer.AnalyzeFilesWithFilter([]string{}, []string{"*.md"})
	require.NoError(t, err)

	// Query again - should not have markdown files
	filteredNodes, err := analyzer.QueryPattern("*")
	require.NoError(t, err)

	hasMarkdownNodesAfterFilter := false
	for _, node := range filteredNodes {
		if strings.HasSuffix(node.FilePath, ".md") {
			hasMarkdownNodesAfterFilter = true
			break
		}
	}
	assert.False(t, hasMarkdownNodesAfterFilter, "Should not have markdown nodes after filtering")

	// Test metric query with filtering
	err = astCache.ClearCache()
	require.NoError(t, err)

	err = analyzer.AnalyzeFilesWithFilter([]string{}, []string{"*.md"})
	require.NoError(t, err)

	// Execute a metric query that would match markdown files if they were included
	metricNodes, err := analyzer.ExecuteAQLQuery("lines(*) > 3")
	require.NoError(t, err)

	hasMarkdownInMetricQuery := false
	for _, node := range metricNodes {
		if strings.HasSuffix(node.FilePath, ".md") {
			hasMarkdownInMetricQuery = true
			break
		}
	}
	assert.False(t, hasMarkdownInMetricQuery, "Metric query should not return markdown files when excluded")
}
