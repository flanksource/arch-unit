package client

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

type GoAnalyzer struct {
	fileSet  *token.FileSet
	imports  map[string]string
	pkgName  string
	filePath string
}

func NewGoAnalyzer() *GoAnalyzer {
	return &GoAnalyzer{
		fileSet: token.NewFileSet(),
		imports: make(map[string]string),
	}
}

func (a *GoAnalyzer) AnalyzeFile(filePath string, rules *models.RuleSet) ([]models.Violation, error) {
	src, err := parser.ParseFile(a.fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}
	
	a.filePath = filePath
	a.pkgName = src.Name.Name
	a.imports = make(map[string]string)
	
	// Collect imports
	for _, imp := range src.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if imp.Name != nil {
			a.imports[imp.Name.Name] = path
		} else {
			parts := strings.Split(path, "/")
			name := parts[len(parts)-1]
			a.imports[name] = path
		}
	}
	
	// Find violations
	var violations []models.Violation
	ast.Inspect(src, func(n ast.Node) bool {
		if violation := a.checkNode(n, rules); violation != nil {
			violations = append(violations, *violation)
		}
		return true
	})
	
	return violations, nil
}

func (a *GoAnalyzer) checkNode(n ast.Node, rules *models.RuleSet) *models.Violation {
	if rules == nil {
		return nil
	}
	
	// Only check CallExpr nodes - this handles both method calls and field access
	// when the field is being called as a function
	if call, ok := n.(*ast.CallExpr); ok {
		return a.checkCallExpr(call, rules)
	}
	
	return nil
}

func (a *GoAnalyzer) checkCallExpr(call *ast.CallExpr, rules *models.RuleSet) *models.Violation {
	var pkgName, methodName string
	
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		methodName = fn.Sel.Name
		
		if ident, ok := fn.X.(*ast.Ident); ok {
			pkgName = a.resolvePackage(ident.Name)
		} else if sel, ok := fn.X.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				pkgName = a.resolvePackage(ident.Name)
				methodName = sel.Sel.Name + "." + methodName
			}
		}
		
	case *ast.Ident:
		// Function call in same package
		pkgName = a.pkgName
		methodName = fn.Name
	}
	
	if pkgName == "" {
		return nil
	}
	
	allowed, rule := rules.IsAllowedForFile(pkgName, methodName, a.filePath)
	if !allowed {
		pos := a.fileSet.Position(call.Pos())
		violationMsg := fmt.Sprintf("Call to %s.%s violates architecture rule", pkgName, methodName)
		if rule.FilePattern != "" {
			violationMsg = fmt.Sprintf("Call to %s.%s violates file-specific rule [%s]", pkgName, methodName, rule.FilePattern)
		}
		return &models.Violation{
			File:          a.filePath,
			Line:          pos.Line,
			Column:        pos.Column,
			CallerPackage: a.pkgName,
			CallerMethod:  a.getCurrentFunction(call),
			CalledPackage: pkgName,
			CalledMethod:  methodName,
			Rule:          rule,
			Message:       violationMsg,
		}
	}
	
	return nil
}



func (a *GoAnalyzer) resolvePackage(name string) string {
	if pkg, ok := a.imports[name]; ok {
		return pkg
	}
	
	// Check if it's a type in the current package
	if strings.ToUpper(name[:1]) == name[:1] {
		return a.pkgName
	}
	
	return name
}

func (a *GoAnalyzer) getCurrentFunction(node ast.Node) string {
	// This is simplified - in a real implementation, we'd track the current function context
	return "unknown"
}

// ExportAST implements the ASTExporter interface to convert Go AST to GenericAST
func (a *GoAnalyzer) ExportAST(filePath string) (*models.GenericAST, error) {
	src, err := parser.ParseFile(a.fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	// Count total lines in file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	lineCount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineCount++
	}

	genericAST := &models.GenericAST{
		Language:    "go",
		FilePath:    filePath,
		LineCount:   lineCount,
		PackageName: src.Name.Name,
		Functions:   []models.Function{},
		Types:       []models.TypeDefinition{},
		Variables:   []models.Variable{},
		Comments:    []models.Comment{},
		Imports:     []models.Import{},
	}

	// Extract imports
	for _, imp := range src.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		
		genericImport := models.Import{
			Path:  importPath,
			Alias: alias,
			Line:  a.fileSet.Position(imp.Pos()).Line,
		}
		genericAST.Imports = append(genericAST.Imports, genericImport)
	}

	// Extract comments
	for _, commentGroup := range src.Comments {
		for _, comment := range commentGroup.List {
			pos := a.fileSet.Position(comment.Pos())
			endPos := a.fileSet.Position(comment.End())
			
			// Determine comment type
			commentType := models.CommentTypeSingleLine
			if strings.HasPrefix(comment.Text, "/*") {
				commentType = models.CommentTypeMultiLine
			}
			if strings.HasPrefix(comment.Text, "///") || strings.HasPrefix(comment.Text, "/**") {
				commentType = models.CommentTypeDocumentation
			}

			genericComment := models.NewComment(
				comment.Text,
				pos.Line,
				endPos.Line,
				commentType,
				"file-level",
			)
			genericAST.Comments = append(genericAST.Comments, genericComment)
		}
	}

	// Extract declarations
	for _, decl := range src.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			function := a.extractFunction(d)
			genericAST.Functions = append(genericAST.Functions, function)
		case *ast.GenDecl:
			a.extractGenDecl(d, genericAST)
		}
	}

	return genericAST, nil
}

// extractFunction converts ast.FuncDecl to models.Function
func (a *GoAnalyzer) extractFunction(fn *ast.FuncDecl) models.Function {
	startPos := a.fileSet.Position(fn.Pos())
	endPos := a.fileSet.Position(fn.End())
	
	function := models.Function{
		Name:       fn.Name.Name,
		NameLength: len(fn.Name.Name),
		StartLine:  startPos.Line,
		EndLine:    endPos.Line,
		LineCount:  endPos.Line - startPos.Line + 1,
		Parameters: []models.Parameter{},
		Comments:   []models.Comment{},
		IsExported: fn.Name.IsExported(),
	}

	// Extract parameters
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			paramType := a.exprToString(param.Type)
			
			// Handle multiple names for same type: func(a, b int)
			for _, name := range param.Names {
				parameter := models.Parameter{
					Name:       name.Name,
					Type:       paramType,
					NameLength: len(name.Name),
				}
				function.Parameters = append(function.Parameters, parameter)
			}
		}
	}

	// Extract return type
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		var returnTypes []string
		for _, result := range fn.Type.Results.List {
			returnTypes = append(returnTypes, a.exprToString(result.Type))
		}
		function.ReturnType = strings.Join(returnTypes, ", ")
	}

	// Extract function comments
	if fn.Doc != nil {
		for _, comment := range fn.Doc.List {
			pos := a.fileSet.Position(comment.Pos())
			endPos := a.fileSet.Position(comment.End())
			
			genericComment := models.NewComment(
				comment.Text,
				pos.Line,
				endPos.Line,
				models.CommentTypeDocumentation,
				fmt.Sprintf("function:%s", fn.Name.Name),
			)
			function.Comments = append(function.Comments, genericComment)
		}
	}

	return function
}

// extractGenDecl handles general declarations (types, vars, consts)
func (a *GoAnalyzer) extractGenDecl(genDecl *ast.GenDecl, genericAST *models.GenericAST) {
	for _, spec := range genDecl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			typeDef := a.extractType(s, genDecl)
			genericAST.Types = append(genericAST.Types, typeDef)
		case *ast.ValueSpec:
			variables := a.extractVariables(s, genDecl)
			genericAST.Variables = append(genericAST.Variables, variables...)
		}
	}
}

// extractType converts ast.TypeSpec to models.TypeDefinition
func (a *GoAnalyzer) extractType(typeSpec *ast.TypeSpec, genDecl *ast.GenDecl) models.TypeDefinition {
	startPos := a.fileSet.Position(typeSpec.Pos())
	endPos := a.fileSet.Position(typeSpec.End())
	
	typeDef := models.TypeDefinition{
		Name:       typeSpec.Name.Name,
		NameLength: len(typeSpec.Name.Name),
		StartLine:  startPos.Line,
		EndLine:    endPos.Line,
		LineCount:  endPos.Line - startPos.Line + 1,
		Fields:     []models.Field{},
		Methods:    []models.Function{},
		Comments:   []models.Comment{},
		IsExported: typeSpec.Name.IsExported(),
	}

	// Determine type kind and extract fields
	switch t := typeSpec.Type.(type) {
	case *ast.StructType:
		typeDef.Kind = "struct"
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				fields := a.extractFields(field)
				typeDef.Fields = append(typeDef.Fields, fields...)
			}
		}
	case *ast.InterfaceType:
		typeDef.Kind = "interface"
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				// Interface methods are treated as fields for now
				fields := a.extractFields(method)
				typeDef.Fields = append(typeDef.Fields, fields...)
			}
		}
	default:
		typeDef.Kind = "type"
	}

	// Extract type comments
	if genDecl.Doc != nil {
		for _, comment := range genDecl.Doc.List {
			pos := a.fileSet.Position(comment.Pos())
			endPos := a.fileSet.Position(comment.End())
			
			genericComment := models.NewComment(
				comment.Text,
				pos.Line,
				endPos.Line,
				models.CommentTypeDocumentation,
				fmt.Sprintf("type:%s", typeSpec.Name.Name),
			)
			typeDef.Comments = append(typeDef.Comments, genericComment)
		}
	}

	return typeDef
}

// extractFields converts ast.Field to models.Field
func (a *GoAnalyzer) extractFields(field *ast.Field) []models.Field {
	var fields []models.Field
	fieldType := a.exprToString(field.Type)
	
	// Handle anonymous fields (embedded types)
	if len(field.Names) == 0 {
		fields = append(fields, models.Field{
			Name:       fieldType, // Use type as name for embedded fields
			Type:       fieldType,
			NameLength: len(fieldType),
			Comments:   []models.Comment{},
			IsExported: true, // Embedded types are usually exported
		})
	} else {
		// Named fields
		for _, name := range field.Names {
			genericField := models.Field{
				Name:       name.Name,
				Type:       fieldType,
				NameLength: len(name.Name),
				Comments:   []models.Comment{},
				IsExported: name.IsExported(),
			}
			fields = append(fields, genericField)
		}
	}

	return fields
}

// extractVariables converts ast.ValueSpec to models.Variable
func (a *GoAnalyzer) extractVariables(valueSpec *ast.ValueSpec, genDecl *ast.GenDecl) []models.Variable {
	var variables []models.Variable
	pos := a.fileSet.Position(valueSpec.Pos())
	
	// Determine if this is a constant
	isConstant := genDecl.Tok == token.CONST
	
	// Get type if specified
	var varType string
	if valueSpec.Type != nil {
		varType = a.exprToString(valueSpec.Type)
	}
	
	// Extract each variable name
	for _, name := range valueSpec.Names {
		variable := models.Variable{
			Name:       name.Name,
			Type:       varType,
			NameLength: len(name.Name),
			Line:       pos.Line,
			IsConstant: isConstant,
			IsExported: name.IsExported(),
			Comments:   []models.Comment{},
		}
		variables = append(variables, variable)
	}

	return variables
}

// exprToString converts an ast.Expr to its string representation
func (a *GoAnalyzer) exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return a.exprToString(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + a.exprToString(e.X)
	case *ast.ArrayType:
		return "[]" + a.exprToString(e.Elt)
	case *ast.MapType:
		return "map[" + a.exprToString(e.Key) + "]" + a.exprToString(e.Value)
	case *ast.ChanType:
		dir := ""
		if e.Dir == ast.SEND {
			dir = "chan<- "
		} else if e.Dir == ast.RECV {
			dir = "<-chan "
		} else {
			dir = "chan "
		}
		return dir + a.exprToString(e.Value)
	case *ast.FuncType:
		return "func" // Simplified
	case *ast.InterfaceType:
		return "interface{}" // Simplified
	case *ast.StructType:
		return "struct{}" // Simplified
	default:
		return "unknown"
	}
}

func AnalyzeGoFiles(rootDir string, files []string, ruleSets []models.RuleSet) (*models.AnalysisResult, error) {
	analyzer := NewGoAnalyzer()
	parser := NewParser(rootDir)
	result := &models.AnalysisResult{
		FileCount: len(files),
	}
	
	for _, file := range files {
		rules := parser.GetRulesForFile(file, ruleSets)
		if rules != nil {
			result.RuleCount += len(rules.Rules)
		}
		
		violations, err := analyzer.AnalyzeFile(file, rules)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze %s: %w", file, err)
		}
		
		result.Violations = append(result.Violations, violations...)
	}
	
	return result, nil
}

func NewParser(rootDir string) *Parser {
	return &Parser{rootDir: rootDir}
}

type Parser struct {
	rootDir string
}

func (p *Parser) GetRulesForFile(filePath string, ruleSets []models.RuleSet) *models.RuleSet {
	var bestMatch *models.RuleSet
	bestMatchDepth := -1
	
	absPath, _ := filepath.Abs(filePath)
	dir := filepath.Dir(absPath)
	
	for i := range ruleSets {
		ruleSet := &ruleSets[i]
		absRulePath, _ := filepath.Abs(ruleSet.Path)
		
		if strings.HasPrefix(dir, absRulePath) {
			depth := strings.Count(absRulePath, string(filepath.Separator))
			if depth > bestMatchDepth {
				bestMatch = ruleSet
				bestMatchDepth = depth
			}
		}
	}
	
	if bestMatch == nil && len(ruleSets) > 0 {
		for i := range ruleSets {
			if ruleSets[i].Path == p.rootDir || ruleSets[i].Path == "." {
				return &ruleSets[i]
			}
		}
	}
	
	return bestMatch
}