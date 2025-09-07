package git

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Git Repository Worktree", func() {
	Describe("worktree data verification", func() {
		It("should contain actual repository data after creation", func() {
			Skip("Skipping worktree test - requires network access")

			// Create temp directory for git cache
			tempDir, err := os.MkdirTemp("", "worktree-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tempDir)

			GinkgoWriter.Printf("Using temp cache directory: %s\n", tempDir)

			// Create git manager
			gitManager := NewGitRepositoryManager(tempDir)
			defer gitManager.Close()

			// Use a small, stable repository for testing
			testGitURL := "https://github.com/flanksource/mission-control-chart"
			testVersion := "HEAD"

			GinkgoWriter.Printf("Testing worktree creation for %s@%s\n", testGitURL, testVersion)

			// Get worktree path - this should create the clone
			worktreePath, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(worktreePath).NotTo(BeEmpty())

			GinkgoWriter.Printf("Expected worktree path: %s\n", worktreePath)

			// Check if worktree exists at expected location
			expectedExists := false
			if _, err := os.Stat(worktreePath); err == nil {
				expectedExists = true
				GinkgoWriter.Printf("✓ Worktree exists at expected location\n")
			}

			// Check for nested cache directory structure (the bug we discovered)
			nestedPath := filepath.Join(tempDir, "github.com", "flanksource", "mission-control-chart", 
				tempDir, "github.com", "flanksource", "mission-control-chart", "worktrees", testVersion)
			nestedExists := false
			if _, err := os.Stat(nestedPath); err == nil {
				nestedExists = true
				GinkgoWriter.Printf("⚠ Worktree found at nested location: %s\n", nestedPath)
			}

			// At least one should exist
			Expect(expectedExists || nestedExists).To(BeTrue(),
				"Worktree should exist either at expected location or nested location")

			// Use the actual worktree path
			actualWorktreePath := worktreePath
			if !expectedExists && nestedExists {
				actualWorktreePath = nestedPath
				Fail("NESTED CACHE BUG DETECTED: Worktree created at nested path instead of expected path")
			}

			// Verify worktree contains actual repository data
			GinkgoWriter.Printf("Verifying worktree contains data at: %s\n", actualWorktreePath)

			// Check that worktree directory exists and is not empty
			entries, err := os.ReadDir(actualWorktreePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).NotTo(BeEmpty())

			GinkgoWriter.Printf("Worktree contains %d entries\n", len(entries))

			// Verify essential files exist
			expectedFiles := []string{
				".git",        // Git metadata
				"README.md",   // Should have a README
				"chart",       // Should have the chart directory (this is mission-control-chart repo)
			}

			for _, expectedFile := range expectedFiles {
				filePath := filepath.Join(actualWorktreePath, expectedFile)
				_, err := os.Stat(filePath)
				Expect(err).NotTo(HaveOccurred(), "Expected file/directory should exist: %s", expectedFile)
				GinkgoWriter.Printf("✓ Found expected file: %s\n", expectedFile)
			}

			// Verify at least one file has actual content (not empty)
			readmePath := filepath.Join(actualWorktreePath, "README.md")
			if content, err := os.ReadFile(readmePath); err == nil {
				Expect(len(content)).To(BeNumerically(">", 0))
				Expect(len(strings.TrimSpace(string(content)))).To(BeNumerically(">", 10))
				GinkgoWriter.Printf("✓ README.md has %d bytes of content\n", len(content))
			}

			// Check chart directory has content too
			chartPath := filepath.Join(actualWorktreePath, "chart")
			if chartEntries, err := os.ReadDir(chartPath); err == nil {
				Expect(chartEntries).NotTo(BeEmpty())
				GinkgoWriter.Printf("✓ Chart directory has %d entries\n", len(chartEntries))
				
				// Look for Chart.yaml specifically
				chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
				if _, err := os.Stat(chartYamlPath); err == nil {
					GinkgoWriter.Printf("✓ Found Chart.yaml in chart directory\n")
				}
			}

			GinkgoWriter.Printf("✓ Worktree validation completed successfully\n")
		})
	})

	Describe("worktree path validation", func() {
		It("should generate correct paths without nested cache structure", func() {
			// Create temp directory for git cache
			tempDir, err := os.MkdirTemp("", "worktree-path-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.RemoveAll(tempDir)

			// Create git manager
			gitManager := NewGitRepositoryManager(tempDir)
			defer gitManager.Close()

			testGitURL := "https://github.com/flanksource/mission-control-chart"
			testVersion := "HEAD"

			// Get worktree path (don't actually create it, just get the path)
			worktreePath, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("Worktree path: %s\n", worktreePath)
			GinkgoWriter.Printf("Temp dir: %s\n", tempDir)

			// The worktree path should:
			// 1. Start with the temp directory
			// 2. Not contain the temp directory path twice (nested)
			
			// Check it starts with temp dir
			Expect(worktreePath).To(HavePrefix(tempDir),
				"Worktree path should start with cache directory")

			// Check for nested cache directory (the bug)
			// If the path contains the temp directory twice, it's the nested bug
			relativePath := strings.TrimPrefix(worktreePath, tempDir)
			hasNestedCache := strings.Contains(relativePath, tempDir)
			
			if hasNestedCache {
				Fail("NESTED CACHE BUG: Worktree path contains nested cache directory")
			} else {
				GinkgoWriter.Printf("✓ Worktree path is correctly structured (no nested cache)\n")
			}

			// Expected pattern: tempDir/github.com/org/repo/clones/version-depth1
			expectedPattern := filepath.Join(tempDir, "github.com", "flanksource", "mission-control-chart", "clones", testVersion+"-depth1")
			Expect(worktreePath).To(Equal(expectedPattern),
				"Worktree path should follow expected pattern")
		})
	})
})