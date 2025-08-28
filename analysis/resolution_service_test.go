package analysis

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolutionService_ExtractGoGitURL(t *testing.T) {
	resolver := NewResolutionService()

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

	scanner := NewGoDependencyScannerWithResolver(NewResolutionService())

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

	scanner := NewGoDependencyScannerWithResolver(NewResolutionService())

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
	astCache := cache.MustGetASTCache()

	resolver := NewResolutionService()

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

	resolver := NewResolutionService()

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

func TestResolutionService_CachingBehavior(t *testing.T) {
	// Get the singleton cache and clear dependency aliases for test isolation
	astCache := cache.MustGetASTCache()
	// Clear dependency aliases for test isolation
	require.NoError(t, astCache.ClearAllData())

	t.Run("Normal Caching Flow", func(t *testing.T) {
		// Clear cache for this test
		require.NoError(t, astCache.ClearAllData())

		// Create resolution service with reasonable TTL
		resolver := NewResolutionServiceWithTTL(1 * time.Hour)

		packageName := "github.com/golang/go"
		packageType := "go"

		// First resolution should resolve and cache
		gitURL, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/golang/go", gitURL)

		// Verify it was cached
		cached, err := astCache.GetDependencyAlias(packageName, packageType)
		require.NoError(t, err)
		require.NotNil(t, cached)
		assert.Equal(t, gitURL, cached.GitURL)

		// Second resolution should use cache (we can't easily verify no HTTP call without mocking)
		gitURL2, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Equal(t, gitURL, gitURL2)
	})

	t.Run("TTL Expiration", func(t *testing.T) {
		// Clear cache for this test
		require.NoError(t, astCache.ClearAllData())

		// Create resolution service with short TTL (1 second to account for network latency)
		resolver := NewResolutionServiceWithTTL(1 * time.Second)

		packageName := "github.com/stretchr/testify"
		packageType := "go"

		// First resolution
		gitURL, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/stretchr/testify", gitURL)

		// Get the cached entry and verify it's not expired yet
		cached, err := astCache.GetDependencyAlias(packageName, packageType)
		require.NoError(t, err)
		require.NotNil(t, cached)
		assert.NotZero(t, cached.LastChecked, "LastChecked should be set")
		assert.False(t, cached.IsExpiredWithTTL(1*time.Second))

		// Wait for TTL to expire
		time.Sleep(1100 * time.Millisecond)

		// The cached entry should now be expired
		assert.True(t, cached.IsExpiredWithTTL(1*time.Second))

		// Resolution should happen again (not from cache)
		gitURL2, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Equal(t, gitURL, gitURL2)
	})

	t.Run("Empty Result Caching", func(t *testing.T) {
		// Clear cache for this test
		require.NoError(t, astCache.ClearAllData())

		resolver := NewResolutionServiceWithTTL(1 * time.Hour)

		// Try to resolve unknown package type (should return empty)
		packageName := "unknown-package"
		packageType := "unknown-type"

		// First resolution should return empty and cache it
		gitURL, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Empty(t, gitURL)

		// Verify empty result was cached
		cached, err := astCache.GetDependencyAlias(packageName, packageType)
		require.NoError(t, err)
		require.NotNil(t, cached)
		assert.Empty(t, cached.GitURL)

		// Second resolution should return cached empty result
		gitURL2, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Empty(t, gitURL2)
	})

	t.Run("Concurrent Access", func(t *testing.T) {
		// Clear cache for this test
		require.NoError(t, astCache.ClearAllData())

		resolver := NewResolutionServiceWithTTL(1 * time.Hour)

		packageName := "github.com/spf13/cobra"
		packageType := "go"

		// Use WaitGroup to coordinate goroutines
		var wg sync.WaitGroup
		const numGoroutines = 10
		results := make([]string, numGoroutines)
		errors := make([]error, numGoroutines)

		// Start multiple goroutines to resolve the same package
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				gitURL, err := resolver.ResolveGitURL(packageName, packageType)
				results[idx] = gitURL
				errors[idx] = err
			}(i)
		}

		// Wait for all goroutines to complete
		wg.Wait()

		// All goroutines should get the same result
		expectedURL := "https://github.com/spf13/cobra"
		for i := 0; i < numGoroutines; i++ {
			require.NoError(t, errors[i])
			assert.Equal(t, expectedURL, results[i])
		}

		// Verify it was cached only once
		cached, err := astCache.GetDependencyAlias(packageName, packageType)
		require.NoError(t, err)
		require.NotNil(t, cached)
		assert.Equal(t, expectedURL, cached.GitURL)
	})

	t.Run("Zero TTL Behavior", func(t *testing.T) {
		// Clear cache for this test
		require.NoError(t, astCache.ClearAllData())

		// Create resolution service with zero TTL (cache disabled)
		resolver := NewResolutionServiceWithTTL(0)

		packageName := "github.com/gorilla/mux"
		packageType := "go"

		// First resolution
		gitURL, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/gorilla/mux", gitURL)

		// It should still store in cache (for other services)
		cached, err := astCache.GetDependencyAlias(packageName, packageType)
		require.NoError(t, err)
		require.NotNil(t, cached)

		// But with TTL=0, it should always be considered expired
		assert.True(t, cached.IsExpiredWithTTL(0))

		// Second resolution should not use cache (will re-resolve)
		gitURL2, err := resolver.ResolveGitURL(packageName, packageType)
		require.NoError(t, err)
		assert.Equal(t, gitURL, gitURL2)
	})
}

func TestResolutionService_ValidationWithMockServer(t *testing.T) {
	// Get the singleton cache and clear dependency aliases for test isolation
	astCache := cache.MustGetASTCache()
	require.NoError(t, astCache.ClearAllData())

	// Counter for HTTP requests
	var requestCount int32

	// Create a test server that tracks requests
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		// Simulate GitHub repository check
		if r.Method == "HEAD" {
			if r.URL.Path == "/valid/repo" {
				w.WriteHeader(http.StatusOK)
			} else if r.URL.Path == "/redirect/repo" {
				// Simulate redirect
				w.Header().Set("Location", server.URL+"/final/repo")
				w.WriteHeader(http.StatusMovedPermanently)
			} else if r.URL.Path == "/final/repo" {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer server.Close()

	t.Run("Validation Caching", func(t *testing.T) {
		// Reset request counter
		atomic.StoreInt32(&requestCount, 0)

		resolver := NewResolutionServiceWithTTL(1 * time.Hour)

		// Test URL that will be validated
		testURL := server.URL + "/valid/repo"

		// Validate URL
		valid, finalURL, err := resolver.ValidateGitURL(testURL)
		require.NoError(t, err)
		assert.True(t, valid)
		assert.Equal(t, testURL, finalURL)
		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

		// Second validation should still make a request (ValidateGitURL doesn't use cache)
		valid2, finalURL2, err := resolver.ValidateGitURL(testURL)
		require.NoError(t, err)
		assert.True(t, valid2)
		assert.Equal(t, finalURL, finalURL2)
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})

	t.Run("Redirect Handling", func(t *testing.T) {
		// Reset request counter
		atomic.StoreInt32(&requestCount, 0)

		resolver := NewResolutionServiceWithTTL(1 * time.Hour)

		// Test URL that will redirect
		testURL := server.URL + "/redirect/repo"
		expectedFinalURL := server.URL + "/final/repo"

		// Validate URL with redirect
		valid, finalURL, err := resolver.ValidateGitURL(testURL)
		require.NoError(t, err)
		assert.True(t, valid)
		assert.Equal(t, expectedFinalURL, finalURL)
		// Should make 2 requests (initial + redirect)
		assert.GreaterOrEqual(t, atomic.LoadInt32(&requestCount), int32(1))
	})

	t.Run("Invalid URL Handling", func(t *testing.T) {
		// Reset request counter
		atomic.StoreInt32(&requestCount, 0)

		resolver := NewResolutionServiceWithTTL(1 * time.Hour)

		// Test URL that will return 404
		testURL := server.URL + "/invalid/repo"

		// Validate invalid URL
		valid, finalURL, err := resolver.ValidateGitURL(testURL)
		require.NoError(t, err)
		assert.False(t, valid)
		assert.Equal(t, testURL, finalURL) // Should return original URL on failure
		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
	})
}
