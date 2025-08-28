package analysis

import (
	"context"
	"testing"

	"github.com/flanksource/arch-unit/models"
)

func TestDefaultCommentAnalyzerConfig(t *testing.T) {
	config := DefaultCommentAnalyzerConfig()

	if config.WordLimit != 10 {
		t.Errorf("Expected word limit 10, got %d", config.WordLimit)
	}

	if config.LowCostModel != "claude-3-haiku-20240307" {
		t.Errorf("Expected model 'claude-3-haiku-20240307', got %q", config.LowCostModel)
	}

	if config.MinDescriptiveScore != 0.7 {
		t.Errorf("Expected min descriptive score 0.7, got %f", config.MinDescriptiveScore)
	}

	if !config.CheckVerbosity {
		t.Errorf("Expected check verbosity to be true")
	}

	if !config.Enabled {
		t.Errorf("Expected enabled to be true")
	}
}

func TestAnalyzeCommentDisabled(t *testing.T) {
	config := CommentAnalyzerConfig{
		Enabled: false,
	}

	analyzer := NewCommentAnalyzer(config, nil)

	comment := models.Comment{
		Text:      "This is a test comment with many words that exceeds the limit",
		WordCount: 12,
	}

	result, err := analyzer.AnalyzeComment(context.Background(), comment)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.AnalysisMethod != "disabled" {
		t.Errorf("Expected analysis method 'disabled', got %q", result.AnalysisMethod)
	}

	if !result.IsSimple {
		t.Errorf("Expected disabled analysis to mark as simple")
	}

	if !result.IsDescriptive {
		t.Errorf("Expected disabled analysis to mark as descriptive")
	}
}

func TestAnalyzeSimpleComment(t *testing.T) {
	config := CommentAnalyzerConfig{
		Enabled:   true,
		WordLimit: 10,
	}

	analyzer := NewCommentAnalyzer(config, nil)

	comment := models.Comment{
		Text:      "Short comment",
		WordCount: 2,
	}

	result, err := analyzer.AnalyzeComment(context.Background(), comment)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.AnalysisMethod != "simple" {
		t.Errorf("Expected analysis method 'simple', got %q", result.AnalysisMethod)
	}

	if !result.IsSimple {
		t.Errorf("Expected simple comment to be marked as simple")
	}

	if !result.IsDescriptive {
		t.Errorf("Expected simple comment to be marked as descriptive")
	}

	if result.IsVerbose {
		t.Errorf("Expected simple comment not to be marked as verbose")
	}

	if result.Score != 0.8 {
		t.Errorf("Expected score 0.8 for simple comment, got %f", result.Score)
	}
}

func TestAnalyzeComplexCommentWithoutAI(t *testing.T) {
	config := CommentAnalyzerConfig{
		Enabled:   true,
		WordLimit: 5,
	}

	analyzer := NewCommentAnalyzer(config, nil) // No AI agent

	comment := models.Comment{
		Text:      "This is a longer comment that exceeds the word limit",
		WordCount: 11,
	}

	result, err := analyzer.AnalyzeComment(context.Background(), comment)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.AnalysisMethod != "fallback" {
		t.Errorf("Expected analysis method 'fallback', got %q", result.AnalysisMethod)
	}

	if result.IsSimple {
		t.Errorf("Expected complex comment not to be marked as simple")
	}

	if result.Score != 0.5 {
		t.Errorf("Expected fallback score 0.5, got %f", result.Score)
	}

	if len(result.Issues) != 1 || result.Issues[0] != "AI analysis not available" {
		t.Errorf("Expected AI not available issue, got %v", result.Issues)
	}
}

func TestExtractJSON(t *testing.T) {
	analyzer := &CommentAnalyzer{}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple JSON",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "JSON with text before",
			input:    `Here is the analysis: {"key": "value", "number": 42}`,
			expected: `{"key": "value", "number": 42}`,
		},
		{
			name:     "JSON with text after",
			input:    `{"key": "value"} - this is the result`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "nested JSON",
			input:    `{"outer": {"inner": {"deep": true}}, "list": [1, 2, 3]}`,
			expected: `{"outer": {"inner": {"deep": true}}, "list": [1, 2, 3]}`,
		},
		{
			name:     "no JSON",
			input:    `This is just text without JSON`,
			expected: ``,
		},
		{
			name:     "malformed JSON",
			input:    `{"key": "value"`,
			expected: ``,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := analyzer.extractJSON(tc.input)
			if result != tc.expected {
				t.Errorf("extractJSON(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestGetPoorQualityComments(t *testing.T) {
	config := CommentAnalyzerConfig{
		MinDescriptiveScore: 0.7,
		CheckVerbosity:      true,
	}

	analyzer := NewCommentAnalyzer(config, nil)

	results := []*CommentQualityResult{
		{
			Comment:       models.Comment{Text: "Good comment"},
			IsDescriptive: true,
			IsVerbose:     false,
			Score:         0.9,
			Issues:        []string{},
		},
		{
			Comment:       models.Comment{Text: "Bad comment"},
			IsDescriptive: false,
			IsVerbose:     false,
			Score:         0.8,
			Issues:        []string{},
		},
		{
			Comment:       models.Comment{Text: "Verbose comment"},
			IsDescriptive: true,
			IsVerbose:     true,
			Score:         0.8,
			Issues:        []string{},
		},
		{
			Comment:       models.Comment{Text: "Low score comment"},
			IsDescriptive: true,
			IsVerbose:     false,
			Score:         0.5,
			Issues:        []string{},
		},
		{
			Comment:       models.Comment{Text: "Comment with issues"},
			IsDescriptive: true,
			IsVerbose:     false,
			Score:         0.8,
			Issues:        []string{"Too vague"},
		},
	}

	poorQuality := analyzer.GetPoorQualityComments(results)

	expectedCount := 4 // All except the first one
	if len(poorQuality) != expectedCount {
		t.Errorf("Expected %d poor quality comments, got %d", expectedCount, len(poorQuality))
	}

	// Check that the good comment is not included
	for _, result := range poorQuality {
		if result.Comment.Text == "Good comment" {
			t.Errorf("Good comment should not be in poor quality results")
		}
	}
}

func TestAnalyzeAST(t *testing.T) {
	config := CommentAnalyzerConfig{
		Enabled:   true,
		WordLimit: 5,
	}

	analyzer := NewCommentAnalyzer(config, nil)

	ast := &models.GenericAST{
		Comments: []models.Comment{
			{Text: "Short", WordCount: 1},
			{Text: "This is a longer comment", WordCount: 6},
		},
		Functions: []models.Function{
			{
				Comments: []models.Comment{
					{Text: "Function comment", WordCount: 2},
				},
			},
		},
		Types: []models.TypeDefinition{
			{
				Comments: []models.Comment{
					{Text: "Type documentation comment", WordCount: 3},
				},
				Fields: []models.Field{
					{
						Comments: []models.Comment{
							{Text: "Field comment", WordCount: 2},
						},
					},
				},
			},
		},
		Variables: []models.Variable{
			{
				Comments: []models.Comment{
					{Text: "Variable comment", WordCount: 2},
				},
			},
		},
	}

	results, err := analyzer.AnalyzeAST(context.Background(), ast)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedCount := 6 // All comments from different sources
	if len(results) != expectedCount {
		t.Errorf("Expected %d comment results, got %d", expectedCount, len(results))
	}

	// Check that we have one complex comment (the one with 6 words)
	complexCount := 0
	for _, result := range results {
		if !result.IsSimple {
			complexCount++
		}
	}

	if complexCount != 1 {
		t.Errorf("Expected 1 complex comment, got %d", complexCount)
	}
}

func TestAnalyzeASTDisabled(t *testing.T) {
	config := CommentAnalyzerConfig{
		Enabled: false,
	}

	analyzer := NewCommentAnalyzer(config, nil)

	ast := &models.GenericAST{
		Comments: []models.Comment{
			{Text: "Test comment", WordCount: 2},
		},
	}

	results, err := analyzer.AnalyzeAST(context.Background(), ast)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if results != nil {
		t.Errorf("Expected nil results when disabled, got %d results", len(results))
	}
}

func BenchmarkAnalyzeSimpleComment(b *testing.B) {
	config := CommentAnalyzerConfig{
		Enabled:   true,
		WordLimit: 10,
	}

	analyzer := NewCommentAnalyzer(config, nil)

	comment := models.Comment{
		Text:      "Simple test comment",
		WordCount: 3,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.AnalyzeComment(ctx, comment)
	}
}

func BenchmarkExtractJSON(b *testing.B) {
	analyzer := &CommentAnalyzer{}

	input := `Here is the analysis result: {"is_verbose": false, "is_descriptive": true, "score": 0.85, "issues": [], "suggestions": ["Consider adding more detail"]} - end of analysis`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzer.extractJSON(input)
	}
}
