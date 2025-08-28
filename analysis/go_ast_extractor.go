package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	flanksourceContext "github.com/flanksource/commons/context"
)

// GoASTExtractor extracts AST information from Go source files
type GoASTExtractor struct {
	cache       *cache.ASTCache
	fileSet     *token.FileSet
	packageName string
	filePath    string
	imports     map[string]string // alias -> package path
}

// NewGoASTExtractor creates a new Go AST extractor
func NewGoASTExtractor(astCache *cache.ASTCache) *GoASTExtractor {
	return &GoASTExtractor{
		cache:   astCache,
		fileSet: token.NewFileSet(),
		imports: make(map[string]string),
	}
}

// ExtractFile extracts AST information from a Go file
func (e *GoASTExtractor) ExtractFile(ctx flanksourceContext.Context, filePath string) error {
	// Check if file needs re-analysis
	needsAnalysis, err := e.cache.NeedsReanalysis(filePath)
	if err != nil {
		return fmt.Errorf("failed to check if file needs analysis: %w", err)
	}

	if !needsAnalysis {
		return nil // File is up to date
	}

	// Parse the Go file
	src, err := parser.ParseFile(e.fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse Go file %s: %w", filePath, err)
	}

	e.filePath = filePath
	e.packageName = src.Name.Name
	e.imports = make(map[string]string)

	// Clear existing AST data for the file
	if err := e.cache.DeleteASTForFile(filePath); err != nil {
		return fmt.Errorf("failed to clear existing AST data: %w", err)
	}

	// Extract imports
	for _, imp := range src.Imports {
		e.extractImport(imp)
	}

	// Extract package-level declarations
	for _, decl := range src.Decls {
		if err := e.extractDeclaration(decl); err != nil {
			return fmt.Errorf("failed to extract declaration: %w", err)
		}
	}

	// Update file metadata
	if err := e.cache.UpdateFileMetadata(filePath); err != nil {
		return fmt.Errorf("failed to update file metadata: %w", err)
	}

	return nil
}

// extractImport processes import declarations
func (e *GoASTExtractor) extractImport(imp *ast.ImportSpec) {
	pkgPath := strings.Trim(imp.Path.Value, `"`)
	alias := ""

	if imp.Name != nil {
		alias = imp.Name.Name
	} else {
		// Use last part of package path as default alias
		parts := strings.Split(pkgPath, "/")
		alias = parts[len(parts)-1]
	}

	e.imports[alias] = pkgPath
}

// extractDeclaration processes top-level declarations
func (e *GoASTExtractor) extractDeclaration(decl ast.Decl) error {
	switch d := decl.(type) {
	case *ast.GenDecl:
		return e.extractGenDecl(d)
	case *ast.FuncDecl:
		return e.extractFuncDecl(d, "")
	}
	return nil
}

// extractGenDecl processes general declarations (types, variables, constants)
func (e *GoASTExtractor) extractGenDecl(decl *ast.GenDecl) error {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if err := e.extractTypeSpec(s); err != nil {
				return err
			}
		case *ast.ValueSpec:
			if err := e.extractValueSpec(s, decl.Tok == token.CONST); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractTypeSpec processes type declarations
func (e *GoASTExtractor) extractTypeSpec(spec *ast.TypeSpec) error {
	typeName := spec.Name.Name
	startPos := e.fileSet.Position(spec.Pos())
	endPos := e.fileSet.Position(spec.End())

	// Create type node
	typeNode := &models.ASTNode{
		FilePath:     e.filePath,
		PackageName:  e.packageName,
		TypeName:     typeName,
		NodeType:     models.NodeTypeType,
		StartLine:    startPos.Line,
		EndLine:      endPos.Line,
		LineCount:    endPos.Line - startPos.Line + 1,
		LastModified: time.Now(),
	}

	typeID, err := e.cache.StoreASTNode(typeNode)
	if err != nil {
		return fmt.Errorf("failed to store type node: %w", err)
	}

	// Extract struct fields if it's a struct
	if structType, ok := spec.Type.(*ast.StructType); ok {
		if err := e.extractStructFields(typeID, typeName, structType); err != nil {
			return err
		}
	}

	// Extract interface methods if it's an interface
	if interfaceType, ok := spec.Type.(*ast.InterfaceType); ok {
		if err := e.extractInterfaceMethods(typeID, typeName, interfaceType); err != nil {
			return err
		}
	}

	return nil
}

// extractStructFields processes struct fields
func (e *GoASTExtractor) extractStructFields(typeID int64, typeName string, structType *ast.StructType) error {
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			fieldNode := &models.ASTNode{
				FilePath:     e.filePath,
				PackageName:  e.packageName,
				TypeName:     typeName,
				FieldName:    name.Name,
				NodeType:     models.NodeTypeField,
				StartLine:    e.fileSet.Position(field.Pos()).Line,
				EndLine:      e.fileSet.Position(field.End()).Line,
				LastModified: time.Now(),
			}

			_, err := e.cache.StoreASTNode(fieldNode)
			if err != nil {
				return fmt.Errorf("failed to store field node: %w", err)
			}
		}
	}
	return nil
}

// extractInterfaceMethods processes interface methods
func (e *GoASTExtractor) extractInterfaceMethods(typeID int64, typeName string, interfaceType *ast.InterfaceType) error {
	for _, method := range interfaceType.Methods.List {
		if len(method.Names) > 0 {
			methodName := method.Names[0].Name

			methodNode := &models.ASTNode{
				FilePath:     e.filePath,
				PackageName:  e.packageName,
				TypeName:     typeName,
				MethodName:   methodName,
				NodeType:     models.NodeTypeMethod,
				StartLine:    e.fileSet.Position(method.Pos()).Line,
				EndLine:      e.fileSet.Position(method.End()).Line,
				LastModified: time.Now(),
			}

			if funcType, ok := method.Type.(*ast.FuncType); ok {
				methodNode.Parameters = e.extractParameters(funcType)
				methodNode.ReturnValues = e.extractReturnValues(funcType)
			}

			_, err := e.cache.StoreASTNode(methodNode)
			if err != nil {
				return fmt.Errorf("failed to store interface method node: %w", err)
			}
		}
	}
	return nil
}

// extractValueSpec processes variable and constant declarations
func (e *GoASTExtractor) extractValueSpec(spec *ast.ValueSpec, isConstant bool) error {
	for _, name := range spec.Names {
		if name.Name == "_" {
			continue // Skip blank identifiers
		}

		varNode := &models.ASTNode{
			FilePath:     e.filePath,
			PackageName:  e.packageName,
			FieldName:    name.Name,
			NodeType:     models.NodeTypeVariable,
			StartLine:    e.fileSet.Position(spec.Pos()).Line,
			EndLine:      e.fileSet.Position(spec.End()).Line,
			LastModified: time.Now(),
		}

		_, err := e.cache.StoreASTNode(varNode)
		if err != nil {
			return fmt.Errorf("failed to store variable node: %w", err)
		}
	}
	return nil
}

// extractFuncDecl processes function declarations
func (e *GoASTExtractor) extractFuncDecl(decl *ast.FuncDecl, receiverType string) error {
	funcName := decl.Name.Name
	startPos := e.fileSet.Position(decl.Pos())
	endPos := e.fileSet.Position(decl.End())

	// Determine if this is a method (has receiver) or function
	typeName := receiverType
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		typeName = e.getReceiverTypeName(decl.Recv.List[0].Type)
	}

	// Calculate cyclomatic complexity
	complexity := e.calculateCyclomaticComplexity(decl.Body)

	// Extract parameter details
	parameters := e.extractParameters(decl.Type)

	// Extract return value details
	returnValues := e.extractReturnValues(decl.Type)

	// Create function/method node
	funcNode := &models.ASTNode{
		FilePath:             e.filePath,
		PackageName:          e.packageName,
		TypeName:             typeName,
		MethodName:           funcName,
		NodeType:             models.NodeTypeMethod,
		StartLine:            startPos.Line,
		EndLine:              endPos.Line,
		LineCount:            endPos.Line - startPos.Line + 1,
		CyclomaticComplexity: complexity,
		Parameters:           parameters,
		ReturnValues:         returnValues,
		LastModified:         time.Now(),
	}

	funcID, err := e.cache.StoreASTNode(funcNode)
	if err != nil {
		return fmt.Errorf("failed to store function node: %w", err)
	}

	// Extract function calls and relationships
	if decl.Body != nil {
		if err := e.extractFunctionCalls(funcID, decl.Body); err != nil {
			return err
		}
	}

	return nil
}

// getReceiverTypeName extracts the receiver type name from receiver expression
func (e *GoASTExtractor) getReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return e.getReceiverTypeName(t.X)
	default:
		return ""
	}
}

// countParameters counts function parameters
func (e *GoASTExtractor) countParameters(funcType *ast.FuncType) int {
	if funcType.Params == nil {
		return 0
	}

	count := 0
	for _, param := range funcType.Params.List {
		if len(param.Names) == 0 {
			count++ // Unnamed parameter
		} else {
			count += len(param.Names)
		}
	}
	return count
}

// countReturns counts function return values
func (e *GoASTExtractor) countReturns(funcType *ast.FuncType) int {
	if funcType.Results == nil {
		return 0
	}

	count := 0
	for _, result := range funcType.Results.List {
		if len(result.Names) == 0 {
			count++ // Unnamed return
		} else {
			count += len(result.Names)
		}
	}
	return count
}

// extractParameters extracts detailed parameter information from a function type
func (e *GoASTExtractor) extractParameters(funcType *ast.FuncType) []models.Parameter {
	if funcType.Params == nil || len(funcType.Params.List) == 0 {
		return nil
	}

	var parameters []models.Parameter
	for _, param := range funcType.Params.List {
		paramType := e.getTypeString(param.Type)

		if len(param.Names) == 0 {
			// Unnamed parameter
			parameters = append(parameters, models.Parameter{
				Name:       "_",
				Type:       paramType,
				NameLength: 1,
			})
		} else {
			// Named parameters
			for _, name := range param.Names {
				parameters = append(parameters, models.Parameter{
					Name:       name.Name,
					Type:       paramType,
					NameLength: len(name.Name),
				})
			}
		}
	}
	return parameters
}

// extractReturnValues extracts return value information from a function type
func (e *GoASTExtractor) extractReturnValues(funcType *ast.FuncType) []models.ReturnValue {
	if funcType.Results == nil || len(funcType.Results.List) == 0 {
		return nil
	}

	var returnValues []models.ReturnValue
	for _, result := range funcType.Results.List {
		returnType := e.getTypeString(result.Type)

		if len(result.Names) == 0 {
			// Unnamed return
			returnValues = append(returnValues, models.ReturnValue{
				Name: "",
				Type: returnType,
			})
		} else {
			// Named returns
			for _, name := range result.Names {
				returnValues = append(returnValues, models.ReturnValue{
					Name: name.Name,
					Type: returnType,
				})
			}
		}
	}
	return returnValues
}

// getTypeString converts an AST expression to a type string
func (e *GoASTExtractor) getTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + e.getTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + e.getTypeString(t.Elt)
		}
		return "[...]" + e.getTypeString(t.Elt)
	case *ast.SliceExpr:
		return "[]" + e.getTypeString(t.X)
	case *ast.MapType:
		return "map[" + e.getTypeString(t.Key) + "]" + e.getTypeString(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + e.getTypeString(t.Value)
		case ast.RECV:
			return "<-chan " + e.getTypeString(t.Value)
		default:
			return "chan " + e.getTypeString(t.Value)
		}
	case *ast.FuncType:
		return "func(...)"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{...}"
	case *ast.SelectorExpr:
		// Package qualified type
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name + "." + t.Sel.Name
		}
		return e.getTypeString(t.X) + "." + t.Sel.Name
	case *ast.Ellipsis:
		return "..." + e.getTypeString(t.Elt)
	default:
		// Fallback for complex types
		return "interface{}"
	}
}

// calculateCyclomaticComplexity calculates cyclomatic complexity of a function
func (e *GoASTExtractor) calculateCyclomaticComplexity(body *ast.BlockStmt) int {
	if body == nil {
		return 1 // Base complexity
	}

	complexity := 1 // Base path
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
			*ast.TypeSwitchStmt, *ast.SelectStmt:
			complexity++
		case *ast.CaseClause:
			// Each case adds complexity (except default case)
			if clause, ok := n.(*ast.CaseClause); ok && clause.List != nil {
				complexity++
			}
		case *ast.CommClause:
			// Each comm case adds complexity
			if clause, ok := n.(*ast.CommClause); ok && clause.Comm != nil {
				complexity++
			}
		}
		return true
	})

	return complexity
}

// extractFunctionCalls extracts function calls and method invocations from function body
func (e *GoASTExtractor) extractFunctionCalls(funcID int64, body *ast.BlockStmt) error {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if err := e.extractCallExpr(funcID, node); err != nil {
				// Log error but continue processing
				fmt.Printf("Warning: failed to extract call expression: %v\n", err)
			}
		}
		return true
	})
	return nil
}

// extractCallExpr processes a function call expression
func (e *GoASTExtractor) extractCallExpr(funcID int64, call *ast.CallExpr) error {
	callLine := e.fileSet.Position(call.Pos()).Line
	callText := e.getCallExprText(call)

	// Determine what's being called
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Simple function call: funcName()
		return e.handleSimpleFunctionCall(funcID, callLine, callText, fun.Name)

	case *ast.SelectorExpr:
		// Method call or package function: receiver.method() or pkg.func()
		return e.handleSelectorCall(funcID, callLine, callText, fun)

	default:
		// Other call types (function literals, etc.)
		return e.storeGenericCall(funcID, callLine, callText)
	}
}

// handleSimpleFunctionCall handles calls to functions in the same package
func (e *GoASTExtractor) handleSimpleFunctionCall(funcID int64, line int, text, funcName string) error {
	// Try to find the function in the current package
	// For now, store as a generic call - we'll enhance this with cross-package resolution later
	return e.cache.StoreASTRelationship(funcID, nil, line, models.RelationshipCall, text)
}

// handleSelectorCall handles method calls and package function calls
func (e *GoASTExtractor) handleSelectorCall(funcID int64, line int, text string, sel *ast.SelectorExpr) error {
	methodName := sel.Sel.Name

	switch x := sel.X.(type) {
	case *ast.Ident:
		// Could be pkg.Function() or variable.Method()
		receiverName := x.Name

		// Check if it's a known import
		if pkgPath, isImport := e.imports[receiverName]; isImport {
			// This is an external library call
			return e.handleLibraryCall(funcID, line, text, pkgPath, "", methodName)
		}

		// Otherwise, it's likely a method call on a local variable/field
		return e.cache.StoreASTRelationship(funcID, nil, line, models.RelationshipCall, text)

	default:
		// More complex expressions
		return e.cache.StoreASTRelationship(funcID, nil, line, models.RelationshipCall, text)
	}
}

// handleLibraryCall processes calls to external libraries
func (e *GoASTExtractor) handleLibraryCall(funcID int64, line int, text, pkgPath, className, methodName string) error {
	// Determine framework/library type
	framework := e.classifyLibrary(pkgPath)

	// Store library node
	libraryID, err := e.cache.StoreLibraryNode(pkgPath, className, methodName, "", models.NodeTypeMethod, "go", framework)
	if err != nil {
		return fmt.Errorf("failed to store library node: %w", err)
	}

	// Store library relationship
	return e.cache.StoreLibraryRelationship(funcID, libraryID, line, models.RelationshipCall, text)
}

// classifyLibrary determines the framework/library category
func (e *GoASTExtractor) classifyLibrary(pkgPath string) string {
	// Standard library
	if !strings.Contains(pkgPath, "/") {
		return "stdlib"
	}

	// Common frameworks
	switch {
	case strings.Contains(pkgPath, "gin-gonic/gin"):
		return "gin"
	case strings.Contains(pkgPath, "gorilla/mux"):
		return "gorilla"
	case strings.Contains(pkgPath, "labstack/echo"):
		return "echo"
	case strings.Contains(pkgPath, "gorm.io"):
		return "gorm"
	case strings.Contains(pkgPath, "database/sql"):
		return "database"
	default:
		return "third-party"
	}
}

// storeGenericCall stores a generic call relationship
func (e *GoASTExtractor) storeGenericCall(funcID int64, line int, text string) error {
	return e.cache.StoreASTRelationship(funcID, nil, line, models.RelationshipCall, text)
}

// getCallExprText extracts text representation of a call expression
func (e *GoASTExtractor) getCallExprText(call *ast.CallExpr) string {
	startPos := e.fileSet.Position(call.Pos())

	// For now, return a basic representation
	// In a full implementation, we'd reconstruct the exact source text
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name + "()"
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			return ident.Name + "." + fun.Sel.Name + "()"
		}
	}

	return fmt.Sprintf("call@%d:%d", startPos.Line, startPos.Column)
}
