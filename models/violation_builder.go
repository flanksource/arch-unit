package models

import (
	"path/filepath"
	"time"
)

// ViolationBuilder provides a fluent API for constructing Violations
// from linter output while properly handling AST nodes
type ViolationBuilder struct {
	violation Violation
	astCache  map[string]*ASTNode // Cache for finding/creating AST nodes
}

// NewViolationBuilder creates a new ViolationBuilder
func NewViolationBuilder() *ViolationBuilder {
	return &ViolationBuilder{
		violation: Violation{
			CreatedAt: time.Now(),
		},
		astCache: make(map[string]*ASTNode),
	}
}

// WithFile sets the file path for the violation
func (vb *ViolationBuilder) WithFile(file string) *ViolationBuilder {
	vb.violation.File = file
	return vb
}

// WithLocation sets the line and column for the violation
func (vb *ViolationBuilder) WithLocation(line, column int) *ViolationBuilder {
	vb.violation.Line = line
	vb.violation.Column = column
	return vb
}

// WithMessage sets the violation message
func (vb *ViolationBuilder) WithMessage(message string) *ViolationBuilder {
	vb.violation.Message = &message
	return vb
}

// WithSource sets the source tool that reported the violation
func (vb *ViolationBuilder) WithSource(source string) *ViolationBuilder {
	vb.violation.Source = source
	return vb
}

// WithCaller creates or finds a caller AST node and associates it with the violation
func (vb *ViolationBuilder) WithCaller(pkg, method string) *ViolationBuilder {
	caller := vb.findOrCreateASTNode(pkg, method, NodeTypeMethod)
	vb.violation.Caller = caller
	if caller != nil {
		vb.violation.CallerID = &caller.ID
	}
	return vb
}

// WithCalled creates or finds a called AST node and associates it with the violation
func (vb *ViolationBuilder) WithCalled(pkg, method string) *ViolationBuilder {
	called := vb.findOrCreateASTNode(pkg, method, NodeTypeMethod)
	vb.violation.Called = called
	if called != nil {
		vb.violation.CalledID = &called.ID
	}
	return vb
}

// WithRule sets the rule for the violation
func (vb *ViolationBuilder) WithRule(rule *Rule) *ViolationBuilder {
	vb.violation.Rule = rule
	return vb
}

// WithRuleFromLinter creates a rule from linter information
func (vb *ViolationBuilder) WithRuleFromLinter(linterName, ruleName string) *ViolationBuilder {
	rule := &Rule{
		Type:    RuleTypeDeny, // Linter violations are denying bad patterns
		Package: linterName,
		Method:  ruleName,
	}
	vb.violation.Rule = rule
	return vb
}

// WithFixable marks the violation as fixable
func (vb *ViolationBuilder) WithFixable(fixable bool) *ViolationBuilder {
	vb.violation.Fixable = fixable
	return vb
}

// WithFixApplicability sets the fix applicability level
func (vb *ViolationBuilder) WithFixApplicability(applicability string) *ViolationBuilder {
	vb.violation.FixApplicability = applicability
	return vb
}

// WithCode sets the code snippet where the violation was found
func (vb *ViolationBuilder) WithCode(code string) *ViolationBuilder {
	vb.violation.Code = &code
	return vb
}

// Build constructs and returns the final Violation
func (vb *ViolationBuilder) Build() Violation {
	return vb.violation
}

// findOrCreateASTNode looks up an existing AST node or creates a synthetic one
func (vb *ViolationBuilder) findOrCreateASTNode(pkg, method string, nodeType NodeType) *ASTNode {
	cacheKey := pkg + "::" + method
	
	// Check cache first
	if cached, exists := vb.astCache[cacheKey]; exists {
		return cached
	}
	
	// TODO: In the future, this could query the database for existing AST nodes
	// For now, create synthetic nodes for linter violations
	node := vb.createSyntheticASTNode(pkg, method, nodeType)
	vb.astCache[cacheKey] = node
	return node
}

// createSyntheticASTNode creates a placeholder AST node for linter violations
func (vb *ViolationBuilder) createSyntheticASTNode(pkg, method string, nodeType NodeType) *ASTNode {
	node := &ASTNode{
		PackageName: pkg,
		MethodName:  method,
		NodeType:    nodeType,
		FilePath:    vb.violation.File,
	}
	
	// Set appropriate fields based on node type
	switch nodeType {
	case NodeTypePackage:
		node.PackageName = pkg
		node.MethodName = ""
	case NodeTypeMethod:
		// For methods like "unknown" from linters, treat as synthetic
		if method == "unknown" {
			node.TypeName = filepath.Base(filepath.Dir(vb.violation.File))
			node.MethodName = "unknown"
		} else {
			node.MethodName = method
		}
	case NodeTypeType:
		node.TypeName = method
		node.MethodName = ""
	}
	
	return node
}