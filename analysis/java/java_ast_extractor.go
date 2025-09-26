package java

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

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	flanksourceContext "github.com/flanksource/commons/context"
)

//go:embed java_ast_extractor.jar
var javaASTExtractorJar []byte

// JavaASTExtractor extracts AST information from Java source files
type JavaASTExtractor struct {
	filePath    string
	packageName string
}

// NewJavaASTExtractor creates a new Java AST extractor
func NewJavaASTExtractor() *JavaASTExtractor {
	return &JavaASTExtractor{}
}

// JavaASTNode represents a node in Java AST
type JavaASTNode struct {
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
	Modifiers            []string             `json:"modifiers"`
	IsInterface          bool                 `json:"is_interface"`
	IsAbstract           bool                 `json:"is_abstract"`
	IsStatic             bool                 `json:"is_static"`
	IsPrivate            bool                 `json:"is_private"`
	IsPublic             bool                 `json:"is_public"`
	IsProtected          bool                 `json:"is_protected"`
	IsFinal              bool                 `json:"is_final"`
	SuperClass           string               `json:"super_class,omitempty"`
	Interfaces           []string             `json:"interfaces,omitempty"`
}

// JavaImport represents an import in Java
type JavaImport struct {
	Source   string `json:"source"`
	Line     int    `json:"line"`
	IsStatic bool   `json:"is_static"`
	IsWild   bool   `json:"is_wild"`
}

// JavaRelationship represents a relationship between Java AST nodes
type JavaRelationship struct {
	FromNode string `json:"from_node"`
	ToNode   string `json:"to_node"`
	Type     string `json:"type"`
	Line     int    `json:"line"`
}

// JavaASTResult represents the complete result from Java AST extraction
type JavaASTResult struct {
	Nodes         []JavaASTNode      `json:"nodes"`
	Imports       []JavaImport       `json:"imports"`
	Relationships []JavaRelationship `json:"relationships"`
	PackageName   string             `json:"package_name"`
	ClassName     string             `json:"class_name"`
}

// ExtractFile extracts AST information from a Java file
func (e *JavaASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*types.ASTResult, error) {
	e.filePath = filePath

	// Extract package name from file content or path
	e.packageName = e.extractPackageName(filePath, content)

	result := &types.ASTResult{
		Nodes:         []*models.ASTNode{},
		Relationships: []*models.ASTRelationship{},
	}

	// Run Java AST extraction (write content to temp file for external tool)
	tempFile, err := os.CreateTemp("", "java_extract_*.java")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()

	if _, err := tempFile.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write content to temp file: %w", err)
	}
	_ = tempFile.Close()

	javaResult, err := e.runJavaASTExtraction(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to extract Java AST: %w", err)
	}

	// Convert Java nodes to AST nodes
	nodeMap := make(map[string]string) // fullName -> node key

	for _, node := range javaResult.Nodes {
		astNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             "",
			MethodName:           "",
			FieldName:            "",
			NodeType:             e.mapJavaNodeType(node.Type),
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
		case "class", "interface", "enum":
			astNode.TypeName = node.Name
		case "method", "constructor":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
				astNode.MethodName = node.Name
			} else {
				astNode.MethodName = node.Name
			}
		case "field":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
			}
			astNode.FieldName = node.Name
		}

		// Generate unique key for the node
		nodeKey := astNode.Key()
		fullName := e.getNodeFullName(node)

		// Check cache for existing node ID
		if existingNodeID, found := cache.GetASTId(nodeKey); found {
			astNode.ID = existingNodeID
		}

		nodeMap[fullName] = nodeKey
		result.AddNode(astNode)
	}

	// Process relationships
	for _, rel := range javaResult.Relationships {
		fromKey := nodeMap[rel.FromNode]
		toKey := nodeMap[rel.ToNode]

		if fromKey != "" && toKey != "" {
			relationship := &models.ASTRelationship{
				LineNo:           rel.Line,
				RelationshipType: e.mapRelationshipType(rel.Type),
			}

			result.AddRelationship(relationship)
		}
	}

	// Process imports
	for _, imp := range javaResult.Imports {
		// Create library relationship for imports
		libRel := &models.LibraryRelationship{
			LineNo:           imp.Line,
			RelationshipType: e.mapImportType(imp),
		}

		result.AddLibrary(libRel)
	}

	return result, nil
}

// runJavaASTExtraction runs the Java AST extraction program
func (e *JavaASTExtractor) runJavaASTExtraction(filePath string) (*JavaASTResult, error) {
	_ = flanksourceContext.NewContext(context.Background())

	// Create temp JAR file
	tempJar, err := os.CreateTemp("", "java_ast_extractor_*.jar")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp jar file: %w", err)
	}
	defer func() { _ = os.Remove(tempJar.Name()) }()

	// Write embedded JAR to temp file
	if _, err := tempJar.Write(javaASTExtractorJar); err != nil {
		return nil, fmt.Errorf("failed to write JAR content: %w", err)
	}
	_ = tempJar.Close()

	// Execute the JAR with java
	cmd := exec.Command("java", "-jar", tempJar.Name(), filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Java AST extraction failed: %w - output: %s", err, string(output))
	}

	// Parse JSON output
	var result JavaASTResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Java AST JSON: %w - raw output: %s", err, string(output))
	}

	return &result, nil
}

// extractPackageName extracts the package name from file content or path
func (e *JavaASTExtractor) extractPackageName(filePath string, content []byte) string {
	// First try to extract from content
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") && strings.HasSuffix(line, ";") {
			packageDecl := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "package"), ";"))
			if packageDecl != "" {
				return packageDecl
			}
		}
	}

	// Fallback to extracting from file path
	dir := filepath.Dir(filePath)

	// Look for src/main/java pattern (Maven)
	if idx := strings.Index(dir, "src/main/java/"); idx >= 0 {
		packagePath := strings.TrimPrefix(dir[idx+len("src/main/java/"):], "/")
		return strings.ReplaceAll(packagePath, "/", ".")
	}

	// Look for src pattern (simple structure)
	if idx := strings.Index(dir, "src/"); idx >= 0 {
		packagePath := strings.TrimPrefix(dir[idx+len("src/"):], "/")
		return strings.ReplaceAll(packagePath, "/", ".")
	}

	// Use directory name as package
	return filepath.Base(dir)
}

// getNodeFullName creates a full name for a node for relationship mapping
func (e *JavaASTExtractor) getNodeFullName(node JavaASTNode) string {
	switch node.Type {
	case "class", "interface", "enum":
		if e.packageName != "" {
			return fmt.Sprintf("%s.%s", e.packageName, node.Name)
		}
		return node.Name
	case "method", "constructor":
		if node.Parent != "" {
			if e.packageName != "" {
				return fmt.Sprintf("%s.%s.%s", e.packageName, node.Parent, node.Name)
			}
			return fmt.Sprintf("%s.%s", node.Parent, node.Name)
		}
		return node.Name
	case "field":
		if node.Parent != "" {
			if e.packageName != "" {
				return fmt.Sprintf("%s.%s.%s", e.packageName, node.Parent, node.Name)
			}
			return fmt.Sprintf("%s.%s", node.Parent, node.Name)
		}
		return node.Name
	default:
		return node.Name
	}
}

// mapJavaNodeType maps Java node types to generic AST node types
func (e *JavaASTExtractor) mapJavaNodeType(javaType string) models.NodeType {
	switch javaType {
	case "class", "interface", "enum":
		return models.NodeTypeType
	case "method", "constructor":
		return models.NodeTypeMethod
	case "field":
		return models.NodeTypeField
	default:
		return models.NodeTypeVariable
	}
}

// mapRelationshipType maps Java relationship types to generic relationship types
func (e *JavaASTExtractor) mapRelationshipType(javaRelType string) models.RelationshipType {
	switch javaRelType {
	case "extends":
		return models.RelationshipTypeInheritance
	case "implements":
		return models.RelationshipTypeImplements
	case "calls":
		return models.RelationshipTypeCall
	case "uses":
		return models.RelationshipTypeReference
	default:
		return models.RelationshipTypeReference
	}
}

// mapImportType maps Java import information to import type
func (e *JavaASTExtractor) mapImportType(imp JavaImport) string {
	if imp.IsStatic {
		return "static"
	}
	if imp.IsWild {
		return "wildcard"
	}
	return "direct"
}
