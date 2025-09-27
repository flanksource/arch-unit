package models

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// AQL AST models for Architecture Query Language

// AQLRule represents a complete AQL rule
type AQLRule struct {
	Name       string          `json:"name" yaml:"name"`
	Statements []*AQLStatement `json:"statements" yaml:"statements"`
	SourceFile string          `json:"source_file,omitempty" yaml:"source_file,omitempty"`
	LineNumber int             `json:"line_number,omitempty" yaml:"line_number,omitempty"`
}

// AQLStatement represents a statement within an AQL rule
type AQLStatement struct {
	Type        AQLStatementType `json:"type" yaml:"type"`
	Condition   *AQLCondition    `json:"condition,omitempty" yaml:"condition,omitempty"`       // For LIMIT statements
	Pattern     *AQLPattern      `json:"pattern,omitempty" yaml:"pattern,omitempty"`           // For single pattern statements
	FromPattern *AQLPattern      `json:"from_pattern,omitempty" yaml:"from_pattern,omitempty"` // For relationship statements
	ToPattern   *AQLPattern      `json:"to_pattern,omitempty" yaml:"to_pattern,omitempty"`     // For relationship statements
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
	Pattern  *AQLPattern     `json:"pattern" yaml:"pattern"`
	Property string          `json:"property,omitempty" yaml:"property,omitempty"` // Backward compatibility
	Operator AQLOperatorType `json:"operator" yaml:"operator"`
	Value    interface{}     `json:"value" yaml:"value"` // Can hold raw values for backward compatibility
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
	Package    string `json:"package,omitempty" yaml:"package,omitempty"`
	Type       string `json:"type,omitempty" yaml:"type,omitempty"`
	Method     string `json:"method,omitempty" yaml:"method,omitempty"`
	Field      string `json:"field,omitempty" yaml:"field,omitempty"`
	FilePath   string `json:"file_path,omitempty" yaml:"file_path,omitempty"` // Doublestar glob pattern for file paths
	Metric     string `json:"metric,omitempty" yaml:"metric,omitempty"`       // "cyclomatic", "parameters", "lines"
	IsWildcard bool   `json:"is_wildcard" yaml:"is_wildcard"`
	Original   string `json:"original" yaml:"original"` // Original pattern text
}

// AQLValue represents a value in AQL expressions
type AQLValue struct {
	Type      AQLValueType `json:"type" yaml:"type"`
	IntValue  int          `json:"int_value,omitempty" yaml:"int_value,omitempty"`
	StrValue  string       `json:"str_value,omitempty" yaml:"str_value,omitempty"`
	BoolValue bool         `json:"bool_value,omitempty" yaml:"bool_value,omitempty"`
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
	valueStr := fmt.Sprintf("%v", c.Value)
	return fmt.Sprintf("%s %s %s", c.Pattern.String(), string(c.Operator), valueStr)
}

// String returns string representation of AQL pattern
func (p *AQLPattern) String() string {
	if p == nil {
		return ""
	}

	if p.Original != "" {
		return p.Original
	}

	// Start with file path if present
	var result string
	if p.FilePath != "" {
		result = "@" + p.FilePath
	}

	// Handle special case for simple wildcard
	if p.Package == "*" && p.Type == "*" && p.Method == "*" && p.Field == "" && p.Metric == "" {
		if p.FilePath != "" {
			result += ":*"
		} else {
			return "*"
		}
	} else {
		// Build AST pattern using appropriate notation
		var astPattern string

		// Special cases for dot notation
		if p.Package != "" && p.Type != "" && p.Method == "*" && p.Field == "" && p.Metric == "" {
			// Package.Type format (e.g., "pkg.*", "*.UserService")
			if p.Package == "*" {
				astPattern = "*." + p.Type
			} else if p.Type == "*" {
				astPattern = p.Package + ".*"
			} else {
				astPattern = p.Package + "." + p.Type
			}
		} else if p.Package == "*" && p.Type != "" && p.Type != "*" && p.Method != "" && p.Method != "*" {
			// Special case for "*.Type:Method" format
			astPattern = "*." + p.Type + ":" + p.Method
		} else {
			// Fall back to colon notation
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
			astPattern = strings.Join(parts, ":")
		}

		if p.FilePath != "" && astPattern != "" {
			result += ":" + astPattern
		} else if astPattern != "" {
			result = astPattern
		}
	}

	if p.Metric != "" {
		result += "." + p.Metric
	}

	return result
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

	// Check for file path patterns using @ prefix or path() function
	if strings.HasPrefix(pattern, "@") {
		// Handle @file_pattern:ast_pattern format
		if colonIndex := strings.Index(pattern, ":"); colonIndex != -1 {
			p.FilePath = pattern[1:colonIndex] // Remove @ prefix
			pattern = pattern[colonIndex+1:]  // Continue with AST pattern
		} else {
			// Only file path pattern, no AST pattern
			p.FilePath = pattern[1:] // Remove @ prefix
			pattern = "*"           // Default to match all AST nodes
		}
	} else if strings.HasPrefix(pattern, "path(") {
		// Handle path(file_pattern) format
		closeIndex := strings.Index(pattern, ")")
		if closeIndex == -1 {
			return nil, fmt.Errorf("invalid path() pattern format: missing closing parenthesis: %s", pattern)
		}
		p.FilePath = pattern[5:closeIndex] // Extract content between path( and )

		// Check for remaining AST pattern after path()
		remaining := strings.TrimSpace(pattern[closeIndex+1:])
		if strings.HasPrefix(remaining, "AND") {
			pattern = strings.TrimSpace(remaining[3:]) // Remove "AND"
		} else if remaining != "" {
			return nil, fmt.Errorf("invalid syntax after path(): %s", remaining)
		} else {
			pattern = "*" // Default to match all AST nodes
		}
	}

	// Check for metric access (e.g., "*.cyclomatic")
	// Only treat as metric if it's a known metric name
	if dotIndex := strings.LastIndex(pattern, "."); dotIndex != -1 {
		possibleMetric := pattern[dotIndex+1:]
		if possibleMetric == "cyclomatic" || possibleMetric == "parameters" || possibleMetric == "params" ||
			possibleMetric == "returns" || possibleMetric == "lines" {
			p.Metric = possibleMetric
			pattern = pattern[:dotIndex]
		}
	}

	// Handle different pattern formats:
	// - "*" -> wildcard for all
	// - "pkg.*" -> package + wildcard type
	// - "*.Type" -> wildcard package + specific type
	// - "*.Type:Method" -> wildcard package + specific type + method
	// - "main.Calculator:Add" -> specific package + type + method

	var parts []string
	if strings.Contains(pattern, ":") {
		// Mixed format: handle dot notation before colon
		if colonIndex := strings.Index(pattern, ":"); colonIndex != -1 {
			beforeColon := pattern[:colonIndex]
			afterColon := pattern[colonIndex+1:]
			
			if strings.Contains(beforeColon, ".") {
				// Handle "*.Type:Method" or "package.Type:Method" 
				dotParts := strings.Split(beforeColon, ".")
				if len(dotParts) == 2 {
					parts = append(dotParts, afterColon)
				} else {
					parts = []string{beforeColon, afterColon}
				}
			} else {
				// Standard colon format
				parts = strings.Split(pattern, ":")
			}
		} else {
			parts = strings.Split(pattern, ":")
		}
	} else if strings.Contains(pattern, ".") {
		// Handle dot patterns
		if strings.HasSuffix(pattern, ".*") {
			// "pkg.*" format
			parts = []string{pattern[:len(pattern)-2], "*"}
		} else if strings.HasPrefix(pattern, "*.") {
			// "*.Type" format
			parts = []string{"*", pattern[2:]}
		} else {
			// Regular dot notation: package.type
			parts = strings.Split(pattern, ".")
		}
	} else {
		// Single component
		parts = []string{pattern}
	}

	// Initialize defaults 
	p.Package = "*"
	p.Type = "" 
	p.Method = ""

	switch len(parts) {
	case 1:
		// Single string - use simple rules for backward compatibility
		single := parts[0]
		if single == "*" {
			// Full wildcard - keep package default, others empty
			// Note: Type and Method remain empty as initialized
		} else if strings.Contains(single, "/") {
			// Contains slash - likely a package path
			p.Package = single
		} else if strings.HasSuffix(single, "*") || strings.HasPrefix(single, "*") {
			// Wildcards - treat as type pattern for backward compatibility  
			p.Type = single
		} else {
			// For backward compatibility: single names without special chars default to package
			// But if it looks like a type (starts with uppercase) or method (common prefixes), handle appropriately
			if len(single) > 0 {
				firstChar := single[0]
				if firstChar >= 'A' && firstChar <= 'Z' {
					// Starts with uppercase - could be type
					p.Type = single
				} else {
					// Default to package for backward compatibility
					p.Package = single
				}
			}
		}
	case 2:
		// package:type or *.type format
		p.Package = parts[0]
		p.Type = parts[1]
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



// Matches checks if an AST node matches this pattern
func (p *AQLPattern) Matches(node *ASTNode) bool {
	// Check file path pattern first if specified
	if p.FilePath != "" && p.FilePath != "*" {
		if !matchesFilePath(node.FilePath, p.FilePath) {
			return false
		}
	}

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

// matchesFilePath performs doublestar glob matching on file paths
func matchesFilePath(filePath, pattern string) bool {
	if pattern == "*" {
		return true
	}

	// Try exact doublestar match
	if match, err := doublestar.Match(pattern, filePath); err == nil && match {
		return true
	}

	// Try matching against relative path (basename)
	if match, err := doublestar.Match(pattern, filepath.Base(filePath)); err == nil && match {
		return true
	}

	return false
}

// GetMetricValue extracts the metric value from an AST node
func (p *AQLPattern) GetMetricValue(node *ASTNode) (int, error) {
	switch p.Metric {
	case "cyclomatic":
		return node.CyclomaticComplexity, nil
	case "parameters", "params":
		// Use ParameterCount if available, otherwise fall back to len(Parameters)
		if node.ParameterCount > 0 {
			return node.ParameterCount, nil
		}
		return len(node.Parameters), nil
	case "returns":
		// Use ReturnCount if available, otherwise fall back to len(ReturnValues)
		if node.ReturnCount > 0 {
			return node.ReturnCount, nil
		}
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

	// Get the metric from Pattern.Metric or fallback to Property field for backward compatibility
	metric := c.Pattern.Metric
	if metric == "" {
		metric = c.Property
	}
	
	if metric == "" {
		return false, fmt.Errorf("condition requires a metric")
	}

	// Create a temporary pattern with the metric for GetMetricValue
	tempPattern := &AQLPattern{
		Package:    c.Pattern.Package,
		Type:       c.Pattern.Type,
		Method:     c.Pattern.Method,
		Field:      c.Pattern.Field,
		Metric:     metric,
		IsWildcard: c.Pattern.IsWildcard,
		Original:   c.Pattern.Original,
	}

	nodeValue, err := tempPattern.GetMetricValue(node)
	if err != nil {
		return false, err
	}

	// Extract numeric value from interface{}
	var compareValue int
	switch v := c.Value.(type) {
	case int:
		compareValue = v
	case float64:
		compareValue = int(v)
	case *AQLValue:
		compareValue = v.IntValue
	default:
		return false, fmt.Errorf("value must be numeric for comparison")
	}

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
	Rules      []*AQLRule `json:"rules" yaml:"rules"`
	SourceFile string     `json:"source_file,omitempty" yaml:"source_file,omitempty"`
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
