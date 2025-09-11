package archunit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
)

// ViolationChecker analyzes AST nodes for architecture rule violations
type ViolationChecker struct {
	fileSet  *token.FileSet
	filePath string
	imports  map[string]string // alias -> full package path
}

// NewViolationChecker creates a new violation checker
func NewViolationChecker() *ViolationChecker {
	return &ViolationChecker{
		fileSet: token.NewFileSet(),
		imports: make(map[string]string),
	}
}

// CheckViolations analyzes a Go file for architecture rule violations
func (v *ViolationChecker) CheckViolations(filePath string, rules *models.RuleSet) ([]models.Violation, error) {
	// Parse the file
	src, err := parser.ParseFile(v.fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Go file: %w", err)
	}

	v.filePath = filePath
	v.imports = make(map[string]string)

	// Collect imports
	for _, imp := range src.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if imp.Name != nil {
			v.imports[imp.Name.Name] = path
		} else {
			parts := strings.Split(path, "/")
			name := parts[len(parts)-1]
			v.imports[name] = path
		}
	}

	// Find violations by walking the AST
	var violations []models.Violation
	ast.Inspect(src, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if violation := v.checkCallExpr(call, rules, src.Name.Name); violation != nil {
				violations = append(violations, *violation)
			}
		}
		return true
	})

	return violations, nil
}

// checkCallExpr examines function calls for rule violations
func (v *ViolationChecker) checkCallExpr(call *ast.CallExpr, rules *models.RuleSet, packageName string) *models.Violation {
	if rules == nil {
		return nil
	}

	var callerNode, calledNode *models.ASTNode

	// Create caller ASTNode
	pos := v.fileSet.Position(call.Pos())
	callerNode = &models.ASTNode{
		FilePath:    v.filePath,
		PackageName: packageName,
		StartLine:   pos.Line,
		NodeType:    models.NodeTypeMethod, // This is a call site, so we'll mark it as method
	}

	// Determine the called function/method
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		methodName := fn.Sel.Name
		var pkgName string

		if ident, ok := fn.X.(*ast.Ident); ok {
			pkgName = v.resolvePackage(ident.Name)
		} else if sel, ok := fn.X.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				pkgName = v.resolvePackage(ident.Name)
				methodName = sel.Sel.Name + "." + methodName
			}
		}

		if pkgName != "" {
			calledNode = &models.ASTNode{
				FilePath:    v.filePath, // We don't know the actual file of the called method
				PackageName: pkgName,
				MethodName:  methodName,
				StartLine:   pos.Line,
				NodeType:    models.NodeTypeMethod,
			}
		}

	case *ast.Ident:
		// Function call in same package
		calledNode = &models.ASTNode{
			FilePath:    v.filePath,
			PackageName: packageName,
			MethodName:  fn.Name,
			StartLine:   pos.Line,
			NodeType:    models.NodeTypeMethod,
		}
	}

	if calledNode == nil {
		return nil
	}

	// Build legacy format strings for rule checking
	pkgName := calledNode.PackageName
	methodName := calledNode.MethodName
	
	allowed, rule := rules.IsAllowedForFile(pkgName, methodName, v.filePath)
	if !allowed {
		// Get the actual source code line
		sourceCode, err := callerNode.GetSourceCode()
		if err != nil {
			logger.Debugf("Failed to get source code for violation at %s:%d: %v", v.filePath, pos.Line, err)
			sourceCode = "" // Continue without source code
		}

		// Create violation message
		violationMsg := fmt.Sprintf("Call to %s.%s violates architecture rule", pkgName, methodName)
		if rule.FilePattern != "" {
			violationMsg = fmt.Sprintf("Call to %s.%s violates file-specific rule [%s]", pkgName, methodName, rule.FilePattern)
		}

		return &models.Violation{
			File:    v.filePath,
			Line:    pos.Line,
			Column:  pos.Column,
			Caller:  callerNode,
			Called:  calledNode,
			Rule:    rule,
			Message: violationMsg,
			Code:    sourceCode,
		}
	}

	return nil
}

// resolvePackage resolves a package alias to its full import path
func (v *ViolationChecker) resolvePackage(name string) string {
	if pkg, ok := v.imports[name]; ok {
		return pkg
	}

	// Check if it's a type in the current package
	if len(name) > 0 && strings.ToUpper(name[:1]) == name[:1] {
		// This might be a type name, but we'll treat it as current package for now
		// In a more sophisticated implementation, we'd track type definitions
		return name // Return the name itself for local types
	}

	return name
}