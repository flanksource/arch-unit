package analysis

import (
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolutionService_ExtractGoGitURL(t *testing.T) {
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	resolver := NewResolutionService(astCache)

	tests := []struct {
		name        string
		packageName string
		expected    string
		shouldError bool
	}{
		{
			name:        "github.com package",
			packageName: "github.com/flanksource/commons",
			expected:    "https://github.com/flanksource/commons",
		},
		{
			name:        "gitlab.com package",
			packageName: "gitlab.com/user/repo",
			expected:    "https://gitlab.com/user/repo",
		},
		{
			name:        "golang.org/x package",
			packageName: "golang.org/x/mod",
			expected:    "https://github.com/golang/mod",
		},
		{
			name:        "gopkg.in with user",
			packageName: "gopkg.in/yaml.v3",
			expected:    "https://github.com/go-yaml/yaml",
		},
		{
			name:        "gopkg.in with version",
			packageName: "gopkg.in/user/repo.v2",
			expected:    "https://github.com/user/repo",
		},
		{
			name:        "unknown package",
			packageName: "example.com/unknown/package",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.extractGoGitURL(tt.packageName)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestResolutionService_DetermineDependencyType(t *testing.T) {
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	scanner := NewGoDependencyScannerWithResolver(NewResolutionService(astCache))

	tests := []struct {
		name        string
		packageName string
		expected    string
	}{
		{
			name:        "golang.org/x package should be stdlib",
			packageName: "golang.org/x/mod",
			expected:    "stdlib",
		},
		{
			name:        "github.com package should be go",
			packageName: "github.com/flanksource/commons",
			expected:    "go",
		},
		{
			name:        "other package should be go",
			packageName: "example.com/some/package",
			expected:    "go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scanner.determineDependencyType(tt.packageName)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestResolutionService_FindRequireLine(t *testing.T) {
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	scanner := NewGoDependencyScannerWithResolver(NewResolutionService(astCache))

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
)`)

	tests := []struct {
		name         string
		packageName  string
		expectedLine int
	}{
		{
			name:         "flanksource/commons in direct require",
			packageName:  "github.com/flanksource/commons",
			expectedLine: 6,
		},
		{
			name:         "stretchr/testify in direct require",
			packageName:  "github.com/stretchr/testify",
			expectedLine: 7,
		},
		{
			name:         "golang.org/x/mod in direct require",
			packageName:  "golang.org/x/mod",
			expectedLine: 8,
		},
		{
			name:         "indirect dependency",
			packageName:  "github.com/davecgh/go-spew",
			expectedLine: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scanner.findRequireLine(content, tt.packageName, 0)
			assert.Equal(t, tt.expectedLine, result)
		})
	}
}

func TestResolutionService_ResolveGitURL_Caching(t *testing.T) {
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	resolver := NewResolutionService(astCache)

	packageName := "github.com/flanksource/commons"
	packageType := "go"

	// First resolution should work and cache the result
	gitURL, err := resolver.ResolveGitURL(packageName, packageType)
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/flanksource/commons", gitURL)

	// Check that it was cached
	cached, err := astCache.GetDependencyAlias(packageName, packageType)
	require.NoError(t, err)
	assert.Equal(t, packageName, cached.PackageName)
	assert.Equal(t, packageType, cached.PackageType)
	assert.Equal(t, gitURL, cached.GitURL)
	assert.False(t, cached.IsExpired())

	// Second resolution should return the cached result
	gitURL2, err := resolver.ResolveGitURL(packageName, packageType)
	require.NoError(t, err)
	assert.Equal(t, gitURL, gitURL2)
}

func TestResolutionService_NormalizeGitURL(t *testing.T) {
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	resolver := NewResolutionService(astCache)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with .git suffix",
			input:    "https://github.com/user/repo.git",
			expected: "https://github.com/user/repo",
		},
		{
			name:     "without https prefix",
			input:    "github.com/user/repo",
			expected: "https://github.com/user/repo",
		},
		{
			name:     "already normalized",
			input:    "https://github.com/user/repo",
			expected: "https://github.com/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.normalizeGitURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}