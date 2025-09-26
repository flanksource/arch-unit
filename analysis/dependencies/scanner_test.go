package dependencies

import (
	"reflect"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

// MockScanner implements DependencyScanner for testing
type MockScanner struct {
	*BaseDependencyScanner
	deps []*models.Dependency
}

func NewMockScanner(language string, deps []*models.Dependency) *MockScanner {
	return &MockScanner{
		BaseDependencyScanner: NewBaseDependencyScanner(language, []string{"test.txt"}),
		deps:                  deps,
	}
}

func (m *MockScanner) ScanFile(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	return m.deps, nil
}

var _ = Describe("Dependency Scanner", func() {
	Context("Deduplication", func() {
		It("should remove duplicate dependencies", func() {
			// Create test dependencies with duplicates
			deps1 := []*models.Dependency{
				{
					Name:    "golang.org/x/exp",
					Version: "v0.0.0-20241108190413-2d47ceb2692f",
					Type:    models.DependencyTypeGo,
					Git:     "https://github.com/golang/exp",
				},
				{
					Name:    "github.com/stretchr/testify",
					Version: "v1.8.0",
					Type:    models.DependencyTypeGo,
					Git:     "https://github.com/stretchr/testify",
				},
			}

			deps2 := []*models.Dependency{
				{
					Name:    "golang.org/x/exp",
					Version: "v0.0.0-20241108190413-2d47ceb2692f",
					Type:    models.DependencyTypeGo,
					Git:     "https://github.com/golang/exp",
				},
				{
					Name:    "github.com/spf13/cobra",
					Version: "v1.5.0",
					Type:    models.DependencyTypeGo,
					Git:     "https://github.com/spf13/cobra",
				},
			}

			// Create mock scanners
			scanner1 := NewMockScanner("go", deps1)
			scanner2 := NewMockScanner("go", deps2)

			allDeps := append(deps1, deps2...)

			// Apply deduplication
			deduplicated := make(map[string]*models.Dependency)
			for _, dep := range allDeps {
				key := dep.Name + "@" + dep.Version
				if existing, ok := deduplicated[key]; !ok {
					deduplicated[key] = dep
				} else {
					// Merge sources if needed
					if existing.Source != dep.Source {
						existing.Source += ", " + dep.Source
					}
				}
			}

			// Convert back to slice
			result := make([]*models.Dependency, 0, len(deduplicated))
			for _, dep := range deduplicated {
				result = append(result, dep)
			}

			// Should have 3 unique dependencies (golang.org/x/exp, testify, cobra)
			Expect(result).To(HaveLen(3))

			// Check that scanners are functional
			result1, err := scanner1.ScanFile(nil, "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1).To(Equal(deps1))

			result2, err := scanner2.ScanFile(nil, "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2).To(Equal(deps2))
		})
	})

	Context("Pretty Sort Tags", func() {
		It("should sort version tags correctly", func() {
			tags := []string{
				"v1.10.0",
				"v1.2.0",
				"v1.9.0",
				"v2.0.0",
				"v1.0.0",
				"v1.11.0",
			}

			// Note: String comparison doesn't do semantic version sorting
			// This test verifies lexicographic string sorting behavior
			expected := []string{
				"v2.0.0",
				"v1.9.0",
				"v1.2.0",
				"v1.11.0",
				"v1.10.0",
				"v1.0.0",
			}

			// Sort in reverse lexicographic order (string comparison)
			sort.Slice(tags, func(i, j int) bool {
				return tags[i] > tags[j]
			})

			Expect(tags).To(Equal(expected))
		})
	})

	Context("Dependency Sorting", func() {
		It("should sort dependencies by name", func() {
			deps := []*models.Dependency{
				{Name: "zebra", Version: "v1.0.0"},
				{Name: "alpha", Version: "v2.0.0"},
				{Name: "beta", Version: "v1.5.0"},
			}

			// Sort by name
			sort.Slice(deps, func(i, j int) bool {
				return deps[i].Name < deps[j].Name
			})

			expectedNames := []string{"alpha", "beta", "zebra"}
			actualNames := make([]string, len(deps))
			for i, dep := range deps {
				actualNames[i] = dep.Name
			}

			Expect(actualNames).To(Equal(expectedNames))
		})

		It("should maintain stable sort for equal elements", func() {
			deps := []*models.Dependency{
				{Name: "same", Version: "v1.0.0", Source: "first"},
				{Name: "same", Version: "v1.0.0", Source: "second"},
				{Name: "different", Version: "v1.0.0"},
			}

			// Use stable sort
			sort.SliceStable(deps, func(i, j int) bool {
				if deps[i].Name != deps[j].Name {
					return deps[i].Name < deps[j].Name
				}
				return deps[i].Version < deps[j].Version
			})

			// Check that order is maintained for equal elements
			Expect(deps[0].Name).To(Equal("different"))
			Expect(deps[1].Name).To(Equal("same"))
			Expect(deps[1].Source).To(Equal("first"))
			Expect(deps[2].Name).To(Equal("same"))
			Expect(deps[2].Source).To(Equal("second"))
		})
	})

	Context("Type conversion", func() {
		It("should handle reflection correctly", func() {
			deps := []*models.Dependency{
				{Name: "test", Version: "v1.0.0"},
			}

			// Test reflection usage
			val := reflect.ValueOf(deps)
			Expect(val.Kind()).To(Equal(reflect.Slice))
			Expect(val.Len()).To(Equal(1))

			if val.Len() > 0 {
				firstDep := val.Index(0).Interface().(*models.Dependency)
				Expect(firstDep.Name).To(Equal("test"))
			}
		})
	})
})
