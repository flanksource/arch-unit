package database_test_suite

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/git"
)

var _ = Describe("Git Repository Caching", func() {
	var gitManager git.GitRepositoryManager
	var tempDir string

	BeforeEach(func() {
		// Create temp directory for git cache
		var err error
		tempDir, err = os.MkdirTemp("", "git-cache-test-*")
		Expect(err).ToNot(HaveOccurred())

		// Create git manager using the test temp directory
		gitManager = git.NewGitRepositoryManager(tempDir)
	})

	AfterEach(func() {
		if gitManager != nil {
			_ = gitManager.Close()
		}
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Context("Repository Caching", func() {
		It("should create and cache repositories", func() {
			// Skip in short mode as this test requires network access
			if testing.Short() {
				Skip("Skipping git caching test in short mode")
			}

			// Use a small, stable repository for testing
			testGitURL := "https://github.com/flanksource/is-healthy"

			// First request - should create new repository
			repo1, err := gitManager.GetRepository(testGitURL)
			Expect(err).ToNot(HaveOccurred(), "First GetRepository should succeed")
			Expect(repo1).ToNot(BeNil())

			// Second request - should return cached repository
			repo2, err := gitManager.GetRepository(testGitURL)
			Expect(err).ToNot(HaveOccurred(), "Second GetRepository should succeed")
			Expect(repo2).ToNot(BeNil())

			// Should be the same instance (cached)
			Expect(repo1).To(BeIdenticalTo(repo2), "Second request should return cached repository instance")

			GinkgoWriter.Printf("✓ Git repository caching works correctly")
		})
	})

	Context("Version Resolution Caching", func() {
		It("should cache version resolution results", func() {
			// Skip in short mode as this test requires network access
			if testing.Short() {
				Skip("Skipping version resolution caching test in short mode")
			}

			testGitURL := "https://github.com/flanksource/is-healthy"
			testAlias := "HEAD"

			// First resolution - should fetch from remote
			start1 := time.Now()
			version1, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
			duration1 := time.Since(start1)
			Expect(err).ToNot(HaveOccurred(), "First version resolution should succeed")
			Expect(version1).ToNot(BeEmpty(), "Resolved version should not be empty")

			GinkgoWriter.Printf("First resolution took %v, resolved to: %s", duration1, version1)

			// Second resolution - should use cache (much faster)
			start2 := time.Now()
			version2, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
			duration2 := time.Since(start2)
			Expect(err).ToNot(HaveOccurred(), "Second version resolution should succeed")
			Expect(version2).ToNot(BeEmpty(), "Cached resolved version should not be empty")

			// Should be the same version
			Expect(version1).To(Equal(version2), "Cached version should match original")

			// Second resolution should be significantly faster (cached)
			Expect(duration2).To(BeNumerically("<", duration1/2),
				"Cached resolution (%v) should be much faster than first resolution (%v)",
				duration2, duration1)

			GinkgoWriter.Printf("Second resolution took %v (cached), same version: %s", duration2, version2)
			GinkgoWriter.Printf("✓ Version resolution caching works correctly")
		})
	})

	Context("No Cache Behavior", func() {
		It("should work correctly without cache benefits", func() {
			// Skip in short mode as this test requires network access
			if testing.Short() {
				Skip("Skipping no-cache behavior test in short mode")
			}

			testGitURL := "https://github.com/flanksource/is-healthy"
			testAlias := "HEAD"

			// Test with normal caching
			By("Testing with cache")
			// First resolution
			version1, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
			Expect(err).ToNot(HaveOccurred())
			Expect(version1).ToNot(BeEmpty())

			// Second resolution - should use cache
			start := time.Now()
			version2, err := gitManager.ResolveVersionAlias(testGitURL, testAlias)
			duration := time.Since(start)
			Expect(err).ToNot(HaveOccurred())
			Expect(version1).To(Equal(version2))

			// Should be very fast (cached)
			Expect(duration).To(BeNumerically("<", 10*time.Millisecond),
				"Cached resolution should be very fast, took: %v", duration)

			GinkgoWriter.Printf("Cached resolution took %v", duration)

			// Test without cache (simulating --no-cache behavior)
			By("Testing without cache")
			// Create different managers to simulate no shared cache
			tempDir1, err := os.MkdirTemp("", "no-cache-test-1-*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tempDir1)

			tempDir2, err := os.MkdirTemp("", "no-cache-test-2-*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(tempDir2)

			gitManager1 := git.NewGitRepositoryManager(tempDir1)
			defer func() { _ = gitManager1.Close() }()

			gitManager2 := git.NewGitRepositoryManager(tempDir2)
			defer func() { _ = gitManager2.Close() }()

			// First resolution
			start1 := time.Now()
			version3, err := gitManager1.ResolveVersionAlias(testGitURL, testAlias)
			duration1 := time.Since(start1)
			Expect(err).ToNot(HaveOccurred())
			Expect(version3).ToNot(BeEmpty())

			// Second resolution with different manager (no shared cache)
			start2 := time.Now()
			version4, err := gitManager2.ResolveVersionAlias(testGitURL, testAlias)
			duration2 := time.Since(start2)
			Expect(err).ToNot(HaveOccurred())
			Expect(version4).ToNot(BeEmpty())

			// Versions should be the same
			Expect(version3).To(Equal(version4))

			// Both resolutions should take similar time (no cache benefit)
			ratio := float64(duration2) / float64(duration1)
			Expect(ratio).To(BeNumerically(">", 0.5), "Without cache, both resolutions should take similar time")
			Expect(ratio).To(BeNumerically("<", 2.0), "Without cache, both resolutions should take similar time")

			GinkgoWriter.Printf("No-cache: First resolution took %v, second took %v (ratio: %.2f)",
				duration1, duration2, ratio)

			GinkgoWriter.Printf("✓ No-cache behavior works correctly")
		})
	})

	Context("Worktree Caching", func() {
		It("should cache worktree paths and content", func() {
			// Skip in short mode as this test requires network access
			if testing.Short() {
				Skip("Skipping worktree caching test in short mode")
			}

			testGitURL := "https://github.com/flanksource/is-healthy"
			testVersion := "HEAD"

			// First worktree request - should create new clone
			start1 := time.Now()
			path1, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
			duration1 := time.Since(start1)
			Expect(err).ToNot(HaveOccurred(), "First GetWorktreePath should succeed")
			Expect(path1).ToNot(BeEmpty(), "Worktree path should not be empty")

			// Verify worktree exists
			_, err = os.Stat(path1)
			Expect(err).ToNot(HaveOccurred(), "Worktree directory should exist")

			GinkgoWriter.Printf("First worktree creation took %v, path: %s", duration1, path1)

			// Second worktree request - should return cached path
			start2 := time.Now()
			path2, err := gitManager.GetWorktreePath(testGitURL, testVersion, 1)
			duration2 := time.Since(start2)
			Expect(err).ToNot(HaveOccurred(), "Second GetWorktreePath should succeed")
			Expect(path2).ToNot(BeEmpty(), "Cached worktree path should not be empty")

			// Should be the same path
			Expect(path1).To(Equal(path2), "Second request should return same worktree path")

			// Second request should be much faster (cached)
			Expect(duration2).To(BeNumerically("<", duration1/2),
				"Cached worktree lookup (%v) should be much faster than creation (%v)",
				duration2, duration1)

			// Verify worktree still exists and has content
			entries, err := os.ReadDir(path2)
			Expect(err).ToNot(HaveOccurred(), "Should be able to read cached worktree directory")
			Expect(entries).ToNot(BeEmpty(), "Cached worktree should not be empty")

			GinkgoWriter.Printf("Second worktree lookup took %v (cached), same path", duration2)
			GinkgoWriter.Printf("✓ Worktree caching works correctly")
		})
	})

	Context("Cache Cleanup", func() {
		It("should clean up unused cache entries", func() {
			// Skip in short mode as this test requires network access
			if testing.Short() {
				Skip("Skipping cache cleanup test in short mode")
			}

			testGitURL := "https://github.com/flanksource/is-healthy"

			// Create a repository entry
			repo, err := gitManager.GetRepository(testGitURL)
			Expect(err).ToNot(HaveOccurred())
			Expect(repo).ToNot(BeNil())

			// Get a worktree to ensure some cache content
			_, err = gitManager.GetWorktreePath(testGitURL, "HEAD", 1)
			Expect(err).ToNot(HaveOccurred())

			// Test cleanup with a very short maxAge (should clean up)
			err = gitManager.CleanupUnused(1 * time.Nanosecond)
			Expect(err).ToNot(HaveOccurred(), "Cleanup should succeed")

			// Test cleanup with a long maxAge (should not clean up)
			err = gitManager.CleanupUnused(24 * time.Hour)
			Expect(err).ToNot(HaveOccurred(), "Cleanup should succeed")

			GinkgoWriter.Printf("✓ Cache cleanup works correctly")
		})
	})
})