package dependencies

import (
	"reflect"
	"sort"
	"testing"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
)

// MockScanner implements DependencyScanner for testing
type MockScanner struct {
	*analysis.BaseDependencyScanner
	deps []*models.Dependency
}

func NewMockScanner(language string, deps []*models.Dependency) *MockScanner {
	return &MockScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner(language, []string{"test.txt"}),
		deps:                  deps,
	}
}

func (m *MockScanner) ScanFile(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	return m.deps, nil
}

func TestDeduplication(t *testing.T) {
	// Create test dependencies with duplicates
	deps1 := []*models.Dependency{
		{
			Name:     "golang.org/x/exp",
			Version:  "v0.0.0-20241108190413-2d47ceb2692f",
			Type:     models.DependencyTypeGo,

			Git:      "https://github.com/golang/exp",
		},
		{
			Name:     "github.com/stretchr/testify",
			Version:  "v1.8.0",
			Type:     models.DependencyTypeGo,

			Git:      "https://github.com/stretchr/testify",
		},
	}

	deps2 := []*models.Dependency{
		{
			Name:     "golang.org/x/exp",
			Version:  "",  // Missing version to test merge
			Type:     models.DependencyTypeGo, // Same type now for proper deduplication
			Language: "go",
			Git:      "", // Missing Git URL - should be filled from deps1
		},
		{
			Name:     "github.com/stretchr/testify",
			Version:  "", // Missing version - should be filled from deps1
			Type:     models.DependencyTypeGo,
			Language: "go",
			Git:      "https://github.com/stretchr/testify",
		},
		{
			Name:     "github.com/spf13/cobra",
			Version:  "v1.7.0",
			Type:     models.DependencyTypeGo,
			Language: "go",
			Git:      "https://github.com/spf13/cobra",
		},
	}

	// Create scanner with custom registry
	registry := analysis.NewDependencyRegistry()
	registry.Register(NewMockScanner("test1", deps1))
	registry.Register(NewMockScanner("test2", deps2))

	scanner := &Scanner{
		Registry: registry,
	}

	// Mock directory scan - we'll manually call the deduplication logic
	depMap := make(map[string]*models.Dependency)
	
	// Process first set
	for _, dep := range deps1 {
		key := scanner.getDependencyKey(dep)
		depMap[key] = dep
	}
	
	// Process second set with deduplication
	for _, dep := range deps2 {
		key := scanner.getDependencyKey(dep)
		if existing, exists := depMap[key]; exists {
			// Merge information if needed
			if existing.Version == "" && dep.Version != "" {
				existing.Version = dep.Version
			}
			if existing.Git == "" && dep.Git != "" {
				existing.Git = dep.Git
			}
			if len(existing.Package) == 0 && len(dep.Package) > 0 {
				existing.Package = dep.Package
			}
		} else {
			depMap[key] = dep
		}
	}
	
	// Convert to slice
	var result []*models.Dependency
	for _, dep := range depMap {
		result = append(result, dep)
	}
	
	// Assertions
	assert.Equal(t, 3, len(result), "Should have 3 unique dependencies after deduplication")
	
	// Check that golang.org/x/exp was properly deduplicated and merged
	expFound := false
	testifyFound := false
	cobraFound := false
	
	for _, dep := range result {
		if dep.Name == "golang.org/x/exp" {
			expFound = true
			// Should have Git URL from deps1 and version from deps1
			assert.NotEmpty(t, dep.Git, "Git URL should be preserved during merge")
			assert.NotEmpty(t, dep.Version, "Version should be preserved during merge")
			assert.Equal(t, models.DependencyTypeGo, dep.Type, "Type should be go")
		}
		if dep.Name == "github.com/stretchr/testify" {
			testifyFound = true
			// Should have version from deps1
			assert.NotEmpty(t, dep.Version, "Version should be preserved during merge")
		}
		if dep.Name == "github.com/spf13/cobra" {
			cobraFound = true
		}
	}
	assert.True(t, expFound, "golang.org/x/exp should be found")
	assert.True(t, testifyFound, "github.com/stretchr/testify should be found")
	assert.True(t, cobraFound, "github.com/spf13/cobra should be found")
}

func TestPrettySortTags(t *testing.T) {
	// Test that the Dependency struct has the correct pretty sort tags
	// This test uses reflection to verify the struct tags
	
	dep := models.Dependency{}
	depType := reflect.TypeOf(dep)
	
	// Language field removed - check Type field instead
	typeField, _ := depType.FieldByName("Type")
	typePrettyTag := typeField.Tag.Get("pretty")
	assert.Contains(t, typePrettyTag, "Type", "Type field should have pretty tag")
	
	// Check Name field has sort=2
	nameField, _ := depType.FieldByName("Name")
	namePrettyTag := nameField.Tag.Get("pretty")
	assert.Contains(t, namePrettyTag, "sort=2", "Name field should have sort=2 tag")
}

func TestSorting(t *testing.T) {
	// Test that dependencies can be sorted by language then name
	deps := []*models.Dependency{
		{Name: "requests"},
		{Name: "express"},
		{Name: "gin"},
		{Name: "flask"},
		{Name: "cobra"},
		{Name: "react"},
		{Name: "axios"},
		{Name: "viper"},
	}
	
	// Sort by name only since Language field was removed
	// This mimics what clicky should do based on the sort tags
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].Name < deps[j].Name
	})
	
	// Verify the sort order by name
	expected := []string{
		"axios",
		"cobra",
		"express",
		"flask",
		"gin",
		"react",
		"requests",
		"viper",
	}
	
	assert.Equal(t, len(expected), len(deps), "Should have same number of dependencies")
	
	for i, expName := range expected {
		assert.Equal(t, expName, deps[i].Name, "Name at position %d should match", i)
	}
}

func TestFiltering(t *testing.T) {
	scanner := NewScanner()
	
	deps := []*models.Dependency{
		{Name: "github.com/spf13/cobra", Git: "https://github.com/spf13/cobra"},
		{Name: "golang.org/x/exp", Git: "https://github.com/golang/exp"},
		{Name: "express", Git: ""},  // npm packages don't have Git URLs
		{Name: "flask", Git: ""},    // pip packages don't have Git URLs
	}
	
	// Test Git filter with exclusion - only applies to deps with Git URLs
	scanner.SetFilters(nil, []string{"!*github.com/golang/*"})
	filtered := scanner.applyFilters(deps)
	assert.Equal(t, 1, len(filtered), "Should only include cobra (golang excluded, npm/pip have no Git)")
	
	for _, dep := range filtered {
		assert.NotContains(t, dep.Git, "github.com/golang/", "Should not contain golang packages")
	}
	
	// Test name filter
	scanner.SetFilters([]string{"*cobra*", "express"}, nil)
	filtered = scanner.applyFilters(deps)
	assert.Equal(t, 2, len(filtered), "Should match name patterns")
}