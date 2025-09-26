package _go

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("GoDependencyScanner ScanGoMod", func() {
	var scanner *GoDependencyScanner

	BeforeEach(func() {
		scanner = NewGoDependencyScanner()
	})

	Context("when scanning simple go.mod", func() {
		It("should parse dependencies correctly", func() {
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
			Expect(err).NotTo(HaveOccurred())

			// Should find 6 dependencies (3 direct + 3 indirect)
			Expect(deps).To(HaveLen(6))

			// Check specific dependencies
			depNames := make(map[string]*models.Dependency)
			for _, dep := range deps {
				depNames[dep.Name] = dep
			}

			// Check flanksource/commons
			commons := depNames["github.com/flanksource/commons"]
			Expect(commons).NotTo(BeNil())
			Expect(commons.Version).To(Equal("v1.2.3"))
			Expect(commons.Type).To(Equal(models.DependencyTypeGo))
			Expect(commons.Source).To(ContainSubstring("go.mod:"))
			Expect(commons.Git).To(BeEmpty()) // No resolver configured, so Git should be empty

			// Check golang.org/x/mod (should be stdlib type)
			xMod := depNames["golang.org/x/mod"]
			Expect(xMod).NotTo(BeNil())
			Expect(xMod.Version).To(Equal("v0.12.0"))
			Expect(xMod.Type).To(Equal(models.DependencyTypeStdlib))
			Expect(xMod.Source).To(ContainSubstring("go.mod:"))
			Expect(xMod.Git).To(BeEmpty()) // No resolver configured, so Git should be empty
		})
	})

	Context("when scanning go.mod with replace directives", func() {
		It("should apply replace directives correctly", func() {
			content := []byte(`module github.com/example/project

go 1.21

require (
	github.com/flanksource/commons v1.2.3
	github.com/local/package v0.0.0
)

replace github.com/local/package => ../local-package

replace github.com/flanksource/commons => github.com/flanksource/commons v1.3.0`)

			deps, err := scanner.ScanFile(nil, "/test/go.mod", content)
			Expect(err).NotTo(HaveOccurred())

			Expect(deps).To(HaveLen(2))

			// Check that replacements are applied
			depNames := make(map[string]*models.Dependency)
			for _, dep := range deps {
				depNames[dep.Name] = dep
			}

			// Check replaced commons version
			commons := depNames["github.com/flanksource/commons"]
			Expect(commons).NotTo(BeNil())
			Expect(commons.Version).To(Equal("v1.3.0")) // Should be replaced version

			// Check local replacement
			localPkg := depNames["github.com/local/package"]
			Expect(localPkg).NotTo(BeNil())
			Expect(localPkg.Version).To(Equal("local:../local-package")) // Should indicate local path
		})
	})
})
