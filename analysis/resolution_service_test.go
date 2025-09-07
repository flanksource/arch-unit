package analysis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("ResolutionService ExtractGoGitURL", func() {
	var resolver *ResolutionService

	BeforeEach(func() {
		resolver = NewResolutionService()
	})

	DescribeTable("extracting Git URLs from package names",
		func(packageName, expected string, shouldError bool) {
			result, err := resolver.extractGoGitURL(nil, packageName)
			if shouldError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("github.com package", "github.com/flanksource/commons", "https://github.com/flanksource/commons", false),
		Entry("gitlab.com package", "gitlab.com/user/repo", "https://gitlab.com/user/repo", false),
		Entry("golang.org/x package", "golang.org/x/mod", "https://github.com/golang/mod", false),
		Entry("gopkg.in with user", "gopkg.in/yaml.v3", "https://github.com/go-yaml/yaml", false),
		Entry("gopkg.in with version", "gopkg.in/user/repo.v2", "https://github.com/user/repo", false),
		Entry("unknown package", "example.com/unknown/package", "", false),
	)
})

var _ = Describe("ResolutionService DetermineDependencyType", func() {
	var scanner *GoDependencyScanner

	BeforeEach(func() {
		scanner = NewGoDependencyScannerWithResolver(NewResolutionService())
	})

	DescribeTable("determining dependency types from package names",
		func(packageName, expected string) {
			result := scanner.determineDependencyType(packageName)
			Expect(string(result)).To(Equal(expected))
		},
		Entry("golang.org/x package should be stdlib", "golang.org/x/mod", "stdlib"),
		Entry("github.com package should be go", "github.com/flanksource/commons", "go"),
		Entry("other package should be go", "example.com/some/package", "go"),
	)
})

var _ = Describe("ResolutionService FindRequireLine", func() {
	var scanner *GoDependencyScanner
	var content []byte

	BeforeEach(func() {
		scanner = NewGoDependencyScannerWithResolver(NewResolutionService())
		content = []byte(`module github.com/example/project

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
	})

	DescribeTable("finding require line numbers for packages",
		func(packageName string, expectedLine int) {
			result := scanner.findRequireLine(content, packageName, 0)
			Expect(result).To(Equal(expectedLine))
		},
		Entry("flanksource/commons in direct require", "github.com/flanksource/commons", 6),
		Entry("stretchr/testify in direct require", "github.com/stretchr/testify", 7),
		Entry("golang.org/x/mod in direct require", "golang.org/x/mod", 8),
		Entry("indirect dependency", "github.com/davecgh/go-spew", 12),
	)
})

var _ = Describe("ResolutionService ResolveGitURL Caching", func() {
	It("should cache Git URL resolution results", func() {
		astCache := cache.MustGetASTCache()
		resolver := NewResolutionService()

		packageName := "github.com/flanksource/commons"
		packageType := "go"

		// First resolution should work and cache the result
		gitURL, err := resolver.ResolveGitURL(nil, packageName, packageType)
		Expect(err).NotTo(HaveOccurred())
		Expect(gitURL).To(Equal("https://github.com/flanksource/commons"))

		// Check that it was cached
		cached, err := astCache.GetDependencyAlias(packageName, packageType)
		Expect(err).NotTo(HaveOccurred())
		Expect(cached.PackageName).To(Equal(packageName))
		Expect(cached.PackageType).To(Equal(packageType))
		Expect(cached.GitURL).To(Equal(gitURL))
		Expect(cached.IsExpired()).To(BeFalse())

		// Second resolution should return the cached result
		gitURL2, err := resolver.ResolveGitURL(nil, packageName, packageType)
		Expect(err).NotTo(HaveOccurred())
		Expect(gitURL2).To(Equal(gitURL))
	})
})

var _ = Describe("ResolutionService NormalizeGitURL", func() {
	var resolver *ResolutionService

	BeforeEach(func() {
		resolver = NewResolutionService()
	})

	DescribeTable("normalizing Git URLs",
		func(input, expected string) {
			result := resolver.normalizeGitURL(input)
			Expect(result).To(Equal(expected))
		},
		Entry("with .git suffix", "https://github.com/user/repo.git", "https://github.com/user/repo"),
		Entry("without https prefix", "github.com/user/repo", "https://github.com/user/repo"),
		Entry("already normalized", "https://github.com/user/repo", "https://github.com/user/repo"),
	)
})

var _ = Describe("ResolutionService CachingBehavior", func() {
	var astCache *cache.ASTCache

	BeforeEach(func() {
		astCache = cache.MustGetASTCache()
	})

	Context("Normal Caching Flow", func() {
		It("should cache and reuse Git URL resolution results", func() {
			// Create resolution service with reasonable TTL
			resolver := NewResolutionServiceWithTTL(1 * time.Hour)

			packageName := "github.com/golang/go"
			packageType := "go"

			// First resolution should resolve and cache
			gitURL, err := resolver.ResolveGitURL(nil, packageName, packageType)
			Expect(err).NotTo(HaveOccurred())
			Expect(gitURL).To(Equal("https://github.com/golang/go"))

			// Verify it was cached
			cached, err := astCache.GetDependencyAlias(packageName, packageType)
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).NotTo(BeNil())
			Expect(cached.GitURL).To(Equal(gitURL))

			// Second resolution should use cache
			gitURL2, err := resolver.ResolveGitURL(nil, packageName, packageType)
			Expect(err).NotTo(HaveOccurred())
			Expect(gitURL2).To(Equal(gitURL))
		})
	})

	Context("TTL Expiration", func() {
		XIt("should resolve again after TTL expires", func() {
			// Create resolution service with short TTL
			resolver := NewResolutionServiceWithTTL(1 * time.Second)

			packageName := "github.com/stretchr/testify"
			packageType := "go"

			gitURL, err := resolver.ResolveGitURL(nil, packageName, packageType)
			Expect(err).NotTo(HaveOccurred())
			Expect(gitURL).To(Equal("https://github.com/stretchr/testify"))

			// Get the cached entry and verify it's not expired yet
			cached, err := astCache.GetDependencyAlias(packageName, packageType)
			Expect(err).NotTo(HaveOccurred())
			Expect(cached).NotTo(BeNil())
			Expect(cached.LastChecked).NotTo(BeZero())
			Expect(cached.IsExpiredWithTTL(1 * time.Second)).To(BeFalse())

			// Wait for TTL to expire
			time.Sleep(1100 * time.Millisecond)

			// The cached entry should now be expired
			Expect(cached.IsExpiredWithTTL(1 * time.Second)).To(BeTrue())

			// Resolution should happen again (not from cache)
			gitURL2, err := resolver.ResolveGitURL(nil, packageName, packageType)
			Expect(err).NotTo(HaveOccurred())
			Expect(gitURL2).To(Equal(gitURL))
		})
	})

})
