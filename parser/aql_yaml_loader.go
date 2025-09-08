package parser

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/flanksource/arch-unit/models"
)

// LoadAQLFromYAML loads AQL rules from YAML content
func LoadAQLFromYAML(content string) (*models.AQLRuleSet, error) {
	var ruleSet models.AQLRuleSet

	if err := yaml.Unmarshal([]byte(content), &ruleSet); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate the rule set
	if err := validateRuleSet(&ruleSet); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &ruleSet, nil
}

// LoadAQLFromFile loads AQL rules from a YAML file
func LoadAQLFromFile(filename string) (*models.AQLRuleSet, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	ruleSet, err := LoadAQLFromYAML(string(content))
	if err != nil {
		return nil, err
	}

	// Set the source file for debugging
	ruleSet.SourceFile = filename
	for _, rule := range ruleSet.Rules {
		if rule.SourceFile == "" {
			rule.SourceFile = filename
		}
	}

	return ruleSet, nil
}

// validateRuleSet performs validation on the loaded rule set
func validateRuleSet(ruleSet *models.AQLRuleSet) error {
	if len(ruleSet.Rules) == 0 {
		return fmt.Errorf("rule set must contain at least one rule")
	}

	for i, rule := range ruleSet.Rules {
		if err := validateRule(rule, i); err != nil {
			return fmt.Errorf("rule %d (%s): %w", i, rule.Name, err)
		}
	}

	return nil
}

// validateRule performs validation on a single rule
func validateRule(rule *models.AQLRule, index int) error {
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	if len(rule.Statements) == 0 {
		return fmt.Errorf("rule must contain at least one statement")
	}

	for j, stmt := range rule.Statements {
		if err := validateStatement(stmt, j); err != nil {
			return fmt.Errorf("statement %d: %w", j, err)
		}
	}

	return nil
}

// validateStatement performs validation on a single statement
func validateStatement(stmt *models.AQLStatement, index int) error {
	switch stmt.Type {
	case models.AQLStatementLimit:
		if stmt.Condition == nil {
			return fmt.Errorf("LIMIT statement requires a condition")
		}
		return validateCondition(stmt.Condition)

	case models.AQLStatementForbid, models.AQLStatementRequire, models.AQLStatementAllow:
		// These can have either a single pattern or from/to patterns
		if stmt.Pattern != nil {
			return validatePattern(stmt.Pattern)
		}
		if stmt.FromPattern != nil && stmt.ToPattern != nil {
			if err := validatePattern(stmt.FromPattern); err != nil {
				return fmt.Errorf("from_pattern: %w", err)
			}
			if err := validatePattern(stmt.ToPattern); err != nil {
				return fmt.Errorf("to_pattern: %w", err)
			}
			return nil
		}
		return fmt.Errorf("%s statement requires either a pattern or both from_pattern and to_pattern", stmt.Type)

	default:
		return fmt.Errorf("unknown statement type: %s", stmt.Type)
	}
}

// validateCondition performs validation on a condition
func validateCondition(condition *models.AQLCondition) error {
	if condition.Pattern == nil {
		return fmt.Errorf("condition requires a pattern")
	}

	if err := validatePattern(condition.Pattern); err != nil {
		return fmt.Errorf("pattern: %w", err)
	}

	// Check that the pattern has a metric or the condition has a property (backward compatibility)
	if condition.Pattern.Metric == "" && condition.Property == "" {
		return fmt.Errorf("condition requires a metric in the pattern or property field")
	}

	// Validate operator
	validOperators := []models.AQLOperatorType{
		models.AQLOperatorGT, models.AQLOperatorLT,
		models.AQLOperatorGTE, models.AQLOperatorLTE,
		models.AQLOperatorEQ, models.AQLOperatorNE,
	}

	valid := false
	for _, op := range validOperators {
		if condition.Operator == op {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid operator: %s", condition.Operator)
	}

	// Validate value
	if condition.Value == nil {
		return fmt.Errorf("condition requires a value")
	}

	return nil
}

// validatePattern performs validation on a pattern
func validatePattern(pattern *models.AQLPattern) error {
	if pattern == nil {
		return fmt.Errorf("pattern cannot be nil")
	}

	// At least one field should be specified (even if it's a wildcard)
	if pattern.Package == "" && pattern.Type == "" && pattern.Method == "" &&
		pattern.Field == "" && pattern.Metric == "" {
		return fmt.Errorf("pattern must specify at least one field")
	}

	// Validate metric if specified
	if pattern.Metric != "" {
		validMetrics := []string{"cyclomatic", "parameters", "params", "returns", "lines"}
		valid := false
		for _, metric := range validMetrics {
			if pattern.Metric == metric {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid metric: %s", pattern.Metric)
		}
	}

	return nil
}

// ConvertLegacyAQL converts old AQL string format to YAML format
func ConvertLegacyAQL(aqlContent string) (string, error) {
	// This is a basic converter for backward compatibility
	// It handles simple cases and can be extended as needed

	ruleSet, err := ParseAQL(aqlContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse legacy AQL: %w", err)
	}

	yamlContent, err := yaml.Marshal(ruleSet)
	if err != nil {
		return "", fmt.Errorf("failed to convert to YAML: %w", err)
	}

	return string(yamlContent), nil
}

// IsLegacyAQLFormat detects if the content is in legacy AQL format
func IsLegacyAQLFormat(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "RULE") || strings.Contains(content, "RULE ")
}
