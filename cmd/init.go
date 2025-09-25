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
