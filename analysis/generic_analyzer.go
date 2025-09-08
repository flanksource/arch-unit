package analysis

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
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
		extractors: map[string]Extractor{
			".go": NewGoASTExtractor(),
			".py": NewPythonASTExtractor(),
			// TODO: Add other extractors after refactoring:
			// ".js":   NewJavaScriptASTExtractor(),
			// ".ts":   NewTypeScriptASTExtractor(),
			// ".tsx":  NewTypeScriptASTExtractor(),
			// ".jsx":  NewJavaScriptASTExtractor(),
			// ".md":   NewMarkdownASTExtractor(),
		},
	}
}

// AnalyzeFile analyzes a single file using the appropriate extractor and manages all DB operations
func (a *GenericAnalyzer) AnalyzeFile(task *clicky.Task, filepath string, content []byte) (*ASTResult, error) {
	// Check if file needs re-analysis
	needsAnalysis, err := a.cache.NeedsReanalysis(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to check if file needs analysis: %w", err)
	}

	if !needsAnalysis {
		task.Debugf("File %s is up to date, skipping analysis", filepath)
		return nil, nil // File is up to date
	}

	// Get appropriate extractor based on file extension
	extractor := a.getExtractor(filepath)
	if extractor == nil {
		task.Debugf("No extractor available for file %s", filepath)
		return nil, nil // No extractor for this file type
	}

	task.Debugf("Starting analysis of %s", filepath)

	// Clear existing AST data for the file
	if err := a.cache.DeleteASTForFile(filepath); err != nil {
		return nil, fmt.Errorf("failed to clear existing AST data: %w", err)
	}

	// Extract AST using the appropriate extractor (pure operation with read-only cache)
	result, err := extractor.ExtractFile(a.cache, filepath, content)
	if err != nil {
		return nil, fmt.Errorf("failed to extract AST from %s: %w", filepath, err)
	}

	if result == nil {
		task.Debugf("No AST data extracted from %s", filepath)
		return nil, nil
	}

	// Store all nodes and build node ID map
	nodeMap := make(map[string]int64)
	for _, node := range result.Nodes {
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
	ext := strings.ToLower(filepath[strings.LastIndex(filepath, "."):])
	return a.extractors[ext]
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
func (a *GenericAnalyzer) AnalyzeFileFromPath(task *clicky.Task, filepath string) (*ASTResult, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filepath, err)
	}

	return a.AnalyzeFile(task, filepath, content)
}