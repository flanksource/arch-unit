package java

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestJavaASTExtractor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Java AST Extractor Suite")
}

var _ = Describe("JavaASTExtractor", func() {
	var extractor *JavaASTExtractor
	var astCache *cache.ASTCache

	BeforeEach(func() {
		extractor = NewJavaASTExtractor()
		astCache = cache.MustGetASTCache()
	})

	Describe("ExtractFile", func() {
		It("should extract AST from a simple Java class", func() {
			testFile := filepath.Join("testdata", "com", "example", "SimpleClass.java")
			javaContent, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, javaContent)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Language).To(Equal("java"))
			Expect(result.PackageName).To(Equal("com.example"))

			// Should have nodes for class, field, constructor, and methods
			Expect(len(result.Nodes)).To(BeNumerically(">", 0))

			// Find the class node
			var classNode *models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == "type" && node.TypeName == "SimpleClass" {
					classNode = node
					break
				}
			}
			Expect(classNode).ToNot(BeNil())

			// Should have import
			Expect(len(result.Libraries)).To(BeNumerically(">", 0))
		})

		It("should handle Java 1.7 features", func() {
			testFile := filepath.Join("testdata", "Java7Features.java")
			javaContent, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, javaContent)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			// Should extract all methods including private ones
			var methodCount int
			for _, node := range result.Nodes {
				if node.NodeType == "method" {
					methodCount++
				}
			}
			Expect(methodCount).To(BeNumerically(">", 3)) // At least processItems, readFile, and private methods
		})

		It("should extract inheritance relationships", func() {
			testFile := filepath.Join("testdata", "com", "example", "Child.java")
			javaContent, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, javaContent)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			// Should have extends and implements relationships
			var extendsRel, implementsRel bool
			for _, rel := range result.Relationships {
				if rel.RelationshipType == models.RelationshipTypeInheritance {
					extendsRel = true
				}
				if rel.RelationshipType == models.RelationshipTypeImplements {
					implementsRel = true
				}
			}
			Expect(extendsRel).To(BeTrue())
			Expect(implementsRel).To(BeTrue())
		})

		It("should handle files without package declaration", func() {
			testFile := filepath.Join("testdata", "NoPackageClass.java")
			javaContent, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, javaContent)

			Expect(err).ToNot(HaveOccurred())
			// Package name should be derived from file path or be empty
			Expect(result.PackageName).ToNot(BeNil())
		})
	})

})
