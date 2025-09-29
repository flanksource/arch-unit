package java

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
)

var ARCHIE_HOME = os.ExpandEnv("$HOME/.arch-unit")

var tempJar = filepath.Join(ARCHIE_HOME, "java_ast_extractor.jar")

func init() {
	if err := os.WriteFile(tempJar, javaASTExtractorJar, 0644); err != nil {
		logger.Errorf("failed to write JAR content: %w", err)
	}
}

//
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
// Now using Go-compatible ASTNode structure directly
type JavaASTResult struct {
	Nodes         []*models.ASTNode  `json:"nodes"`
	Imports       []JavaImport       `json:"imports"`
	Relationships []JavaRelationship `json:"relationships"`
	PackageName   string             `json:"package_name"`
	ClassName     string             `json:"class_name"`
}

func (j JavaASTResult) String() string {
	s := fmt.Sprintf("%s.%s", j.PackageName, j.ClassName)
	s += fmt.Sprintf("\nNodes: %d", len(j.Nodes))
	s += fmt.Sprintf("\nImports: %d", len(j.Imports))
	s += fmt.Sprintf("\nRelationships: %d", len(j.Relationships))

	return s
}

// ExtractFile extracts AST information from a Java file
func (e *JavaASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*types.ASTResult, error) {
	e.filePath = filePath

	// Extract package name from file content or path
	e.packageName = e.extractPackageName(filePath, content)

	result := types.NewASTResult(filePath, "java")
	result.PackageName = e.packageName

	javaResult, err := e.runJavaASTExtraction(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract Java AST: %w", err)
	}

	// Java extractor now produces ASTNode-compatible JSON directly
	// Just need to set LastModified and check cache for existing IDs
	nodeMap := make(map[string]string) // fullName -> node key

	for _, astNode := range javaResult.Nodes {

		// Generate unique key for the node
		nodeKey := astNode.Key()
		fullName := e.getNodeFullName(astNode)

		// Check cache for existing node ID
		if existingNodeID, found := cache.GetASTId(nodeKey); found {
			astNode.ID = existingNodeID
		}

		nodeMap[fullName] = nodeKey
		result.AddNode(astNode)
	}

	// Process relationships
	for _, rel := range javaResult.Relationships {
		// For inheritance relationships, we need to find the actual node IDs
		var fromNodeID int64
		var toNodeID *int64

		// Find the from node by matching the relationship's fromNode against our nodeMap
		for fullName, nodeKey := range nodeMap {
			if fullName == rel.FromNode {
				// Find the actual node with this key
				for _, node := range result.Nodes {
					if node.Key() == nodeKey {
						fromNodeID = node.ID
						break
					}
				}
				break
			}
		}

		// Find the to node (may be external, so it's optional)
		// For inheritance, the ToNode might be a simple class name without package
		// Try to find it in our nodes first, otherwise leave as nil (external reference)
		for fullName, nodeKey := range nodeMap {
			if fullName == rel.ToNode || strings.HasSuffix(fullName, "."+rel.ToNode) {
				// Find the actual node with this key
				for _, node := range result.Nodes {
					if node.Key() == nodeKey {
						toNodeID = &node.ID
						break
					}
				}
				break
			}
		}

		relationship := &models.ASTRelationship{
			FromASTID:        fromNodeID,
			ToASTID:          toNodeID,
			LineNo:           rel.Line,
			RelationshipType: e.mapRelationshipType(rel.Type),
		}

		result.AddRelationship(relationship)
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

	logger.Tracef("[java] analyzing %s", filePath)

	// Execute the JAR with java
	cmd := exec.Command("java", "-jar", tempJar, filePath)
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
func (e *JavaASTExtractor) getNodeFullName(node *models.ASTNode) string {
	switch node.NodeType {
	case "type":
		if node.PackageName != "" {
			return fmt.Sprintf("%s.%s", node.PackageName, node.TypeName)
		}
		return node.TypeName
	case "method":
		if node.TypeName != "" {
			if node.PackageName != "" {
				return fmt.Sprintf("%s.%s.%s", node.PackageName, node.TypeName, node.MethodName)
			}
			return fmt.Sprintf("%s.%s", node.TypeName, node.MethodName)
		}
		return node.MethodName
	case "field":
		if node.TypeName != "" {
			if node.PackageName != "" {
				return fmt.Sprintf("%s.%s.%s", node.PackageName, node.TypeName, node.FieldName)
			}
			return fmt.Sprintf("%s.%s", node.TypeName, node.FieldName)
		}
		return node.FieldName
	default:
		if node.MethodName != "" {
			return node.MethodName
		} else if node.FieldName != "" {
			return node.FieldName
		} else if node.TypeName != "" {
			return node.TypeName
		}
		return ""
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
