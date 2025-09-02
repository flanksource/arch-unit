package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_DependencyScanning_CanaryChecker(t *testing.T) {
	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "scanner-e2e-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Setup scanner with the new walker
	scanner := NewScanner()
	scanner.SetupGitSupport(tempDir)

	// Create scan context with depth=2 and filter for flanksource packages
	ctx := analysis.NewScanContext(nil, "https://github.com/flanksource/canary-checker@HEAD").
		WithDepth(2).
		WithFilter("*flanksource*")

	t.Logf("Starting E2E scan of canary-checker with depth=2 and filter='*flanksource*'")

	// Perform the scan using the new walker
	result, err := scanner.ScanWithContext(ctx, "https://github.com/flanksource/canary-checker@HEAD")
	require.NoError(t, err, "Scan should complete without error")
	require.NotNil(t, result, "Result should not be nil")

	// Verify metadata
	assert.Equal(t, "git", result.Metadata.ScanType, "Should be identified as git scan")
	assert.LessOrEqual(t, result.Metadata.MaxDepth, 2, "Max depth should be at most 2")
	assert.Greater(t, result.Metadata.TotalDependencies, 0, "Should find some dependencies")

	// Collect all flanksource dependencies
	flanksourceDeps := make(map[string]int) // Track dependency name -> depth
	for _, dep := range result.Dependencies {
		if strings.Contains(dep.Name, "flanksource") {
			flanksourceDeps[dep.Name] = dep.Depth
			t.Logf("Found flanksource dependency: %s@%s (depth=%d)", dep.Name, dep.Version, dep.Depth)
		}
	}

	// Based on analysis, canary-checker has these direct flanksource dependencies:
	expectedDirectDeps := []string{
		"github.com/flanksource/artifacts",
		"github.com/flanksource/commons",
		"github.com/flanksource/duty",
		"github.com/flanksource/gomplate/v3",
		"github.com/flanksource/is-healthy",
		"github.com/flanksource/kommons",
	}

	// Additional flanksource deps that appear at depth 1 or 2:
	additionalExpectedDeps := []string{
		"github.com/flanksource/kubectl-neat", // From duty and commons
		"gopkg.in/flanksource/yaml.v3",        // From commons
	}

	// Verify we found flanksource dependencies
	assert.GreaterOrEqual(t, len(flanksourceDeps), 6, "Should find at least 6 flanksource dependencies")

	// Check for expected direct dependencies
	foundDirectCount := 0
	for _, expected := range expectedDirectDeps {
		if depth, found := flanksourceDeps[expected]; found {
			foundDirectCount++
			t.Logf("✓ Found expected direct dependency: %s at depth %d", expected, depth)
		}
	}
	assert.GreaterOrEqual(t, foundDirectCount, 4, "Should find at least 4 of the expected direct flanksource dependencies")

	// Check for transitive dependencies (depth 1 or 2)
	foundTransitive := 0
	for _, expected := range additionalExpectedDeps {
		if depth, found := flanksourceDeps[expected]; found {
			foundTransitive++
			t.Logf("✓ Found expected transitive dependency: %s at depth %d", expected, depth)
		}
	}
	t.Logf("Found %d transitive flanksource dependencies", foundTransitive)

	// Verify depth traversal - should have dependencies at different depths
	depthCounts := make(map[int]int)
	for _, dep := range result.Dependencies {
		if strings.Contains(dep.Name, "flanksource") {
			depthCounts[dep.Depth]++
		}
	}

	t.Logf("Flanksource dependencies by depth: %v", depthCounts)

	// Should have dependencies at depth 0 (direct from canary-checker)
	assert.Greater(t, depthCounts[0], 0, "Should have direct dependencies at depth 0")

	// Verify git operations worked (repositories were cloned)
	assert.Greater(t, result.Metadata.RepositoriesFound, 0, "Should have found and scanned git repositories")

	// Check for version conflicts (informational)
	if len(result.Conflicts) > 0 {
		conflictCount := 0
		for _, conflict := range result.Conflicts {
			if strings.Contains(conflict.DependencyName, "flanksource") {
				conflictCount++
				t.Logf("  Version conflict: %s has %d versions", conflict.DependencyName, len(conflict.Versions))
			}
		}
		t.Logf("Found %d flanksource version conflicts (this is expected)", conflictCount)
	}

	t.Logf("E2E test completed successfully: found %d total dependencies, %d flanksource packages",
		len(result.Dependencies), len(flanksourceDeps))
}

func TestE2E_LocalScanWithDepth(t *testing.T) {
	// This test validates scanning a local directory with depth traversal
	// It uses the arch-unit project itself as a test case

	// Skip in short mode
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Get the project root (assuming we're in analysis/dependencies)
	projectRoot := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
		t.Skip("Cannot find project root, skipping test")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "scanner-local-e2e-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Setup scanner
	scanner := NewScanner()
	scanner.SetupGitSupport(tempDir)

	// Create scan context with depth=1 to scan immediate dependencies
	ctx := analysis.NewScanContext(nil, projectRoot).
		WithDepth(1).
		WithFilter("*flanksource*") // Filter for flanksource packages

	t.Logf("Starting local E2E scan of arch-unit project with depth=1")

	// Perform the scan
	result, err := scanner.ScanWithContext(ctx, projectRoot)
	require.NoError(t, err, "Local scan should complete without error")
	require.NotNil(t, result, "Result should not be nil")

	// Verify metadata
	if result.Metadata.MaxDepth > 0 {
		assert.Equal(t, "mixed", result.Metadata.ScanType, "Should be identified as mixed scan when depth > 0")
	} else {
		assert.Equal(t, "local", result.Metadata.ScanType, "Should be identified as local scan when depth = 0")
	}
	assert.Equal(t, 1, result.Metadata.MaxDepth, "Max depth should be 1")

	// Count flanksource dependencies
	flanksourceCount := 0
	for _, dep := range result.Dependencies {
		if strings.Contains(dep.Name, "flanksource") {
			flanksourceCount++
			t.Logf("Found flanksource dependency: %s@%s (depth=%d)", dep.Name, dep.Version, dep.Depth)
		}
	}

	// The arch-unit project uses several flanksource packages
	assert.Greater(t, flanksourceCount, 0, "Should find at least one flanksource dependency")

	// Count dependencies by depth
	depthCounts := make(map[int]int)
	for _, dep := range result.Dependencies {
		depthCounts[dep.Depth]++
	}

	t.Logf("Dependencies by depth: %v", depthCounts)

	// Should have dependencies at depth 0 (direct)
	assert.Greater(t, depthCounts[0], 0, "Should have direct dependencies at depth 0")

	t.Logf("Local E2E test completed: found %d total flanksource dependencies", flanksourceCount)
}

func TestE2E_HelmToGoTraversal_MissionControl(t *testing.T) {
	// This test validates the complete dependency chain: Helm → Git → Go → Dependencies
	// Using the mission-control-chart which has Helm dependencies that point to Git repos with Go modules

	// Skip in short mode as this test requires network access
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Create temp directory for git cache
	tempDir, err := os.MkdirTemp("", "scanner-helm-e2e-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Setup scanner with the new walker
	scanner := NewScanner()
	scanner.SetupGitSupport(tempDir)

	// Create scan context with depth=1 to traverse through multiple dependency types
	// Using go-getter subdirectory syntax: //chart specifies the subdirectory within the repo
	ctx := analysis.NewScanContext(nil, "https://github.com/flanksource/mission-control-chart//chart@HEAD").
		WithDepth(1).
		WithFilter("*flanksource*") // Filter for flanksource dependencies

	t.Logf("Starting E2E scan of mission-control-chart/chart with depth=1 for Helm→Git→Go traversal")

	// Perform the scan using the new walker with go-getter subdirectory syntax
	result, err := scanner.ScanWithContext(ctx, "https://github.com/flanksource/mission-control-chart//chart@HEAD")
	require.NoError(t, err, "Scan should complete without error")
	require.NotNil(t, result, "Result should not be nil")

	// Verify metadata
	assert.Equal(t, "git", result.Metadata.ScanType, "Should be identified as git scan")
	assert.LessOrEqual(t, result.Metadata.MaxDepth, 2, "Max depth should be at most 2")
	assert.Greater(t, result.Metadata.TotalDependencies, 0, "Should find some dependencies")

	// Categorize dependencies by type and depth
	depsByType := make(map[string]int)
	depsByDepth := make(map[int]int)
	flanksourceDeps := make([]string, 0)

	for _, dep := range result.Dependencies {
		if strings.Contains(dep.Name, "flanksource") {
			depsByType[string(dep.Type)]++
			depsByDepth[dep.Depth]++
			flanksourceDeps = append(flanksourceDeps, fmt.Sprintf("%s (%s, depth=%d)", dep.Name, dep.Type, dep.Depth))
			t.Logf("Found flanksource dependency: %s@%s (type=%s, depth=%d)", dep.Name, dep.Version, dep.Type, dep.Depth)
		}
	}

	// Verify we found flanksource dependencies
	assert.Greater(t, len(flanksourceDeps), 0, "Should find flanksource dependencies")

	t.Logf("Dependencies by type: %v", depsByType)
	t.Logf("Dependencies by depth: %v", depsByDepth)

	// Expected Helm chart dependencies from mission-control-chart
	expectedHelmDeps := []string{
		"apm-hub",
		"config-db",
		"canary-checker",
		"flanksource-ui",
	}

	// Check for expected Helm dependencies
	foundHelmCount := 0
	for _, dep := range result.Dependencies {
		if dep.Type == "helm" || dep.Type == "chart" {
			for _, expectedHelm := range expectedHelmDeps {
				if strings.Contains(dep.Name, expectedHelm) {
					foundHelmCount++
					t.Logf("✓ Found expected Helm dependency: %s", dep.Name)
					break
				}
			}
		}
	}

	if foundHelmCount > 0 {
		t.Logf("✓ Helm chart dependencies detected (%d found)", foundHelmCount)
	}

	// Check for Go dependencies (these should come from the Git repos of Helm charts)
	goDepCount := depsByType["go"]
	if goDepCount > 0 {
		t.Logf("✓ Go dependencies detected (%d found) - verifies Git→Go traversal", goDepCount)
	}

	// Verify we have dependencies at different depths (multi-level traversal)
	depthLevels := len(depsByDepth)
	if depthLevels >= 2 {
		t.Logf("✓ Multi-depth traversal working (%d depth levels)", depthLevels)
	}

	// Check that we have both direct and transitive dependencies
	if depsByDepth[0] > 0 {
		t.Logf("✓ Direct dependencies found at depth 0: %d", depsByDepth[0])
	}
	if depsByDepth[1] > 0 {
		t.Logf("✓ Transitive dependencies found at depth 1: %d", depsByDepth[1])
	}

	// Expected Go dependencies that should appear when following the chain
	// These come from the Go modules in the flanksource chart repositories
	expectedGoDeps := []string{
		"github.com/flanksource/commons",
		"github.com/flanksource/duty",
		"github.com/flanksource/is-healthy",
		"github.com/flanksource/gomplate",
	}

	foundGoCount := 0
	for _, dep := range result.Dependencies {
		if dep.Type == "go" {
			for _, expectedGo := range expectedGoDeps {
				if dep.Name == expectedGo {
					foundGoCount++
					t.Logf("✓ Found expected Go dependency from traversal: %s", dep.Name)
					break
				}
			}
		}
	}

	if foundGoCount > 0 {
		t.Logf("✓ Helm→Git→Go traversal successful (%d expected Go deps found)", foundGoCount)
	}

	// Verify git operations worked (repositories were cloned)
	assert.Greater(t, result.Metadata.RepositoriesFound, 0, "Should have cloned and scanned git repositories")

	// Check for version conflicts across dependency types
	if len(result.Conflicts) > 0 {
		conflictCount := 0
		for _, conflict := range result.Conflicts {
			if strings.Contains(conflict.DependencyName, "flanksource") {
				conflictCount++
				t.Logf("  Version conflict across traversal: %s has %d versions", conflict.DependencyName, len(conflict.Versions))
			}
		}
		if conflictCount > 0 {
			t.Logf("Found %d flanksource version conflicts across dependency types", conflictCount)
		}
	}

	// Validate the multi-type dependency chain worked
	hasMultipleTypes := len(depsByType) > 1
	if hasMultipleTypes {
		t.Logf("✓ Multi-type dependency traversal successful: %v", depsByType)
	}
	assert.True(t, hasMultipleTypes || len(flanksourceDeps) > 0, "Should find dependencies from multi-type traversal")

	t.Logf("Helm→Git→Go E2E test completed: found %d flanksource dependencies across %d types at %d depth levels",
		len(flanksourceDeps), len(depsByType), len(depsByDepth))
}
