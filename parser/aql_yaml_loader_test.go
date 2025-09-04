package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flanksource/arch-unit/models"
)

func TestLoadAQLFromYAML_SimpleRule(t *testing.T) {
	yaml := `
rules:
  - name: "Simple Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "*"
            metric: "cyclomatic"
          operator: ">"
          value: 10
`

	ruleSet, err := LoadAQLFromYAML(yaml)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	assert.Equal(t, "Simple Rule", rule.Name)
	require.Len(t, rule.Statements, 1)

	stmt := rule.Statements[0]
	assert.Equal(t, models.AQLStatementLimit, stmt.Type)
	require.NotNil(t, stmt.Condition)

	// Verify the condition structure
	assert.Equal(t, models.AQLOperatorGT, stmt.Condition.Operator)
	assert.Equal(t, "cyclomatic", stmt.Condition.Pattern.Metric)
	assert.Equal(t, "*", stmt.Condition.Pattern.Package)
	assert.Equal(t, 10, stmt.Condition.Value)
}

func TestLoadAQLFromYAML_ComplexRule(t *testing.T) {
	yaml := `
rules:
  - name: "Architecture Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            type: "Controller*"
            metric: "cyclomatic"
          operator: ">"
          value: 15
      - type: FORBID
        from_pattern:
          type: "Controller*"
        to_pattern:
          type: "Repository*"
      - type: REQUIRE
        from_pattern:
          type: "Controller*"
        to_pattern:
          type: "Service*"
      - type: ALLOW
        from_pattern:
          type: "Service*"
        to_pattern:
          type: "Repository*"
`

	ruleSet, err := LoadAQLFromYAML(yaml)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	assert.Equal(t, "Architecture Rule", rule.Name)
	require.Len(t, rule.Statements, 4)

	// Verify LIMIT statement
	limitStmt := rule.Statements[0]
	assert.Equal(t, models.AQLStatementLimit, limitStmt.Type)
	assert.Equal(t, "Controller*", limitStmt.Condition.Pattern.Type)
	assert.Equal(t, models.AQLOperatorGT, limitStmt.Condition.Operator)
	assert.Equal(t, "cyclomatic", limitStmt.Condition.Pattern.Metric)
	assert.Equal(t, 15, limitStmt.Condition.Value)

	// Verify FORBID statement
	forbidStmt := rule.Statements[1]
	assert.Equal(t, models.AQLStatementForbid, forbidStmt.Type)
	assert.Equal(t, "Controller*", forbidStmt.FromPattern.Type)
	assert.Equal(t, "Repository*", forbidStmt.ToPattern.Type)

	// Verify REQUIRE statement
	requireStmt := rule.Statements[2]
	assert.Equal(t, models.AQLStatementRequire, requireStmt.Type)
	assert.Equal(t, "Controller*", requireStmt.FromPattern.Type)
	assert.Equal(t, "Service*", requireStmt.ToPattern.Type)

	// Verify ALLOW statement
	allowStmt := rule.Statements[3]
	assert.Equal(t, models.AQLStatementAllow, allowStmt.Type)
	assert.Equal(t, "Service*", allowStmt.FromPattern.Type)
	assert.Equal(t, "Repository*", allowStmt.ToPattern.Type)
}

func TestLoadAQLFromYAML_MultipleRules(t *testing.T) {
	yaml := `
rules:
  - name: "Complexity Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "*"
            metric: "cyclomatic"
          operator: ">"
          value: 20
  - name: "Layer Rule"
    statements:
      - type: FORBID
        from_pattern:
          type: "*Controller"
        to_pattern:
          type: "*Repository"
`

	ruleSet, err := LoadAQLFromYAML(yaml)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 2)

	// Verify first rule
	rule1 := ruleSet.Rules[0]
	assert.Equal(t, "Complexity Rule", rule1.Name)
	require.Len(t, rule1.Statements, 1)
	
	stmt1 := rule1.Statements[0]
	assert.Equal(t, models.AQLStatementLimit, stmt1.Type)
	assert.Equal(t, 20, stmt1.Condition.Value)

	// Verify second rule
	rule2 := ruleSet.Rules[1]
	assert.Equal(t, "Layer Rule", rule2.Name)
	require.Len(t, rule2.Statements, 1)
	
	stmt2 := rule2.Statements[0]
	assert.Equal(t, models.AQLStatementForbid, stmt2.Type)
	assert.Equal(t, "*Controller", stmt2.FromPattern.Type)
	assert.Equal(t, "*Repository", stmt2.ToPattern.Type)
}

func TestLoadAQLFromYAML_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name          string
		yaml          string
		expectedError string
	}{
		{
			name:          "Empty rules",
			yaml:          `rules: []`,
			expectedError: "rule set must contain at least one rule",
		},
		{
			name: "Missing rule name",
			yaml: `
rules:
  - statements:
      - type: LIMIT
        condition:
          pattern:
            metric: "cyclomatic"
          operator: ">"
          value: 10
`,
			expectedError: "rule name is required",
		},
		{
			name: "Missing statements",
			yaml: `
rules:
  - name: "Test Rule"
    statements: []
`,
			expectedError: "rule must contain at least one statement",
		},
		{
			name: "LIMIT without condition",
			yaml: `
rules:
  - name: "Test Rule"
    statements:
      - type: LIMIT
`,
			expectedError: "LIMIT statement requires a condition",
		},
		{
			name: "Invalid operator",
			yaml: `
rules:
  - name: "Test Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            metric: "cyclomatic"
          operator: "invalid"
          value: 10
`,
			expectedError: "invalid operator: invalid",
		},
		{
			name: "Missing value",
			yaml: `
rules:
  - name: "Test Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            metric: "cyclomatic"
          operator: ">"
`,
			expectedError: "condition requires a value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadAQLFromYAML(tc.yaml)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
	}
}

func TestLoadAQLFromYAML_BackwardCompatibility(t *testing.T) {
	// Test backward compatibility with property field instead of metric in pattern
	yaml := `
rules:
  - name: "Backward Compatibility Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "*"
          property: "cyclomatic"
          operator: ">"
          value: 10
`

	ruleSet, err := LoadAQLFromYAML(yaml)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	stmt := rule.Statements[0]
	assert.Equal(t, "cyclomatic", stmt.Condition.Property)
	assert.Equal(t, "", stmt.Condition.Pattern.Metric)
}

func TestIsLegacyAQLFormat(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Legacy AQL format",
			content:  `RULE "Test" { LIMIT(*.cyclomatic > 10) }`,
			expected: true,
		},
		{
			name:     "Legacy AQL with whitespace",
			content:  ` RULE "Test" { LIMIT(*.cyclomatic > 10) }`,
			expected: true,
		},
		{
			name: "YAML format",
			content: `rules:
  - name: "Test"
    statements:
      - type: LIMIT`,
			expected: false,
		},
		{
			name:     "JSON format",
			content:  `{"rules": []}`,
			expected: false,
		},
		{
			name:     "Empty content",
			content:  "",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsLegacyAQLFormat(tc.content)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestLoadAQLFromYAML_CompletePattern(t *testing.T) {
	yaml := `
rules:
  - name: "Complete Pattern Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "internal/service"
            type: "UserService"
            method: "CreateUser"
            field: "id"
            metric: "parameters"
          operator: "<="
          value: 3
`

	ruleSet, err := LoadAQLFromYAML(yaml)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	stmt := rule.Statements[0]
	pattern := stmt.Condition.Pattern

	assert.Equal(t, "internal/service", pattern.Package)
	assert.Equal(t, "UserService", pattern.Type)
	assert.Equal(t, "CreateUser", pattern.Method)
	assert.Equal(t, "id", pattern.Field)
	assert.Equal(t, "parameters", pattern.Metric)
	assert.Equal(t, models.AQLOperatorLTE, stmt.Condition.Operator)
	assert.Equal(t, 3, stmt.Condition.Value)
}