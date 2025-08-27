package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky/ai"
	"github.com/flanksource/commons/logger"
)

// CommentQualityResult represents the result of comment quality analysis
type CommentQualityResult struct {
	Comment        models.Comment `json:"comment"`
	IsSimple       bool           `json:"is_simple"`
	IsDescriptive  bool           `json:"is_descriptive"`
	IsVerbose      bool           `json:"is_verbose"`
	Score          float64        `json:"score"`
	Issues         []string       `json:"issues"`
	Suggestions    []string       `json:"suggestions"`
	AnalysisMethod string         `json:"analysis_method"` // "simple" or "ai"
}

// CommentAnalyzerConfig holds configuration for comment analysis
type CommentAnalyzerConfig struct {
	WordLimit          int     `json:"word_limit"`
	LowCostModel       string  `json:"low_cost_model"`
	MinDescriptiveScore float64 `json:"min_descriptive_score"`
	CheckVerbosity     bool    `json:"check_verbosity"`
	Enabled            bool    `json:"enabled"`
}

// DefaultCommentAnalyzerConfig returns default configuration
func DefaultCommentAnalyzerConfig() CommentAnalyzerConfig {
	return CommentAnalyzerConfig{
		WordLimit:          10,
		LowCostModel:       "claude-3-haiku-20240307",
		MinDescriptiveScore: 0.7,
		CheckVerbosity:     true,
		Enabled:            true,
	}
}

// CommentAnalyzer analyzes comment quality using AI for complex comments
type CommentAnalyzer struct {
	config  CommentAnalyzerConfig
	aiAgent ai.Agent
}

// NewCommentAnalyzer creates a new comment analyzer
func NewCommentAnalyzer(config CommentAnalyzerConfig, aiAgent ai.Agent) *CommentAnalyzer {
	return &CommentAnalyzer{
		config:  config,
		aiAgent: aiAgent,
	}
}

// AnalyzeComment analyzes a single comment for quality issues
func (ca *CommentAnalyzer) AnalyzeComment(ctx context.Context, comment models.Comment) (*CommentQualityResult, error) {
	if !ca.config.Enabled {
		return &CommentQualityResult{
			Comment:        comment,
			IsSimple:       true,
			IsDescriptive:  true,
			Score:          1.0,
			AnalysisMethod: "disabled",
		}, nil
	}

	// For simple comments (under word limit), assume they're fine
	if comment.IsSimpleComment(ca.config.WordLimit) {
		return &CommentQualityResult{
			Comment:        comment,
			IsSimple:       true,
			IsDescriptive:  true,
			IsVerbose:      false,
			Score:          0.8, // Good default for simple comments
			AnalysisMethod: "simple",
		}, nil
	}

	// Use AI for complex comments
	if ca.aiAgent == nil {
		logger.Warnf("AI agent not available for comment analysis, skipping complex comment")
		return &CommentQualityResult{
			Comment:        comment,
			IsSimple:       false,
			IsDescriptive:  true, // Assume OK when AI not available
			IsVerbose:      false,
			Score:          0.5,
			Issues:         []string{"AI analysis not available"},
			AnalysisMethod: "fallback",
		}, nil
	}

	return ca.analyzeWithAI(ctx, comment)
}

// AnalyzeComments analyzes multiple comments
func (ca *CommentAnalyzer) AnalyzeComments(ctx context.Context, comments []models.Comment) ([]*CommentQualityResult, error) {
	results := make([]*CommentQualityResult, len(comments))
	
	for i, comment := range comments {
		result, err := ca.AnalyzeComment(ctx, comment)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze comment at line %d: %w", comment.StartLine, err)
		}
		results[i] = result
	}
	
	return results, nil
}

// analyzeWithAI uses AI to analyze complex comments
func (ca *CommentAnalyzer) analyzeWithAI(ctx context.Context, comment models.Comment) (*CommentQualityResult, error) {
	prompt := ca.buildAnalysisPrompt(comment)
	
	request := ai.PromptRequest{
		Name:   "comment-quality-analysis",
		Prompt: prompt,
		Context: map[string]string{
			"file":    comment.Context,
			"line":    fmt.Sprintf("%d", comment.StartLine),
			"type":    string(comment.Type),
		},
	}

	response, err := ca.aiAgent.ExecutePrompt(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	return ca.parseAIResponse(comment, response)
}

// buildAnalysisPrompt creates a structured prompt for AI analysis
func (ca *CommentAnalyzer) buildAnalysisPrompt(comment models.Comment) string {
	contextInfo := "Unknown context"
	if comment.Context != "" {
		contextInfo = comment.Context
	}

	prompt := fmt.Sprintf(`Analyze this code comment for quality issues:

Comment Text: "%s"
Context: %s (line %d)
Type: %s
Word Count: %d

Please assess the comment for:
1. **Verbosity**: Is it overly verbose with unnecessary words, repetition, or redundant information?
2. **Descriptiveness**: Is it descriptive and helpful? Does it explain WHY something is done, not just WHAT?
3. **Quality Issues**: Are there specific problems like being too vague, obvious, or misleading?

Guidelines:
- Comments should explain intent, not implementation details
- Avoid stating the obvious (e.g., "increment counter" for counter++)
- Good comments explain WHY, business logic, edge cases, or non-obvious behavior
- Verbose comments repeat information or use too many words

Respond in valid JSON format:
{
  "is_verbose": boolean,
  "is_descriptive": boolean,
  "score": float (0.0-1.0, where 1.0 is excellent),
  "issues": ["issue1", "issue2"],
  "suggestions": ["suggestion1", "suggestion2"]
}`, comment.Text, contextInfo, comment.StartLine, comment.Type, comment.WordCount)

	return prompt
}

// parseAIResponse parses the AI response into a CommentQualityResult
func (ca *CommentAnalyzer) parseAIResponse(comment models.Comment, response *ai.PromptResponse) (*CommentQualityResult, error) {
	if response.Error != "" {
		return nil, fmt.Errorf("AI response error: %s", response.Error)
	}

	// Try to extract JSON from the response
	jsonStr := ca.extractJSON(response.Result)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON found in AI response")
	}

	var aiResult struct {
		IsVerbose     bool     `json:"is_verbose"`
		IsDescriptive bool     `json:"is_descriptive"`
		Score         float64  `json:"score"`
		Issues        []string `json:"issues"`
		Suggestions   []string `json:"suggestions"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &aiResult); err != nil {
		logger.Warnf("Failed to parse AI response JSON: %v, response: %s", err, jsonStr)
		// Fallback analysis
		return &CommentQualityResult{
			Comment:        comment,
			IsSimple:       false,
			IsDescriptive:  true,
			IsVerbose:      false,
			Score:          0.5,
			Issues:         []string{"Failed to parse AI analysis"},
			AnalysisMethod: "ai-fallback",
		}, nil
	}

	return &CommentQualityResult{
		Comment:        comment,
		IsSimple:       false,
		IsDescriptive:  aiResult.IsDescriptive,
		IsVerbose:      aiResult.IsVerbose,
		Score:          aiResult.Score,
		Issues:         aiResult.Issues,
		Suggestions:    aiResult.Suggestions,
		AnalysisMethod: "ai",
	}, nil
}

// extractJSON extracts JSON from AI response that might contain extra text
func (ca *CommentAnalyzer) extractJSON(response string) string {
	// Look for JSON object starting with { and ending with }
	start := strings.Index(response, "{")
	if start == -1 {
		return ""
	}

	// Find the matching closing brace
	braceCount := 0
	for i := start; i < len(response); i++ {
		if response[i] == '{' {
			braceCount++
		} else if response[i] == '}' {
			braceCount--
			if braceCount == 0 {
				return response[start : i+1]
			}
		}
	}

	return ""
}

// GetPoorQualityComments filters results to only those with quality issues
func (ca *CommentAnalyzer) GetPoorQualityComments(results []*CommentQualityResult) []*CommentQualityResult {
	var poor []*CommentQualityResult

	for _, result := range results {
		if result.Score < ca.config.MinDescriptiveScore || 
		   (ca.config.CheckVerbosity && result.IsVerbose) ||
		   !result.IsDescriptive ||
		   len(result.Issues) > 0 {
			poor = append(poor, result)
		}
	}

	return poor
}

// AnalyzeAST analyzes all comments in a GenericAST
func (ca *CommentAnalyzer) AnalyzeAST(ctx context.Context, ast *models.GenericAST) ([]*CommentQualityResult, error) {
	if !ca.config.Enabled {
		return nil, nil
	}

	// Get all comments from the AST
	allComments := ast.Comments

	// Add comments from functions, types, and variables
	for _, fn := range ast.Functions {
		allComments = append(allComments, fn.Comments...)
	}

	for _, typ := range ast.Types {
		allComments = append(allComments, typ.Comments...)
		for _, field := range typ.Fields {
			allComments = append(allComments, field.Comments...)
		}
	}

	for _, variable := range ast.Variables {
		allComments = append(allComments, variable.Comments...)
	}

	if len(allComments) == 0 {
		return nil, nil
	}

	return ca.AnalyzeComments(ctx, allComments)
}