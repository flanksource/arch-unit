package models_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("CountWords", func() {
	DescribeTable("counting words in different scenarios",
		func(input string, expected int) {
			result := models.CountWords(input)
			Expect(result).To(Equal(expected))
		},
		Entry("empty string", "", 0),
		Entry("single word", "hello", 1),
		Entry("multiple words", "hello world test", 3),
		Entry("words with extra spaces", "  hello   world  ", 2),
		Entry("words with newlines", "hello\nworld\ntest", 3),
		Entry("mixed whitespace", "hello\tworld\n\rtest", 3),
	)
})

var _ = Describe("NewComment", func() {
	It("should create a comment with correct properties", func() {
		comment := models.NewComment("This is a test comment", 10, 10, models.CommentTypeSingleLine, "function:test")

		Expect(comment.Text).To(Equal("This is a test comment"))
		Expect(comment.StartLine).To(Equal(10))
		Expect(comment.EndLine).To(Equal(10))
		Expect(comment.Type).To(Equal(models.CommentTypeSingleLine))
		Expect(comment.Context).To(Equal("function:test"))
		Expect(comment.WordCount).To(Equal(5))
	})
})

var _ = Describe("Comment.IsSimpleComment", func() {
	DescribeTable("determining if comments are simple based on word limit",
		func(wordCount, wordLimit int, expected bool) {
			comment := models.Comment{WordCount: wordCount}
			result := comment.IsSimpleComment(wordLimit)
			Expect(result).To(Equal(expected))
		},
		Entry("simple comment under limit", 8, 10, true),
		Entry("comment exactly at limit", 10, 10, true),
		Entry("comment over limit", 12, 10, false),
		Entry("zero word limit", 1, 0, false),
	)
})

var _ = Describe("GenericAST.GetAllNames", func() {
	It("should return all names from functions, types, and variables", func() {
		ast := &models.GenericAST{
			Functions: []models.Function{
				{Name: "testFunction", Parameters: []models.Parameter{{Name: "param1"}, {Name: "param2"}}},
				{Name: "anotherFunction"},
			},
			Types: []models.TypeDefinition{
				{Name: "TestStruct", Fields: []models.Field{{Name: "field1"}, {Name: "field2"}}},
			},
			Variables: []models.Variable{
				{Name: "globalVar"},
				{Name: "anotherVar"},
			},
		}

		names := ast.GetAllNames()
		expected := []string{"testFunction", "param1", "param2", "anotherFunction", "TestStruct", "field1", "field2", "globalVar", "anotherVar"}

		Expect(names).To(HaveLen(len(expected)))
		
		nameMap := make(map[string]bool)
		for _, name := range names {
			nameMap[name] = true
		}

		for _, expectedName := range expected {
			Expect(nameMap).To(HaveKey(expectedName))
		}
	})
})

var _ = Describe("GenericAST.GetLongNames", func() {
	It("should return names longer than the specified threshold", func() {
		ast := &models.GenericAST{
			Functions: []models.Function{
				{Name: "short"},
				{Name: "veryLongFunctionName"},
			},
			Variables: []models.Variable{
				{Name: "a"},
				{Name: "anotherVeryLongVariableName"},
			},
		}

		longNames := ast.GetLongNames(10)
		expected := []string{"veryLongFunctionName", "anotherVeryLongVariableName"}

		Expect(longNames).To(HaveLen(len(expected)))
		Expect(longNames).To(Equal(expected))
	})
})

var _ = Describe("GenericAST.GetComplexComments", func() {
	It("should return comments exceeding the word limit", func() {
		ast := &models.GenericAST{
			Comments: []models.Comment{
				{WordCount: 5},
				{WordCount: 15},
				{WordCount: 8},
				{WordCount: 12},
			},
		}

		complexComments := ast.GetComplexComments(10)

		Expect(complexComments).To(HaveLen(2))
		for _, comment := range complexComments {
			Expect(comment.WordCount).To(BeNumerically(">", 10))
		}
	})
})

var _ = Describe("GenericAST.GetMultiLineComments", func() {
	It("should return multi-line and documentation comments", func() {
		ast := &models.GenericAST{
			Comments: []models.Comment{
				{Type: models.CommentTypeSingleLine},
				{Type: models.CommentTypeMultiLine},
				{Type: models.CommentTypeDocumentation},
				{Type: models.CommentTypeSingleLine},
			},
		}

		multiLineComments := ast.GetMultiLineComments()

		Expect(multiLineComments).To(HaveLen(2))
		expectedTypes := []models.CommentType{models.CommentTypeMultiLine, models.CommentTypeDocumentation}
		for i, comment := range multiLineComments {
			Expect(comment.Type).To(Equal(expectedTypes[i]))
		}
	})
})

var _ = Describe("Performance tests", func() {
	It("should count words efficiently", func() {
		text := "This is a sample text with multiple words that we want to benchmark the word counting function with"
		
		// Simple performance test - just ensure it completes
		result := models.CountWords(text)
		Expect(result).To(Equal(18))
	})

	It("should get all names efficiently", func() {
		ast := &models.GenericAST{
			Functions: make([]models.Function, 100),
			Types:     make([]models.TypeDefinition, 50),
			Variables: make([]models.Variable, 200),
		}

		for i := range ast.Functions {
			ast.Functions[i].Name = "function" + string(rune(i))
		}
		for i := range ast.Types {
			ast.Types[i].Name = "type" + string(rune(i))
		}
		for i := range ast.Variables {
			ast.Variables[i].Name = "variable" + string(rune(i))
		}

		names := ast.GetAllNames()
		Expect(names).To(HaveLen(350))
	})
})

var _ = Describe("ASTNode Pretty", func() {
	DescribeTable("should show only current name for different node types",
		func(node *models.ASTNode, expectedContent string) {
			result := node.Pretty()
			Expect(result.String()).To(ContainSubstring(expectedContent))
		},
		Entry("package node", &models.ASTNode{NodeType: models.NodeTypePackage, PackageName: "main", TypeName: "MyType"}, "main"),
		Entry("type node", &models.ASTNode{NodeType: models.NodeTypeType, PackageName: "main", TypeName: "MyType"}, "MyType"),
		Entry("method node", &models.ASTNode{NodeType: models.NodeTypeMethod, PackageName: "main", TypeName: "MyType", MethodName: "DoSomething"}, "DoSomething()"),
		Entry("field node", &models.ASTNode{NodeType: models.NodeTypeField, PackageName: "main", TypeName: "MyType", FieldName: "myField"}, "myField"),
		Entry("variable node", &models.ASTNode{NodeType: models.NodeTypeVariable, PackageName: "main", FieldName: "myVar"}, "myVar"),
	)

	It("should not contain full path separators for complex nodes", func() {
		node := &models.ASTNode{
			NodeType:    models.NodeTypeMethod,
			PackageName: "com.example.pkg",
			TypeName:    "MyClass",
			MethodName:  "complexMethod",
		}

		result := node.Pretty()
		resultStr := result.String()
		Expect(resultStr).To(ContainSubstring("complexMethod()"))
		Expect(resultStr).NotTo(ContainSubstring("com.example.pkg"))
		Expect(resultStr).NotTo(ContainSubstring("MyClass"))
		// Check that it doesn't contain separator dots (but allow dots in method parentheses)
		Expect(resultStr).NotTo(ContainSubstring("com.example.pkg.MyClass"))
		Expect(resultStr).NotTo(ContainSubstring("MyClass.complexMethod"))
	})

	It("should include line numbers when provided", func() {
		node := &models.ASTNode{
			NodeType:   models.NodeTypeMethod,
			MethodName: "TestMethod",
			StartLine:  42,
		}

		result := node.Pretty()
		Expect(result.String()).To(ContainSubstring("L42"))
	})

	It("should not include line numbers when StartLine is 0", func() {
		node := &models.ASTNode{
			NodeType:   models.NodeTypeMethod,
			MethodName: "TestMethod",
			StartLine:  0,
		}

		result := node.Pretty()
		Expect(result.String()).NotTo(ContainSubstring("L"))
	})
})

var _ = Describe("ASTNode TreeNode Interface", func() {
	var nodes []*models.ASTNode

	BeforeEach(func() {
		nodes = []*models.ASTNode{
			{ID: 1, NodeType: models.NodeTypePackage, PackageName: "main", ParentID: nil},
			{ID: 2, NodeType: models.NodeTypeType, TypeName: "MyType", ParentID: &[]int64{1}[0]},
			{ID: 3, NodeType: models.NodeTypeMethod, MethodName: "Method1", ParentID: &[]int64{2}[0]},
			{ID: 4, NodeType: models.NodeTypeField, FieldName: "field1", ParentID: &[]int64{2}[0]},
			{ID: 5, NodeType: models.NodeTypeType, TypeName: "AnotherType", ParentID: &[]int64{1}[0]},
		}
		models.PopulateNodeHierarchy(nodes)
	})

	AfterEach(func() {
		models.ClearNodeHierarchy()
	})

	It("should return correct children for parent nodes", func() {
		// Package node should have type children
		packageNode := nodes[0]  // ID: 1
		children := packageNode.GetChildren()

		Expect(children).To(HaveLen(2))
		// Check that children are the type nodes
		childIDs := make([]int64, len(children))
		for i, child := range children {
			astChild := child.(*models.ASTNode)
			childIDs[i] = astChild.ID
		}
		Expect(childIDs).To(ContainElements(int64(2), int64(5)))
	})

	It("should return correct children for type nodes", func() {
		// Type node should have method and field children
		typeNode := nodes[1] // ID: 2
		children := typeNode.GetChildren()

		Expect(children).To(HaveLen(2))
		childIDs := make([]int64, len(children))
		for i, child := range children {
			astChild := child.(*models.ASTNode)
			childIDs[i] = astChild.ID
		}
		Expect(childIDs).To(ContainElements(int64(3), int64(4)))
	})

	It("should return empty children for leaf nodes", func() {
		// Method and field nodes should be leaves
		methodNode := nodes[2] // ID: 3
		fieldNode := nodes[3]  // ID: 4

		Expect(methodNode.GetChildren()).To(BeEmpty())
		Expect(fieldNode.GetChildren()).To(BeEmpty())
	})
})

var _ = Describe("BuildASTNodeTree", func() {
	It("should create a tree structure with correct root", func() {
		nodes := []*models.ASTNode{
			{ID: 1, NodeType: models.NodeTypePackage, PackageName: "main", ParentID: nil},
			{ID: 2, NodeType: models.NodeTypeType, TypeName: "MyType", ParentID: &[]int64{1}[0]},
		}

		tree := models.BuildASTNodeTree(nodes)

		Expect(tree).NotTo(BeNil())

		// Check root pretty formatting
		pretty := tree.Pretty()
		Expect(pretty.Content).To(ContainSubstring("AST Nodes (2)"))

		// Check root children (should return root-level nodes only)
		children := tree.GetChildren()
		Expect(children).To(HaveLen(1)) // Only the package node has no parent

		packageChild := children[0].(*models.ASTNode)
		Expect(packageChild.ID).To(Equal(int64(1)))
	})

	It("should handle empty node list", func() {
		tree := models.BuildASTNodeTree([]*models.ASTNode{})

		Expect(tree).NotTo(BeNil())
		pretty := tree.Pretty()
		Expect(pretty.Content).To(ContainSubstring("AST Nodes (0)"))

		children := tree.GetChildren()
		Expect(children).To(BeEmpty())
	})

	It("should work with clicky formatting", func() {
		nodes := []*models.ASTNode{
			{ID: 1, NodeType: models.NodeTypePackage, PackageName: "example", ParentID: nil},
			{ID: 2, NodeType: models.NodeTypeMethod, MethodName: "testMethod", ParentID: &[]int64{1}[0]},
		}

		tree := models.BuildASTNodeTree(nodes)

		// This should not panic and should return formatted output
		// Note: We can't test actual clicky.Format output here without importing clicky in tests
		// but we can verify the interface is correctly implemented
		Expect(tree.Pretty()).NotTo(BeNil())
		Expect(tree.GetChildren()).NotTo(BeNil())
	})
})

var _ = Describe("Params Pretty", func() {
	It("should format empty params", func() {
		params := models.Params{}
		result := params.Pretty()
		Expect(result.String()).To(BeEmpty())
	})

	It("should format single param on one line", func() {
		params := models.Params{
			"key1": models.Value{Value: "value1", FieldType: models.FieldTypeString},
		}
		result := params.Pretty()
		Expect(result.String()).To(ContainSubstring("key1="))
		Expect(result.String()).NotTo(ContainSubstring("\n"))
	})

	It("should format three params on one line", func() {
		params := models.Params{
			"key1": models.Value{Value: "value1", FieldType: models.FieldTypeString},
			"key2": models.Value{Value: "value2", FieldType: models.FieldTypeString},
			"key3": models.Value{Value: "value3", FieldType: models.FieldTypeString},
		}
		result := params.Pretty()
		Expect(result.String()).To(ContainSubstring("key1="))
		Expect(result.String()).To(ContainSubstring("key2="))
		Expect(result.String()).To(ContainSubstring("key3="))
		Expect(result.String()).NotTo(ContainSubstring("\n"))
	})

	It("should format four params with newlines", func() {
		params := models.Params{
			"key1": models.Value{Value: "value1", FieldType: models.FieldTypeString},
			"key2": models.Value{Value: "value2", FieldType: models.FieldTypeString},
			"key3": models.Value{Value: "value3", FieldType: models.FieldTypeString},
			"key4": models.Value{Value: "value4", FieldType: models.FieldTypeString},
		}
		result := params.Pretty()
		Expect(result.String()).To(ContainSubstring("key1="))
		Expect(result.String()).To(ContainSubstring("key2="))
		Expect(result.String()).To(ContainSubstring("key3="))
		Expect(result.String()).To(ContainSubstring("key4="))
		Expect(result.String()).To(ContainSubstring("\n"))
	})

	It("should format five params with newlines", func() {
		params := models.Params{
			"key1": models.Value{Value: "value1", FieldType: models.FieldTypeString},
			"key2": models.Value{Value: "value2", FieldType: models.FieldTypeString},
			"key3": models.Value{Value: "value3", FieldType: models.FieldTypeString},
			"key4": models.Value{Value: "value4", FieldType: models.FieldTypeString},
			"key5": models.Value{Value: "value5", FieldType: models.FieldTypeString},
		}
		result := params.Pretty()
		Expect(result.String()).To(ContainSubstring("key1="))
		Expect(result.String()).To(ContainSubstring("key2="))
		Expect(result.String()).To(ContainSubstring("key3="))
		Expect(result.String()).To(ContainSubstring("key4="))
		Expect(result.String()).To(ContainSubstring("key5="))
		Expect(result.String()).To(ContainSubstring("\n"))
	})
})
