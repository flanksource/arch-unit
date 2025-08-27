package models

import (
	"fmt"
	"strconv"
	"strings"
)

// AQL AST models for Architecture Query Language

// AQLRule represents a complete AQL rule
type AQLRule struct {
	Name       string          `json:"name"`
	Statements []*AQLStatement `json:"statements"`
	SourceFile string          `json:"source_file,omitempty"`
	LineNumber int             `json:"line_number,omitempty"`
}

// AQLStatement represents a statement within an AQL rule
type AQLStatement struct {
	Type        AQLStatementType `json:"type"`
	Condition   *AQLCondition    `json:"condition,omitempty"`    // For LIMIT statements
	Pattern     *AQLPattern      `json:"pattern,omitempty"`      // For single pattern statements
	FromPattern *AQLPattern      `json:"from_pattern,omitempty"` // For relationship statements
	ToPattern   *AQLPattern      `json:"to_pattern,omitempty"`   // For relationship statements
}

// AQLStatementType represents the type of AQL statement
type AQLStatementType string

const (
	AQLStatementLimit   AQLStatementType = "LIMIT"
	AQLStatementForbid  AQLStatementType = "FORBID"
	AQLStatementRequire AQLStatementType = "REQUIRE"
	AQLStatementAllow   AQLStatementType = "ALLOW"
)

// AQLCondition represents a conditional expression in AQL
type AQLCondition struct {
	Pattern  *AQLPattern     `json:"pattern"`
	Property string          `json:"property,omitempty"` // Backward compatibility
	Operator AQLOperatorType `json:"operator"`
	Value    *AQLValue       `json:"value"`
}

// AQLOperatorType represents comparison operators
type AQLOperatorType string

const (
	AQLOperatorGT  AQLOperatorType = ">"
	AQLOperatorLT  AQLOperatorType = "<"
	AQLOperatorGTE AQLOperatorType = ">="
	AQLOperatorLTE AQLOperatorType = "<="
	AQLOperatorEQ  AQLOperatorType = "=="
	AQLOperatorNE  AQLOperatorType = "!="
)

// Backward compatibility aliases
type ComparisonOperator = AQLOperatorType

const (
	OpGreaterThan      = AQLOperatorGT
	OpGreaterThanEqual = AQLOperatorGTE
	OpLessThan         = AQLOperatorLT
	OpLessThanEqual    = AQLOperatorLTE
	OpEqual            = AQLOperatorEQ
	OpNotEqual         = AQLOperatorNE
)

// AQLPattern represents a pattern for matching AST elements
type AQLPattern struct {
	Package    string `json:"package,omitempty"`
	Type       string `json:"type,omitempty"`
	Method     string `json:"method,omitempty"`
	Field      string `json:"field,omitempty"`
	Metric     string `json:"metric,omitempty"` // "cyclomatic", "parameters", "lines"
	IsWildcard bool   `json:"is_wildcard"`
	Original   string `json:"original"` // Original pattern text
}

// AQLValue represents a value in AQL expressions
type AQLValue struct {
	Type      AQLValueType `json:"type"`
	IntValue  int          `json:"int_value,omitempty"`
	StrValue  string       `json:"str_value,omitempty"`
	BoolValue bool         `json:"bool_value,omitempty"`
}

// AQLValueType represents the type of a value
type AQLValueType string

const (
	AQLValueInt    AQLValueType = "int"
	AQLValueString AQLValueType = "string"
	AQLValueBool   AQLValueType = "bool"
)

// String returns string representation of AQL rule
func (r *AQLRule) String() string {
	var parts []string
	for _, stmt := range r.Statements {
		parts = append(parts, stmt.String())
	}
	return fmt.Sprintf("RULE %q {\n  %s\n}", r.Name, strings.Join(parts, ",\n  "))
}

// String returns string representation of AQL statement
func (s *AQLStatement) String() string {
	switch s.Type {
	case AQLStatementLimit:
		if s.Condition != nil {
			return fmt.Sprintf("LIMIT(%s)", s.Condition.String())
		}
	case AQLStatementForbid:
		if s.FromPattern != nil && s.ToPattern != nil {
			return fmt.Sprintf("FORBID(%s -> %s)", s.FromPattern.String(), s.ToPattern.String())
		} else if s.Pattern != nil {
			return fmt.Sprintf("FORBID(%s)", s.Pattern.String())
		}
	case AQLStatementRequire:
		if s.FromPattern != nil && s.ToPattern != nil {
			return fmt.Sprintf("REQUIRE(%s -> %s)", s.FromPattern.String(), s.ToPattern.String())
		} else if s.Pattern != nil {
			return fmt.Sprintf("REQUIRE(%s)", s.Pattern.String())
		}
	case AQLStatementAllow:
		if s.FromPattern != nil && s.ToPattern != nil {
			return fmt.Sprintf("ALLOW(%s -> %s)", s.FromPattern.String(), s.ToPattern.String())
		} else if s.Pattern != nil {
			return fmt.Sprintf("ALLOW(%s)", s.Pattern.String())
		}
	}
	return string(s.Type)
}

// String returns string representation of AQL condition
func (c *AQLCondition) String() string {
	return fmt.Sprintf("%s %s %s", c.Pattern.String(), string(c.Operator), c.Value.String())
}

// String returns string representation of AQL pattern
func (p *AQLPattern) String() string {
	if p.Original != "" {
		return p.Original
	}

	parts := []string{}
	if p.Package != "" {
		parts = append(parts, p.Package)
	}
	if p.Type != "" {
		parts = append(parts, p.Type)
	}
	if p.Method != "" {
		parts = append(parts, p.Method)
	}
	if p.Field != "" {
		parts = append(parts, p.Field)
	}

	pattern := strings.Join(parts, ":")
	if p.Metric != "" {
		pattern += "." + p.Metric
	}

	return pattern
}

// String returns string representation of AQL value
func (v *AQLValue) String() string {
	switch v.Type {
	case AQLValueInt:
		return strconv.Itoa(v.IntValue)
	case AQLValueString:
		return fmt.Sprintf("%q", v.StrValue)
	case AQLValueBool:
		return strconv.FormatBool(v.BoolValue)
	default:
		return ""
	}
}

// ParsePattern parses a pattern string into an AQLPattern
func ParsePattern(pattern string) (*AQLPattern, error) {
	p := &AQLPattern{
		Original:   pattern,
		IsWildcard: strings.Contains(pattern, "*"),
	}

	// Check for metric access (e.g., "*.cyclomatic")
	// Only treat as metric if it's a known metric name
	if dotIndex := strings.LastIndex(pattern, "."); dotIndex != -1 {
		possibleMetric := pattern[dotIndex+1:]
		if possibleMetric == "cyclomatic" || possibleMetric == "parameters" ||
			possibleMetric == "returns" || possibleMetric == "lines" {
			p.Metric = possibleMetric
			pattern = pattern[:dotIndex]
		} else if !strings.Contains(pattern, ":") {
			// Convert dot notation to colon notation
			// e.g., "widgets.Table" -> "widgets:Table"
			// or "widgets.Table.draw" -> "widgets:Table:draw"
			pattern = strings.ReplaceAll(pattern, ".", ":")
		}
	}

	// Split by colons to get package:type:method:field
	parts := strings.Split(pattern, ":")

	switch len(parts) {
	case 1:
		// Single string - determine if it's likely a method, type, or package
		single := parts[0]
		if single == "*" {
			p.Package = "*"
		} else if strings.Contains(single, "/") {
			// Contains slash, likely a package path
			p.Package = single
		} else if isLikelyMethodName(single) {
			// Looks like a method name - search for it anywhere
			p.Package = "*"
			p.Type = "*"
			p.Method = single
		} else if isLikelyTypeName(single) {
			// Looks like a type name - search for type in any package
			p.Package = "*"
			p.Type = single
		} else {
			// Default to package for backward compatibility
			p.Package = single
		}
	case 2:
		// Two parts - could be package:type or type:method
		// If first part doesn't look like a package, treat as type:method
		if !strings.Contains(parts[0], "/") && isLikelyTypeName(parts[0]) && isLikelyMethodName(parts[1]) {
			// Type:Method pattern - assume any package
			p.Package = "*"
			p.Type = parts[0]
			p.Method = parts[1]
		} else {
			// Traditional package:type
			p.Package = parts[0]
			p.Type = parts[1]
		}
	case 3:
		// package:type:method
		p.Package = parts[0]
		p.Type = parts[1]
		p.Method = parts[2]
	case 4:
		// package:type:method:field
		p.Package = parts[0]
		p.Type = parts[1]
		p.Method = parts[2]
		p.Field = parts[3]
	default:
		return nil, fmt.Errorf("invalid pattern format: %s", pattern)
	}

	return p, nil
}

// isLikelyMethodName checks if a string looks like a method name
func isLikelyMethodName(s string) bool {
	if s == "" || s == "*" {
		return false
	}

	// Check for wildcards in method-like patterns
	cleanName := strings.TrimPrefix(strings.TrimSuffix(s, "*"), "*")
	if cleanName == "" {
		return false
	}

	// Common method prefixes
	methodPrefixes := []string{
		"Get", "Set", "Create", "Update", "Delete", "Remove",
		"Find", "Search", "Save", "Load", "Process", "Validate",
		"Handle", "Execute", "Run", "Start", "Stop", "Init",
		"Read", "Write", "Open", "Close", "Connect", "Disconnect",
		"Parse", "Format", "Convert", "Transform", "Calculate",
		"Check", "Verify", "Test", "Is", "Has", "Can", "Should",
		"Add", "Append", "Insert", "Push", "Pop", "Clear",
		"get", "set", "create", "update", "delete", "remove",
	}

	for _, prefix := range methodPrefixes {
		if strings.HasPrefix(cleanName, prefix) {
			return true
		}
	}

	// Check if starts with lowercase (common for methods in many languages)
	// But exclude common package names
	if len(cleanName) > 0 && cleanName[0] >= 'a' && cleanName[0] <= 'z' {
		// Common package names that shouldn't be treated as methods
		packageNames := []string{
			"controllers", "models", "services", "utils", "helpers",
			"handlers", "middleware", "repositories", "views", "templates",
			"internal", "pkg", "cmd", "api", "web", "app", "lib",
			"config", "database", "auth", "tests", "docs", "scripts",
		}
		for _, pkg := range packageNames {
			if cleanName == pkg {
				return false
			}
		}
		return true
	}

	// Check for test methods
	if strings.HasPrefix(cleanName, "Test") || strings.HasPrefix(cleanName, "Benchmark") {
		return true
	}

	return false
}

// isLikelyTypeName checks if a string looks like a type/class name
func isLikelyTypeName(s string) bool {
	if s == "" || s == "*" {
		return false
	}

	// Check for wildcards
	cleanName := strings.TrimPrefix(strings.TrimSuffix(s, "*"), "*")
	if cleanName == "" {
		return false
	}

	// Common type suffixes
	typeSuffixes := []string{
		"Controller", "Service", "Repository", "Model", "View",
		"Manager", "Handler", "Factory", "Builder", "Provider",
		"Adapter", "Decorator", "Observer", "Strategy", "Command",
		"Client", "Server", "Request", "Response", "Error",
		"Config", "Options", "Settings", "Context", "State",
		"Interface", "Abstract", "Base", "Impl", "Mock",
		"DTO", "DAO", "Entity", "Domain", "Aggregate",
	}

	for _, suffix := range typeSuffixes {
		if strings.HasSuffix(cleanName, suffix) {
			return true
		}
	}

	// Check if starts with uppercase (common for types/classes)
	if len(cleanName) > 0 && cleanName[0] >= 'A' && cleanName[0] <= 'Z' {
		// But not if it looks like a method prefix
		if !isLikelyMethodName(s) {
			return true
		}
	}

	return false
}

// Matches checks if an AST node matches this pattern
func (p *AQLPattern) Matches(node *ASTNode) bool {
	if p.Package != "" && p.Package != "*" {
		if !matchesWildcard(node.PackageName, p.Package) {
			return false
		}
	}

	if p.Type != "" && p.Type != "*" {
		if !matchesWildcard(node.TypeName, p.Type) {
			return false
		}
	}

	if p.Method != "" && p.Method != "*" {
		if !matchesWildcard(node.MethodName, p.Method) {
			return false
		}
	}

	if p.Field != "" && p.Field != "*" {
		if !matchesWildcard(node.FieldName, p.Field) {
			return false
		}
	}

	return true
}

// GetMetricValue extracts the metric value from an AST node
func (p *AQLPattern) GetMetricValue(node *ASTNode) (int, error) {
	switch p.Metric {
	case "cyclomatic":
		return node.CyclomaticComplexity, nil
	case "parameters":
		return len(node.Parameters), nil
	case "returns":
		return len(node.ReturnValues), nil
	case "lines":
		return node.LineCount, nil
	default:
		return 0, fmt.Errorf("unknown metric: %s", p.Metric)
	}
}

// matchesWildcard performs wildcard matching
func matchesWildcard(text, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if !strings.Contains(pattern, "*") {
		return text == pattern
	}

	// Simple wildcard matching
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		// *substring*
		substring := pattern[1 : len(pattern)-1]
		return strings.Contains(text, substring)
	} else if strings.HasPrefix(pattern, "*") {
		// *suffix
		suffix := pattern[1:]
		return strings.HasSuffix(text, suffix)
	} else if strings.HasSuffix(pattern, "*") {
		// prefix*
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(text, prefix)
	}

	return text == pattern
}

// Evaluate evaluates a condition against an AST node
func (c *AQLCondition) Evaluate(node *ASTNode) (bool, error) {
	if !c.Pattern.Matches(node) {
		return false, nil
	}

	if c.Pattern.Metric == "" {
		return false, fmt.Errorf("condition requires a metric")
	}

	nodeValue, err := c.Pattern.GetMetricValue(node)
	if err != nil {
		return false, err
	}

	compareValue := c.Value.IntValue

	switch c.Operator {
	case AQLOperatorGT:
		return nodeValue > compareValue, nil
	case AQLOperatorLT:
		return nodeValue < compareValue, nil
	case AQLOperatorGTE:
		return nodeValue >= compareValue, nil
	case AQLOperatorLTE:
		return nodeValue <= compareValue, nil
	case AQLOperatorEQ:
		return nodeValue == compareValue, nil
	case AQLOperatorNE:
		return nodeValue != compareValue, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", c.Operator)
	}
}

// AQLRuleSet represents a collection of AQL rules
type AQLRuleSet struct {
	Rules      []*AQLRule `json:"rules"`
	SourceFile string     `json:"source_file,omitempty"`
}

// AddRule adds a rule to the rule set
func (rs *AQLRuleSet) AddRule(rule *AQLRule) {
	rs.Rules = append(rs.Rules, rule)
}

// GetRuleByName returns a rule by name
func (rs *AQLRuleSet) GetRuleByName(name string) *AQLRule {
	for _, rule := range rs.Rules {
		if rule.Name == name {
			return rule
		}
	}
	return nil
}

// String returns string representation of rule set
func (rs *AQLRuleSet) String() string {
	var parts []string
	for _, rule := range rs.Rules {
		parts = append(parts, rule.String())
	}
	return strings.Join(parts, "\n\n")
}
