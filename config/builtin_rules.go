package config

import (
	"github.com/flanksource/arch-unit/models"
)

// BuiltinRuleCategory represents the category of a built-in rule
type BuiltinRuleCategory string

const (
	CategoryArchitecture BuiltinRuleCategory = "architecture"
	CategorySecurity     BuiltinRuleCategory = "security"
	CategoryQuality      BuiltinRuleCategory = "quality"
	CategoryTesting      BuiltinRuleCategory = "testing"
	CategoryPerformance  BuiltinRuleCategory = "performance"
)

// BuiltinRule represents a predefined rule that can be enabled
type BuiltinRule struct {
	Name        string
	Description string
	Category    BuiltinRuleCategory
	Default     bool // Enabled by default
	Languages   []string // Empty means all languages
	Config      map[string]interface{}
	Apply       func(config *models.Config, ruleConfig models.BuiltinRuleConfig) error
}

// BuiltinRules defines all available built-in rules
var BuiltinRules = map[string]BuiltinRule{
	// Architecture Rules
	"clean_architecture": {
		Name:        "Clean Architecture",
		Description: "Enforce clean architecture boundaries",
		Category:    CategoryArchitecture,
		Default:     false,
		Apply:       applyCleanArchitectureRule,
	},
	"layered_architecture": {
		Name:        "Layered Architecture",
		Description: "Enforce layered architecture (presentation, business, data)",
		Category:    CategoryArchitecture,
		Default:     false,
		Apply:       applyLayeredArchitectureRule,
	},
	"no_circular_dependencies": {
		Name:        "No Circular Dependencies",
		Description: "Prevent circular dependencies between packages",
		Category:    CategoryArchitecture,
		Default:     true,
		Apply:       applyNoCircularDepsRule,
	},

	// Security Rules
	"no_hardcoded_secrets": {
		Name:        "No Hardcoded Secrets",
		Description: "Prevent hardcoded passwords, tokens, and API keys",
		Category:    CategorySecurity,
		Default:     true,
		Config: map[string]interface{}{
			"patterns": []string{"password", "secret", "token", "api_key", "apikey"},
		},
		Apply: applyNoHardcodedSecretsRule,
	},
	"secure_imports": {
		Name:        "Secure Imports",
		Description: "Prevent usage of insecure packages",
		Category:    CategorySecurity,
		Default:     true,
		Apply:       applySecureImportsRule,
	},

	// Quality Rules
	"no_fmt_print": {
		Name:        "No fmt.Print in Production",
		Description: "Disallow fmt.Print* functions in production code",
		Category:    CategoryQuality,
		Default:     true,
		Languages:   []string{"go"},
		Apply:       applyNoFmtPrintRule,
	},
	"proper_error_handling": {
		Name:        "Proper Error Handling",
		Description: "Enforce proper error handling patterns",
		Category:    CategoryQuality,
		Default:     true,
		Languages:   []string{"go"},
		Apply:       applyProperErrorHandlingRule,
	},
	"no_console_log": {
		Name:        "No console.log in Production",
		Description: "Disallow console.log in production code",
		Category:    CategoryQuality,
		Default:     true,
		Languages:   []string{"javascript", "typescript"},
		Apply:       applyNoConsoleLogRule,
	},
	"no_print_statements": {
		Name:        "No Print Statements",
		Description: "Disallow print() in production Python code",
		Category:    CategoryQuality,
		Default:     true,
		Languages:   []string{"python"},
		Apply:       applyNoPrintStatementsRule,
	},
	"no_system_out_print": {
		Name:        "No System.out.print",
		Description: "Disallow System.out.print* in production Java code",
		Category:    CategoryQuality,
		Default:     true,
		Languages:   []string{"java"},
		Apply:       applyNoSystemOutPrintRule,
	},

	// Testing Rules
	"test_naming_convention": {
		Name:        "Test Naming Convention",
		Description: "Enforce naming conventions for test functions",
		Category:    CategoryTesting,
		Default:     true,
		Config: map[string]interface{}{
			"go":         "Test.*",
			"python":     "test_.*",
			"java":       "test.*",
			"javascript": "test.*|.*\\.test|.*\\.spec",
		},
		Apply: applyTestNamingConventionRule,
	},
	"test_coverage": {
		Name:        "Test Coverage Requirements",
		Description: "Enforce minimum test coverage",
		Category:    CategoryTesting,
		Default:     false,
		Config: map[string]interface{}{
			"threshold": 80,
		},
		Apply: applyTestCoverageRule,
	},

	// Performance Rules
	"no_n_plus_one": {
		Name:        "No N+1 Queries",
		Description: "Detect potential N+1 database query patterns",
		Category:    CategoryPerformance,
		Default:     false,
		Apply:       applyNoNPlusOneRule,
	},
}

// Apply rule functions

func applyCleanArchitectureRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// Domain should not depend on infrastructure
	addRuleIfNotExists(config, "domain/**", models.RuleConfig{
		Imports: []string{
			"!infrastructure/*",
			"!application/*",
			"!presentation/*",
		},
	})

	// Application can depend on domain but not infrastructure or presentation
	addRuleIfNotExists(config, "application/**", models.RuleConfig{
		Imports: []string{
			"!infrastructure/*",
			"!presentation/*",
		},
	})

	// Infrastructure can depend on domain and application
	addRuleIfNotExists(config, "infrastructure/**", models.RuleConfig{
		Imports: []string{
			"!presentation/*",
		},
	})

	return nil
}

func applyLayeredArchitectureRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// Presentation layer
	addRuleIfNotExists(config, "**/presentation/**", models.RuleConfig{
		Imports: []string{
			"!**/data/**",  // No direct data access from presentation
		},
	})

	// Business layer
	addRuleIfNotExists(config, "**/business/**", models.RuleConfig{
		Imports: []string{
			"!**/presentation/**", // Business should not depend on presentation
		},
	})

	// Data layer
	addRuleIfNotExists(config, "**/data/**", models.RuleConfig{
		Imports: []string{
			"!**/presentation/**", // Data should not depend on presentation
			"!**/business/**",     // Data should not depend on business
		},
	})

	return nil
}

func applyNoCircularDepsRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// This would be enforced at analysis time, not through import rules
	// Add a marker for the analyzer to check
	return nil
}

func applyNoHardcodedSecretsRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// This would be enforced through linter configurations and analysis
	return nil
}

func applySecureImportsRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// Add rules for all files
	addImportRules(config, "**", []string{
		"!crypto/md5",      // Weak hash
		"!crypto/sha1",     // Weak hash
		"!math/rand",       // Not cryptographically secure
	})

	// Python specific
	addImportRules(config, "**/*.py", []string{
		"!pickle",          // Security risk with untrusted data
		"!eval",            // Code injection risk
		"!exec",            // Code injection risk
	})

	// JavaScript/TypeScript specific
	addImportRules(config, "**/*.{js,ts,jsx,tsx}", []string{
		"!eval",            // Code injection risk
	})

	return nil
}

func applyNoFmtPrintRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	addImportRules(config, "**/*.go", []string{
		"!fmt:Print",
		"!fmt:Printf",
		"!fmt:Println",
	})

	// Allow in test files
	addImportRules(config, "**/*_test.go", []string{
		"+fmt:Print",
		"+fmt:Printf",
		"+fmt:Println",
	})

	// Allow in main files
	addImportRules(config, "**/main.go", []string{
		"+fmt:Print",
		"+fmt:Printf",
		"+fmt:Println",
	})

	return nil
}

func applyProperErrorHandlingRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// This would be enforced through linter configurations
	if config.Linters == nil {
		config.Linters = make(map[string]models.LinterConfig)
	}

	if linter, exists := config.Linters["golangci-lint"]; exists {
		linter.Args = append(linter.Args, "--enable=errcheck")
		config.Linters["golangci-lint"] = linter
	}

	return nil
}

func applyNoConsoleLogRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// Add ESLint rule for no-console
	if config.Linters == nil {
		config.Linters = make(map[string]models.LinterConfig)
	}

	if linter, exists := config.Linters["eslint"]; exists {
		linter.Args = append(linter.Args, "--rule", "no-console:error")
		config.Linters["eslint"] = linter
	}

	return nil
}

func applyNoPrintStatementsRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	addImportRules(config, "**/*.py", []string{
		"!print",
	})

	// Allow in test files
	addImportRules(config, "**/test_*.py", []string{
		"+print",
	})
	addImportRules(config, "**/*_test.py", []string{
		"+print",
	})

	return nil
}

func applyNoSystemOutPrintRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	addImportRules(config, "**/*.java", []string{
		"!System.out:print",
		"!System.out:println",
		"!System.err:print",
		"!System.err:println",
	})

	// Allow in test files
	addImportRules(config, "**/Test*.java", []string{
		"+System.out:print",
		"+System.out:println",
	})
	addImportRules(config, "**/*Test.java", []string{
		"+System.out:print",
		"+System.out:println",
	})

	return nil
}

func applyTestNamingConventionRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// This would be enforced through analysis
	return nil
}

func applyTestCoverageRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// This would be enforced through CI/CD pipeline
	return nil
}

func applyNoNPlusOneRule(config *models.Config, ruleConfig models.BuiltinRuleConfig) error {
	// This would be enforced through analysis
	return nil
}

// Helper functions

func addRuleIfNotExists(config *models.Config, pattern string, rule models.RuleConfig) {
	if config.Rules == nil {
		config.Rules = make(map[string]models.RuleConfig)
	}

	if _, exists := config.Rules[pattern]; !exists {
		config.Rules[pattern] = rule
	} else {
		// Merge with existing rule
		existing := config.Rules[pattern]
		existing.Imports = append(existing.Imports, rule.Imports...)
		config.Rules[pattern] = existing
	}
}

func addImportRules(config *models.Config, pattern string, imports []string) {
	if config.Rules == nil {
		config.Rules = make(map[string]models.RuleConfig)
	}

	if rule, exists := config.Rules[pattern]; exists {
		rule.Imports = append(rule.Imports, imports...)
		config.Rules[pattern] = rule
	} else {
		config.Rules[pattern] = models.RuleConfig{
			Imports: imports,
		}
	}
}

// ApplyBuiltinRules applies all enabled built-in rules to the configuration
func ApplyBuiltinRules(config *models.Config) error {
	if config.BuiltinRules == nil {
		return nil
	}

	for ruleName, ruleConfig := range config.BuiltinRules {
		if !ruleConfig.Enabled {
			continue
		}

		if rule, exists := BuiltinRules[ruleName]; exists {
			if err := rule.Apply(config, ruleConfig); err != nil {
				return err
			}
		}
	}

	return nil
}