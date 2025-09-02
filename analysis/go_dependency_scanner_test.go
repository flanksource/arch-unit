package analysis

import (
	"testing"

	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoDependencyScanner_ScanGoMod(t *testing.T) {
	scanner := NewGoDependencyScanner()

	t.Run("simple go.mod", func(t *testing.T) {
		content := []byte(`module github.com/example/project

go 1.21

require (
	github.com/flanksource/commons v1.2.3
	github.com/stretchr/testify v1.8.4
	golang.org/x/mod v0.12.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)`)

		deps, err := scanner.ScanFile(nil, "/test/go.mod", content)
		require.NoError(t, err)

		// Should find 6 dependencies (3 direct + 3 indirect)
		assert.Len(t, deps, 6)

		// Check specific dependencies
		depNames := make(map[string]*models.Dependency)
		for _, dep := range deps {
			depNames[dep.Name] = dep
		}

		// Check flanksource/commons
		commons := depNames["github.com/flanksource/commons"]
		require.NotNil(t, commons)
		assert.Equal(t, "v1.2.3", commons.Version)
		assert.Equal(t, models.DependencyTypeGo, commons.Type)
		assert.Contains(t, commons.Source, "go.mod:")
		assert.Empty(t, commons.Git) // No resolver configured, so Git should be empty

		// Check golang.org/x/mod (should be stdlib type)
		xMod := depNames["golang.org/x/mod"]
		require.NotNil(t, xMod)
		assert.Equal(t, "v0.12.0", xMod.Version)
		assert.Equal(t, models.DependencyTypeStdlib, xMod.Type)
		assert.Contains(t, xMod.Source, "go.mod:")
		assert.Empty(t, xMod.Git) // No resolver configured, so Git should be empty
	})

	t.Run("go.mod with replace directives", func(t *testing.T) {
		content := []byte(`module github.com/example/project

go 1.21

require (
	github.com/flanksource/commons v1.2.3
	github.com/local/package v0.0.0
)

replace github.com/local/package => ../local-package

replace github.com/flanksource/commons => github.com/flanksource/commons v1.3.0`)

		deps, err := scanner.ScanFile(nil, "/test/go.mod", content)
		require.NoError(t, err)


		assert.Len(t, deps, 2)

		// Check that replacements are applied
		depNames := make(map[string]*models.Dependency)
		for _, dep := range deps {
			depNames[dep.Name] = dep
		}

		// Check replaced commons version
		commons := depNames["github.com/flanksource/commons"]
		require.NotNil(t, commons)
		assert.Equal(t, "v1.3.0", commons.Version) // Should be replaced version

		// Check local replacement
		localPkg := depNames["github.com/local/package"]
		require.NotNil(t, localPkg)
		assert.Equal(t, "local:../local-package", localPkg.Version) // Should indicate local path
	})

	t.Run("go.mod with various local replacements", func(t *testing.T) {
		content := []byte(`module github.com/example/project

go 1.21

require (
	github.com/relative/package v0.0.0
	github.com/absolute/package v0.0.0
	github.com/current/package v0.0.0
)

replace github.com/relative/package => ../relative-package

replace github.com/absolute/package => /absolute/path/to/package

replace github.com/current/package => ./current-package`)

		deps, err := scanner.ScanFile(nil, "/test/go.mod", content)
		require.NoError(t, err)

		assert.Len(t, deps, 3)

		// Check that all local replacements are handled
		depNames := make(map[string]*models.Dependency)
		for _, dep := range deps {
			depNames[dep.Name] = dep
		}

		// Check relative path replacement
		relativePkg := depNames["github.com/relative/package"]
		require.NotNil(t, relativePkg)
		assert.Equal(t, "local:../relative-package", relativePkg.Version)

		// Check absolute path replacement
		absolutePkg := depNames["github.com/absolute/package"]
		require.NotNil(t, absolutePkg)
		assert.Equal(t, "local:/absolute/path/to/package", absolutePkg.Version)

		// Check current directory replacement
		currentPkg := depNames["github.com/current/package"]
		require.NotNil(t, currentPkg)
		assert.Equal(t, "local:./current-package", currentPkg.Version)
	})

	t.Run("empty go.mod", func(t *testing.T) {
		content := []byte(`module github.com/example/project

go 1.21`)

		deps, err := scanner.ScanFile(nil, "/test/go.mod", content)
		require.NoError(t, err)
		assert.Empty(t, deps)
	})

	t.Run("malformed go.mod", func(t *testing.T) {
		content := []byte(`this is not a valid go.mod file`)

		deps, err := scanner.ScanFile(nil, "/test/go.mod", content)
		assert.Error(t, err)
		assert.Nil(t, deps)
	})
}

func TestGoDependencyScanner_WithResolver(t *testing.T) {
	resolver := NewResolutionService()
	scanner := NewGoDependencyScannerWithResolver(resolver)

	t.Run("go.mod with resolver", func(t *testing.T) {
		content := []byte(`module github.com/example/project

go 1.21

require (
	github.com/flanksource/commons v1.2.3
	golang.org/x/mod v0.12.0
)`)

		deps, err := scanner.ScanFile(nil, "/test/go.mod", content)
		require.NoError(t, err)
		assert.Len(t, deps, 2)

		depNames := make(map[string]*models.Dependency)
		for _, dep := range deps {
			depNames[dep.Name] = dep
		}

		// Check flanksource/commons - should have Git URL resolved
		commons := depNames["github.com/flanksource/commons"]
		require.NotNil(t, commons)
		assert.Equal(t, models.DependencyTypeGo, commons.Type)
		assert.Equal(t, "https://github.com/flanksource/commons", commons.Git)
		assert.Contains(t, commons.Source, "go.mod:")

		// Check golang.org/x/mod - should be stdlib and have Git URL resolved
		xMod := depNames["golang.org/x/mod"]
		require.NotNil(t, xMod)
		assert.Equal(t, models.DependencyTypeStdlib, xMod.Type)
		assert.Equal(t, "https://github.com/golang/mod", xMod.Git)
		assert.Contains(t, xMod.Source, "go.mod:")
	})
}

func TestGoDependencyScanner_ScanGoSum(t *testing.T) {
	scanner := NewGoDependencyScanner()

	t.Run("simple go.sum", func(t *testing.T) {
		content := []byte(`github.com/flanksource/commons v1.2.3 h1:abc123/def456
github.com/flanksource/commons v1.2.3/go.mod h1:xyz789/uvw012
github.com/stretchr/testify v1.8.4 h1:ghi345/jkl678
github.com/stretchr/testify v1.8.4/go.mod h1:mno901/pqr234
golang.org/x/mod v0.12.0 h1:stu567/vwx890
golang.org/x/mod v0.12.0/go.mod h1:bcd234/efg567`)

		deps, err := scanner.ScanFile(nil, "/test/go.sum", content)
		require.NoError(t, err)

		// Should find 3 unique modules (excluding /go.mod entries)
		assert.Len(t, deps, 3)

		depNames := make(map[string]*models.Dependency)
		for _, dep := range deps {
			depNames[dep.Name] = dep
		}

		// Check specific dependencies
		commons := depNames["github.com/flanksource/commons"]
		require.NotNil(t, commons)
		assert.Equal(t, "v1.2.3", commons.Version)
		assert.Equal(t, models.DependencyTypeGo, commons.Type)
		assert.Empty(t, commons.Git) // No resolver configured, so Git should be empty

		testify := depNames["github.com/stretchr/testify"]
		require.NotNil(t, testify)
		assert.Equal(t, "v1.8.4", testify.Version)
	})

	t.Run("empty go.sum", func(t *testing.T) {
		content := []byte(``)

		deps, err := scanner.ScanFile(nil, "/test/go.sum", content)
		require.NoError(t, err)
		assert.Empty(t, deps)
	})

	t.Run("go.sum with comments", func(t *testing.T) {
		content := []byte(`// This is a comment
github.com/flanksource/commons v1.2.3 h1:abc123/def456

// Another comment
github.com/stretchr/testify v1.8.4 h1:ghi345/jkl678`)

		deps, err := scanner.ScanFile(nil, "/test/go.sum", content)
		require.NoError(t, err)

		assert.Len(t, deps, 2)
	})
}

func TestGoDependencyScanner_SupportedFiles(t *testing.T) {
	scanner := NewGoDependencyScanner()

	supported := scanner.SupportedFiles()
	assert.Contains(t, supported, "go.mod")
	assert.Contains(t, supported, "go.sum")
	assert.Len(t, supported, 2)
}

func TestGoDependencyScanner_Language(t *testing.T) {
	scanner := NewGoDependencyScanner()

	assert.Equal(t, "go", scanner.Language())
}

func TestGoDependencyScanner_NonGoFile(t *testing.T) {
	scanner := NewGoDependencyScanner()

	// Test with a non-Go file
	deps, err := scanner.ScanFile(nil, "/test/package.json", []byte(`{"name": "test"}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a go.sum file")
	assert.Nil(t, deps)
}
