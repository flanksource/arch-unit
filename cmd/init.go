package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flanksource/arch-unit/config"
	"github.com/flanksource/arch-unit/languages"
	"github.com/flanksource/arch-unit/models"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	force        bool
	interactive  bool
	styleGuide   string
	strictness   string
	setupLinters bool
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize arch-unit.yaml configuration file",
	Long: `Initialize a new arch-unit.yaml configuration file in the specified directory
with example rules, linter integrations, and documentation.

Examples:
  # Initialize in current directory
  arch-unit init

  # Initialize in specific directory
  arch-unit init ./src

  # Force overwrite existing file
  arch-unit init --force`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing configuration file")
	initCmd.Flags().BoolVarP(&interactive, "interactive", "i", true, "Run interactive setup")
	initCmd.Flags().StringVar(&styleGuide, "style", "", "Style guide to use (google-go, google-python, google-java, airbnb-javascript, pep8, rust-official)")
	initCmd.Flags().StringVar(&strictness, "strictness", "moderate", "Strictness level (strict, moderate, lenient)")
	initCmd.Flags().BoolVar(&setupLinters, "setup-linters", false, "Create linter configuration files if missing")
}

const exampleArchUnitFile = `# .ARCHUNIT - Architecture Rules Configuration
# 
# This file defines architectural constraints for your codebase.
# Rules are applied to all files in this directory and subdirectories.
#
# SYNTAX:
#   !pattern         - Deny access to package/folder
#   pattern          - Allow access (default)
#   +pattern         - Override parent rules
#   package:method   - Method-specific rules
#   package:!method  - Deny specific method
#   *:method         - Apply to all packages
#
# PATTERNS:
#   internal/        - Match internal package/folder
#   *.test           - Match packages ending with .test
#   api.*            - Match api and all sub-packages
#   */private/*      - Match private in any path
#
# EXAMPLES:

# Prevent access to internal packages
!internal/

# Prevent test utilities from being used in production code
!testing
!*_test

# Prevent direct database access outside of repository layer
!database/sql
!github.com/lib/pq

# Allow specific packages to use internal
# (Place in subdirectory .ARCHUNIT files)
# +internal/

# Method-specific rules
# Prevent calling test methods in production
*:!Test*
*:!test*

# Prevent fmt.Println in production (use logger instead)
fmt:!Println
fmt:!Printf

# Python-specific examples (for Python projects)
# !django.db
# !sqlalchemy
# unittest:!*
# pytest:!*
`

func runInit(cmd *cobra.Command, args []string) error {
	targetDir := "."
	if len(args) > 0 {
		targetDir = args[0]
	}

	// Ensure directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if file exists and not forcing
	configPath := filepath.Join(targetDir, config.ConfigFileName)
	if _, err := os.Stat(configPath); err == nil && !force {
		return fmt.Errorf("%s file already exists. Use --force to overwrite", config.ConfigFileName)
	}

	var generatedConfig *models.Config
	var err error

	// Run interactive setup if enabled
	if interactive && styleGuide == "" {
		generatedConfig, err = RunInteractiveInit(targetDir)
		if err != nil {
			return fmt.Errorf("interactive setup failed: %w", err)
		}

		// Create linter configs if requested during interactive setup
		if setupLinters {
			questions := &InitQuestions{
				EnabledLinters: make(map[string]bool),
			}
			for linter, linterConfig := range generatedConfig.Linters {
				if linterConfig.Enabled {
					questions.EnabledLinters[linter] = true
				}
			}
			if questions.SetupMissingLinters || setupLinters {
				if err := CreateLinterConfigs(targetDir, questions.EnabledLinters); err != nil {
					fmt.Printf("Warning: Failed to create some linter configs: %v\n", err)
				}
			}
		}
	} else {
		// Non-interactive mode - use provided flags or defaults
		generatedConfig = generateNonInteractiveConfig(targetDir, styleGuide, strictness)

		if setupLinters {
			enabledLinters := make(map[string]bool)
			for linter, linterConfig := range generatedConfig.Linters {
				if linterConfig.Enabled {
					enabledLinters[linter] = true
				}
			}
			if err := CreateLinterConfigs(targetDir, enabledLinters); err != nil {
				fmt.Printf("Warning: Failed to create some linter configs: %v\n", err)
			}
		}
	}

	// Apply built-in rules and variable interpolation
	if err := config.ApplyBuiltinRules(generatedConfig); err != nil {
		return fmt.Errorf("failed to apply built-in rules: %w", err)
	}

	if err := config.InterpolateVariables(generatedConfig); err != nil {
		return fmt.Errorf("failed to interpolate variables: %w", err)
	}

	// Write the configuration file
	return writeConfigFile(configPath, generatedConfig)
}

func generateNonInteractiveConfig(targetDir string, styleGuideName string, strictnessLevel string) *models.Config {
	// Detect languages
	detectedLanguages, _ := languages.DetectLanguagesInDirectory(targetDir)

	generatedConfig := &models.Config{
		Version:      "1.0",
		Debounce:     "30s",
		Variables:    make(map[string]interface{}),
		BuiltinRules: make(map[string]models.BuiltinRuleConfig),
		Rules:        make(map[string]models.RuleConfig),
		Linters:      make(map[string]models.LinterConfig),
		Languages:    make(map[string]models.LanguageConfig),
	}

	// Apply style guide if specified
	if styleGuideName != "" {
		if err := config.ApplyStyleGuide(generatedConfig, styleGuideName); err != nil {
			fmt.Printf("Warning: Failed to apply style guide %s: %v\n", styleGuideName, err)
		}
	}

	// Add detected languages
	for _, lang := range detectedLanguages {
		generatedConfig.Languages[lang] = models.LanguageConfig{
			Includes: languages.GetDefaultIncludesForLanguage(lang),
		}
	}

	// Apply strictness-based variables
	strictness := config.StrictnessLevel(strictnessLevel)
	for _, lang := range detectedLanguages {
		practices := config.GetBestPracticesForLanguage(lang, strictness)
		for k, v := range practices {
			generatedConfig.Variables[k] = v
		}
	}

	// Enable default built-in rules based on strictness
	for name, rule := range config.BuiltinRules {
		if strictnessLevel == "strict" || rule.Default {
			generatedConfig.BuiltinRules[name] = models.BuiltinRuleConfig{
				Enabled: true,
			}
		}
	}

	// Detect and enable linters with existing configs
	linterConfigs, _ := config.DetectLinterConfigs(targetDir)
	for _, lang := range detectedLanguages {
		if linters, ok := config.DefaultLintersByLanguage[lang]; ok {
			for _, linter := range linters {
				if hasConfig, found := linterConfigs[linter]; found && hasConfig {
					generatedConfig.Linters[linter] = models.LinterConfig{
						Enabled: true,
					}
				}
			}
		}
	}

	return generatedConfig
}

func writeConfigFile(configPath string, config *models.Config) error {
	// Marshal to YAML
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	// Add header comments
	header := `# arch-unit.yaml - Architecture Rules Configuration
# Generated by arch-unit init
#
# This file defines architectural constraints and code quality standards
# for your codebase using arch-unit.
#
`

	if config.GeneratedFrom != "" {
		header += fmt.Sprintf("# Generated from: %s\n\n", config.GeneratedFrom)
	}

	yamlStr := header + string(yamlData)

	// Add helpful footer
	footer := `

# To learn more about arch-unit configuration:
# - Run 'arch-unit check' to validate your codebase
# - Add custom rules in the 'rules' section
# - Modify variables to adjust thresholds
# - Enable/disable built-in rules as needed
`
	yamlStr += footer

	// Write file
	if err := os.WriteFile(configPath, []byte(yamlStr), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	fmt.Printf("✓ Created %s\n", configPath)

	// Provide summary
	if len(config.Languages) > 0 {
		fmt.Println("\nConfigured for languages:")
		for lang := range config.Languages {
			fmt.Printf("  • %s\n", lang)
		}
	}

	if len(config.Linters) > 0 {
		fmt.Println("\nEnabled linters:")
		for linter, cfg := range config.Linters {
			if cfg.Enabled {
				fmt.Printf("  • %s\n", linter)
			}
		}
	}

	enabledRules := 0
	for _, rule := range config.BuiltinRules {
		if rule.Enabled {
			enabledRules++
		}
	}
	if enabledRules > 0 {
		fmt.Printf("\nEnabled %d built-in rules\n", enabledRules)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. Review and customize arch-unit.yaml")
	fmt.Println("  2. Run 'arch-unit check' to analyze your codebase")

	return nil
}

func createYAMLConfigFile(targetDir string) error {
	configPath := filepath.Join(targetDir, config.ConfigFileName)

	// Check if file exists
	if _, err := os.Stat(configPath); err == nil && !force {
		return fmt.Errorf("%s file already exists. Use --force to overwrite", config.ConfigFileName)
	}

	// Create smart default config based on detected languages and linter configs
	fmt.Println("Detecting languages and linter configurations in the project...")
	defaultConfig, err := config.CreateSmartDefaultConfig(targetDir)
	if err != nil {
		// Fall back to minimal config if detection fails
		fmt.Println("Warning: Could not detect languages, using minimal configuration")
		defaultConfig = config.CreateMinimalDefaultConfig()
	}

	// Log config detection results
	linterConfigs, _ := config.DetectLinterConfigs(targetDir)
	if info := config.GetLinterConfigInfo(linterConfigs); info != "" {
		fmt.Println(info)
	}

	// Marshal to YAML with custom formatting for better readability
	yamlData, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal default configuration: %w", err)
	}

	// Add helpful comments to the generated YAML
	yamlStr := string(yamlData)

	// Prepend header comments
	header := `# arch-unit.yaml - Architecture Analysis Configuration
# This file was auto-generated based on detected languages in your project.
# Customize it to define your architecture rules and linter settings.

`

	// Add comments about detected languages
	if len(defaultConfig.Languages) > 0 {
		header += "# Detected languages: "
		first := true
		for lang := range defaultConfig.Languages {
			if !first {
				header += ", "
			}
			header += lang
			first = false
		}
		header += "\n# Enabled linters: "
		first = true
		for linter, config := range defaultConfig.Linters {
			if config.Enabled {
				if !first {
					header += ", "
				}
				header += linter
				first = false
			}
		}
		header += "\n\n"
	} else {
		header += "# No languages detected. Add language configurations as needed.\n\n"
	}

	yamlStr = header + yamlStr

	// Add helpful footer
	footer := `
# To add custom architecture rules, add them to the 'rules' section:
# rules:
#   "**":
#     imports:
#       - "!internal/*"  # Deny access to internal packages
#       - "!*_test"      # Deny test packages in production code

# To customize linter settings, modify the 'linters' section above.
# To add custom file exclusions, use the 'global_excludes' section.
`
	yamlStr += footer

	// Write file
	if err := os.WriteFile(configPath, []byte(yamlStr), 0644); err != nil {
		return fmt.Errorf("failed to create %s file: %w", config.ConfigFileName, err)
	}

	fmt.Printf("✓ Created %s file at %s\n", config.ConfigFileName, configPath)

	// Provide feedback on what was detected
	if len(defaultConfig.Languages) > 0 {
		fmt.Println("  Auto-detected configuration based on your project languages")
		for linter, config := range defaultConfig.Linters {
			if config.Enabled {
				fmt.Printf("  • %s enabled\n", linter)
			}
		}
	}

	fmt.Println("\n  Edit this file to customize your architecture rules and linter settings")
	fmt.Println("  Run 'arch-unit check' to validate your codebase")

	return nil
}
