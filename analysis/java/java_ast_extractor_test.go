package java

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestJavaASTExtractor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Java AST Extractor Suite")
}

var _ = BeforeSuite(func() {
	// Configure logger to use Ginkgo's writer for proper test output integration
	logger.Use(GinkgoWriter)
})

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

	Context("when testing IsPrivate functionality", func() {
		// NOTE: These tests are currently failing because the embedded Java AST extractor JAR
		// does not properly set the IsPrivate field. When tested directly with:
		//
		//   java -jar analysis/java/java_ast_extractor.jar analysis/java/testdata/VisibilityTestClass.java
		//
		// The output correctly shows "isPrivate": true for private elements, but the embedded
		// JAR in the Go binary returns "isPrivate": false for all elements.
		//
		// TODO: Update the embedded JAR file to the latest version that includes IsPrivate support

		It("should correctly identify private and public Java elements", func() {

			testFile := filepath.Join("testdata", "VisibilityTestClass.java")
			javaContent, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, javaContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Create a map of element names to their privacy status for easy lookup
			elementPrivacy := make(map[string]bool)

			for _, node := range result.Nodes {
				var elementName string
				switch node.NodeType {
				case models.NodeTypeType:
					elementName = node.TypeName
				case models.NodeTypeMethod:
					elementName = node.MethodName
				case models.NodeTypeField:
					elementName = node.FieldName
				}

				if elementName != "" {
					elementPrivacy[elementName] = node.IsPrivate
				}
			}

			// Verify class visibility
			Expect(elementPrivacy["VisibilityTestClass"]).To(BeFalse(), "Public class should not be private")
			Expect(elementPrivacy["PackagePrivateClass"]).To(BeFalse(), "Package-private class should not be private in Java")

			// Verify field visibility
			Expect(elementPrivacy["privateField"]).To(BeTrue(), "Private field should be private")
			Expect(elementPrivacy["publicField"]).To(BeFalse(), "Public field should not be private")
			Expect(elementPrivacy["packageField"]).To(BeFalse(), "Package-private field should not be private")
			Expect(elementPrivacy["protectedField"]).To(BeFalse(), "Protected field should not be private")

			// Verify constant visibility
			Expect(elementPrivacy["PRIVATE_CONSTANT"]).To(BeTrue(), "Private constant should be private")
			Expect(elementPrivacy["PUBLIC_CONSTANT"]).To(BeFalse(), "Public constant should not be private")

			// Verify method visibility
			Expect(elementPrivacy["privateMethod"]).To(BeTrue(), "Private method should be private")
			Expect(elementPrivacy["getPrivateField"]).To(BeFalse(), "Public method should not be private")
			Expect(elementPrivacy["packageMethod"]).To(BeFalse(), "Package-private method should not be private")
			Expect(elementPrivacy["protectedMethod"]).To(BeFalse(), "Protected method should not be private")
			Expect(elementPrivacy["getSecret"]).To(BeTrue(), "Private static method should be private")
			Expect(elementPrivacy["getPublicConstant"]).To(BeFalse(), "Public static method should not be private")
		})

		It("should handle nested class visibility correctly", func() {
			testFile := filepath.Join("testdata", "VisibilityTestClass.java")
			javaContent, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, javaContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Create a map of class names to their privacy status
			classPrivacy := make(map[string]bool)
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeType {
					classPrivacy[node.TypeName] = node.IsPrivate
				}
			}

			// Verify nested class visibility
			Expect(classPrivacy["PrivateNestedClass"]).To(BeTrue(), "PrivateNestedClass should be private")
			Expect(classPrivacy["PublicNestedClass"]).To(BeFalse(), "PublicNestedClass should not be private")
		})

		It("should correctly identify constructor visibility", func() {
			testFile := filepath.Join("testdata", "VisibilityTestClass.java")
			javaContent, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, javaContent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Find constructors and their privacy status
			// Constructors are methods with the same name as their class
			constructorPrivacy := make(map[string]bool) // key: unique identifier, value: isPrivate
			for _, node := range result.Nodes {
				if node.NodeType == "method" && node.TypeName == "VisibilityTestClass" && node.MethodName == "VisibilityTestClass" {
					// Use line number to uniquely identify each constructor
					key := fmt.Sprintf("line_%d", node.StartLine)
					constructorPrivacy[key] = node.IsPrivate
				}
			}

			// Verify constructor visibility based on line numbers from VisibilityTestClass.java
			// Private constructor: VisibilityTestClass(String secret) - line 27
			Expect(constructorPrivacy["line_27"]).To(BeTrue(), "Private constructor should be private")
			// Public constructor: VisibilityTestClass() - line 32
			Expect(constructorPrivacy["line_32"]).To(BeFalse(), "Public constructor should not be private")
			// Package-private constructor: VisibilityTestClass(int value) - line 37
			Expect(constructorPrivacy["line_37"]).To(BeFalse(), "Package-private constructor should not be private")
		})
	})

})
