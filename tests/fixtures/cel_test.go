package fixtures_test

import (
	"testing"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/tests/fixtures"
)

func TestCELEvaluator(t *testing.T) {
	evaluator, err := fixtures.NewCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	// Test with some sample nodes
	nodes := []*models.ASTNode{
		{
			TypeName:             "UserController",
			MethodName:           "GetUser",
			NodeType:             models.NodeTypeMethod,
			CyclomaticComplexity: 5,
			LineCount:            50,
		},
		{
			TypeName:             "OrderController",
			MethodName:           "CreateOrder",
			NodeType:             models.NodeTypeMethod,
			CyclomaticComplexity: 10,
			LineCount:            100,
		},
	}

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "All nodes are controllers",
			expression: `nodes.all(n, n.type_name.endsWith("Controller"))`,
			expected:   true,
		},
		{
			name:       "At least one GetUser method",
			expression: `nodes.exists(n, n.method_name == "GetUser")`,
			expected:   true,
		},
		{
			name:       "All have complexity > 3",
			expression: `nodes.all(n, n.cyclomatic_complexity > 3)`,
			expected:   true,
		},
		{
			name:       "None have complexity > 20",
			expression: `!nodes.exists(n, n.cyclomatic_complexity > 20)`,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateNodes(tt.expression, nodes)
			if err != nil {
				t.Fatalf("Failed to evaluate expression: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %v but got %v for expression: %s", tt.expected, result, tt.expression)
			}
		})
	}
}

func TestCELEvaluatorOutput(t *testing.T) {
	evaluator, err := fixtures.NewCELEvaluator()
	if err != nil {
		t.Fatalf("Failed to create CEL evaluator: %v", err)
	}

	output := `AST Overview
Found 10 nodes
Type: UserController
Method: GetUser`

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "Output contains AST Overview",
			expression: `output.contains("AST Overview")`,
			expected:   true,
		},
		{
			name:       "Output contains UserController",
			expression: `output.contains("UserController")`,
			expected:   true,
		},
		{
			name:       "Output does not contain Error",
			expression: `!output.contains("Error")`,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateOutput(tt.expression, output)
			if err != nil {
				t.Fatalf("Failed to evaluate expression: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %v but got %v for expression: %s", tt.expected, result, tt.expression)
			}
		})
	}
}