package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "embed"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	flanksourceContext "github.com/flanksource/commons/context"
)

// TypeScriptASTExtractor extracts AST information from TypeScript source files
type TypeScriptASTExtractor struct {
	filePath    string
	packageName string
	depsManager *NodeDependenciesManager
}

// NewTypeScriptASTExtractor creates a new TypeScript AST extractor
func NewTypeScriptASTExtractor() *TypeScriptASTExtractor {
	return &TypeScriptASTExtractor{
		depsManager: NewNodeDependenciesManager(),
	}
}

// TypeScriptASTNode represents a node in TypeScript AST
type TypeScriptASTNode struct {
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
	IsGeneric            bool                 `json:"is_generic"`
	IsAbstract           bool                 `json:"is_abstract"`
	IsReadonly           bool                 `json:"is_readonly"`
	TypeParams           []string             `json:"type_params"`
	Modifiers            []string             `json:"modifiers"`
}

// TypeScriptImport represents an import in TypeScript
type TypeScriptImport struct {
	Source    string   `json:"source"`
	Imported  []string `json:"imported"`
	TypesOnly bool     `json:"types_only"`
	Line      int      `json:"line"`
}

// TypeScriptRelationship represents a relationship between TypeScript entities
type TypeScriptRelationship struct {
	FromEntity string `json:"from_entity"`
	ToEntity   string `json:"to_entity"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
}

// TypeScriptASTResult contains the complete AST analysis result
type TypeScriptASTResult struct {
	Module        string                   `json:"module"`
	Nodes         []TypeScriptASTNode      `json:"nodes"`
	Imports       []TypeScriptImport       `json:"imports"`
	Relationships []TypeScriptRelationship `json:"relationships"`
}

// ExtractFile extracts AST information from a TypeScript file
func (e *TypeScriptASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*ASTResult, error) {
	e.filePath = filePath

	// Extract package name from file path or package.json
	e.packageName = e.extractPackageName(filePath)

	result := &ASTResult{
		Nodes:         []*models.ASTNode{},
		Relationships: []*models.ASTRelationship{},
	}

	// Run TypeScript AST extraction (write content to temp file for external tool)
	tempFile, err := os.CreateTemp("", "ts_extract_*.ts")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write content to temp file: %w", err)
	}
	_ = tempFile.Close()

	tsResult, err := e.runTypeScriptASTExtraction(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to extract TypeScript AST: %w", err)
	}

	// Convert TypeScript nodes to AST nodes
	nodeMap := make(map[string]string) // fullName -> node key
	for _, node := range tsResult.Nodes {
		astNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             "",
			MethodName:           "",
			FieldName:            "",
			NodeType:             e.mapTypeScriptNodeType(node.Type),
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
		case "class", "interface", "enum", "type":
			astNode.TypeName = node.Name
		case "method", "constructor":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
				astNode.MethodName = node.Name
			} else {
				astNode.MethodName = node.Name
			}
		case "function":
			astNode.MethodName = node.Name
		case "variable", "property", "field":
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
	for _, rel := range tsResult.Relationships {
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
func (e *TypeScriptASTExtractor) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)

	// Look for package.json
	for currentDir := dir; currentDir != "/" && currentDir != ""; currentDir = filepath.Dir(currentDir) {
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

// runTypeScriptASTExtraction runs the TypeScript AST extraction script
func (e *TypeScriptASTExtractor) runTypeScriptASTExtraction(filePath string) (*TypeScriptASTResult, error) {
	// Create parser script with proper module resolution
	ctx := flanksourceContext.NewContext(context.Background())
	scriptPath, err := e.depsManager.CreateParserScript(ctx, typescriptASTExtractorScript, "typescript")
	if err != nil {
		return nil, fmt.Errorf("failed to create parser script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Execute the script with Node.js
	cmd := exec.Command("node", scriptPath, filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("TypeScript AST extraction failed: %w - output: %s", err, string(output))
	}

	// Parse JSON output
	var result TypeScriptASTResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse TypeScript AST JSON: %w", err)
	}

	return &result, nil
}

// mapTypeScriptNodeType maps TypeScript node types to generic AST node types
func (e *TypeScriptASTExtractor) mapTypeScriptNodeType(tsType string) string {
	switch tsType {
	case "class", "interface", "enum", "type":
		return models.NodeTypeType
	case "function", "method", "constructor":
		return models.NodeTypeMethod
	case "variable", "property", "field":
		return models.NodeTypeField
	default:
		return models.NodeTypeVariable
	}
}

// mapRelationshipType maps TypeScript relationship types to generic relationship types
func (e *TypeScriptASTExtractor) mapRelationshipType(tsRelType string) string {
	switch tsRelType {
	case "extends":
		return models.RelationshipExtends
	case "implements":
		return models.RelationshipImplements
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

// getNodeFullName returns the full qualified name of a TypeScript node
func (e *TypeScriptASTExtractor) getNodeFullName(node TypeScriptASTNode) string {
	parts := []string{e.packageName}

	if node.Parent != "" {
		parts = append(parts, node.Parent)
	}

	parts = append(parts, node.Name)
	return strings.Join(parts, ".")
}

//go:embed typescript_ast_extractor.js
var typescriptASTExtractorScript string
