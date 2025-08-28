package fixtures_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureGlobExpansion(t *testing.T) {
	// Create a temp directory with test files
	tempDir := t.TempDir()

	// Create test files with different extensions
	testFiles := []string{
		"model1.scad",
		"model2.scad",
		"subdir/model3.scad",
		"readme.md",
		"script.go",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte("test content"), 0644)
		require.NoError(t, err)
	}

	// Create a fixture file with glob pattern
	fixtureContent := `---
files: "**/*.scad"
exec: openscad {{.file}} -o {{.dir}}/{{.filename}}.stl
---

## Test Fixtures

| Test Name | Expected Output |
|-----------|-----------------|
| Compile Model | success |
`

	fixtureFile := filepath.Join(tempDir, "test_fixture.md")
	err := os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse the fixture file
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	// Should have 3 fixtures (one for each .scad file)
	assert.Len(t, fixtureNodes, 3, "Should create one fixture for each matching file")

	// Check that each fixture has the correct template variables
	expectedFiles := []string{
		"model1.scad",
		"model2.scad",
		"subdir/model3.scad",
	}

	for i, node := range fixtureNodes {
		assert.NotNil(t, node.Test)
		assert.NotNil(t, node.Test.TemplateVars)

		// Check that the test name includes the file
		assert.Contains(t, node.Test.Name, expectedFiles[i])

		// Check template variables
		assert.Equal(t, expectedFiles[i], node.Test.TemplateVars["file"])
		
		// Check filename (without extension)
		expectedFilename := filepath.Base(expectedFiles[i])
		expectedFilename = expectedFilename[:len(expectedFilename)-5] // Remove .scad
		assert.Equal(t, expectedFilename, node.Test.TemplateVars["filename"])

		// Check that exec command is set from frontmatter
		assert.Equal(t, "openscad {{.file}} -o {{.dir}}/{{.filename}}.stl", node.Test.Exec)
	}
}

func TestFixtureGlobExpansionNoMatches(t *testing.T) {
	// Create a temp directory with no matching files
	tempDir := t.TempDir()

	// Create test files that don't match
	testFiles := []string{
		"readme.md",
		"script.go",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		err := os.WriteFile(fullPath, []byte("test content"), 0644)
		require.NoError(t, err)
	}

	// Create a fixture file with glob pattern that won't match
	fixtureContent := `---
files: "**/*.scad"
exec: openscad {{.file}}
---

## Test Fixtures

| Test Name | Expected Output |
|-----------|-----------------|
| Compile Model | success |
`

	fixtureFile := filepath.Join(tempDir, "test_fixture.md")
	err := os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse the fixture file
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	// When no files match, should return the original fixture
	assert.Len(t, fixtureNodes, 1, "Should return original fixture when no files match")
	assert.NotNil(t, fixtureNodes[0].Test)
}

func TestFixtureWithoutGlobPattern(t *testing.T) {
	tempDir := t.TempDir()

	// Create a fixture file without files pattern
	fixtureContent := `---
exec: echo "test"
---

## Test Fixtures

| Test Name | Expected Output |
|-----------|-----------------|
| Simple Test | success |
`

	fixtureFile := filepath.Join(tempDir, "test_fixture.md")
	err := os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse the fixture file
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	// Should have 1 fixture (no expansion)
	assert.Len(t, fixtureNodes, 1, "Should have one fixture without expansion")
	assert.NotNil(t, fixtureNodes[0].Test)
	assert.Nil(t, fixtureNodes[0].Test.TemplateVars, "Should not have template variables")
}

func TestTemplateVariableContent(t *testing.T) {
	tempDir := t.TempDir()

	// Create a nested directory structure
	err := os.MkdirAll(filepath.Join(tempDir, "models", "parts"), 0755)
	require.NoError(t, err)

	// Create a test file in nested directory
	testFile := filepath.Join(tempDir, "models", "parts", "gear.scad")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Create a fixture file with glob pattern
	fixtureContent := `---
files: "**/parts/*.scad"
exec: openscad {{.file}}
---

## Test Fixtures

| Test Name | Expected Output |
|-----------|-----------------|
| Process Part | success |
`

	fixtureFile := filepath.Join(tempDir, "test_fixture.md")
	err = os.WriteFile(fixtureFile, []byte(fixtureContent), 0644)
	require.NoError(t, err)

	// Parse the fixture file
	fixtureNodes, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	require.NoError(t, err)

	// Should have 1 fixture
	require.Len(t, fixtureNodes, 1, "Should have one matching file")

	test := fixtureNodes[0].Test
	require.NotNil(t, test)
	require.NotNil(t, test.TemplateVars)

	// Check all template variables
	assert.Equal(t, "models/parts/gear.scad", test.TemplateVars["file"])
	assert.Equal(t, "gear", test.TemplateVars["filename"])
	assert.Equal(t, "models/parts", test.TemplateVars["dir"])
	assert.Equal(t, "gear.scad", test.TemplateVars["basename"])
	assert.Equal(t, ".scad", test.TemplateVars["ext"])

	// Check that absolute paths are also set
	assert.NotEmpty(t, test.TemplateVars["absfile"])
	assert.NotEmpty(t, test.TemplateVars["absdir"])
	assert.True(t, filepath.IsAbs(test.TemplateVars["absfile"]))
	assert.True(t, filepath.IsAbs(test.TemplateVars["absdir"]))
}