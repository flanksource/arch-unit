package models

import (
	"testing"
)

func TestRuleMatches(t *testing.T) {
	tests := []struct {
		name     string
		rule     Rule
		pkg      string
		method   string
		expected bool
	}{
		{
			name:     "exact package match",
			rule:     Rule{Pattern: "internal"},
			pkg:      "internal",
			method:   "",
			expected: true,
		},
		{
			name:     "package with slash match",
			rule:     Rule{Pattern: "internal/"},
			pkg:      "internal/utils",
			method:   "",
			expected: true,
		},
		{
			name:     "wildcard suffix match",
			rule:     Rule{Pattern: "*_test"},
			pkg:      "utils_test",
			method:   "",
			expected: true,
		},
		{
			name:     "wildcard prefix match",
			rule:     Rule{Pattern: "test*"},
			pkg:      "testing",
			method:   "",
			expected: true,
		},
		{
			name:     "method specific match",
			rule:     Rule{Package: "fmt", Method: "Println"},
			pkg:      "fmt",
			method:   "Println",
			expected: true,
		},
		{
			name:     "method wildcard match",
			rule:     Rule{Package: "*", Method: "Test*"},
			pkg:      "anything",
			method:   "TestSomething",
			expected: true,
		},
		{
			name:     "no match different package",
			rule:     Rule{Pattern: "internal"},
			pkg:      "external",
			method:   "",
			expected: false,
		},
		{
			name:     "no match different method",
			rule:     Rule{Package: "fmt", Method: "Println"},
			pkg:      "fmt",
			method:   "Printf",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rule.Matches(tt.pkg, tt.method)
			if result != tt.expected {
				t.Errorf("Matches() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRuleSetIsAllowed(t *testing.T) {
	tests := []struct {
		name        string
		ruleSet     RuleSet
		pkg         string
		method      string
		allowed     bool
		hasViolation bool
	}{
		{
			name: "deny rule blocks access",
			ruleSet: RuleSet{
				Rules: []Rule{
					{Type: RuleTypeDeny, Pattern: "internal"},
				},
			},
			pkg:          "internal",
			method:       "",
			allowed:      false,
			hasViolation: true,
		},
		{
			name: "override rule allows previously denied",
			ruleSet: RuleSet{
				Rules: []Rule{
					{Type: RuleTypeDeny, Pattern: "internal"},
					{Type: RuleTypeOverride, Pattern: "internal/api"},
				},
			},
			pkg:          "internal/api",
			method:       "",
			allowed:      true,
			hasViolation: false,
		},
		{
			name: "no matching rules allows access",
			ruleSet: RuleSet{
				Rules: []Rule{
					{Type: RuleTypeDeny, Pattern: "internal"},
				},
			},
			pkg:          "external",
			method:       "",
			allowed:      true,
			hasViolation: false,
		},
		{
			name: "method specific deny",
			ruleSet: RuleSet{
				Rules: []Rule{
					{Type: RuleTypeDeny, Package: "fmt", Method: "Println"},
				},
			},
			pkg:          "fmt",
			method:       "Println",
			allowed:      false,
			hasViolation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, rule := tt.ruleSet.IsAllowed(tt.pkg, tt.method)
			if allowed != tt.allowed {
				t.Errorf("IsAllowed() allowed = %v, want %v", allowed, tt.allowed)
			}
			if (rule != nil) != tt.hasViolation {
				t.Errorf("IsAllowed() rule = %v, want violation = %v", rule, tt.hasViolation)
			}
		})
	}
}