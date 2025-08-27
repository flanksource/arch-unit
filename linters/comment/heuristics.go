package comment

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

// HeuristicType represents the type of comment quality issue
type HeuristicType string

const (
	HeuristicRedundant    HeuristicType = "redundant"
	HeuristicVerbose      HeuristicType = "verbose"
	HeuristicOutdated     HeuristicType = "outdated"
	HeuristicMissing      HeuristicType = "missing"
	HeuristicInconsistent HeuristicType = "inconsistent"
	HeuristicTrivial      HeuristicType = "trivial"
)

// HeuristicResult represents the result of a heuristic check
type HeuristicResult struct {
	Type        HeuristicType `json:"type"`
	Severity    string        `json:"severity"` // "error", "warning", "info"
	Message     string        `json:"message"`
	Suggestion  string        `json:"suggestion"` // What to do about it
	Confidence  float64       `json:"confidence"` // 0.0 to 1.0
	AutoFixable bool          `json:"auto_fixable"`
}

// CommentHeuristics provides comment quality analysis using heuristics
type CommentHeuristics struct {
	// Configuration
	MaxCommentWords     int     `json:"max_comment_words"`
	MinFunctionLength   int     `json:"min_function_length"`
	RedundancyThreshold float64 `json:"redundancy_threshold"`

	// Patterns for detecting issues
	trivialPatterns  []*regexp.Regexp
	redundantPhrases []string
	outdatedKeywords []string
}

// NewCommentHeuristics creates a new comment heuristics analyzer
func NewCommentHeuristics() *CommentHeuristics {
	h := &CommentHeuristics{
		MaxCommentWords:     50,
		MinFunctionLength:   10,
		RedundancyThreshold: 0.8,
		redundantPhrases: []string{
			"this function",
			"this method",
			"this variable",
			"getter for",
			"setter for",
			"returns the",
			"gets the",
			"sets the",
		},
		outdatedKeywords: []string{
			"TODO:",
			"FIXME:",
			"HACK:",
			"deprecated",
			"temporary",
			"quick fix",
		},
	}

	// Compile trivial comment patterns
	trivialPatternStrings := []string{
		`^//\s*$`,                         // Empty comments
		`^//\s*(TODO|FIXME|XXX|HACK)\s*$`, // Empty TODOs
		`^//\s*\w+\s*$`,                   // Single word comments
		`^//\s*(get|set|return|initialize)\s*\w*\s*$`, // Trivial action words
	}

	for _, pattern := range trivialPatternStrings {
		if re, err := regexp.Compile(pattern); err == nil {
			h.trivialPatterns = append(h.trivialPatterns, re)
		}
	}

	return h
}

// AnalyzeComment analyzes a single comment for quality issues
func (h *CommentHeuristics) AnalyzeComment(comment *models.Comment, context interface{}) ([]*HeuristicResult, error) {
	var results []*HeuristicResult

	// Skip empty or very short comments
	if comment.WordCount == 0 || strings.TrimSpace(comment.Text) == "" {
		return results, nil
	}

	// Check for trivial comments
	if trivialResult := h.checkTrivialComment(comment); trivialResult != nil {
		results = append(results, trivialResult)
	}

	// Check for redundant comments
	if redundantResult := h.checkRedundantComment(comment, context); redundantResult != nil {
		results = append(results, redundantResult)
	}

	// Check for verbose comments
	if verboseResult := h.checkVerboseComment(comment); verboseResult != nil {
		results = append(results, verboseResult)
	}

	// Check for outdated indicators
	if outdatedResult := h.checkOutdatedComment(comment); outdatedResult != nil {
		results = append(results, outdatedResult)
	}

	return results, nil
}

// AnalyzeFunction analyzes all comments associated with a function
func (h *CommentHeuristics) AnalyzeFunction(function *models.Function) ([]*HeuristicResult, error) {
	var results []*HeuristicResult

	// Analyze each comment
	for _, comment := range function.Comments {
		commentResults, err := h.AnalyzeComment(&comment, function)
		if err != nil {
			continue // Skip problematic comments
		}
		results = append(results, commentResults...)
	}

	// Check for missing documentation
	if missingResult := h.checkMissingDocumentation(function); missingResult != nil {
		results = append(results, missingResult)
	}

	return results, nil
}

// checkTrivialComment checks if a comment is trivial or empty
func (h *CommentHeuristics) checkTrivialComment(comment *models.Comment) *HeuristicResult {
	commentText := strings.TrimSpace(comment.Text)

	// Check against trivial patterns
	for _, pattern := range h.trivialPatterns {
		if pattern.MatchString(commentText) {
			return &HeuristicResult{
				Type:        HeuristicTrivial,
				Severity:    "info",
				Message:     "Trivial comment that adds no value",
				Suggestion:  "Remove this comment or make it more descriptive",
				Confidence:  0.9,
				AutoFixable: true,
			}
		}
	}

	// Check for comments that just repeat the code structure
	if h.isStructuralRepeat(commentText) {
		return &HeuristicResult{
			Type:        HeuristicTrivial,
			Severity:    "info",
			Message:     "Comment just repeats code structure",
			Suggestion:  "Remove or enhance with actual explanation of purpose",
			Confidence:  0.8,
			AutoFixable: true,
		}
	}

	return nil
}

// checkRedundantComment checks if a comment is redundant with the code
func (h *CommentHeuristics) checkRedundantComment(comment *models.Comment, context interface{}) *HeuristicResult {
	commentText := strings.ToLower(strings.TrimSpace(comment.Text))

	// Check for common redundant phrases
	for _, phrase := range h.redundantPhrases {
		if strings.Contains(commentText, phrase) {
			// Calculate similarity with code if we have function context
			similarity := h.calculateCodeSimilarity(comment, context)
			if similarity > h.RedundancyThreshold {
				return &HeuristicResult{
					Type:        HeuristicRedundant,
					Severity:    "warning",
					Message:     "Comment appears to be redundant with the code",
					Suggestion:  "Remove redundant comment or add meaningful explanation",
					Confidence:  similarity,
					AutoFixable: true,
				}
			}
		}
	}

	return nil
}

// checkVerboseComment checks if a comment is unnecessarily verbose
func (h *CommentHeuristics) checkVerboseComment(comment *models.Comment) *HeuristicResult {
	if comment.WordCount > h.MaxCommentWords {
		return &HeuristicResult{
			Type:        HeuristicVerbose,
			Severity:    "info",
			Message:     fmt.Sprintf("Comment is verbose (%d words, max %d recommended)", comment.WordCount, h.MaxCommentWords),
			Suggestion:  "Consider breaking into smaller, focused comments or simplifying",
			Confidence:  float64(comment.WordCount-h.MaxCommentWords) / float64(h.MaxCommentWords),
			AutoFixable: false, // Requires human judgment
		}
	}

	return nil
}

// checkOutdatedComment checks for indicators that a comment might be outdated
func (h *CommentHeuristics) checkOutdatedComment(comment *models.Comment) *HeuristicResult {
	commentText := strings.ToLower(comment.Text)

	for _, keyword := range h.outdatedKeywords {
		if strings.Contains(commentText, keyword) {
			severity := "info"
			if strings.Contains(commentText, "deprecated") || strings.Contains(commentText, "fixme") {
				severity = "warning"
			}

			return &HeuristicResult{
				Type:        HeuristicOutdated,
				Severity:    severity,
				Message:     fmt.Sprintf("Comment contains potentially outdated keyword: %s", keyword),
				Suggestion:  "Review and update or remove outdated comment",
				Confidence:  0.6,
				AutoFixable: false,
			}
		}
	}

	return nil
}

// checkMissingDocumentation checks if a function is missing documentation
func (h *CommentHeuristics) checkMissingDocumentation(function *models.Function) *HeuristicResult {
	// Only check exported functions (Go convention)
	if !function.IsExported {
		return nil
	}

	// Check if function is complex enough to warrant documentation
	if function.LineCount < h.MinFunctionLength && len(function.Parameters) <= 2 {
		return nil
	}

	// Check if we have any documentation comments
	hasDocumentation := false
	for _, comment := range function.Comments {
		if comment.Type == models.CommentTypeDocumentation || comment.WordCount > 5 {
			hasDocumentation = true
			break
		}
	}

	if !hasDocumentation {
		return &HeuristicResult{
			Type:        HeuristicMissing,
			Severity:    "warning",
			Message:     "Exported function lacks documentation",
			Suggestion:  "Add documentation describing the function's purpose, parameters, and return values",
			Confidence:  0.8,
			AutoFixable: false,
		}
	}

	return nil
}

// isStructuralRepeat checks if comment just repeats code structure
func (h *CommentHeuristics) isStructuralRepeat(comment string) bool {
	comment = strings.ToLower(comment)

	// Common structural repetition patterns
	structuralPhrases := []string{
		"function to",
		"method to",
		"class for",
		"struct for",
		"variable for",
		"field for",
	}

	for _, phrase := range structuralPhrases {
		if strings.Contains(comment, phrase) {
			return true
		}
	}

	return false
}

// calculateCodeSimilarity calculates similarity between comment and code
func (h *CommentHeuristics) calculateCodeSimilarity(comment *models.Comment, context interface{}) float64 {
	// Simplified similarity calculation
	// In a real implementation, this would be more sophisticated

	if function, ok := context.(*models.Function); ok {
		commentWords := strings.Fields(strings.ToLower(comment.Text))
		functionName := strings.ToLower(function.Name)

		// Count how many comment words appear in the function name
		matches := 0
		for _, word := range commentWords {
			if strings.Contains(functionName, word) && len(word) > 2 {
				matches++
			}
		}

		if len(commentWords) > 0 {
			return float64(matches) / float64(len(commentWords))
		}
	}

	return 0.0
}

// GenerateCommentFix generates a fix suggestion for a comment issue
func (h *CommentHeuristics) GenerateCommentFix(result *HeuristicResult, comment *models.Comment) *CommentFix {
	switch result.Type {
	case HeuristicTrivial, HeuristicRedundant:
		return &CommentFix{
			Type:            FixTypeRemove,
			OriginalComment: comment,
			NewComment:      nil, // Remove the comment
			Explanation:     result.Suggestion,
		}
	case HeuristicVerbose:
		return &CommentFix{
			Type:            FixTypeEdit,
			OriginalComment: comment,
			NewComment:      h.summarizeComment(comment),
			Explanation:     "Simplified verbose comment",
		}
	default:
		return &CommentFix{
			Type:            FixTypeNone,
			OriginalComment: comment,
			Explanation:     result.Suggestion,
		}
	}
}

// summarizeComment creates a shorter version of a verbose comment
func (h *CommentHeuristics) summarizeComment(comment *models.Comment) *models.Comment {
	// Simplified summarization - just take first sentence
	text := comment.Text
	if idx := strings.Index(text, "."); idx != -1 && idx < len(text)/2 {
		text = text[:idx+1]
	}

	return &models.Comment{
		Text:      strings.TrimSpace(text),
		StartLine: comment.StartLine,
		EndLine:   comment.EndLine,
		Type:      comment.Type,
		Context:   comment.Context,
		WordCount: models.CountWords(text),
	}
}
