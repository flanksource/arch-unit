package python

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

// PythonASTExtractor extracts AST information from Python source files
type PythonASTExtractor struct {
	filePath    string
	packageName string
}

// NewPythonASTExtractor creates a new Python AST extractor
func NewPythonASTExtractor() *PythonASTExtractor {
	return &PythonASTExtractor{}
}

// PythonASTNode represents a node in Python AST
type PythonASTNode struct {
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
	Decorators           []string             `json:"decorators"`
	BaseClasses          []string             `json:"base_classes"`
}

// PythonImport represents an import in Python
type PythonImport struct {
	Module string `json:"module"`
	Name   string `json:"name"`
	Alias  string `json:"alias"`
	Line   int    `json:"line"`
}

// PythonRelationship represents a relationship between Python entities
type PythonRelationship struct {
	FromEntity string `json:"from_entity"`
	ToEntity   string `json:"to_entity"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
}

// PythonASTResult contains the complete AST analysis result
type PythonASTResult struct {
	Module        string               `json:"module"`
	Nodes         []PythonASTNode      `json:"nodes"`
	Imports       []PythonImport       `json:"imports"`
	Relationships []PythonRelationship `json:"relationships"`
}

// ExtractFile extracts AST information from a Python file
func (e *PythonASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*types.ASTResult, error) {
	// Create result container
	result := types.NewASTResult(filePath, "python")

	e.filePath = filePath

	// Extract package name from file path
	e.packageName = e.extractPackageName(filePath)
	result.PackageName = e.packageName

	// Run Python AST extraction
	pythonResult, err := e.runPythonASTExtraction(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract Python AST: %w", err)
	}

	// Convert Python AST results to generic AST nodes
	for _, node := range pythonResult.Nodes {
		astNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             "",
			MethodName:           "",
			FieldName:            "",
			NodeType:             e.mapPythonNodeType(node.Type),
			StartLine:            node.StartLine,
			EndLine:              node.EndLine,
			LineCount:            node.EndLine - node.StartLine + 1,
			CyclomaticComplexity: node.CyclomaticComplexity,
			ParameterCount:       node.ParameterCount,
			ReturnCount:          node.ReturnCount,
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
		case "variable", "attribute":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
			}
			astNode.FieldName = node.Name
		}

		result.AddNode(astNode)
	}

	// Convert relationships
	for _, rel := range pythonResult.Relationships {
		astRel := &models.ASTRelationship{
			FromASTID:        0,   // Will be filled by analyzer
			ToASTID:          nil, // Will be resolved by analyzer if possible
			LineNo:           rel.Line,
			RelationshipType: models.RelationshipType(e.mapRelationshipType(rel.Type)),
			Text:             rel.Text,
		}
		result.AddRelationship(astRel)
	}

	// Convert imports to library relationships
	for _, imp := range pythonResult.Imports {
		libRel := &models.LibraryRelationship{
			ASTID:            0, // Will be filled by analyzer
			LibraryID:        0, // Will be resolved by analyzer
			LineNo:           imp.Line,
			RelationshipType: string(models.RelationshipImport),
			Text:             fmt.Sprintf("import %s (module=%s;alias=%s;framework=python)", imp.Module, imp.Module, imp.Alias),
		}
		result.Libraries = append(result.Libraries, libRel)
	}

	return result, nil
}

// extractPackageName extracts package name from file path
func (e *PythonASTExtractor) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)
	parts := strings.Split(dir, string(filepath.Separator))

	// Look for common Python package indicators
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "src" || parts[i] == "lib" || parts[i] == "app" {
			if i < len(parts)-1 {
				return strings.Join(parts[i+1:], ".")
			}
		}
		// Check for __init__.py in directory
		initPath := filepath.Join(strings.Join(parts[:i+1], string(filepath.Separator)), "__init__.py")
		if _, err := os.Stat(initPath); err == nil {
			return strings.Join(parts[i:], ".")
		}
	}

	// Default to last directory name
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "main"
}

// runPythonASTExtraction runs the Python AST extraction script
func (e *PythonASTExtractor) runPythonASTExtraction(filePath string) (*PythonASTResult, error) {
	// Create temp file with Python script
	tmpFile, err := os.CreateTemp("", "python_ast_*.py")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer func() { _ = tmpFile.Close() }()

	if _, err := tmpFile.WriteString(pythonASTExtractorScript); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}

	// Execute the script
	cmd := exec.Command("python3", tmpFile.Name(), filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try with python if python3 fails
		cmd = exec.Command("python", tmpFile.Name(), filePath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("python AST extraction failed: %w - output: %s", err, string(output))
		}
	}

	// Parse JSON output
	var result PythonASTResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Python AST JSON: %w", err)
	}

	return &result, nil
}

// mapPythonNodeType maps Python node types to generic AST node types
func (e *PythonASTExtractor) mapPythonNodeType(pythonType string) string {
	switch pythonType {
	case "class":
		return models.NodeTypeType
	case "function", "method":
		return models.NodeTypeMethod
	case "variable", "attribute":
		return models.NodeTypeField
	default:
		return models.NodeTypeVariable
	}
}

// mapRelationshipType maps Python relationship types to generic relationship types
func (e *PythonASTExtractor) mapRelationshipType(pythonRelType string) string {
	switch pythonRelType {
	case "inherits":
		return models.RelationshipInheritance
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

// pythonASTExtractorScript is the embedded Python script for AST extraction
//go:embed python_ast_extractor.py
var pythonASTExtractorScript string
