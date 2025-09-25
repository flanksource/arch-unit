package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/config"
	"github.com/flanksource/arch-unit/languages"
	_ "github.com/flanksource/arch-unit/languages/handlers" // Register language handlers
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
)

// InitQuestions holds the answers from the interactive questionnaire
type InitQuestions struct {
	// Language Detection
	DetectedLanguages   []string
	ConfirmLanguages    bool
	AdditionalLanguages []string

	// Linter Setup (optional, only during init)
	DetectedLinters     map[string]bool
	SetupMissingLinters bool
	EnabledLinters      map[string]bool

	// Architecture
	ArchitecturePattern string // "layered", "clean", "hexagonal", "none"

	// Code Quality
	StrictnessLevel string // "strict", "moderate", "lenient"
	StyleGuide      string // "google", "airbnb", "pep8", "custom"

	// Built-in Rules
	EnabledBuiltinRules map[string]bool
}

// RunInteractiveInit runs the interactive initialization
func RunInteractiveInit(targetDir string) (*models.Config, error) {
	fmt.Println("🚀 Welcome to arch-unit interactive setup!")
	fmt.Println("This wizard will help you configure architecture rules and code quality standards.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	questions := &InitQuestions{
		EnabledLinters:      make(map[string]bool),
		EnabledBuiltinRules: make(map[string]bool),
	}

	// Step 1: Language Detection
	if err := detectAndConfirmLanguages(targetDir, questions, scanner); err != nil {
		return nil, err
	}

	// Step 2: Style Guide Selection
	if err := selectStyleGuide(questions, scanner); err != nil {
		return nil, err
	}

	// Step 3: Strictness Level
	if err := selectStrictnessLevel(questions, scanner); err != nil {
		return nil, err
	}

	// Step 4: Architecture Pattern
	if err := selectArchitecturePattern(questions, scanner); err != nil {
		return nil, err
	}

	// Step 5: Linter Detection and Setup
	if err := detectAndConfigureLinters(targetDir, questions, scanner); err != nil {
		return nil, err
	}

	// Step 6: Built-in Rules Selection
	if err := selectBuiltinRules(questions, scanner); err != nil {
		return nil, err
	}

	// Generate configuration based on answers
	return generateConfigFromAnswers(questions), nil
}

func detectAndConfirmLanguages(targetDir string, questions *InitQuestions, scanner *bufio.Scanner) error {
	// Detect languages
	detectedLanguages, err := languages.DetectLanguagesInDirectory(targetDir)
	if err != nil {
		logger.Warnf("Failed to detect languages: %v", err)
		detectedLanguages = []string{}
	}

	questions.DetectedLanguages = detectedLanguages

	if len(detectedLanguages) > 0 {
		fmt.Printf("📂 Detected languages: %s\n", strings.Join(detectedLanguages, ", "))
		fmt.Print("Is this correct? (y/n) [y]: ")

		scanner.Scan()
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		questions.ConfirmLanguages = answer != "n" && answer != "no"

		if !questions.ConfirmLanguages {
			fmt.Print("Enter languages (comma-separated, e.g., go,python,java): ")
			scanner.Scan()
			langs := strings.Split(scanner.Text(), ",")
			for i, lang := range langs {
				langs[i] = strings.TrimSpace(lang)
			}
			questions.DetectedLanguages = langs
		}
	} else {
		fmt.Println("No languages detected.")
		fmt.Print("Enter languages to configure (comma-separated, e.g., go,python,java): ")
		scanner.Scan()
		langs := strings.Split(scanner.Text(), ",")
		for i, lang := range langs {
			langs[i] = strings.TrimSpace(lang)
		}
		questions.DetectedLanguages = langs
	}

	return nil
}

func selectStyleGuide(questions *InitQuestions, scanner *bufio.Scanner) error {
	fmt.Println("\n📋 Select a style guide:")

	// Build options based on detected languages
	options := []string{"custom"}
	optionMap := make(map[string]string)

	for _, lang := range questions.DetectedLanguages {
		// Try to get handler from registry
		if handler, ok := languages.DefaultRegistry.GetHandler(lang); ok {
			styleGuides := handler.GetStyleGuideOptions()
			for _, guide := range styleGuides {
				if _, exists := optionMap[guide.ID]; !exists {
					options = append(options, guide.ID)
					optionMap[guide.ID] = guide.DisplayName
				}
			}
		} else {
			// Fallback for languages not yet in registry
			switch lang {
			case "java":
				options = append(options, "google-java")
				optionMap["google-java"] = "Google Java Style Guide"
			case "javascript", "typescript":
				if _, exists := optionMap["airbnb-javascript"]; !exists {
					options = append(options, "airbnb-javascript")
					optionMap["airbnb-javascript"] = "Airbnb JavaScript Style Guide"
				}
			case "rust":
				options = append(options, "rust-official")
				optionMap["rust-official"] = "Rust Official Style Guide"
			}
		}
	}

	// Display options
	for i, opt := range options {
		if desc, exists := optionMap[opt]; exists {
			fmt.Printf("  %d. %s - %s\n", i+1, opt, desc)
		} else {
			fmt.Printf("  %d. %s\n", i+1, opt)
		}
	}

	fmt.Printf("Select style guide [1-%d] [1]: ", len(options))
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	if input == "" {
		questions.StyleGuide = options[0]
	} else {
		var idx int
		_, _ = fmt.Sscanf(input, "%d", &idx)
		if idx > 0 && idx <= len(options) {
			questions.StyleGuide = options[idx-1]
		} else {
			questions.StyleGuide = "custom"
		}
	}

	fmt.Printf("✓ Selected: %s\n", questions.StyleGuide)
	return nil
}

func selectStrictnessLevel(questions *InitQuestions, scanner *bufio.Scanner) error {
	fmt.Println("\n🎯 Select strictness level:")
	fmt.Println("  1. strict   - Lower thresholds, more rules, best for new projects")
	fmt.Println("  2. moderate - Balanced defaults, good for most projects")
	fmt.Println("  3. lenient  - Higher thresholds, fewer rules, good for legacy code")

	fmt.Print("Select strictness [1-3] [2]: ")
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	switch input {
	case "1":
		questions.StrictnessLevel = "strict"
	case "3":
		questions.StrictnessLevel = "lenient"
	default:
		questions.StrictnessLevel = "moderate"
	}

	fmt.Printf("✓ Selected: %s\n", questions.StrictnessLevel)
	return nil
}

func selectArchitecturePattern(questions *InitQuestions, scanner *bufio.Scanner) error {
	fmt.Println("\n🏗️  Select architecture pattern:")
	fmt.Println("  1. none      - No specific architecture rules")
	fmt.Println("  2. layered   - Layered architecture (presentation, business, data)")
	fmt.Println("  3. clean     - Clean architecture (domain, application, infrastructure)")
	fmt.Println("  4. hexagonal - Hexagonal/Ports & Adapters architecture")

	fmt.Print("Select architecture [1-4] [1]: ")
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	switch input {
	case "2":
		questions.ArchitecturePattern = "layered"
	case "3":
		questions.ArchitecturePattern = "clean"
	case "4":
		questions.ArchitecturePattern = "hexagonal"
	default:
		questions.ArchitecturePattern = "none"
	}

	fmt.Printf("✓ Selected: %s\n", questions.ArchitecturePattern)
	return nil
}

func detectAndConfigureLinters(targetDir string, questions *InitQuestions, scanner *bufio.Scanner) error {
	fmt.Println("\n🔍 Detecting linter configurations...")

	// Detect existing linter configs
	linterConfigs, err := config.DetectLinterConfigs(targetDir)
	if err != nil {
		logger.Warnf("Failed to detect linter configs: %v", err)
		linterConfigs = make(map[string]bool)
	}

	questions.DetectedLinters = linterConfigs

	// Show detected linters
	foundLinters := []string{}
	missingLinters := []string{}

	for _, lang := range questions.DetectedLanguages {
		if linters, ok := config.DefaultLintersByLanguage[lang]; ok {
			for _, linter := range linters {
				if hasConfig, found := linterConfigs[linter]; found && hasConfig {
					foundLinters = append(foundLinters, linter)
					questions.EnabledLinters[linter] = true
				} else {
					missingLinters = append(missingLinters, linter)
				}
			}
		}
	}

	if len(foundLinters) > 0 {
		fmt.Printf("✓ Found configurations for: %s\n", strings.Join(foundLinters, ", "))
	}

	if len(missingLinters) > 0 {
		fmt.Printf("⚠️  No configurations found for: %s\n", strings.Join(missingLinters, ", "))
		fmt.Print("Would you like to create default configurations for missing linters? (y/n) [n]: ")

		scanner.Scan()
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		questions.SetupMissingLinters = answer == "y" || answer == "yes"

		if questions.SetupMissingLinters {
			for _, linter := range missingLinters {
				questions.EnabledLinters[linter] = true
			}
			fmt.Println("✓ Will create default configurations for missing linters")
		}
	}

	return nil
}

func selectBuiltinRules(questions *InitQuestions, scanner *bufio.Scanner) error {
	fmt.Println("\n📚 Select built-in rules to enable:")

	// Group rules by category
	rulesByCategory := make(map[config.BuiltinRuleCategory][]string)
	for name, rule := range config.BuiltinRules {
		// Check if rule applies to selected languages
		if len(rule.Languages) > 0 {
			applies := false
			for _, lang := range questions.DetectedLanguages {
				for _, ruleLang := range rule.Languages {
					if lang == ruleLang {
						applies = true
						break
					}
				}
			}
			if !applies {
				continue
			}
		}
		rulesByCategory[rule.Category] = append(rulesByCategory[rule.Category], name)
	}

	// Display by category
	categories := []config.BuiltinRuleCategory{
		config.CategoryArchitecture,
		config.CategorySecurity,
		config.CategoryQuality,
		config.CategoryTesting,
		config.CategoryPerformance,
	}

	for _, category := range categories {
		rules, exists := rulesByCategory[category]
		if !exists || len(rules) == 0 {
			continue
		}

		fmt.Printf("\n%s Rules:\n", strings.Title(string(category)))
		for _, ruleName := range rules {
			rule := config.BuiltinRules[ruleName]

			// Default selection based on strictness
			defaultEnabled := rule.Default
			if questions.StrictnessLevel == "strict" {
				defaultEnabled = true
			} else if questions.StrictnessLevel == "lenient" {
				defaultEnabled = false
			}

			fmt.Printf("  - %s: %s\n", ruleName, rule.Description)
			fmt.Printf("    Enable? (y/n) [%s]: ", boolToYN(defaultEnabled))

			scanner.Scan()
			input := strings.ToLower(strings.TrimSpace(scanner.Text()))

			if input == "" {
				questions.EnabledBuiltinRules[ruleName] = defaultEnabled
			} else {
				questions.EnabledBuiltinRules[ruleName] = input == "y" || input == "yes"
			}
		}
	}

	return nil
}

func generateConfigFromAnswers(questions *InitQuestions) *models.Config {
	generatedConfig := &models.Config{
		Version:      "1.0",
		Debounce:     "30s",
		Variables:    make(map[string]interface{}),
		BuiltinRules: make(map[string]models.BuiltinRuleConfig),
		Rules:        make(map[string]models.RuleConfig),
		Linters:      make(map[string]models.LinterConfig),
		Languages:    make(map[string]models.LanguageConfig),
	}

	// Apply style guide if selected
	if questions.StyleGuide != "custom" && questions.StyleGuide != "" {
		if err := config.ApplyStyleGuide(generatedConfig, questions.StyleGuide); err != nil {
			logger.Warnf("Failed to apply style guide %s: %v", questions.StyleGuide, err)
		}
	}

	// Add languages
	for _, lang := range questions.DetectedLanguages {
		generatedConfig.Languages[lang] = models.LanguageConfig{
			Includes: languages.GetDefaultIncludesForLanguage(lang),
		}
	}

	// Apply strictness-based variables
	strictness := config.StrictnessLevel(questions.StrictnessLevel)
	for _, lang := range questions.DetectedLanguages {
		practices := config.GetBestPracticesForLanguage(lang, strictness)
		for k, v := range practices {
			generatedConfig.Variables[k] = v
		}
	}

	// Enable selected built-in rules
	for ruleName, enabled := range questions.EnabledBuiltinRules {
		if enabled {
			generatedConfig.BuiltinRules[ruleName] = models.BuiltinRuleConfig{
				Enabled: true,
			}
		}
	}

	// Apply architecture pattern
	switch questions.ArchitecturePattern {
	case "layered":
		generatedConfig.BuiltinRules["layered_architecture"] = models.BuiltinRuleConfig{Enabled: true}
	case "clean":
		generatedConfig.BuiltinRules["clean_architecture"] = models.BuiltinRuleConfig{Enabled: true}
	case "hexagonal":
		// Would need to implement hexagonal architecture rules
		generatedConfig.BuiltinRules["clean_architecture"] = models.BuiltinRuleConfig{Enabled: true}
	}

	// Enable linters
	for linter, enabled := range questions.EnabledLinters {
		if enabled {
			generatedConfig.Linters[linter] = models.LinterConfig{
				Enabled: true,
			}
		}
	}

	// Apply built-in rules to generate import rules
	if err := config.ApplyBuiltinRules(generatedConfig); err != nil {
		logger.Warnf("Failed to apply built-in rules: %v", err)
	}

	// Add default quality rules based on language
	for _, lang := range questions.DetectedLanguages {
		pattern := getPatternForLanguage(lang)
		if pattern != "" {
			if _, exists := generatedConfig.Rules[pattern]; !exists {
				generatedConfig.Rules[pattern] = models.RuleConfig{
					Quality: &models.QualityConfig{
						MaxFileLength:       getVariableInt("max_file_length", generatedConfig.Variables, 400),
						MaxFunctionNameLen:  getVariableInt("max_function_length", generatedConfig.Variables, 50),
						MaxVariableNameLen:  getVariableInt("max_variable_name_length", generatedConfig.Variables, 30),
						MaxParameterNameLen: getVariableInt("max_parameter_name_length", generatedConfig.Variables, 25),
					},
				}
			}
		}
	}

	return generatedConfig
}

func getPatternForLanguage(lang string) string {
	// Use the language registry
	return languages.DefaultRegistry.GetFilePatternForLanguage(lang)
}

func getVariableInt(name string, variables map[string]interface{}, defaultVal int) int {
	if val, ok := variables[name]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		}
	}
	return defaultVal
}

func boolToYN(b bool) string {
	if b {
		return "y"
	}
	return "n"
}

// CreateLinterConfigs creates default linter configuration files if requested
func CreateLinterConfigs(targetDir string, linters map[string]bool) error {
	for linter, enabled := range linters {
		if !enabled {
			continue
		}

		switch linter {
		case "golangci-lint":
			if err := createGolangciConfig(targetDir); err != nil {
				return err
			}
		case "ruff":
			if err := createRuffConfig(targetDir); err != nil {
				return err
			}
		case "eslint":
			if err := createEslintConfig(targetDir); err != nil {
				return err
			}
		case "checkstyle":
			if err := createCheckstyleConfig(targetDir); err != nil {
				return err
			}
		}
	}
	return nil
}

func createGolangciConfig(targetDir string) error {
	configPath := filepath.Join(targetDir, ".golangci.yml")
	if _, err := os.Stat(configPath); err == nil {
		return nil // Already exists
	}

	config := `run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - typecheck
    - unused
    - gosec
    - gofmt
    - goimports

linters-settings:
  errcheck:
    check-type-assertions: true
  govet:
    check-shadowing: true
`

	return os.WriteFile(configPath, []byte(config), 0644)
}

func createRuffConfig(targetDir string) error {
	configPath := filepath.Join(targetDir, "ruff.toml")
	if _, err := os.Stat(configPath); err == nil {
		return nil // Already exists
	}

	config := `# Ruff configuration
line-length = 100

select = [
    "E",  # pycodestyle errors
    "W",  # pycodestyle warnings
    "F",  # pyflakes
    "N",  # pep8-naming
    "UP", # pyupgrade
    "B",  # flake8-bugbear
    "A",  # flake8-builtins
    "S",  # flake8-bandit
]

ignore = [
    "E501", # line too long (handled by formatter)
]

[per-file-ignores]
"*_test.py" = ["S101"] # Allow assert in tests
"test_*.py" = ["S101"]
`

	return os.WriteFile(configPath, []byte(config), 0644)
}

func createEslintConfig(targetDir string) error {
	configPath := filepath.Join(targetDir, ".eslintrc.json")
	if _, err := os.Stat(configPath); err == nil {
		return nil // Already exists
	}

	config := `{
  "env": {
    "browser": true,
    "es2021": true,
    "node": true
  },
  "extends": [
    "eslint:recommended"
  ],
  "parserOptions": {
    "ecmaVersion": "latest",
    "sourceType": "module"
  },
  "rules": {
    "no-console": "error",
    "no-unused-vars": "error",
    "no-undef": "error"
  }
}`

	return os.WriteFile(configPath, []byte(config), 0644)
}

func createCheckstyleConfig(targetDir string) error {
	configPath := filepath.Join(targetDir, "checkstyle.xml")
	if _, err := os.Stat(configPath); err == nil {
		return nil // Already exists
	}

	config := `<?xml version="1.0"?>
<!DOCTYPE module PUBLIC
    "-//Checkstyle//DTD Checkstyle Configuration 1.3//EN"
    "https://checkstyle.org/dtds/configuration_1_3.dtd">

<module name="Checker">
    <module name="TreeWalker">
        <!-- Naming Conventions -->
        <module name="TypeName"/>
        <module name="MethodName"/>
        <module name="PackageName"/>
        <module name="ParameterName"/>
        <module name="LocalVariableName"/>
        
        <!-- Code Quality -->
        <module name="MethodLength">
            <property name="max" value="40"/>
        </module>
        <module name="ParameterNumber">
            <property name="max" value="7"/>
        </module>
        
        <!-- Imports -->
        <module name="AvoidStarImport"/>
        <module name="UnusedImports"/>
    </module>
    
    <!-- File Length -->
    <module name="FileLength">
        <property name="max" value="2000"/>
    </module>
</module>`

	return os.WriteFile(configPath, []byte(config), 0644)
}
