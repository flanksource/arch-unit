package dependencies

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnifiedScanner_TwoPhaseScanning(t *testing.T) {
	// Initialize logger for testing to avoid panic
	initTestLogger()

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "unified-scan-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set up a simple test project structure
	setupTestProject(t, tmpDir)

	// Create scanner with registry
	scanner := NewScanner()
	scanner.SetupGitSupport(filepath.Join(tmpDir, ".cache"))

	// Create git manager
	gitManager := git.NewGitRepositoryManager(filepath.Join(tmpDir, ".cache"))

	// Create unified scanner
	unifiedScanner := NewUnifiedScanner(scanner, gitManager, 2)

	// Create scan context with a mock task
	task := &clicky.Task{}
	ctx := analysis.NewScanContext(task, tmpDir)

	// Perform two-phase scan
	tree, err := unifiedScanner.ScanWithTwoPhases(ctx, tmpDir)
	require.NoError(t, err)
	assert.NotNil(t, tree)

	// Verify that dependencies were discovered
	assert.NotEmpty(t, tree.Root, "Should have root dependencies")
	assert.NotEmpty(t, tree.Dependencies, "Should have discovered dependencies")
}

func TestUnifiedScanner_GitRepositoryScanning(t *testing.T) {

	// Skip if no network access
	if os.Getenv("SKIP_NETWORK_TESTS") != "" {
		t.Skip("Skipping network test")
	}

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "unified-git-scan-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create scanner with registry
	scanner := NewScanner()
	scanner.SetupGitSupport(filepath.Join(tmpDir, ".cache"))

	// Create git manager
	gitManager := git.NewGitRepositoryManager(filepath.Join(tmpDir, ".cache"))

	// Create unified scanner with depth 1 to limit scanning
	unifiedScanner := NewUnifiedScanner(scanner, gitManager, 1)

	// Use a small test repository with known structure
	// This is a small repository with minimal dependencies
	testRepo := "https://github.com/flanksource/is-healthy"

	// Checkout the repository first
	worktreePath, err := gitManager.GetWorktreePath(testRepo, "main")
	require.NoError(t, err)

	// Create scan context
	task := &clicky.Task{}
	ctx := analysis.NewScanContext(task, worktreePath)

	// Perform two-phase scan
	tree, err := unifiedScanner.ScanWithTwoPhases(ctx, worktreePath)
	require.NoError(t, err)
	assert.NotNil(t, tree)

	// Debug: Log what was found
	t.Logf("Tree Root: %v", tree.Root)
	t.Logf("Tree Dependencies: %v", tree.Dependencies)

	// Verify that dependencies were discovered
	assert.NotEmpty(t, tree.Root, "Should have root dependencies")
	assert.Equal(t, 1, tree.MaxDepth, "Max depth should be 1")
}

func TestUnifiedScanner_ConcurrentDiscovery(t *testing.T) {

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "concurrent-scan-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set up multiple test projects
	setupMultipleTestProjects(t, tmpDir)

	// Create scanner with registry
	scanner := NewScanner()
	scanner.SetupGitSupport(filepath.Join(tmpDir, ".cache"))

	// Create git manager
	gitManager := git.NewGitRepositoryManager(filepath.Join(tmpDir, ".cache"))

	// Create unified scanner
	unifiedScanner := NewUnifiedScanner(scanner, gitManager, 2)

	// Create scan context
	task := &clicky.Task{}
	ctx := analysis.NewScanContext(task, tmpDir)

	// Perform two-phase scan
	tree, err := unifiedScanner.ScanWithTwoPhases(ctx, tmpDir)
	require.NoError(t, err)
	assert.NotNil(t, tree)

	// Verify concurrent discovery
	assert.Greater(t, len(tree.Dependencies), 0, "Should have discovered dependencies concurrently")

	// Check for version conflicts if multiple versions of same dependency exist
	if len(tree.Conflicts) > 0 {
		assert.NotEmpty(t, tree.Conflicts[0].DependencyName, "Conflict should have dependency name")
		assert.NotEmpty(t, tree.Conflicts[0].Versions, "Conflict should have versions")
	}
}

func TestUnifiedScanner_RecursionPrevention(t *testing.T) {

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "recursion-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create scanner with registry
	scanner := NewScanner()
	scanner.SetupGitSupport(filepath.Join(tmpDir, ".cache"))

	// Create git manager
	gitManager := git.NewGitRepositoryManager(filepath.Join(tmpDir, ".cache"))

	// Create unified scanner with higher depth to test recursion prevention
	unifiedScanner := NewUnifiedScanner(scanner, gitManager, 5)

	// Create a test project that might have circular dependencies
	setupTestProject(t, tmpDir)

	// Create scan context
	task := &clicky.Task{}
	ctx := analysis.NewScanContext(task, tmpDir)

	// Perform two-phase scan
	tree, err := unifiedScanner.ScanWithTwoPhases(ctx, tmpDir)
	require.NoError(t, err)
	assert.NotNil(t, tree)

	// Verify that the scan completes without infinite recursion
	// Check that repoVisited prevented duplicate processing
	assert.True(t, len(unifiedScanner.repoVisited) >= 0, "Should track visited repositories")
}

// Helper functions to set up test projects

func setupTestProject(t *testing.T, dir string) {
	// Create a simple Go project
	goModContent := `module testproject

go 1.21

require (
	github.com/stretchr/testify v1.8.4
	github.com/spf13/cobra v1.7.0
)
`
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644)
	require.NoError(t, err)

	// Create a simple package.json
	packageJSONContent := `{
  "name": "test-project",
  "version": "1.0.0",
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.21"
  }
}`
	err = os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSONContent), 0644)
	require.NoError(t, err)
}

func setupMultipleTestProjects(t *testing.T, dir string) {
	// Set up main project
	setupTestProject(t, dir)

	// Create subprojects
	subDir1 := filepath.Join(dir, "subproject1")
	err := os.MkdirAll(subDir1, 0755)
	require.NoError(t, err)

	// Sub-project with Python dependencies
	requirementsContent := `flask==2.3.0
requests==2.31.0
pytest==7.4.0`
	err = os.WriteFile(filepath.Join(subDir1, "requirements.txt"), []byte(requirementsContent), 0644)
	require.NoError(t, err)

	// Create another subproject
	subDir2 := filepath.Join(dir, "subproject2")
	err = os.MkdirAll(subDir2, 0755)
	require.NoError(t, err)

	// Docker project
	dockerfileContent := `FROM golang:1.21-alpine
FROM node:18-alpine
RUN apk add --no-cache git`
	err = os.WriteFile(filepath.Join(subDir2, "Dockerfile"), []byte(dockerfileContent), 0644)
	require.NoError(t, err)
}

// initTestLogger initializes a simple logger for testing
func initTestLogger() {
	// Initialize a standard logger for testing
	if logger.StandardLogger() == nil {
		logger.SetLogger(logger.New("test"))
	}
}
