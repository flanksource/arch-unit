package javascript

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	flanksourceContext "github.com/flanksource/commons/context"
)

//go:embed javascript_ast_extractor.js
var javascriptASTExtractorScript string

// JavaScriptASTExtractor extracts AST information from JavaScript source files
type JavaScriptASTExtractor struct {
	filePath    string
	packageName string
	depsManager *NodeDependenciesManager
}

// NewJavaScriptASTExtractor creates a new JavaScript AST extractor
func NewJavaScriptASTExtractor() *JavaScriptASTExtractor {
	return &JavaScriptASTExtractor{
		depsManager: NewNodeDependenciesManager(),
	}
}

// JavaScriptASTNode represents a node in JavaScript AST
type JavaScriptASTNode struct {
	Type                 string               `json:"type"`
	Name                 string               `json:"name"`
	StartLine            int                  `json:"start_line"`
	EndLine              int                  `json:"end_line"`
	ParameterCount       int                  `json:"parameter_count"`
	ReturnCount          int                  `json:"return_count"`
	Parameters           []models.Parameter   `json:"parameters,omitempty"`
	ReturnValues         []models.ReturnValue `json:"return_values,omitempty"`
	CyclomaticComplexity int                  `json:"cyclomatic_complexity"`
	Parent               string               `json:"parent"`
	IsAsync              bool                 `json:"is_async"`
	IsGenerator          bool                 `json:"is_generator"`
	IsArrow              bool                 `json:"is_arrow"`
	ExportType           string               `json:"export_type"` // "default", "named", ""
}

// JavaScriptImport represents an import in JavaScript
type JavaScriptImport struct {
	Source   string   `json:"source"`
	Imported []string `json:"imported"`
	Line     int      `json:"line"`
	Type     string   `json:"type"` // "import", "require"
}

// JavaScriptRelationship represents a relationship between JavaScript entities
type JavaScriptRelationship struct {
	FromEntity string `json:"from_entity"`
	ToEntity   string `json:"to_entity"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
}

// JavaScriptASTResult contains the complete AST analysis result
type JavaScriptASTResult struct {
	Module        string                   `json:"module"`
	Nodes         []JavaScriptASTNode      `json:"nodes"`
	Imports       []JavaScriptImport       `json:"imports"`
	Relationships []JavaScriptRelationship `json:"relationships"`
}

// ExtractFile extracts AST information from a JavaScript file
func (e *JavaScriptASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*types.ASTResult, error) {
	e.filePath = filePath

	// Extract package name from file path or package.json
	e.packageName = e.extractPackageName(filePath)

	result := &types.ASTResult{
		Nodes:         []*models.ASTNode{},
		Relationships: []*models.ASTRelationship{},
	}

	// Run JavaScript AST extraction (write content to temp file for external tool)
	tempFile, err := os.CreateTemp("", "js_extract_*.js")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()

	if _, err := tempFile.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write content to temp file: %w", err)
	}
	_ = tempFile.Close()

	jsResult, err := e.runJavaScriptASTExtraction(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to extract JavaScript AST: %w", err)
	}

	// Convert JavaScript nodes to AST nodes
	nodeMap := make(map[string]string) // fullName -> node key
	for _, node := range jsResult.Nodes {
		astNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             "",
			MethodName:           "",
			FieldName:            "",
			NodeType:             e.mapJavaScriptNodeType(node.Type),
			StartLine:            node.StartLine,
			EndLine:              node.EndLine,
			LineCount:            node.EndLine - node.StartLine + 1,
			CyclomaticComplexity: node.CyclomaticComplexity,
			Parameters:           node.Parameters,
			ReturnValues:         node.ReturnValues,
			LastModified:         time.Now(),
		}

		// Set appropriate fields based on node type
		switch node.Type {
		case "class":
			astNode.TypeName = node.Name
		case "method":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
				astNode.MethodName = node.Name
			} else {
				astNode.MethodName = node.Name
			}
		case "function":
			astNode.MethodName = node.Name
		case "variable", "property":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
			}
			astNode.FieldName = node.Name
		}

		// Generate unique key for the node
		nodeKey := astNode.Key()
		fullName := e.getNodeFullName(node)
		nodeMap[fullName] = nodeKey

		result.Nodes = append(result.Nodes, astNode)
	}

	// Convert relationships
	for _, rel := range jsResult.Relationships {
		fromKey, fromExists := nodeMap[rel.FromEntity]
		if !fromExists {
			continue
		}

		var toKey *string
		if toNodeKey, toExists := nodeMap[rel.ToEntity]; toExists {
			toKey = &toNodeKey
		}

		// Look up existing node IDs from cache if available
		var fromID int64
		var toID *int64

		if id, exists := cache.GetASTId(fromKey); exists {
			fromID = id
		}

		if toKey != nil {
			if id, exists := cache.GetASTId(*toKey); exists {
				toID = &id
			}
		}

		astRel := &models.ASTRelationship{
			FromASTID:        fromID,
			ToASTID:          toID,
			LineNo:           rel.Line,
			RelationshipType: models.RelationshipType(e.mapRelationshipType(rel.Type)),
			Text:             rel.Text,
		}

		result.Relationships = append(result.Relationships, astRel)
	}

	return result, nil
}

// extractPackageName extracts package name from file path or package.json
func (e *JavaScriptASTExtractor) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)

	// Look for package.json with iteration limit to prevent infinite loops
	maxIterations := 20
	iteration := 0
	for currentDir := dir; currentDir != "/" && currentDir != "" && currentDir != "." && iteration < maxIterations; currentDir = filepath.Dir(currentDir) {
		iteration++

		// Additional safety check - if we haven't moved up, break
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}

		packageJSONPath := filepath.Join(currentDir, "package.json")
		if data, err := os.ReadFile(packageJSONPath); err == nil {
			var packageJSON map[string]interface{}
			if json.Unmarshal(data, &packageJSON) == nil {
				if name, ok := packageJSON["name"].(string); ok {
					return name
				}
			}
		}
	}

	// Default to directory structure
	parts := strings.Split(dir, string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "src" || parts[i] == "lib" || parts[i] == "app" {
			if i < len(parts)-1 {
				return strings.Join(parts[i+1:], "/")
			}
		}
	}

	// Default to last directory name
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "main"
}

// runJavaScriptASTExtraction runs the JavaScript AST extraction script
func (e *JavaScriptASTExtractor) runJavaScriptASTExtraction(filePath string) (*JavaScriptASTResult, error) {
	// Create parser script with proper module resolution
	ctx := flanksourceContext.NewContext(context.Background())
	scriptPath, err := e.depsManager.CreateParserScript(ctx, javascriptASTExtractorScript, "javascript")
	if err != nil {
		return nil, fmt.Errorf("failed to create parser script: %w", err)
	}
	defer func() { _ = os.Remove(scriptPath) }()

	// Execute the script with Node.js
	cmd := exec.Command("node", scriptPath, filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("JavaScript AST extraction failed: %w - output: %s", err, string(output))
	}

	// Parse JSON output
	var result JavaScriptASTResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JavaScript AST JSON: %w", err)
	}

	return &result, nil
}

// mapJavaScriptNodeType maps JavaScript node types to generic AST node types
func (e *JavaScriptASTExtractor) mapJavaScriptNodeType(jsType string) string {
	switch jsType {
	case "class":
		return models.NodeTypeType
	case "function", "method":
		return models.NodeTypeMethod
	case "variable", "property":
		return models.NodeTypeField
	default:
		return models.NodeTypeVariable
	}
}

// mapRelationshipType maps JavaScript relationship types to generic relationship types
func (e *JavaScriptASTExtractor) mapRelationshipType(jsRelType string) string {
	switch jsRelType {
	case "extends":
		return models.RelationshipExtends
	case "calls":
		return models.RelationshipCall
	case "imports":
		return models.RelationshipImport
	case "uses":
		return models.RelationshipReference
	default:
		return models.RelationshipReference
	}
}

// getNodeFullName returns the full qualified name of a JavaScript node
func (e *JavaScriptASTExtractor) getNodeFullName(node JavaScriptASTNode) string {
	parts := []string{e.packageName}

	if node.Parent != "" {
		parts = append(parts, node.Parent)
	}

	parts = append(parts, node.Name)
	return strings.Join(parts, ".")
}

func init() {
	// Register JavaScript AST extractor
	jsExtractor := NewJavaScriptASTExtractor()
	analysis.RegisterExtractor("javascript", jsExtractor)
}
