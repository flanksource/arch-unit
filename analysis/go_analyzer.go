package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
)

// GoAnalyzer is a cache-unaware Go AST analyzer
type GoAnalyzer struct {
	*BaseAnalyzer
	depScanner *GoDependencyScanner
}

// NewGoAnalyzer creates a new Go AST analyzer
func NewGoAnalyzer() *GoAnalyzer {
	return &GoAnalyzer{
		BaseAnalyzer: NewBaseAnalyzer("go"),
		depScanner:   NewGoDependencyScanner(),
	}
}

// AnalyzeFile analyzes a Go source file and returns AST results
func (a *GoAnalyzer) AnalyzeFile(task *clicky.Task, filePath string, content []byte) (*ASTResult, error) {
	// Create result container
	result := NewASTResult(filePath, a.Language)
	
	// Check if this is a dependency file (go.mod or go.sum)
	if strings.HasSuffix(filePath, "go.mod") || strings.HasSuffix(filePath, "go.sum") {
		// Create scan context - use directory of the file as scan root
		ctx := NewScanContext(task, filepath.Dir(filePath))
		
		// Scan dependencies
		deps, err := a.depScanner.ScanFile(ctx, filePath, content)
		if err != nil {
			a.LogWarning(task, "Failed to scan dependencies from %s: %v", filePath, err)
			// Continue with empty dependencies rather than failing
		} else {
			for _, dep := range deps {
				result.AddDependency(dep)
			}
			a.LogProgress(task, "Found %d dependencies in %s", len(deps), filePath)
		}
		
		// For dependency files, we only extract dependencies, not AST
		return result, nil
	}
	
	// Parse the Go file for AST
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}
	
	// Extract package name
	result.PackageName = file.Name.Name
	// Debug: ("Package: %s", result.PackageName)
	
	// Extract imports
	// Don't overwrite the task name - keep showing the filename
	// task.SetStatus("Extracting imports")
	imports := a.extractImports(file)
	task.Debugf("Found %d imports", len(imports))
	
	// Extract types
	// Don't overwrite the task name - keep showing the filename
	// task.SetStatus("Extracting types")
	types := a.extractTypes(task, file, filePath, result.PackageName, fset)
	for _, node := range types {
		result.AddNode(node)
	}
	task.Debugf("Found %d types", len(types))
	
	// Extract functions and methods
	// Don't overwrite the task name - keep showing the filename
	// task.SetStatus("Extracting functions")
	methods := a.extractFunctions(task, file, filePath, result.PackageName, fset)
	for _, node := range methods {
		result.AddNode(node)
	}
	task.Debugf("Found %d functions/methods", len(methods))
	
	// Extract package-level variables and constants
	// Don't overwrite the task name - keep showing the filename
	// task.SetStatus("Extracting variables")
	vars := a.extractVariables(task, file, filePath, result.PackageName, fset)
	for _, node := range vars {
		result.AddNode(node)
	}
	task.Debugf("Found %d variables/constants", len(vars))
	
	// Extract relationships
	// Don't overwrite the task name - keep showing the filename
	// task.SetStatus("Analyzing relationships")
	relationships := a.extractRelationships(task, file, result.Nodes, imports)
	for _, rel := range relationships {
		result.AddRelationship(rel)
	}
	task.Debugf("Found %d relationships", len(relationships))
	
	// Extract library dependencies
	// Don't overwrite the task name - keep showing the filename
	// task.SetStatus("Analyzing dependencies")
	libraries := a.extractLibraries(imports, result.Nodes)
	for _, lib := range libraries {
		result.AddLibrary(lib)
	}
	task.Debugf("Found %d library dependencies", len(libraries))
	
	task.SetStatus("Analysis complete")
	return result, nil
}

// extractImports extracts import statements from the file
func (a *GoAnalyzer) extractImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		alias := ""
		
		if imp.Name != nil {
			alias = imp.Name.Name
		} else {
			// Extract package name from import path
			parts := strings.Split(path, "/")
			alias = parts[len(parts)-1]
		}
		
		imports[alias] = path
	}
	
	return imports
}

// extractTypes extracts type declarations (structs, interfaces, type aliases)
func (a *GoAnalyzer) extractTypes(task *clicky.Task, file *ast.File, filePath, packageName string, fset *token.FileSet) []*models.ASTNode {
	var nodes []*models.ASTNode
	
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.TypeSpec:
			position := fset.Position(node.Pos())
			endPosition := fset.Position(node.End())
			
			astNode := &models.ASTNode{
				FilePath:     filePath,
				PackageName:  packageName,
				TypeName:     node.Name.Name,
				NodeType:     models.NodeTypeType,
				StartLine:    position.Line,
				EndLine:      endPosition.Line,
				LineCount:    endPosition.Line - position.Line + 1,
				LastModified: time.Now(),
			}
			
			// Determine specific type
			switch t := node.Type.(type) {
			case *ast.StructType:
				// Extract struct fields
				if t.Fields != nil {
					for _, field := range t.Fields.List {
						for _, name := range field.Names {
							fieldPos := fset.Position(field.Pos())
							fieldEndPos := fset.Position(field.End())
							
							fieldNode := &models.ASTNode{
								FilePath:     filePath,
								PackageName:  packageName,
								TypeName:     node.Name.Name,
								FieldName:    name.Name,
								NodeType:     models.NodeTypeField,
								StartLine:    fieldPos.Line,
								EndLine:      fieldEndPos.Line,
								LineCount:    fieldEndPos.Line - fieldPos.Line + 1,
								LastModified: time.Now(),
							}
							nodes = append(nodes, fieldNode)
						}
					}
				}
			case *ast.InterfaceType:
				// Mark as interface
				astNode.NodeType = models.NodeTypeType
			}
			
			nodes = append(nodes, astNode)
		}
		return true
	})
	
	return nodes
}

// extractFunctions extracts function and method declarations
func (a *GoAnalyzer) extractFunctions(task *clicky.Task, file *ast.File, filePath, packageName string, fset *token.FileSet) []*models.ASTNode {
	var nodes []*models.ASTNode
	
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			position := fset.Position(fn.Pos())
			endPosition := fset.Position(fn.End())
			
			node := &models.ASTNode{
				FilePath:             filePath,
				PackageName:          packageName,
				MethodName:           fn.Name.Name,
				NodeType:             models.NodeTypeMethod,
				StartLine:            position.Line,
				EndLine:              endPosition.Line,
				LineCount:            endPosition.Line - position.Line + 1,
				CyclomaticComplexity: a.calculateComplexity(fn),
				LastModified:         time.Now(),
			}
			
			// If it's a method (has receiver), add type information
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				if t := a.extractReceiverType(fn.Recv.List[0].Type); t != "" {
					node.TypeName = t
				}
			}
			
			// Extract parameters
			if fn.Type.Params != nil {
				for _, param := range fn.Type.Params.List {
					for _, name := range param.Names {
						node.Parameters = append(node.Parameters, models.Parameter{
							Name: name.Name,
							Type: a.typeToString(param.Type),
						})
					}
				}
			}
			
			// Extract return values
			if fn.Type.Results != nil {
				for _, result := range fn.Type.Results.List {
					returnType := a.typeToString(result.Type)
					if len(result.Names) > 0 {
						for _, name := range result.Names {
							node.ReturnValues = append(node.ReturnValues, models.ReturnValue{
								Name: name.Name,
								Type: returnType,
							})
						}
					} else {
						node.ReturnValues = append(node.ReturnValues, models.ReturnValue{
							Type: returnType,
						})
					}
				}
			}
			
			nodes = append(nodes, node)
		}
	}
	
	return nodes
}

// extractVariables extracts package-level variables and constants
func (a *GoAnalyzer) extractVariables(task *clicky.Task, file *ast.File, filePath, packageName string, fset *token.FileSet) []*models.ASTNode {
	var nodes []*models.ASTNode
	
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				if val, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range val.Names {
						// Skip unexported variables
						if !ast.IsExported(name.Name) && name.Name != "_" {
							continue
						}
						
						position := fset.Position(val.Pos())
						endPosition := fset.Position(val.End())
						
						node := &models.ASTNode{
							FilePath:     filePath,
							PackageName:  packageName,
							FieldName:    name.Name,
							NodeType:     models.NodeTypeVariable,
							StartLine:    position.Line,
							EndLine:      endPosition.Line,
							LineCount:    endPosition.Line - position.Line + 1,
							LastModified: time.Now(),
						}
						
						nodes = append(nodes, node)
					}
				}
			}
		}
	}
	
	return nodes
}

// calculateComplexity calculates cyclomatic complexity for a function
func (a *GoAnalyzer) calculateComplexity(fn *ast.FuncDecl) int {
	complexity := 1 // Base complexity
	
	ast.Inspect(fn, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt:
			complexity++
		case *ast.CaseClause:
			complexity++
		}
		return true
	})
	
	return complexity
}

// countParameters counts the number of parameters in a function
func (a *GoAnalyzer) countParameters(fn *ast.FuncDecl) int {
	if fn.Type.Params == nil {
		return 0
	}
	
	count := 0
	for _, param := range fn.Type.Params.List {
		count += len(param.Names)
		if len(param.Names) == 0 {
			count++ // Unnamed parameter
		}
	}
	return count
}

// countReturns counts the number of return values in a function
func (a *GoAnalyzer) countReturns(fn *ast.FuncDecl) int {
	if fn.Type.Results == nil {
		return 0
	}
	
	count := 0
	for _, result := range fn.Type.Results.List {
		if len(result.Names) > 0 {
			count += len(result.Names)
		} else {
			count++ // Unnamed return
		}
	}
	return count
}

// extractReceiverType extracts the type name from a method receiver
func (a *GoAnalyzer) extractReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return a.extractReceiverType(t.X)
	}
	return ""
}

// typeToString converts an AST type expression to a string
func (a *GoAnalyzer) typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + a.typeToString(t.X)
	case *ast.ArrayType:
		return "[]" + a.typeToString(t.Elt)
	case *ast.SelectorExpr:
		return a.typeToString(t.X) + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", a.typeToString(t.Key), a.typeToString(t.Value))
	case *ast.ChanType:
		return "chan " + a.typeToString(t.Value)
	default:
		return "unknown"
	}
}

// extractRelationships analyzes relationships between nodes
func (a *GoAnalyzer) extractRelationships(task *clicky.Task, file *ast.File, nodes []*models.ASTNode, imports map[string]string) []*models.ASTRelationship {
	var relationships []*models.ASTRelationship
	
	// TODO: Implement relationship extraction
	// - Method calls
	// - Type embedding
	// - Interface implementation
	// - Import usage
	
	return relationships
}

// extractLibraries identifies external library dependencies
func (a *GoAnalyzer) extractLibraries(imports map[string]string, nodes []*models.ASTNode) []*models.LibraryRelationship {
	var libraries []*models.LibraryRelationship
	
	for _, path := range imports {
		// Skip standard library imports
		if !strings.Contains(path, ".") && !strings.Contains(path, "/") {
			continue
		}
		
		// Create a library node for this import
		libNode := &models.LibraryNode{
			Package:  path,
			NodeType: "package",
			Language: "go",
		}
		
		// Determine if it's a known framework
		if strings.HasPrefix(path, "github.com/gin-gonic/gin") {
			libNode.Framework = "gin"
		} else if strings.HasPrefix(path, "github.com/gorilla/mux") {
			libNode.Framework = "gorilla"
		} else if strings.HasPrefix(path, "github.com/spf13/cobra") {
			libNode.Framework = "cobra"
		}
		
		lib := &models.LibraryRelationship{
			RelationshipType: "import",
			Text:             path,
			LibraryNode:      libNode,
		}
		
		libraries = append(libraries, lib)
	}
	
	return libraries
}