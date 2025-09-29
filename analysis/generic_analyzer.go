package analysis

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	flanksourceContext "github.com/flanksource/commons/context"
)

// GenericAnalyzer is a single analyzer that handles all languages
// It orchestrates the extraction process and manages all DB operations
type GenericAnalyzer struct {
	cache      *cache.ASTCache
	extractors map[string]Extractor
}

// NewGenericAnalyzer creates a new generic analyzer with extractors for all supported languages
func NewGenericAnalyzer(astCache *cache.ASTCache) *GenericAnalyzer {
	return &GenericAnalyzer{
		cache: astCache,
		// extractors will be looked up from the registry as needed
		extractors: make(map[string]Extractor),
	}
}

// AnalyzeFile analyzes a single file using the appropriate extractor and manages all DB operations
func (a *GenericAnalyzer) AnalyzeFile(task *clicky.Task, filepath string, content []byte) (*types.ASTResult, error) {
	// Check if file needs re-analysis
	needsAnalysis, err := a.cache.NeedsReanalysis(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to check if file needs analysis: %w", err)
	}


	if !needsAnalysis {
		task.Debugf("File %s is up to date, retrieving from cache", filepath)
		cachedResult, err := a.getCachedASTResult(filepath)
		if err != nil {
			task.Warnf("Failed to retrieve cached data for %s: %v, forcing fresh analysis", filepath, err)
			needsAnalysis = true
		} else if cachedResult != nil {
			task.Debugf("Successfully retrieved cached AST data for %s: %d nodes, %d relationships",
				filepath, len(cachedResult.Nodes), len(cachedResult.Relationships))
			return cachedResult, nil
		} else {
			task.Debugf("No cached AST data found for %s, forcing fresh analysis", filepath)
			needsAnalysis = true
		}
	}

	// Continue with fresh analysis if needed
	if !needsAnalysis {
		// This shouldn't happen, but just in case
		return nil, nil
	}

	// Get appropriate extractor based on file extension
	extractor := a.getExtractor(filepath)
	if extractor == nil {
		task.Debugf("No extractor available for file %s", filepath)
		return nil, nil // No extractor for this file type
	}

	task.Debugf("Starting fresh analysis of %s", filepath)


	// Extract AST using the appropriate extractor (pure operation with read-only cache)
	task.Debugf("Calling extractor for %s (type: %T)", filepath, extractor)
	result, err := extractor.ExtractFile(a.cache, filepath, content)
	if err != nil {
		return nil, fmt.Errorf("failed to extract AST from %s: %w", filepath, err)
	}

	if result == nil {
		task.Warnf("Extractor returned nil result for %s (this may indicate the extractor failed silently)", filepath)
		return nil, nil
	}


	task.Debugf("Extracted AST data from %s: %d nodes, %d relationships, %d libraries",
		filepath, len(result.Nodes), len(result.Relationships), len(result.Libraries))

	// Store all nodes and build node ID map
	nodeMap := make(map[string]int64)
	for _, node := range result.Nodes {
		// Skip nil nodes and log a warning
		if node == nil {
			task.Warnf("Skipping nil AST node found in results for %s", filepath)
			continue
		}

		nodeID, err := a.cache.StoreASTNode(node)
		if err != nil {
			return nil, fmt.Errorf("failed to store AST node: %w", err)
		}
		nodeMap[node.Key()] = nodeID
	}

	// Store relationships and update with actual node IDs
	for _, rel := range result.Relationships {
		// Update FromASTID based on node map
		if fromNode := a.findNodeForRelationship(rel, result.Nodes); fromNode != nil {
			if fromID, exists := nodeMap[fromNode.Key()]; exists {
				rel.FromASTID = fromID
			}
		}

		if err := a.cache.StoreASTRelationship(rel.FromASTID, rel.ToASTID, rel.LineNo, string(rel.RelationshipType), rel.Text); err != nil {
			return nil, fmt.Errorf("failed to store AST relationship: %w", err)
		}
	}

	// Store library relationships
	for _, libRel := range result.Libraries {
		// Update ASTID based on node map
		if fromNode := a.findNodeForLibraryRelationship(libRel, result.Nodes); fromNode != nil {
			if fromID, exists := nodeMap[fromNode.Key()]; exists {
				libRel.ASTID = fromID
			}
		}

		// Parse library info from Text field
		libInfo := a.parseLibraryInfo(libRel.Text)

		// Store or get library node
		libraryID, err := a.cache.StoreLibraryNode(
			libInfo["pkg"],
			libInfo["class"],
			libInfo["method"],
			"",
			models.NodeTypeMethod,
			"go", // TODO: make this dynamic based on file type
			libInfo["framework"],
		)
		if err != nil {
			return nil, fmt.Errorf("failed to store library node: %w", err)
		}

		// Store library relationship
		if err := a.cache.StoreLibraryRelationship(libRel.ASTID, libraryID, libRel.LineNo, string(libRel.RelationshipType), libRel.Text); err != nil {
			return nil, fmt.Errorf("failed to store library relationship: %w", err)
		}
	}

	// Update file metadata
	if err := a.cache.UpdateFileMetadata(filepath); err != nil {
		return nil, fmt.Errorf("failed to update file metadata: %w", err)
	}

	task.Infof("Analyzed %s: %d nodes, %d relationships, %d libraries",
		filepath, len(result.Nodes), len(result.Relationships), len(result.Libraries))

	return result, nil
}

// getExtractor returns the appropriate extractor for the given file path
func (a *GenericAnalyzer) getExtractor(filepath string) Extractor {
	// Check cache first
	ext := strings.ToLower(filepath[strings.LastIndex(filepath, "."):])
	if extractor, exists := a.extractors[ext]; exists {
		return extractor
	}

	// Look up in registry and cache the result
	if extractor, _, found := DefaultExtractorRegistry.GetExtractorForFile(filepath); found {
		a.extractors[ext] = extractor
		return extractor
	}

	return nil
}

// findNodeForRelationship finds the source node for a relationship
func (a *GenericAnalyzer) findNodeForRelationship(rel *models.ASTRelationship, nodes []*models.ASTNode) *models.ASTNode {
	// This is a simplified approach - in practice, you'd need to track which node
	// generated which relationship during extraction
	// For now, we'll use the first method node as a fallback
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeMethod {
			return node
		}
	}
	return nil
}

// findNodeForLibraryRelationship finds the source node for a library relationship
func (a *GenericAnalyzer) findNodeForLibraryRelationship(libRel *models.LibraryRelationship, nodes []*models.ASTNode) *models.ASTNode {
	// Similar to above, simplified approach
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeMethod {
			return node
		}
	}
	return nil
}

// parseLibraryInfo parses library information from the text field
func (a *GenericAnalyzer) parseLibraryInfo(text string) map[string]string {
	info := make(map[string]string)

	// Extract information from text like "call() (pkg=fmt;class=;method=Println;framework=stdlib)"
	startIdx := strings.Index(text, "(pkg=")
	if startIdx == -1 {
		return info
	}

	endIdx := strings.LastIndex(text, ")")
	if endIdx == -1 {
		return info
	}

	infoStr := text[startIdx+1 : endIdx] // Skip "(" and ")"
	pairs := strings.Split(infoStr, ";")

	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			info[parts[0]] = parts[1]
		}
	}

	return info
}

// Convenience method to analyze a file from file path
func (a *GenericAnalyzer) AnalyzeFileFromPath(task *clicky.Task, filepath string) (*types.ASTResult, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filepath, err)
	}

	return a.AnalyzeFile(task, filepath, content)
}

// AnalyzeFileWithRules analyzes a file and checks for rule violations
func (a *GenericAnalyzer) AnalyzeFileWithRules(task *clicky.Task, filepath string, content []byte, ruleSets []models.RuleSet) (*types.ASTResult, error) {
	// For rule checking, always force analysis (bypass cache)
	// Get appropriate extractor based on file extension
	extractor := a.getExtractor(filepath)
	if extractor == nil {
		return nil, nil // No extractor for this file type
	}

	// Extract AST directly
	result, err := extractor.ExtractFile(a.cache, filepath, content)
	if err != nil || result == nil {
		return result, err
	}

	// If no rules provided, return AST-only result
	if len(ruleSets) == 0 {
		return result, nil
	}

	// Check for rule violations
	ruleAnalyzer := NewRuleViolationAnalyzer()
	violations, err := ruleAnalyzer.AnalyzeViolations(result, ruleSets)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze rule violations: %w", err)
	}

	// Add violations to result
	result.Violations = violations
	result.ViolationCount = len(violations)

	return result, nil
}

// AnalyzeGoFiles analyzes multiple Go files and returns aggregated results
// This is a compatibility function for tests and existing code
func AnalyzeGoFiles(rootDir string, files []string, ruleSets []models.RuleSet) (*models.AnalysisResult, error) {
	cache := cache.MustGetASTCache()
	analyzer := NewGenericAnalyzer(cache)

	result := &models.AnalysisResult{
		FileCount: len(files),
	}

	// Create a proper task for file analysis
	task := clicky.StartTask("analyze-files-batch", func(ctx flanksourceContext.Context, t *clicky.Task) (*models.AnalysisResult, error) {
		for _, file := range files {
			// Get rules for this file
			ruleCount := 0
			for _, ruleSet := range ruleSets {
				ruleCount += len(ruleSet.Rules)
			}
			result.RuleCount += ruleCount

			// Read file content
			content, err := os.ReadFile(file)
			if err != nil {
				// Skip files that can't be read
				continue
			}

			// Analyze with rules
			astResult, err := analyzer.AnalyzeFileWithRules(t, file, content, ruleSets)
			if err != nil {
				// Skip files with analysis errors
				continue
			}

		// Add violations to overall result
		if astResult != nil {
			result.Violations = append(result.Violations, astResult.Violations...)
		}
		}
		return result, nil
	})

	// Wait for task completion and get the result
	taskResult, err := task.GetResult()
	if err != nil {
		return nil, err
	}

	return taskResult, nil
}

// getCachedASTResult retrieves AST data from cache and reconstructs it as types.ASTResult
func (a *GenericAnalyzer) getCachedASTResult(filepath string) (*types.ASTResult, error) {
	// Get file nodes from cache
	nodes, err := a.cache.GetASTNodesByFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached nodes: %w", err)
	}

	// If no nodes exist, there's no cached data
	if len(nodes) == 0 {
		return nil, nil
	}

	// Create ASTResult from cached data
	language := a.detectLanguageFromPath(filepath)
	result := types.NewASTResult(filepath, language)

	// Add all cached nodes
	for _, node := range nodes {
		result.AddNode(node)
	}

	// Get relationships for each node
	for _, node := range nodes {
		// Get all relationships for this node (as source)
		nodeRelationships, err := a.cache.GetASTRelationships(node.ID, "")
		if err != nil {
			// Log warning but continue
			fmt.Printf("Warning: failed to get relationships for node %d: %v\n", node.ID, err)
		} else {
			for _, rel := range nodeRelationships {
				result.AddRelationship(rel)
			}
		}

		// Get library relationships for this node
		libRelationships, err := a.cache.GetLibraryRelationships(node.ID, "")
		if err != nil {
			// Log warning but continue
			fmt.Printf("Warning: failed to get library relationships for node %d: %v\n", node.ID, err)
		} else {
			for _, libRel := range libRelationships {
				result.AddLibrary(libRel)
			}
		}
	}

	// Set package name from first node if available
	if len(nodes) > 0 && nodes[0].PackageName != "" {
		result.PackageName = nodes[0].PackageName
	}

	return result, nil
}

// detectLanguageFromPath detects language from file path or virtual path
func (a *GenericAnalyzer) detectLanguageFromPath(filepath string) string {
	// Handle virtual paths
	if strings.HasPrefix(filepath, "sql://") {
		return "sql"
	}
	if strings.HasPrefix(filepath, "openapi://") {
		return "openapi"
	}
	if strings.HasPrefix(filepath, "virtual://") {
		// Extract type from virtual path: virtual://type/identifier
		parts := strings.Split(strings.TrimPrefix(filepath, "virtual://"), "/")
		if len(parts) > 0 {
			return parts[0]
		}
		return "custom"
	}

	// Handle regular file extensions
	switch {
	case strings.HasSuffix(filepath, ".go"):
		return "go"
	case strings.HasSuffix(filepath, ".py"):
		return "python"
	case strings.HasSuffix(filepath, ".js") || strings.HasSuffix(filepath, ".jsx") ||
		 strings.HasSuffix(filepath, ".mjs") || strings.HasSuffix(filepath, ".cjs"):
		return "javascript"
	case strings.HasSuffix(filepath, ".ts") || strings.HasSuffix(filepath, ".tsx"):
		return "typescript"
	case strings.HasSuffix(filepath, ".java"):
		return "java"
	case strings.HasSuffix(filepath, ".rs"):
		return "rust"
	case strings.HasSuffix(filepath, ".sql"):
		return "sql"
	default:
		return ""
	}
}
