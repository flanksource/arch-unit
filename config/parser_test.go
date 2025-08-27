package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/models"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory and config file
	tempDir := t.TempDir()

	configContent := `
version: "1.0"
debounce: "30s"
rules:
  "**":
    imports:
      - "!internal/"
      - "!fmt:Println"
    debounce: "30s"
  "**/*_test.go":
    imports:
      - "+testing"
      - "+fmt:Println"
    debounce: "10s"
linters:
  golangci-lint:
    enabled: true
`

	configPath := filepath.Join(tempDir, ConfigFileName)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Test loading config
	parser := NewParser(tempDir)
	config, err := parser.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate config
	if config.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", config.Version)
	}

	if config.Debounce != "30s" {
		t.Errorf("Expected debounce '30s', got '%s'", config.Debounce)
	}

	// Check rules
	if len(config.Rules) != 2 {
		t.Errorf("Expected 2 rule patterns, got %d", len(config.Rules))
	}

	globalRules, exists := config.Rules["**"]
	if !exists {
		t.Fatal("Expected global rules '**' to exist")
	}

	if len(globalRules.Imports) != 2 {
		t.Errorf("Expected 2 global import rules, got %d", len(globalRules.Imports))
	}

	// Check linters
	if len(config.Linters) != 1 {
		t.Errorf("Expected 1 linter, got %d", len(config.Linters))
	}

	golangciLint, exists := config.Linters["golangci-lint"]
	if !exists {
		t.Fatal("Expected golangci-lint config to exist")
	}

	if !golangciLint.Enabled {
		t.Error("Expected golangci-lint to be enabled")
	}
}

func TestGetRulesForFile(t *testing.T) {
	config := &models.Config{
		Rules: map[string]models.RuleConfig{
			"**": {
				Imports: []string{"!internal/", "!fmt:Println"},
			},
			"**/*_test.go": {
				Imports: []string{"+testing", "+fmt:Println"},
			},
			"**/main.go": {
				Imports: []string{"+os:Exit"},
			},
		},
	}

	testCases := []struct {
		filePath       string
		expectedRules  int
		shouldHaveFmt  bool
		shouldHaveTest bool
	}{
		{
			filePath:       "service.go",
			expectedRules:  2,    // global rules
			shouldHaveFmt:  true, // deny rule
			shouldHaveTest: false,
		},
		{
			filePath:       "service_test.go",
			expectedRules:  4,    // global + test rules
			shouldHaveFmt:  true, // both deny and allow
			shouldHaveTest: true,
		},
		{
			filePath:       "cmd/main.go",
			expectedRules:  3,    // global + main rules
			shouldHaveFmt:  true, // deny rule
			shouldHaveTest: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.filePath, func(t *testing.T) {
			ruleSet, err := config.GetRulesForFile(tc.filePath)
			if err != nil {
				t.Fatalf("Failed to get rules for %s: %v", tc.filePath, err)
			}

			if len(ruleSet.Rules) != tc.expectedRules {
				t.Errorf("Expected %d rules for %s, got %d", tc.expectedRules, tc.filePath, len(ruleSet.Rules))
			}

			hasFmtRule := false
			hasTestRule := false

			for _, rule := range ruleSet.Rules {
				if rule.Package == "fmt" && rule.Method == "Println" {
					hasFmtRule = true
				}
				if rule.Pattern == "testing" {
					hasTestRule = true
				}
			}

			if hasFmtRule != tc.shouldHaveFmt {
				t.Errorf("Expected fmt rule presence %v for %s, got %v", tc.shouldHaveFmt, tc.filePath, hasFmtRule)
			}

			if hasTestRule != tc.shouldHaveTest {
				t.Errorf("Expected test rule presence %v for %s, got %v", tc.shouldHaveTest, tc.filePath, hasTestRule)
			}
		})
	}
}
