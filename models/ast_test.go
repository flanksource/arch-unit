package models

import (
	"testing"
)

func TestCountWords(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "single word",
			input:    "hello",
			expected: 1,
		},
		{
			name:     "multiple words",
			input:    "hello world test",
			expected: 3,
		},
		{
			name:     "words with extra spaces",
			input:    "  hello   world  ",
			expected: 2,
		},
		{
			name:     "words with newlines",
			input:    "hello\nworld\ntest",
			expected: 3,
		},
		{
			name:     "mixed whitespace",
			input:    "hello\tworld\n\rtest",
			expected: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CountWords(tc.input)
			if result != tc.expected {
				t.Errorf("CountWords(%q) = %d, expected %d", tc.input, result, tc.expected)
			}
		})
	}
}

func TestNewComment(t *testing.T) {
	comment := NewComment("This is a test comment", 10, 10, CommentTypeSingleLine, "function:test")
	
	if comment.Text != "This is a test comment" {
		t.Errorf("Expected text 'This is a test comment', got %q", comment.Text)
	}
	
	if comment.StartLine != 10 {
		t.Errorf("Expected start line 10, got %d", comment.StartLine)
	}
	
	if comment.EndLine != 10 {
		t.Errorf("Expected end line 10, got %d", comment.EndLine)
	}
	
	if comment.Type != CommentTypeSingleLine {
		t.Errorf("Expected type CommentTypeSingleLine, got %s", comment.Type)
	}
	
	if comment.Context != "function:test" {
		t.Errorf("Expected context 'function:test', got %q", comment.Context)
	}
	
	if comment.WordCount != 5 {
		t.Errorf("Expected word count 5, got %d", comment.WordCount)
	}
}

func TestCommentIsSimple(t *testing.T) {
	testCases := []struct {
		name      string
		comment   Comment
		wordLimit int
		expected  bool
	}{
		{
			name:      "simple comment under limit",
			comment:   Comment{WordCount: 8},
			wordLimit: 10,
			expected:  true,
		},
		{
			name:      "comment exactly at limit",
			comment:   Comment{WordCount: 10},
			wordLimit: 10,
			expected:  true,
		},
		{
			name:      "comment over limit",
			comment:   Comment{WordCount: 12},
			wordLimit: 10,
			expected:  false,
		},
		{
			name:      "zero word limit",
			comment:   Comment{WordCount: 1},
			wordLimit: 0,
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.comment.IsSimpleComment(tc.wordLimit)
			if result != tc.expected {
				t.Errorf("IsSimpleComment(%d) = %v, expected %v", tc.wordLimit, result, tc.expected)
			}
		})
	}
}

func TestGenericASTGetAllNames(t *testing.T) {
	ast := &GenericAST{
		Functions: []Function{
			{Name: "testFunction", Parameters: []Parameter{{Name: "param1"}, {Name: "param2"}}},
			{Name: "anotherFunction"},
		},
		Types: []TypeDefinition{
			{Name: "TestStruct", Fields: []Field{{Name: "field1"}, {Name: "field2"}}},
		},
		Variables: []Variable{
			{Name: "globalVar"},
			{Name: "anotherVar"},
		},
	}

	names := ast.GetAllNames()
	expected := []string{"testFunction", "param1", "param2", "anotherFunction", "TestStruct", "field1", "field2", "globalVar", "anotherVar"}

	if len(names) != len(expected) {
		t.Errorf("Expected %d names, got %d", len(expected), len(names))
	}

	// Create a map to check if all expected names are present
	nameMap := make(map[string]bool)
	for _, name := range names {
		nameMap[name] = true
	}

	for _, expectedName := range expected {
		if !nameMap[expectedName] {
			t.Errorf("Expected name %q not found in result", expectedName)
		}
	}
}

func TestGenericASTGetLongNames(t *testing.T) {
	ast := &GenericAST{
		Functions: []Function{
			{Name: "short"},                    // 5 chars
			{Name: "veryLongFunctionName"},     // 20 chars
		},
		Variables: []Variable{
			{Name: "a"},                        // 1 char
			{Name: "anotherVeryLongVariableName"}, // 27 chars
		},
	}

	longNames := ast.GetLongNames(10)
	expected := []string{"veryLongFunctionName", "anotherVeryLongVariableName"}

	if len(longNames) != len(expected) {
		t.Errorf("Expected %d long names, got %d", len(expected), len(longNames))
	}

	for i, name := range longNames {
		if name != expected[i] {
			t.Errorf("Expected long name %q at index %d, got %q", expected[i], i, name)
		}
	}
}

func TestGenericASTGetComplexComments(t *testing.T) {
	ast := &GenericAST{
		Comments: []Comment{
			{WordCount: 5},  // Simple
			{WordCount: 15}, // Complex
			{WordCount: 8},  // Simple
			{WordCount: 12}, // Complex
		},
	}

	complexComments := ast.GetComplexComments(10)

	if len(complexComments) != 2 {
		t.Errorf("Expected 2 complex comments, got %d", len(complexComments))
	}

	for _, comment := range complexComments {
		if comment.WordCount <= 10 {
			t.Errorf("Expected complex comment to have more than 10 words, got %d", comment.WordCount)
		}
	}
}

func TestGenericASTGetMultiLineComments(t *testing.T) {
	ast := &GenericAST{
		Comments: []Comment{
			{Type: CommentTypeSingleLine},
			{Type: CommentTypeMultiLine},
			{Type: CommentTypeDocumentation},
			{Type: CommentTypeSingleLine},
		},
	}

	multiLineComments := ast.GetMultiLineComments()

	if len(multiLineComments) != 2 {
		t.Errorf("Expected 2 multi-line comments, got %d", len(multiLineComments))
	}

	expectedTypes := []CommentType{CommentTypeMultiLine, CommentTypeDocumentation}
	for i, comment := range multiLineComments {
		if comment.Type != expectedTypes[i] {
			t.Errorf("Expected comment type %s at index %d, got %s", expectedTypes[i], i, comment.Type)
		}
	}
}

func BenchmarkCountWords(b *testing.B) {
	text := "This is a sample text with multiple words that we want to benchmark the word counting function with"
	
	for i := 0; i < b.N; i++ {
		CountWords(text)
	}
}

func BenchmarkGetAllNames(b *testing.B) {
	ast := &GenericAST{
		Functions: make([]Function, 100),
		Types:     make([]TypeDefinition, 50),
		Variables: make([]Variable, 200),
	}
	
	// Populate with dummy data
	for i := range ast.Functions {
		ast.Functions[i].Name = "function" + string(rune(i))
	}
	for i := range ast.Types {
		ast.Types[i].Name = "type" + string(rune(i))
	}
	for i := range ast.Variables {
		ast.Variables[i].Name = "variable" + string(rune(i))
	}
	
	for i := 0; i < b.N; i++ {
		ast.GetAllNames()
	}
}