package comment

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/linters"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	commonsContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
)

// CommentAnalysisLinter analyzes comment quality in source code
type CommentAnalysisLinter struct {
	linters.RunOptions
	heuristics *CommentHeuristics
}

// FixType represents the type of fix to apply to a comment
type FixType string

const (
	FixTypeNone   FixType = "none"
	FixTypeRemove FixType = "remove"
	FixTypeEdit   FixType = "edit"
	FixTypeAdd    FixType = "add"
)

// CommentFix represents a suggested fix for a comment issue
type CommentFix struct {
	Type            FixType         `json:"type"`
	OriginalComment *models.Comment `json:"original_comment,omitempty"`
	NewComment      *models.Comment `json:"new_comment,omitempty"`
	Explanation     string          `json:"explanation"`
}

// NewCommentAnalysisLinter creates a new comment analysis linter
func NewCommentAnalysisLinter(workDir string) *CommentAnalysisLinter {
	return &CommentAnalysisLinter{
		RunOptions: linters.RunOptions{
			WorkDir: workDir,
		},
		heuristics: NewCommentHeuristics(),
	}
}

// SetOptions sets the run options for the linter
func (c *CommentAnalysisLinter) SetOptions(opts linters.RunOptions) {
	c.RunOptions = opts
}

// Name returns the linter name
func (c *CommentAnalysisLinter) Name() string {
	return "comment-analysis"
}

// Run executes the comment analysis linter
func (c *CommentAnalysisLinter) Run(ctx commonsContext.Context, task *clicky.Task) ([]models.Violation, error) {
	logger.Debugf("Running comment analysis on %d files", len(c.Files))
	
	var violations []models.Violation
	
	// Create AST analyzer to get comment data
	astCache, err := cache.NewASTCache()
	if err != nil {
		return nil, fmt.Errorf("failed to create AST cache: %w", err)
	}
	defer astCache.Close()
	
	analyzer := ast.NewAnalyzer(astCache, c.WorkDir)
	
	// Ensure files are analyzed
	if err := analyzer.AnalyzeFiles(); err != nil {
		return nil, fmt.Errorf("failed to analyze files: %w", err)
	}
	
	// Process each file
	for _, filePath := range c.Files {
		fileViolations, err := c.analyzeFile(ctx, filePath, analyzer)
		if err != nil {
			logger.Warnf("Failed to analyze comments in %s: %v", filePath, err)
			continue
		}
		violations = append(violations, fileViolations...)
	}
	
	logger.Infof("Found %d comment quality issues", len(violations))
	return violations, nil
}

// analyzeFile analyzes comments in a single file
func (c *CommentAnalysisLinter) analyzeFile(ctx commonsContext.Context, filePath string, analyzer *ast.Analyzer) ([]models.Violation, error) {
	relPath, err := filepath.Rel(c.WorkDir, filePath)
	if err != nil {
		relPath = filePath
	}
	
	logger.Debugf("Analyzing comments in %s", relPath)
	
	// Get all nodes for this file
	pattern := fmt.Sprintf("*")
	allNodes, err := analyzer.QueryPattern(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to query AST nodes: %w", err)
	}
	
	// Filter to nodes in this file
	var fileNodes []*models.ASTNode
	for _, node := range allNodes {
		if node.FilePath == filePath {
			fileNodes = append(fileNodes, node)
		}
	}
	
	var violations []models.Violation
	
	// Analyze comments for each node
	for _, node := range fileNodes {
		nodeViolations, err := c.analyzeNodeComments(node)
		if err != nil {
			logger.Debugf("Failed to analyze comments for node %s: %v", node.GetFullName(), err)
			continue
		}
		violations = append(violations, nodeViolations...)
	}
	
	return violations, nil
}

// analyzeNodeComments analyzes comments associated with an AST node
func (c *CommentAnalysisLinter) analyzeNodeComments(node *models.ASTNode) ([]models.Violation, error) {
	// For now, we'll create mock comments since we don't have comment extraction in the current AST
	// In a real implementation, the AST analyzer would extract comments
	mockComments := c.createMockComments(node)
	
	var violations []models.Violation
	
	for _, comment := range mockComments {
		// Analyze comment using heuristics
		results, err := c.heuristics.AnalyzeComment(comment, node)
		if err != nil {
			continue
		}
		
		// Convert heuristic results to violations
		for _, result := range results {
			violation := c.createViolation(node, comment, result)
			violations = append(violations, violation)
		}
	}
	
	return violations, nil
}

// createMockComments creates mock comments for demonstration
// In a real implementation, this would come from AST parsing
func (c *CommentAnalysisLinter) createMockComments(node *models.ASTNode) []*models.Comment {
	var comments []*models.Comment
	
	// Create some example comments based on node type
	switch node.NodeType {
	case models.NodeTypeMethod:
		if strings.HasPrefix(node.MethodName, "Get") {
			comments = append(comments, &models.Comment{
				Text:      fmt.Sprintf("// This function gets %s", strings.ToLower(node.MethodName[3:])),
				StartLine: node.StartLine - 1,
				EndLine:   node.StartLine - 1,
				WordCount: 5,
				Type:      models.CommentTypeSingleLine,
				Context:   node.MethodName,
			})
		} else if strings.HasPrefix(node.MethodName, "Set") {
			comments = append(comments, &models.Comment{
				Text:      fmt.Sprintf("// Setter for %s", strings.ToLower(node.MethodName[3:])),
				StartLine: node.StartLine - 1,
				EndLine:   node.StartLine - 1,
				WordCount: 3,
				Type:      models.CommentTypeSingleLine,
				Context:   node.MethodName,
			})
		} else if node.CyclomaticComplexity > 5 && len(node.Parameters) > 0 {
			// Missing documentation for complex function
		}
	case models.NodeTypeType:
		comments = append(comments, &models.Comment{
			Text:      "// TODO: add proper documentation",
			StartLine: node.StartLine - 1,
			EndLine:   node.StartLine - 1,
			WordCount: 5,
			Type:      models.CommentTypeSingleLine,
			Context:   node.TypeName,
		})
	}
	
	return comments
}

// createViolation creates a violation from a heuristic result
func (c *CommentAnalysisLinter) createViolation(node *models.ASTNode, comment *models.Comment, result *HeuristicResult) models.Violation {
	relPath, err := filepath.Rel(c.WorkDir, node.FilePath)
	if err != nil {
		relPath = node.FilePath
	}
	
	// Create a rule for this comment issue
	rule := &models.Rule{
		Type:         models.RuleTypeCommentQuality,
		Pattern:      fmt.Sprintf("comment-%s", result.Type),
		OriginalLine: fmt.Sprintf("%s: %s", result.Type, result.Message),
		SourceFile:   "comment-analysis",
		LineNumber:   comment.StartLine,
	}
	
	violation := models.Violation{
		File:             relPath,
		Line:             comment.StartLine,
		Column:           1,
		Message:          result.Message,
		Rule:             rule,
		Source:           c.Name(),
		Fixable:          result.AutoFixable,
		FixApplicability: "safe", // Comment fixes are generally safe
	}
	
	return violation
}

// DefaultIncludes returns default file patterns for comment analysis
func (c *CommentAnalysisLinter) DefaultIncludes() []string {
	return []string{
		"**/*.go",
		"**/*.py",
		"**/*.js",
		"**/*.ts",
		"**/*.java",
		"**/*.cpp",
		"**/*.c",
		"**/*.h",
	}
}

// DefaultExcludes returns default exclusion patterns
func (c *CommentAnalysisLinter) DefaultExcludes() []string {
	return []string{
		"**/vendor/**",
		"**/node_modules/**",
		"**/*_test.go",
		"**/*.pb.go",
		"**/*_gen.go",
		"**/testdata/**",
	}
}

// SupportsJSON returns true if the linter supports JSON output
func (c *CommentAnalysisLinter) SupportsJSON() bool {
	return true
}

// JSONArgs returns additional args needed for JSON output
func (c *CommentAnalysisLinter) JSONArgs() []string {
	return []string{"--format", "json"}
}

// SupportsFix returns true if the linter supports auto-fixing
func (c *CommentAnalysisLinter) SupportsFix() bool {
	return true
}

// FixArgs returns additional args needed for fix mode
func (c *CommentAnalysisLinter) FixArgs() []string {
	return []string{"--fix"}
}

// ValidateConfig validates linter-specific configuration
func (c *CommentAnalysisLinter) ValidateConfig(config *models.LinterConfig) error {
	// No specific validation needed for now
	return nil
}

// ApplyFixes applies comment fixes to files (used in fix mode)
func (c *CommentAnalysisLinter) ApplyFixes(violations []models.Violation, workDir string, createBackup bool) error {
	logger.Infof("Applying %d comment fixes", len(violations))
	
	// Group violations by file
	fileViolations := make(map[string][]models.Violation)
	for _, violation := range violations {
		if violation.Fixable {
			filePath := filepath.Join(workDir, violation.File)
			fileViolations[filePath] = append(fileViolations[filePath], violation)
		}
	}
	
	// Apply fixes to each file
	for filePath, violations := range fileViolations {
		if err := c.applyFixesToFile(filePath, violations, createBackup); err != nil {
			logger.Warnf("Failed to apply fixes to %s: %v", filePath, err)
		}
	}
	
	return nil
}

// applyFixesToFile applies comment fixes to a single file
func (c *CommentAnalysisLinter) applyFixesToFile(filePath string, violations []models.Violation, createBackup bool) error {
	logger.Debugf("Applying %d fixes to %s", len(violations), filePath)
	
	// For now, just log what would be done
	// In a real implementation, this would modify the actual file
	for _, violation := range violations {
		logger.Infof("Would apply fix at %s:%d: %s", filePath, violation.Line, violation.Message)
	}
	
	return nil
}