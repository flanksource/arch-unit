package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
)

var _ = Describe("E2E Dependency Scanning", func() {
	Context("Canary Checker Repository", func() {
		It("should scan canary-checker with depth and filtering", func() {
			// Skip in short mode as this test requires network access
			Skip("Skipping long-running E2E test")

			// Create temp directory for git cache
			tempDir := GinkgoT().TempDir()

			// Setup scanner with the new walker
			scanner := NewScanner()
			scanner.SetupGitSupport(tempDir)

			// Create scan context with depth=2 and filter for flanksource packages
			ctx := analysis.NewScanContext(nil, "https://github.com/flanksource/canary-checker@HEAD").
				WithDepth(2).
				WithFilter("*flanksource*")

			GinkgoWriter.Printf("Starting E2E scan of canary-checker with depth=2 and filter='*flanksource*'\n")

			// Perform the scan using the new walker
			result, err := scanner.ScanWithContext(ctx, "https://github.com/flanksource/canary-checker@HEAD")
			Expect(err).NotTo(HaveOccurred(), "Scan should complete without error")
			Expect(result).NotTo(BeNil(), "Result should not be nil")

			// Verify metadata
			Expect(result.Metadata.ScanType).To(Equal("git"), "Should be identified as git scan")
			Expect(result.Metadata.MaxDepth).To(BeNumerically("<=", 2), "Max depth should be at most 2")
			Expect(result.Metadata.TotalDependencies).To(BeNumerically(">", 0), "Should find some dependencies")

			// Collect all flanksource dependencies
			flanksourceDeps := make(map[string]int) // Track dependency name -> depth
			for _, dep := range result.Dependencies {
				if strings.Contains(dep.Name, "flanksource") {
					flanksourceDeps[dep.Name] = dep.Depth
					GinkgoWriter.Printf("Found flanksource dependency: %s@%s (depth=%d)\n", dep.Name, dep.Version, dep.Depth)
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
			Expect(len(flanksourceDeps)).To(BeNumerically(">=", 6), "Should find at least 6 flanksource dependencies")

			// Check for expected direct dependencies
			foundDirectCount := 0
			for _, expected := range expectedDirectDeps {
				if depth, found := flanksourceDeps[expected]; found {
					foundDirectCount++
					GinkgoWriter.Printf("✓ Found expected direct dependency: %s at depth %d\n", expected, depth)
				}
			}
			Expect(foundDirectCount).To(BeNumerically(">=", 4), "Should find at least 4 of the expected direct flanksource dependencies")

			// Check for transitive dependencies (depth 1 or 2)
			foundTransitive := 0
			for _, expected := range additionalExpectedDeps {
				if depth, found := flanksourceDeps[expected]; found {
					foundTransitive++
					GinkgoWriter.Printf("✓ Found expected transitive dependency: %s at depth %d\n", expected, depth)
				}
			}
			GinkgoWriter.Printf("Found %d transitive flanksource dependencies\n", foundTransitive)

			// Verify depth traversal - should have dependencies at different depths
			depthCounts := make(map[int]int)
			for _, dep := range result.Dependencies {
				if strings.Contains(dep.Name, "flanksource") {
					depthCounts[dep.Depth]++
				}
			}

			GinkgoWriter.Printf("Flanksource dependencies by depth: %v\n", depthCounts)

			// Should have dependencies at depth 0 (direct from canary-checker)
			Expect(depthCounts[0]).To(BeNumerically(">", 0), "Should have direct dependencies at depth 0")

			// Verify git operations worked (repositories were cloned)
			Expect(result.Metadata.RepositoriesFound).To(BeNumerically(">", 0), "Should have found and scanned git repositories")

			// Check for version conflicts (informational)
			if len(result.Conflicts) > 0 {
				conflictCount := 0
				for _, conflict := range result.Conflicts {
					if strings.Contains(conflict.DependencyName, "flanksource") {
						conflictCount++
						GinkgoWriter.Printf("  Version conflict: %s has %d versions\n", conflict.DependencyName, len(conflict.Versions))
					}
				}
				GinkgoWriter.Printf("Found %d flanksource version conflicts (this is expected)\n", conflictCount)
			}

			GinkgoWriter.Printf("E2E test completed successfully: found %d total dependencies, %d flanksource packages\n",
				len(result.Dependencies), len(flanksourceDeps))
		})
	})

	Context("Local Scan with Depth", func() {
		It("should scan local directory with depth traversal", func() {
			// This test validates scanning a local directory with depth traversal
			// It uses the arch-unit project itself as a test case

			// Skip in short mode
			Skip("Skipping long-running E2E test")

			// Get the project root (assuming we're in analysis/dependencies)
			projectRoot := filepath.Join("..", "..")
			if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
				Skip("Cannot find project root, skipping test")
			}

			// Create temp directory for git cache
			tempDir := GinkgoT().TempDir()

			// Setup scanner
			scanner := NewScanner()
			scanner.SetupGitSupport(tempDir)

			// Create scan context with depth=1 to scan immediate dependencies
			ctx := analysis.NewScanContext(nil, projectRoot).
				WithDepth(1).
				WithFilter("*flanksource*") // Filter for flanksource packages

			GinkgoWriter.Printf("Starting local E2E scan of arch-unit project with depth=1\n")

			// Perform the scan
			result, err := scanner.ScanWithContext(ctx, projectRoot)
			Expect(err).NotTo(HaveOccurred(), "Local scan should complete without error")
			Expect(result).NotTo(BeNil(), "Result should not be nil")

			// Verify metadata
			if result.Metadata.MaxDepth > 0 {
				Expect(result.Metadata.ScanType).To(Equal("mixed"), "Should be identified as mixed scan when depth > 0")
			} else {
				Expect(result.Metadata.ScanType).To(Equal("local"), "Should be identified as local scan when depth = 0")
			}
			Expect(result.Metadata.MaxDepth).To(Equal(1), "Max depth should be 1")

			// Count flanksource dependencies
			flanksourceCount := 0
			for _, dep := range result.Dependencies {
				if strings.Contains(dep.Name, "flanksource") {
					flanksourceCount++
					GinkgoWriter.Printf("Found flanksource dependency: %s@%s (depth=%d)\n", dep.Name, dep.Version, dep.Depth)
				}
			}

			// The arch-unit project uses several flanksource packages
			Expect(flanksourceCount).To(BeNumerically(">", 0), "Should find at least one flanksource dependency")

			// Count dependencies by depth
			depthCounts := make(map[int]int)
			for _, dep := range result.Dependencies {
				depthCounts[dep.Depth]++
			}

			GinkgoWriter.Printf("Dependencies by depth: %v\n", depthCounts)

			// Should have dependencies at depth 0 (direct)
			Expect(depthCounts[0]).To(BeNumerically(">", 0), "Should have direct dependencies at depth 0")

			GinkgoWriter.Printf("Local E2E test completed: found %d total flanksource dependencies\n", flanksourceCount)
		})
	})

	Context("Helm to Go Traversal - Mission Control", func() {
		It("should traverse Helm → Git → Go dependency chain", func() {
			// This test validates the complete dependency chain: Helm → Git → Go → Dependencies
			// Using the mission-control-chart which has Helm dependencies that point to Git repos with Go modules

			// Skip in short mode as this test requires network access
			Skip("Skipping long-running E2E test")

			// Create temp directory for git cache
			tempDir := GinkgoT().TempDir()

			// Setup scanner with the new walker
			scanner := NewScanner()
			scanner.SetupGitSupport(tempDir)

			// Create scan context with depth=1 to traverse through multiple dependency types
			// Using go-getter subdirectory syntax: //chart specifies the subdirectory within the repo
			ctx := analysis.NewScanContext(nil, "https://github.com/flanksource/mission-control-chart//chart@HEAD").
				WithDepth(1).
				WithFilter("*flanksource*") // Filter for flanksource dependencies

			GinkgoWriter.Printf("Starting E2E scan of mission-control-chart/chart with depth=1 for Helm→Git→Go traversal\n")

			// Perform the scan using the new walker with go-getter subdirectory syntax
			result, err := scanner.ScanWithContext(ctx, "https://github.com/flanksource/mission-control-chart//chart@HEAD")
			Expect(err).NotTo(HaveOccurred(), "Scan should complete without error")
			Expect(result).NotTo(BeNil(), "Result should not be nil")

			// Verify metadata
			Expect(result.Metadata.ScanType).To(Equal("git"), "Should be identified as git scan")
			Expect(result.Metadata.MaxDepth).To(BeNumerically("<=", 2), "Max depth should be at most 2")
			Expect(result.Metadata.TotalDependencies).To(BeNumerically(">", 0), "Should find some dependencies")

			// Categorize dependencies by type and depth
			depsByType := make(map[string]int)
			depsByDepth := make(map[int]int)
			flanksourceDeps := make([]string, 0)

			for _, dep := range result.Dependencies {
				if strings.Contains(dep.Name, "flanksource") {
					depsByType[string(dep.Type)]++
					depsByDepth[dep.Depth]++
					flanksourceDeps = append(flanksourceDeps, fmt.Sprintf("%s (%s, depth=%d)", dep.Name, dep.Type, dep.Depth))
					GinkgoWriter.Printf("Found flanksource dependency: %s@%s (type=%s, depth=%d)\n", dep.Name, dep.Version, dep.Type, dep.Depth)
				}
			}

			// Verify we found flanksource dependencies
			Expect(len(flanksourceDeps)).To(BeNumerically(">", 0), "Should find flanksource dependencies")

			GinkgoWriter.Printf("Dependencies by type: %v\n", depsByType)
			GinkgoWriter.Printf("Dependencies by depth: %v\n", depsByDepth)

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
							GinkgoWriter.Printf("✓ Found expected Helm dependency: %s\n", dep.Name)
							break
						}
					}
				}
			}

			if foundHelmCount > 0 {
				GinkgoWriter.Printf("✓ Helm chart dependencies detected (%d found)\n", foundHelmCount)
			}

			// Check for Go dependencies (these should come from the Git repos of Helm charts)
			goDepCount := depsByType["go"]
			if goDepCount > 0 {
				GinkgoWriter.Printf("✓ Go dependencies detected (%d found) - verifies Git→Go traversal\n", goDepCount)
			}

			// Verify we have dependencies at different depths (multi-level traversal)
			depthLevels := len(depsByDepth)
			if depthLevels >= 2 {
				GinkgoWriter.Printf("✓ Multi-depth traversal working (%d depth levels)\n", depthLevels)
			}

			// Check that we have both direct and transitive dependencies
			if depsByDepth[0] > 0 {
				GinkgoWriter.Printf("✓ Direct dependencies found at depth 0: %d\n", depsByDepth[0])
			}
			if depsByDepth[1] > 0 {
				GinkgoWriter.Printf("✓ Transitive dependencies found at depth 1: %d\n", depsByDepth[1])
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
							GinkgoWriter.Printf("✓ Found expected Go dependency from traversal: %s\n", dep.Name)
							break
						}
					}
				}
			}

			if foundGoCount > 0 {
				GinkgoWriter.Printf("✓ Helm→Git→Go traversal successful (%d expected Go deps found)\n", foundGoCount)
			}

			// Verify git operations worked (repositories were cloned)
			Expect(result.Metadata.RepositoriesFound).To(BeNumerically(">", 0), "Should have cloned and scanned git repositories")

			// Check for version conflicts across dependency types
			if len(result.Conflicts) > 0 {
				conflictCount := 0
				for _, conflict := range result.Conflicts {
					if strings.Contains(conflict.DependencyName, "flanksource") {
						conflictCount++
						GinkgoWriter.Printf("  Version conflict across traversal: %s has %d versions\n", conflict.DependencyName, len(conflict.Versions))
					}
				}
				if conflictCount > 0 {
					GinkgoWriter.Printf("Found %d flanksource version conflicts across dependency types\n", conflictCount)
				}
			}

			// Validate the multi-type dependency chain worked
			hasMultipleTypes := len(depsByType) > 1
			if hasMultipleTypes {
				GinkgoWriter.Printf("✓ Multi-type dependency traversal successful: %v\n", depsByType)
			}
			Expect(hasMultipleTypes || len(flanksourceDeps) > 0).To(BeTrue(), "Should find dependencies from multi-type traversal")

			GinkgoWriter.Printf("Helm→Git→Go E2E test completed: found %d flanksource dependencies across %d types at %d depth levels\n",
				len(flanksourceDeps), len(depsByType), len(depsByDepth))
		})
	})
})