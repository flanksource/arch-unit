package fixtures_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/fixtures"
	"github.com/flanksource/arch-unit/tests/fixtures/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobExpansionIntegration(t *testing.T) {
	// Register exec fixture type
	fixtures.Register(&types.ExecFixture{})

	// Create temp directory with test structure
	tempDir := t.TempDir()

	// Create a directory structure with SCAD files
	scadFiles := map[string]string{
		"gear.scad":               "// Gear model",
		"parts/wheel.scad":        "// Wheel model",
		"parts/axle.scad":         "// Axle model",
		"assemblies/frame.scad":   "// Frame assembly",
		"docs/readme.txt":         "Documentation",
	}

	for path, content := range scadFiles {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create fixture file with glob pattern
	fixtureContent := `---
files: "**/*.scad"
exec: echo "Converting {{.file}} to STL - output will be {{.dir}}/{{.filename}}.stl"
---

# SCAD to STL Conversion

Convert all SCAD files to STL format for 3D printing.

| Test Name | Expected Output | CEL Validation |
|-----------|-----------------|----------------|
| Convert SCAD | Converting | stdout.contains("STL") |
`

	fixtureFile := filepath.Join(tempDir, "convert_scad.md")
	err := os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse the fixture file
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	// Should have 4 fixtures (one for each .scad file)
	assert.Len(t, fixtureNodes, 4, "Should create one fixture for each .scad file")

	// Get exec fixture type
	fixtureType, ok := fixtures.Get("exec")
	require.True(t, ok, "exec fixture type should be registered")

	// Execute each fixture and verify output
	for _, node := range fixtureNodes {
		require.NotNil(t, node.Test)

		opts := fixtures.RunOptions{
			WorkDir: tempDir,
		}

		result := fixtureType.Run(context.Background(), *node.Test, opts)
		assert.True(t, result.IsOK(), "Test failed for %s: %s", node.Test.Name, result.Error)

		// Check that output contains expected content
		assert.Contains(t, result.Stdout, "Converting")
		assert.Contains(t, result.Stdout, "STL")
		assert.Contains(t, result.Stdout, ".scad")

		// Verify template variables were expanded correctly
		if strings.Contains(node.Test.Name, "gear.scad") {
			assert.Contains(t, result.Stdout, "gear.scad")
			assert.Contains(t, result.Stdout, "gear.stl")
		} else if strings.Contains(node.Test.Name, "wheel.scad") {
			assert.Contains(t, result.Stdout, "parts/wheel.scad")
			assert.Contains(t, result.Stdout, "parts/wheel.stl")
		}
	}
}

func TestMultipleFixturesPerFile(t *testing.T) {
	// Register exec fixture type
	fixtures.Register(&types.ExecFixture{})

	// Create temp directory
	tempDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"file1.txt",
		"file2.txt",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		err := os.WriteFile(fullPath, []byte("content"), 0644)
		require.NoError(t, err)
	}

	// Create fixture with multiple test cases
	fixtureContent := `---
files: "*.txt"
exec: echo "{{.basename}}"
---

# Multiple Tests Per File

| Test Name | Expected Output | CEL Validation |
|-----------|-----------------|----------------|
| Test A | file | stdout.contains(".txt") |
| Test B | txt | stdout.contains("file") |
`

	fixtureFile := filepath.Join(tempDir, "multi_test.md")
	err := os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse the fixture file
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	// Should have 4 fixtures (2 tests × 2 files)
	assert.Len(t, fixtureNodes, 4, "Should create fixtures for each test × each file")

	// Verify naming
	names := make(map[string]bool)
	for _, node := range fixtureNodes {
		require.NotNil(t, node.Test)
		names[node.Test.Name] = true
		// Each name should be unique and contain both test name and file
		assert.True(t, strings.Contains(node.Test.Name, "[") && strings.Contains(node.Test.Name, "]"),
			"Test name should include file reference: %s", node.Test.Name)
	}
	assert.Len(t, names, 4, "All test names should be unique")
}