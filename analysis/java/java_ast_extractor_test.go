package java

import (
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
			javaContent := []byte("package com.example;\n\nimport java.util.List;\n\npublic class SimpleClass {\n    private String name;\n\n    public SimpleClass(String name) {\n        this.name = name;\n    }\n\n    public String getName() {\n        return name;\n    }\n\n    public void setName(String name) {\n        this.name = name;\n    }\n}")

			result, err := extractor.ExtractFile(astCache, "SimpleClass.java", javaContent)

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
			javaContent := []byte("package com.example;\n\nimport java.util.List;\nimport java.util.ArrayList;\nimport java.io.IOException;\n\npublic class Java7Features {\n    private List<String> items = new ArrayList<>();\n\n    public void processItems(String type) {\n        switch (type) {\n            case \"TYPE_A\":\n                processTypeA();\n                break;\n            case \"TYPE_B\":\n                processTypeB();\n                break;\n            default:\n                processDefault();\n        }\n\n        int value = 0b1010_1100;\n        long number = 1_000_000L;\n    }\n\n    public void readFile(String filename) throws IOException {\n        try (java.io.BufferedReader reader = new java.io.BufferedReader(\n                new java.io.FileReader(filename))) {\n            String line = reader.readLine();\n        }\n    }\n\n    private void processTypeA() {}\n    private void processTypeB() {}\n    private void processDefault() {}\n}")

			result, err := extractor.ExtractFile(astCache, "Java7Features.java", javaContent)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.PackageName).To(Equal("com.example"))

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
			javaContent := []byte("package com.example;\n\npublic class Child extends Parent implements Service {\n    public void childMethod() {\n        parentMethod();\n    }\n\n    public void serviceMethod() {\n    }\n}")

			result, err := extractor.ExtractFile(astCache, "Child.java", javaContent)

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

		It("should handle package extraction from content", func() {
			javaContent := []byte("package com.test.deep.package;\n\npublic class TestClass {\n    public void method() {}\n}")

			result, err := extractor.ExtractFile(astCache, "TestClass.java", javaContent)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.PackageName).To(Equal("com.test.deep.package"))
		})

		It("should handle files without package declaration", func() {
			javaContent := []byte("public class NoPackageClass {\n    public void method() {}\n}")

			result, err := extractor.ExtractFile(astCache, "NoPackageClass.java", javaContent)

			Expect(err).ToNot(HaveOccurred())
			// Package name should be derived from file path or be empty
			Expect(result.PackageName).ToNot(BeNil())
		})
	})

})
