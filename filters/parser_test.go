package filters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/models"
)

func TestParseLine(t *testing.T) {
	parser := NewParser("/test")

	tests := []struct {
		name            string
		line            string
		expectedType    models.RuleType
		expectedPkg     string
		expectedMethod  string
		expectedPattern string
		expectError     bool
	}{
		{
			name:            "deny pattern",
			line:            "!internal/",
			expectedType:    models.RuleTypeDeny,
			expectedPattern: "internal/",
		},
		{
			name:            "override pattern",
			line:            "+internal/api",
			expectedType:    models.RuleTypeOverride,
			expectedPattern: "internal/api",
		},
		{
			name:            "allow pattern",
			line:            "utils/",
			expectedType:    models.RuleTypeAllow,
			expectedPattern: "utils/",
		},
		{
			name:           "method deny",
			line:           "fmt:!Println",
			expectedType:   models.RuleTypeDeny,
			expectedPkg:    "fmt",
			expectedMethod: "Println",
		},
		{
			name:           "wildcard method",
			line:           "*:!Test*",
			expectedType:   models.RuleTypeDeny,
			expectedPkg:    "*",
			expectedMethod: "Test*",
		},
		{
			name:         "comment line",
			line:         "# This is a comment",
			expectedType: models.RuleTypeAllow,
		},
		{
			name:         "empty line",
			line:         "",
			expectedType: models.RuleTypeAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip comment and empty line tests for actual parsing
			if tt.line == "" || tt.line[0] == '#' {
				return
			}

			rule, err := parser.parseLine(tt.line, "test.ARCHUNIT", 1, "/test")

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if rule == nil {
				t.Errorf("Expected rule but got nil")
				return
			}

			if rule.Type != tt.expectedType {
				t.Errorf("Type = %v, want %v", rule.Type, tt.expectedType)
			}

			if rule.Package != tt.expectedPkg {
				t.Errorf("Package = %v, want %v", rule.Package, tt.expectedPkg)
			}

			if rule.Method != tt.expectedMethod {
				t.Errorf("Method = %v, want %v", rule.Method, tt.expectedMethod)
			}

			if rule.Pattern != tt.expectedPattern {
				t.Errorf("Pattern = %v, want %v", rule.Pattern, tt.expectedPattern)
			}
		})
	}
}

func TestParseRuleFile(t *testing.T) {
	// Create a temporary .ARCHUNIT file
	tmpDir := t.TempDir()
	archUnitPath := filepath.Join(tmpDir, ".ARCHUNIT")

	content := `# Test rules
!internal/
!testing

# Method rules
fmt:!Println
*:!Test*

# Override
+internal/api
`

	err := os.WriteFile(archUnitPath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	parser := NewParser(tmpDir)
	ruleSet, err := parser.parseRuleFile(archUnitPath)
	if err != nil {
		t.Fatalf("Failed to parse rule file: %v", err)
	}

	if len(ruleSet.Rules) != 5 {
		t.Errorf("Expected 5 rules, got %d", len(ruleSet.Rules))
	}

	// Check specific rules
	expectedRules := []struct {
		Type    models.RuleType
		Pattern string
		Package string
		Method  string
	}{
		{models.RuleTypeDeny, "internal/", "", ""},
		{models.RuleTypeDeny, "testing", "", ""},
		{models.RuleTypeDeny, "", "fmt", "Println"},
		{models.RuleTypeDeny, "", "*", "Test*"},
		{models.RuleTypeOverride, "internal/api", "", ""},
	}

	for i, expected := range expectedRules {
		if i >= len(ruleSet.Rules) {
			break
		}

		rule := ruleSet.Rules[i]
		if rule.Type != expected.Type {
			t.Errorf("Rule %d: Type = %v, want %v", i, rule.Type, expected.Type)
		}
		if rule.Pattern != expected.Pattern {
			t.Errorf("Rule %d: Pattern = %v, want %v", i, rule.Pattern, expected.Pattern)
		}
		if rule.Package != expected.Package {
			t.Errorf("Rule %d: Package = %v, want %v", i, rule.Package, expected.Package)
		}
		if rule.Method != expected.Method {
			t.Errorf("Rule %d: Method = %v, want %v", i, rule.Method, expected.Method)
		}
	}
}
