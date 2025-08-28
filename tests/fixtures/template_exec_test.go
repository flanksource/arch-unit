package fixtures_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/fixtures"
	"github.com/flanksource/arch-unit/tests/fixtures/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateVariableExecution(t *testing.T) {
	// Register exec fixture type if not already registered
	fixtures.Register(&types.ExecFixture{})

	// Create temp directory with test files
	tempDir := t.TempDir()

	// Create test files
	testFile1 := filepath.Join(tempDir, "test1.txt")
	testFile2 := filepath.Join(tempDir, "subdir", "test2.txt")
	
	err := os.WriteFile(testFile1, []byte("content1"), 0644)
	require.NoError(t, err)
	
	err = os.MkdirAll(filepath.Dir(testFile2), 0755)
	require.NoError(t, err)
	err = os.WriteFile(testFile2, []byte("content2"), 0644)
	require.NoError(t, err)

	// Create fixture with template variables
	fixtureContent := `---
files: "**/*.txt"
exec: echo "Processing {{.file}} in {{.dir}}"
---

## Test Files

| Test Name | Expected Output | CEL Validation |
|-----------|-----------------|----------------|
| Process File | Processing | stdout.contains("Processing") |
`

	fixtureFile := filepath.Join(tempDir, "template_test.md")
	err = os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse fixtures
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	// Should have 2 fixtures (one for each .txt file)
	assert.Len(t, fixtureNodes, 2, "Should create fixture for each matching file")

	// Create evaluator
	evaluator, err := fixtures.NewCELEvaluator()
	require.NoError(t, err)

	// Execute each fixture
	for i, node := range fixtureNodes {
		require.NotNil(t, node.Test)
		
		// Get the appropriate fixture type
		fixtureType, ok := fixtures.Get("exec")
		require.True(t, ok, "exec fixture type should be registered")

		// Run the fixture
		opts := fixtures.RunOptions{
			WorkDir:   tempDir,
			Evaluator: evaluator,
		}

		result := fixtureType.Run(context.Background(), *node.Test, opts)

		// Check that the command was executed successfully
		assert.True(t, result.IsOK(), "Test %d failed: %s", i, result.Error)

		// Check that output contains the expected content
		if i == 0 {
			assert.Contains(t, result.Stdout, "test1.txt")
		} else {
			assert.Contains(t, result.Stdout, "test2.txt")
			assert.Contains(t, result.Stdout, "subdir")
		}
		assert.Contains(t, result.Stdout, "Processing")
	}
}

func TestComplexTemplateSubstitution(t *testing.T) {
	// Register exec fixture type if not already registered
	fixtures.Register(&types.ExecFixture{})

	// Create temp directory
	tempDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tempDir, "models", "part.scad")
	err := os.MkdirAll(filepath.Dir(testFile), 0755)
	require.NoError(t, err)
	err = os.WriteFile(testFile, []byte("// SCAD content"), 0644)
	require.NoError(t, err)

	// Create fixture that uses multiple template variables
	fixtureContent := `---
files: "**/*.scad"
exec: echo "File={{.file}} Name={{.filename}} Dir={{.dir}} Ext={{.ext}} Base={{.basename}}"
---

| Test Name | Expected Output |
|-----------|-----------------|
| Template Test | File= |
`

	fixtureFile := filepath.Join(tempDir, "complex_template.md")
	err = os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse fixtures
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	require.Len(t, fixtureNodes, 1)
	node := fixtureNodes[0]
	require.NotNil(t, node.Test)

	// Get exec fixture type
	fixtureType, ok := fixtures.Get("exec")
	require.True(t, ok, "exec fixture type should be registered")

	// Run the fixture
	opts := fixtures.RunOptions{
		WorkDir: tempDir,
	}

	result := fixtureType.Run(context.Background(), *node.Test, opts)

	// Check that all template variables were substituted correctly
	assert.True(t, result.IsOK(), "Test failed: %s", result.Error)
	assert.Contains(t, result.Stdout, "File=models/part.scad")
	assert.Contains(t, result.Stdout, "Name=part")
	assert.Contains(t, result.Stdout, "Dir=models")
	assert.Contains(t, result.Stdout, "Ext=.scad")
	assert.Contains(t, result.Stdout, "Base=part.scad")
}