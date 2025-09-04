package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAQLParser_SimpleRule(t *testing.T) {
	aql := `RULE "Simple Rule" {
		LIMIT(*.cyclomatic > 10)
	}`

	ruleSet, err := ParseAQLFile(aql)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	assert.Equal(t, "Simple Rule", rule.Name)
	require.Len(t, rule.Statements, 1)

	stmt := rule.Statements[0]
	assert.Equal(t, models.AQLStatementLimit, stmt.Type)
	require.NotNil(t, stmt.Condition)

	// Verify the condition structure
	assert.Equal(t, models.OpGreaterThan, stmt.Condition.Operator)
	assert.Equal(t, "cyclomatic", stmt.Condition.Property)
	assert.Equal(t, 10.0, stmt.Condition.Value)
}

func TestAQLParser_ComplexRule(t *testing.T) {
	aql := `RULE "Architecture Rule" {
		LIMIT(Controller*.cyclomatic > 15)
		FORBID(Controller* -> Repository*)
		REQUIRE(Controller* -> Service*)
		ALLOW(Service* -> Repository*)
	}`

	ruleSet, err := ParseAQLFile(aql)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	assert.Equal(t, "Architecture Rule", rule.Name)
	require.Len(t, rule.Statements, 4)

	// Verify LIMIT statement
	limitStmt := rule.Statements[0]
	assert.Equal(t, models.AQLStatementLimit, limitStmt.Type)
	assert.Equal(t, "Controller*", limitStmt.Pattern.String())
	assert.Equal(t, models.OpGreaterThan, limitStmt.Condition.Operator)
	assert.Equal(t, "cyclomatic", limitStmt.Condition.Property)
	assert.Equal(t, 15.0, limitStmt.Condition.Value)

	// Verify FORBID statement
	forbidStmt := rule.Statements[1]
	assert.Equal(t, models.AQLStatementForbid, forbidStmt.Type)
	assert.Equal(t, "Controller*", forbidStmt.FromPattern.String())
	assert.Equal(t, "Repository*", forbidStmt.ToPattern.String())

	// Verify REQUIRE statement
	requireStmt := rule.Statements[2]
	assert.Equal(t, models.AQLStatementRequire, requireStmt.Type)
	assert.Equal(t, "Controller*", requireStmt.FromPattern.String())
	assert.Equal(t, "Service*", requireStmt.ToPattern.String())

	// Verify ALLOW statement
	allowStmt := rule.Statements[3]
	assert.Equal(t, models.AQLStatementAllow, allowStmt.Type)
	assert.Equal(t, "Service*", allowStmt.FromPattern.String())
	assert.Equal(t, "Repository*", allowStmt.ToPattern.String())
}

func TestAQLParser_MultipleRules(t *testing.T) {
	aql := `
	RULE "Complexity Rule" {
		LIMIT(*.cyclomatic > 10)
	}

	RULE "Layer Rule" {
		FORBID(Model* -> Controller*)
		REQUIRE(Controller* -> Service*)
	}`

	ruleSet, err := ParseAQLFile(aql)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 2)

	assert.Equal(t, "Complexity Rule", ruleSet.Rules[0].Name)
	assert.Equal(t, "Layer Rule", ruleSet.Rules[1].Name)

	// First rule should have 1 statement
	assert.Len(t, ruleSet.Rules[0].Statements, 1)

	// Second rule should have 2 statements
	assert.Len(t, ruleSet.Rules[1].Statements, 2)
}

func TestAQLParser_PatternVariants(t *testing.T) {
	tests := []struct {
		pattern  string
		expected models.AQLPattern
	}{
		{
			pattern:  "*",
			expected: models.AQLPattern{Package: "*", Type: "*", Method: "*"},
		},
		{
			pattern:  "pkg.*",
			expected: models.AQLPattern{Package: "pkg", Type: "*", Method: "*"},
		},
		{
			pattern:  "*.UserService",
			expected: models.AQLPattern{Package: "*", Type: "UserService", Method: "*"},
		},
		{
			pattern:  "*.UserService:Create*",
			expected: models.AQLPattern{Package: "*", Type: "UserService", Method: "Create*"},
		},
		{
			pattern:  "api.controller.*",
			expected: models.AQLPattern{Package: "api.controller", Type: "*", Method: "*"},
		},
		{
			pattern:  "main.Calculator:Add",
			expected: models.AQLPattern{Package: "main", Type: "Calculator", Method: "Add"},
		},
	}

	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			aql := `RULE "Test" {
				LIMIT(` + test.pattern + `.cyclomatic > 5)
			}`

			ruleSet, err := ParseAQLFile(aql)
			require.NoError(t, err)
			require.Len(t, ruleSet.Rules, 1)
			require.Len(t, ruleSet.Rules[0].Statements, 1)

			stmt := ruleSet.Rules[0].Statements[0]
			pattern := stmt.Pattern

			assert.Equal(t, test.expected.Package, pattern.Package)
			assert.Equal(t, test.expected.Type, pattern.Type)
			assert.Equal(t, test.expected.Method, pattern.Method)
		})
	}
}

func TestAQLParser_ConditionOperators(t *testing.T) {
	tests := []struct {
		condition string
		operator  models.ComparisonOperator
		property  string
		value     interface{}
	}{
		{"*.cyclomatic > 10", models.OpGreaterThan, "cyclomatic", 10.0},
		{"*.cyclomatic >= 5", models.OpGreaterThanEqual, "cyclomatic", 5.0},
		{"*.cyclomatic < 20", models.OpLessThan, "cyclomatic", 20.0},
		{"*.cyclomatic <= 15", models.OpLessThanEqual, "cyclomatic", 15.0},
		{"*.cyclomatic == 1", models.OpEqual, "cyclomatic", 1.0},
		{"*.cyclomatic != 0", models.OpNotEqual, "cyclomatic", 0.0},
		{"*.lines > 100", models.OpGreaterThan, "lines", 100.0},
		{"*.params < 5", models.OpLessThan, "params", 5.0},
	}

	for _, test := range tests {
		t.Run(test.condition, func(t *testing.T) {
			aql := `RULE "Test" {
				LIMIT(` + test.condition + `)
			}`

			ruleSet, err := ParseAQLFile(aql)
			require.NoError(t, err)
			require.Len(t, ruleSet.Rules, 1)
			require.Len(t, ruleSet.Rules[0].Statements, 1)

			stmt := ruleSet.Rules[0].Statements[0]
			condition := stmt.Condition

			assert.Equal(t, test.operator, condition.Operator)
			assert.Equal(t, test.property, condition.Property)
			assert.Equal(t, test.value, condition.Value)
		})
	}
}

func TestAQLParser_ErrorCases(t *testing.T) {
	errorCases := []struct {
		name          string
		aql           string
		expectedError string
	}{
		{
			name: "Missing rule name",
			aql: `RULE {
				LIMIT(*.cyclomatic > 10)
			}`,
			expectedError: "expected rule name",
		},
		{
			name: "Missing opening brace",
			aql: `RULE "Test"
				LIMIT(*.cyclomatic > 10)
			}`,
			expectedError: "expected '{' after rule name",
		},
		{
			name: "Missing closing brace",
			aql: `RULE "Test" {
				LIMIT(*.cyclomatic > 10)`,
			expectedError: "expected '}' to close rule",
		},
		{
			name: "Invalid pattern",
			aql: `RULE "Test" {
				LIMIT(invalid..pattern.cyclomatic > 10)
			}`,
			expectedError: "expected identifier",
		},
		{
			name: "Missing condition in LIMIT",
			aql: `RULE "Test" {
				LIMIT(*.cyclomatic)
			}`,
			expectedError: "expected operator",
		},
		{
			name: "Invalid operator",
			aql: `RULE "Test" {
				LIMIT(*.cyclomatic ?? 10)
			}`,
			expectedError: "expected operator",
		},
		{
			name: "Missing value",
			aql: `RULE "Test" {
				LIMIT(*.cyclomatic >)
			}`,
			expectedError: "expected value",
		},
		{
			name: "Missing arrow in relationship",
			aql: `RULE "Test" {
				FORBID(Controller* Repository*)
			}`,
			expectedError: "expected ')' after pattern",
		},
		{
			name: "Invalid statement type",
			aql: `RULE "Test" {
				INVALID(*.cyclomatic > 10)
			}`,
			expectedError: "unexpected token",
		},
	}

	for _, test := range errorCases {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseAQLFile(test.aql)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), test.expectedError)
		})
	}
}

func TestAQLParser_WhitespaceHandling(t *testing.T) {
	// Test with various whitespace patterns
	aql := `


	RULE    "Spaced Rule"    {
		LIMIT   (   Controller*.cyclomatic   >   10   )
		FORBID  (  Controller*   ->   Repository*  )
	}


	`

	ruleSet, err := ParseAQLFile(aql)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	assert.Equal(t, "Spaced Rule", rule.Name)
	assert.Len(t, rule.Statements, 2)

	// Verify statements parsed correctly despite spacing
	assert.Equal(t, models.AQLStatementLimit, rule.Statements[0].Type)
	assert.Equal(t, models.AQLStatementForbid, rule.Statements[1].Type)
}

func TestAQLParser_Comments(t *testing.T) {
	aql := `// This is a comment
	RULE "Commented Rule" { // Another comment
		// This rule limits complexity
		LIMIT(*.cyclomatic > 10) // Max complexity is 10
		// And forbids direct access
		FORBID(Controller* -> Model*) // Controllers can't access models directly
	}
	// End of rules`

	ruleSet, err := ParseAQLFile(aql)
	require.NoError(t, err)
	require.Len(t, ruleSet.Rules, 1)

	rule := ruleSet.Rules[0]
	assert.Equal(t, "Commented Rule", rule.Name)
	assert.Len(t, rule.Statements, 2)
}

func TestAQLParser_LargeRuleSet(t *testing.T) {
	var aqlBuilder strings.Builder

	// Generate a large rule set
	for i := 0; i < 100; i++ {
		aqlBuilder.WriteString(fmt.Sprintf(`
		RULE "Rule %d" {
			LIMIT(*.cyclomatic > %d)
			FORBID(Controller%d* -> Model%d*)
			REQUIRE(Controller%d* -> Service%d*)
		}
		`, i, i%20+1, i, i, i, i))
	}

	ruleSet, err := ParseAQLFile(aqlBuilder.String())
	require.NoError(t, err)
	assert.Len(t, ruleSet.Rules, 100)

	// Verify some rules
	for i := 0; i < 10; i++ {
		rule := ruleSet.Rules[i]
		assert.Equal(t, fmt.Sprintf("Rule %d", i), rule.Name)
		assert.Len(t, rule.Statements, 3)
	}
}

func TestAQLParser_PatternStringRepresentation(t *testing.T) {
	tests := []struct {
		pattern  models.AQLPattern
		expected string
	}{
		{
			pattern:  models.AQLPattern{Package: "*", Type: "*", Method: "*"},
			expected: "*",
		},
		{
			pattern:  models.AQLPattern{Package: "pkg", Type: "*", Method: "*"},
			expected: "pkg.*",
		},
		{
			pattern:  models.AQLPattern{Package: "*", Type: "User", Method: "*"},
			expected: "*.User",
		},
		{
			pattern:  models.AQLPattern{Package: "*", Type: "User", Method: "Create"},
			expected: "*.User:Create",
		},
		{
			pattern:  models.AQLPattern{Package: "api", Type: "Controller", Method: "*"},
			expected: "api.Controller",
		},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			result := test.pattern.String()
			assert.Equal(t, test.expected, result)
		})
	}
}
