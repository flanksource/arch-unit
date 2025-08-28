package analysis

import (
	"context"
	"fmt"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky/ai"
	"github.com/flanksource/commons/logger"
)

// AIAnalyzer integrates with clicky/ai for code quality analysis
type AIAnalyzer struct {
	agent           ai.Agent
	commentAnalyzer *CommentAnalyzer
	config          AIAnalyzerConfig
}

// AIAnalyzerConfig holds configuration for AI-powered analysis
type AIAnalyzerConfig struct {
	CommentAnalysis CommentAnalyzerConfig `json:"comment_analysis"`
	Model           string                `json:"model"`
	MaxConcurrent   int                   `json:"max_concurrent"`
	Debug           bool                  `json:"debug"`
}

// DefaultAIAnalyzerConfig returns default AI analyzer configuration
func DefaultAIAnalyzerConfig() AIAnalyzerConfig {
	return AIAnalyzerConfig{
		CommentAnalysis: DefaultCommentAnalyzerConfig(),
		Model:           "claude-3-haiku-20240307", // Low-cost model for analysis
		MaxConcurrent:   3,
		Debug:           false,
	}
}

// NewAIAnalyzer creates a new AI analyzer with clicky integration
func NewAIAnalyzer(config AIAnalyzerConfig) (*AIAnalyzer, error) {
	// Create AI agent configuration
	agentConfig := ai.AgentConfig{
		Type:          ai.AgentTypeClaude,
		Model:         config.Model,
		MaxConcurrent: config.MaxConcurrent,
		Debug:         config.Debug,
		Temperature:   0.1, // Low temperature for consistent analysis
	}

	// Create agent manager
	manager := ai.NewAgentManager(agentConfig)

	agent, err := manager.GetAgent(ai.AgentTypeClaude)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI agent: %w", err)
	}

	// Create comment analyzer
	commentAnalyzer := NewCommentAnalyzer(config.CommentAnalysis, agent)

	return &AIAnalyzer{
		agent:           agent,
		commentAnalyzer: commentAnalyzer,
		config:          config,
	}, nil
}

// NewAIAnalyzerWithAgent creates an AI analyzer with a provided agent
func NewAIAnalyzerWithAgent(agent ai.Agent, config AIAnalyzerConfig) *AIAnalyzer {
	commentAnalyzer := NewCommentAnalyzer(config.CommentAnalysis, agent)

	return &AIAnalyzer{
		agent:           agent,
		commentAnalyzer: commentAnalyzer,
		config:          config,
	}
}

// AnalysisResult holds the complete analysis results for a file
type AnalysisResult struct {
	FilePath          string                  `json:"file_path"`
	Language          string                  `json:"language"`
	LineCount         int                     `json:"line_count"`
	CommentResults    []*CommentQualityResult `json:"comment_results"`
	QualityViolations []models.Violation      `json:"quality_violations"`
	Summary           AnalysisSummary         `json:"summary"`
}

// AnalysisSummary provides a summary of the analysis
type AnalysisSummary struct {
	TotalComments       int     `json:"total_comments"`
	ComplexComments     int     `json:"complex_comments"`
	PoorQualityComments int     `json:"poor_quality_comments"`
	AverageScore        float64 `json:"average_score"`
	ViolationCount      int     `json:"violation_count"`
}

// AnalyzeFile performs complete AI-powered analysis on a file's AST
func (aa *AIAnalyzer) AnalyzeFile(ctx context.Context, ast *models.GenericAST) (*AnalysisResult, error) {
	result := &AnalysisResult{
		FilePath:          ast.FilePath,
		Language:          ast.Language,
		LineCount:         ast.LineCount,
		CommentResults:    []*CommentQualityResult{},
		QualityViolations: []models.Violation{},
	}

	// Analyze comments
	if aa.config.CommentAnalysis.Enabled {
		commentResults, err := aa.commentAnalyzer.AnalyzeAST(ctx, ast)
		if err != nil {
			logger.Warnf("Comment analysis failed for %s: %v", ast.FilePath, err)
		} else {
			result.CommentResults = commentResults
		}
	}

	// Generate violations from analysis results
	violations := aa.generateViolations(ast, result.CommentResults)
	result.QualityViolations = violations

	// Calculate summary
	result.Summary = aa.calculateSummary(result.CommentResults, violations)

	return result, nil
}

// AnalyzeFiles performs analysis on multiple files
func (aa *AIAnalyzer) AnalyzeFiles(ctx context.Context, asts []*models.GenericAST) ([]*AnalysisResult, error) {
	results := make([]*AnalysisResult, len(asts))

	for i, ast := range asts {
		result, err := aa.AnalyzeFile(ctx, ast)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze file %s: %w", ast.FilePath, err)
		}
		results[i] = result
	}

	return results, nil
}

// generateViolations converts analysis results to violations
func (aa *AIAnalyzer) generateViolations(ast *models.GenericAST, commentResults []*CommentQualityResult) []models.Violation {
	var violations []models.Violation

	// Generate violations from comment analysis
	poorComments := aa.commentAnalyzer.GetPoorQualityComments(commentResults)
	for _, result := range poorComments {
		violation := models.Violation{
			File:          ast.FilePath,
			Line:          result.Comment.StartLine,
			Column:        1,
			CallerPackage: ast.PackageName,
			CallerMethod:  result.Comment.Context,
			CalledPackage: "comment-quality",
			CalledMethod:  "analysis",
			Message:       aa.formatCommentViolationMessage(result),
			Source:        "ai-analyzer",
		}
		violations = append(violations, violation)
	}

	return violations
}

// formatCommentViolationMessage creates a readable violation message
func (aa *AIAnalyzer) formatCommentViolationMessage(result *CommentQualityResult) string {
	var issues []string

	if result.IsVerbose {
		issues = append(issues, "overly verbose")
	}
	if !result.IsDescriptive {
		issues = append(issues, "not descriptive")
	}
	if result.Score < aa.config.CommentAnalysis.MinDescriptiveScore {
		issues = append(issues, fmt.Sprintf("low quality score (%.2f)", result.Score))
	}

	message := fmt.Sprintf("Comment quality issues: %s", joinStrings(issues, ", "))

	if len(result.Issues) > 0 {
		message += fmt.Sprintf(" - Issues: %s", joinStrings(result.Issues, ", "))
	}

	if len(result.Suggestions) > 0 {
		message += fmt.Sprintf(" - Suggestions: %s", joinStrings(result.Suggestions, ", "))
	}

	return message
}

// calculateSummary calculates analysis summary statistics
func (aa *AIAnalyzer) calculateSummary(commentResults []*CommentQualityResult, violations []models.Violation) AnalysisSummary {
	summary := AnalysisSummary{
		ViolationCount: len(violations),
	}

	if len(commentResults) == 0 {
		return summary
	}

	summary.TotalComments = len(commentResults)

	var totalScore float64
	for _, result := range commentResults {
		totalScore += result.Score
		if !result.IsSimple {
			summary.ComplexComments++
		}
	}

	summary.AverageScore = totalScore / float64(len(commentResults))
	summary.PoorQualityComments = len(aa.commentAnalyzer.GetPoorQualityComments(commentResults))

	return summary
}

// GetCommentAnalyzer returns the comment analyzer for direct access
func (aa *AIAnalyzer) GetCommentAnalyzer() *CommentAnalyzer {
	return aa.commentAnalyzer
}

// Close closes the AI analyzer and its resources
func (aa *AIAnalyzer) Close() error {
	if aa.agent != nil {
		return aa.agent.Close()
	}
	return nil
}

// joinStrings is a utility function to join strings with a separator
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// ValidateConfig validates the AI analyzer configuration
func (config *AIAnalyzerConfig) Validate() error {
	if config.Model == "" {
		config.Model = "claude-3-haiku-20240307"
	}

	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = 3
	}

	if config.CommentAnalysis.WordLimit <= 0 {
		config.CommentAnalysis.WordLimit = 10
	}

	if config.CommentAnalysis.MinDescriptiveScore < 0 || config.CommentAnalysis.MinDescriptiveScore > 1 {
		config.CommentAnalysis.MinDescriptiveScore = 0.7
	}

	return nil
}
