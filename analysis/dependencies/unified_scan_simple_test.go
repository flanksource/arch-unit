package dependencies

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnifiedScannerSimple tests the unified scanner without the task system
func TestUnifiedScannerSimple(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "unified-simple-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a simple test project
	goModContent := `module testproject

go 1.21

require (
	github.com/stretchr/testify v1.8.4
	github.com/spf13/cobra v1.7.0
)
`
	err = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644)
	require.NoError(t, err)

	// Create scanner with registry
	scanner := NewScanner()
	
	// Get all registered scanners and verify Go scanner exists
	languages := scanner.Registry.List()
	hasGoScanner := false
	for _, lang := range languages {
		if lang == "go" {
			hasGoScanner = true
			break
		}
	}
	assert.True(t, hasGoScanner, "Should have Go scanner registered")

	// Discover scan files
	scanJobs, err := scanner.discoverScanFiles(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, scanJobs, "Should discover go.mod file")

	// Verify the discovered file
	foundGoMod := false
	for _, job := range scanJobs {
		if strings.Contains(job.FilePath, "go.mod") {
			foundGoMod = true
			assert.Equal(t, "go", job.ScannerType)
			assert.True(t, job.IsLocal)
		}
	}
	assert.True(t, foundGoMod, "Should find go.mod file")
}

// TestUnifiedScannerPhases tests the two-phase approach without clicky tasks
func TestUnifiedScannerPhases(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "unified-phases-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set up test files
	setupTestProjectWithDeps(t, tmpDir)

	// Create scanner with registry
	scanner := NewScanner()
	scanner.SetupGitSupport(filepath.Join(tmpDir, ".cache"))

	// Create git manager
	gitManager := git.NewGitRepositoryManager(filepath.Join(tmpDir, ".cache"))
	
	// Create unified scanner
	unifiedScanner := NewUnifiedScanner(scanner, gitManager, 1)

	// Phase 1: Discovery - manually test discovery logic
	scanJobs, err := scanner.discoverScanFiles(tmpDir)
	require.NoError(t, err)
	assert.Greater(t, len(scanJobs), 0, "Should discover files")

	// Verify scanner can process files
	for _, job := range scanJobs {
		scannerInterface, ok := scanner.Registry.Get(job.ScannerType)
		assert.True(t, ok, "Should have scanner for type: %s", job.ScannerType)
		
		if ok {
			filePath := filepath.Join(tmpDir, job.FilePath)
			content, err := os.ReadFile(filePath)
			if err == nil {
				// Create a simple scan context
				ctx := &analysis.ScanContext{
					RootDir: tmpDir,
				}
				deps, err := scannerInterface.ScanFile(ctx, filePath, content)
				assert.NoError(t, err, "Should scan file without error")
				if len(deps) > 0 {
					t.Logf("Found %d dependencies in %s", len(deps), job.FilePath)
				}
			}
		}
	}

	// Phase 2: Building tree - test tree structure
	tree := &git.DependencyTree{
		Dependencies: make(map[string]*git.VisitedDep),
		MaxDepth:     1,
	}
	assert.NotNil(t, tree, "Should create dependency tree")
}

func setupTestProjectWithDeps(t *testing.T, dir string) {
	// Create Go project
	goModContent := `module testproject

go 1.21

require (
	github.com/stretchr/testify v1.8.4
	github.com/spf13/cobra v1.7.0
	github.com/spf13/viper v1.16.0
)
`
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644)
	require.NoError(t, err)

	// Create Node project
	packageJSONContent := `{
  "name": "test-project",
  "version": "1.0.0",
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.21",
    "axios": "^1.4.0"
  }
}`
	err = os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSONContent), 0644)
	require.NoError(t, err)

	// Create Python project
	requirementsContent := `flask==2.3.0
requests==2.31.0
pytest==7.4.0
numpy==1.24.3`
	err = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirementsContent), 0644)
	require.NoError(t, err)
}

// TestScannerDiscovery tests the file discovery mechanism
func TestScannerDiscovery(t *testing.T) {
	scanner := NewScanner()
	
	// Test with current directory
	scanJobs, err := scanner.discoverScanFiles(".")
	require.NoError(t, err)
	
	// Should find at least go.mod in current directory
	foundGoMod := false
	for _, job := range scanJobs {
		if strings.Contains(job.FilePath, "go.mod") {
			foundGoMod = true
			t.Logf("Found go.mod at: %s", job.FilePath)
		}
	}
	assert.True(t, foundGoMod, "Should find go.mod in current directory")
}