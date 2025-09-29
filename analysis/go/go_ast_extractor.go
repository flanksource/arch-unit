package _go

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"time"
	"unicode"

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

// GoASTExtractor extracts AST information from Go source files
type GoASTExtractor struct {
	fileSet     *token.FileSet
	packageName string
	filePath    string
	imports     map[string]string // alias -> package path
}

// NewGoASTExtractor creates a new Go AST extractor
func NewGoASTExtractor() *GoASTExtractor {
	return &GoASTExtractor{
		fileSet: token.NewFileSet(),
		imports: make(map[string]string),
	}
}

// ExtractFile extracts AST information from a Go file
func (e *GoASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*types.ASTResult, error) {
	// Create result container
	result := types.NewASTResult(filePath, "go")

	// Parse the Go file
	src, err := parser.ParseFile(e.fileSet, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file %s: %w", filePath, err)
	}

	e.filePath = filePath
	e.packageName = src.Name.Name
	result.PackageName = e.packageName
	e.imports = make(map[string]string)

	// Extract imports
	for _, imp := range src.Imports {
		e.extractImport(imp)
	}

	// Extract package-level declarations
	for _, decl := range src.Decls {
		if err := e.extractDeclaration(cache, decl, result); err != nil {
			return nil, fmt.Errorf("failed to extract declaration: %w", err)
		}
	}

	return result, nil
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
func (e *GoASTExtractor) extractDeclaration(cache cache.ReadOnlyCache, decl ast.Decl, result *types.ASTResult) error {
	switch d := decl.(type) {
	case *ast.GenDecl:
		return e.extractGenDecl(cache, d, result)
	case *ast.FuncDecl:
		return e.extractFuncDecl(cache, d, "", result)
	}
	return nil
}

// extractGenDecl processes general declarations (types, variables, constants)
func (e *GoASTExtractor) extractGenDecl(cache cache.ReadOnlyCache, decl *ast.GenDecl, result *types.ASTResult) error {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if err := e.extractTypeSpec(cache, s, result); err != nil {
				return err
			}
		case *ast.ValueSpec:
			if err := e.extractValueSpec(cache, s, decl.Tok == token.CONST, result); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractTypeSpec processes type declarations
func (e *GoASTExtractor) extractTypeSpec(cache cache.ReadOnlyCache, spec *ast.TypeSpec, result *types.ASTResult) error {
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
		IsPrivate:    e.isPrivate(typeName),
		LastModified: time.Now(),
	}

	result.AddNode(typeNode)

	// Extract struct fields if it's a struct
	if structType, ok := spec.Type.(*ast.StructType); ok {
		if err := e.extractStructFields(cache, typeNode, typeName, structType, result); err != nil {
			return err
		}
	}

	// Extract interface methods if it's an interface
	if interfaceType, ok := spec.Type.(*ast.InterfaceType); ok {
		if err := e.extractInterfaceMethods(cache, typeNode, typeName, interfaceType, result); err != nil {
			return err
		}
	}

	return nil
}

// extractStructFields processes struct fields
func (e *GoASTExtractor) extractStructFields(cache cache.ReadOnlyCache, parentNode *models.ASTNode, typeName string, structType *ast.StructType, result *types.ASTResult) error {
	for _, field := range structType.Fields.List {
		// Get field type with full qualified name
		fieldType := e.getFullQualifiedTypeString(field.Type)

		// Extract default value from struct tag if present
		var defaultValue *string
		if field.Tag != nil {
			tagValue := strings.Trim(field.Tag.Value, "`")
			if defaultVal := e.extractDefaultFromTag(tagValue); defaultVal != "" {
				defaultValue = &defaultVal
			}
		}

		for _, name := range field.Names {
			fieldNode := &models.ASTNode{
				FilePath:     e.filePath,
				PackageName:  e.packageName,
				TypeName:     typeName,
				FieldName:    name.Name,
				NodeType:     models.NodeTypeField,
				StartLine:    e.fileSet.Position(field.Pos()).Line,
				EndLine:      e.fileSet.Position(field.End()).Line,
				FieldType:    &fieldType,
				DefaultValue: defaultValue,
				IsPrivate:    e.isPrivate(name.Name),
				LastModified: time.Now(),
			}

			result.AddNode(fieldNode)
		}
	}
	return nil
}

// extractInterfaceMethods processes interface methods
func (e *GoASTExtractor) extractInterfaceMethods(cache cache.ReadOnlyCache, parentNode *models.ASTNode, typeName string, interfaceType *ast.InterfaceType, result *types.ASTResult) error {
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
				IsPrivate:    e.isPrivate(methodName),
				LastModified: time.Now(),
			}

			if funcType, ok := method.Type.(*ast.FuncType); ok {
				methodNode.Parameters = e.extractParameters(funcType)
				methodNode.ReturnValues = e.extractReturnValues(funcType)
				methodNode.ParameterCount = len(methodNode.Parameters)
				methodNode.ReturnCount = len(methodNode.ReturnValues)
			}

			result.AddNode(methodNode)
		}
	}
	return nil
}

// extractValueSpec processes variable and constant declarations
func (e *GoASTExtractor) extractValueSpec(cache cache.ReadOnlyCache, spec *ast.ValueSpec, isConstant bool, result *types.ASTResult) error {
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
			IsPrivate:    e.isPrivate(name.Name),
			LastModified: time.Now(),
		}

		result.AddNode(varNode)
	}
	return nil
}

// extractFuncDecl processes function declarations
func (e *GoASTExtractor) extractFuncDecl(cache cache.ReadOnlyCache, decl *ast.FuncDecl, receiverType string, result *types.ASTResult) error {
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
		ParameterCount:       len(parameters),
		ReturnCount:          len(returnValues),
		IsPrivate:            e.isPrivate(funcName),
		LastModified:         time.Now(),
	}

	result.AddNode(funcNode)

	// Extract function calls and relationships
	if decl.Body != nil {
		if err := e.extractFunctionCalls(cache, funcNode, decl.Body, result); err != nil {
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
// countReturns counts function return values
func (e *GoASTExtractor) extractParameters(funcType *ast.FuncType) []models.Parameter {
	if funcType.Params == nil || len(funcType.Params.List) == 0 {
		return nil
	}

	var parameters []models.Parameter
	for _, param := range funcType.Params.List {
		paramType := e.getFullQualifiedTypeString(param.Type)

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
		returnType := e.getFullQualifiedTypeString(result.Type)

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

// getFullQualifiedTypeString converts an AST expression to a fully qualified type string
func (e *GoASTExtractor) getFullQualifiedTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		// Check if this is a primitive type
		if e.isPrimitiveType(t.Name) {
			return t.Name
		}
		// For non-primitive types in the same package, prefix with package name
		if e.packageName != "" {
			return e.packageName + "." + t.Name
		}
		return t.Name
	case *ast.StarExpr:
		return "*" + e.getFullQualifiedTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + e.getFullQualifiedTypeString(t.Elt)
		}
		return "[...]" + e.getFullQualifiedTypeString(t.Elt)
	case *ast.SliceExpr:
		return "[]" + e.getFullQualifiedTypeString(t.X)
	case *ast.MapType:
		return "map[" + e.getFullQualifiedTypeString(t.Key) + "]" + e.getFullQualifiedTypeString(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + e.getFullQualifiedTypeString(t.Value)
		case ast.RECV:
			return "<-chan " + e.getFullQualifiedTypeString(t.Value)
		default:
			return "chan " + e.getFullQualifiedTypeString(t.Value)
		}
	case *ast.FuncType:
		return "func(...)"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{...}"
	case *ast.SelectorExpr:
		// Package qualified type - resolve the full package path
		if ident, ok := t.X.(*ast.Ident); ok {
			// Check if this is a known import
			if pkgPath, exists := e.imports[ident.Name]; exists {
				return pkgPath + "." + t.Sel.Name
			}
			return ident.Name + "." + t.Sel.Name
		}
		return e.getFullQualifiedTypeString(t.X) + "." + t.Sel.Name
	case *ast.Ellipsis:
		return "..." + e.getFullQualifiedTypeString(t.Elt)
	default:
		// Fallback for complex types
		return "interface{}"
	}
}

// isPrimitiveType checks if a type name represents a Go primitive type
func (e *GoASTExtractor) isPrimitiveType(typeName string) bool {
	primitives := map[string]bool{
		"bool":       true,
		"byte":       true,
		"complex64":  true,
		"complex128": true,
		"error":      true,
		"float32":    true,
		"float64":    true,
		"int":        true,
		"int8":       true,
		"int16":      true,
		"int32":      true,
		"int64":      true,
		"rune":       true,
		"string":     true,
		"uint":       true,
		"uint8":      true,
		"uint16":     true,
		"uint32":     true,
		"uint64":     true,
		"uintptr":    true,
	}
	return primitives[typeName]
}

// extractDefaultFromTag extracts default value from struct tag
func (e *GoASTExtractor) extractDefaultFromTag(tag string) string {
	// Look for common default value patterns in tags
	// Examples: `json:"name" default:"value"` or `default:"value"`

	// Split by spaces and look for default:"value" pattern
	parts := strings.Fields(tag)
	for _, part := range parts {
		if strings.HasPrefix(part, "default:") {
			// Extract value from default:"value"
			if strings.Contains(part, ":") {
				defaultPart := strings.SplitN(part, ":", 2)[1]
				// Remove quotes
				defaultPart = strings.Trim(defaultPart, `"'`)
				return defaultPart
			}
		}
	}

	// Also check for validate tag with default values
	for _, part := range parts {
		if strings.HasPrefix(part, "validate:") && strings.Contains(part, "default=") {
			// Extract from validate:"required,default=value"
			if idx := strings.Index(part, "default="); idx != -1 {
				remaining := part[idx+8:] // len("default=") = 8
				// Find end of default value (either comma or end of string)
				endIdx := strings.Index(remaining, ",")
				if endIdx == -1 {
					endIdx = len(remaining)
				}
				defaultVal := remaining[:endIdx]
				defaultVal = strings.Trim(defaultVal, `"'`)
				return defaultVal
			}
		}
	}

	return ""
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
func (e *GoASTExtractor) extractFunctionCalls(cache cache.ReadOnlyCache, funcNode *models.ASTNode, body *ast.BlockStmt, result *types.ASTResult) error {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if err := e.extractCallExpr(cache, funcNode, node, result); err != nil {
				// Log error but continue processing
				fmt.Printf("Warning: failed to extract call expression: %v\n", err)
			}
		}
		return true
	})
	return nil
}

// extractCallExpr processes a function call expression
func (e *GoASTExtractor) extractCallExpr(cache cache.ReadOnlyCache, funcNode *models.ASTNode, call *ast.CallExpr, result *types.ASTResult) error {
	callLine := e.fileSet.Position(call.Pos()).Line
	callText := e.getCallExprText(call)

	// Determine what's being called
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Simple function call: funcName()
		return e.handleSimpleFunctionCall(cache, funcNode, callLine, callText, fun.Name, result)

	case *ast.SelectorExpr:
		// Method call or package function: receiver.method() or pkg.func()
		return e.handleSelectorCall(cache, funcNode, callLine, callText, fun, result)

	default:
		// Other call types (function literals, etc.)
		return e.storeGenericCall(funcNode, callLine, callText, result)
	}
}

// handleSimpleFunctionCall handles calls to functions in the same package
func (e *GoASTExtractor) handleSimpleFunctionCall(cache cache.ReadOnlyCache, funcNode *models.ASTNode, line int, text, funcName string, result *types.ASTResult) error {
	// Try to find the function in the current package using cache lookup
	targetKey := fmt.Sprintf("%s/%s:%s", e.filePath, "", funcName) // Empty type name for package-level functions
	if targetID, exists := cache.GetASTId(targetKey); exists {
		// Create relationship with known target
		rel := &models.ASTRelationship{
			FromASTID:        0, // Will be filled when funcNode gets its ID
			ToASTID:          &targetID,
			LineNo:           line,
			RelationshipType: models.RelationshipCall,
			Text:             text,
		}
		result.AddRelationship(rel)
	} else {
		// Store as generic call - target not yet known
		rel := &models.ASTRelationship{
			FromASTID:        0, // Will be filled when funcNode gets its ID
			ToASTID:          nil,
			LineNo:           line,
			RelationshipType: models.RelationshipCall,
			Text:             text,
		}
		result.AddRelationship(rel)
	}
	return nil
}

// handleSelectorCall handles method calls and package function calls
func (e *GoASTExtractor) handleSelectorCall(cache cache.ReadOnlyCache, funcNode *models.ASTNode, line int, text string, sel *ast.SelectorExpr, result *types.ASTResult) error {
	methodName := sel.Sel.Name

	switch x := sel.X.(type) {
	case *ast.Ident:
		// Could be pkg.Function() or variable.Method()
		receiverName := x.Name

		// Check if it's a known import
		if pkgPath, isImport := e.imports[receiverName]; isImport {

			// This is an external library call
			return e.handleLibraryCall(funcNode, line, text, pkgPath, "", methodName, result)
		}

		// Otherwise, it's likely a method call on a local variable/field
		rel := &models.ASTRelationship{
			FromASTID:        0, // Will be filled when funcNode gets its ID
			ToASTID:          nil,
			LineNo:           line,
			RelationshipType: models.RelationshipCall,
			Text:             text,
		}
		result.AddRelationship(rel)

	default:
		// More complex expressions
		rel := &models.ASTRelationship{
			FromASTID:        0, // Will be filled when funcNode gets its ID
			ToASTID:          nil,
			LineNo:           line,
			RelationshipType: models.RelationshipCall,
			Text:             text,
		}
		result.AddRelationship(rel)
	}
	return nil
}

// handleLibraryCall processes calls to external libraries
func (e *GoASTExtractor) handleLibraryCall(funcNode *models.ASTNode, line int, text, pkgPath, className, methodName string, result *types.ASTResult) error {
	// Determine framework/library type
	framework := e.classifyLibrary(pkgPath)

	// Create library relationship (will be stored by analyzer)
	libraryRel := &models.LibraryRelationship{
		ASTID:            0, // Will be filled when funcNode gets its ID
		LibraryID:        0, // Will be resolved by analyzer using package/class/method info
		LineNo:           line,
		RelationshipType: models.RelationshipCall,
		Text:             fmt.Sprintf("%s (pkg=%s;class=%s;method=%s;framework=%s)", text, pkgPath, className, methodName, framework),
	}
	result.Libraries = append(result.Libraries, libraryRel)
	return nil
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
func (e *GoASTExtractor) storeGenericCall(funcNode *models.ASTNode, line int, text string, result *types.ASTResult) error {
	rel := &models.ASTRelationship{
		FromASTID:        0, // Will be filled when funcNode gets its ID
		ToASTID:          nil,
		LineNo:           line,
		RelationshipType: models.RelationshipCall,
		Text:             text,
	}
	result.AddRelationship(rel)
	return nil
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

// isPrivate determines if a Go identifier is private (unexported)
// In Go, names starting with lowercase letters are unexported (private)
func (e *GoASTExtractor) isPrivate(name string) bool {
	if name == "" {
		return false
	}
	// Find the first letter (skip underscores)
	for _, r := range name {
		if unicode.IsLetter(r) {
			return !unicode.IsUpper(r)
		}
		// If it's not a letter and not underscore, consider it public
		if r != '_' {
			return false
		}
	}
	// If only underscores or no letters, consider it private
	return true
}
