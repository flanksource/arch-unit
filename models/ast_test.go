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
