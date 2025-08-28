package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/linters/eslint"
	"github.com/flanksource/arch-unit/linters/golangci"
	"github.com/flanksource/arch-unit/linters/markdownlint"
	"github.com/flanksource/arch-unit/linters/pyright"
	"github.com/flanksource/arch-unit/linters/ruff"
	"github.com/flanksource/arch-unit/linters/vale"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate default configuration file",
	Long: `Generate or display arch-unit.yaml configuration with comprehensive examples and documentation.

This command creates a complete configuration file containing:
- Language-specific file patterns (Go, Python, JS/TS, Markdown)
- Linter configurations with built-in defaults
- Architecture rule examples
- Advanced configuration options

CONFIGURATION STRUCTURE:

  version: "v1"                    # Configuration format version
  
  rules:                          # Architecture dependency rules
    - name: "Rule Name"
      description: "Rule purpose"
      forbid:
        - from: "source/**"       # Source pattern
          to: "target/**"         # Target pattern restriction
  
  languages:                      # File pattern definitions
    go:
      includes: ["**/*.go"]       # Go source files
      excludes: ["**/vendor/**"]  # Exclude patterns
      
  linters:                        # External linter configuration
    golangci-lint:
      enabled: true               # Enable/disable linter
      languages: ["go"]           # Target languages
      args: ["--config=.golangci.yml"]  # CLI arguments

LINTER CONFIGURATION:

  Available Linters:
    golangci-lint  - Go static analysis (recommended for Go projects)
    ruff          - Fast Python linter and formatter
    pyright       - Python/TypeScript type checker
    eslint        - JavaScript/TypeScript linter
    markdownlint  - Markdown style and syntax checker
    vale          - Prose and documentation linter
  
  Linter Options:
    enabled: true/false           # Enable linter
    languages: ["go", "python"]   # Target languages
    includes: ["custom/**"]       # Additional file patterns
    excludes: ["ignore/**"]       # Exclusion patterns
    args: ["--config", "file"]    # Command-line arguments
    output_format: "json"         # Output format preference

ARCHITECTURE PATTERNS:

  Layered Architecture:
    rules:
      - name: "Controller Layer"
        forbid:
          - from: "**/controllers/**"
            to: "**/models/**"    # Controllers can't access models directly
          - from: "**/controllers/**"
            to: "database/**"     # Controllers can't access DB
  
  Test Isolation:
    rules:
      - name: "No Test in Production"
        forbid:
          - from: "**/*.go"
            to: "testing"         # No test imports in production
          - from: "**/*.go"
            to: "*_test"          # No test package imports
  
  Import Restrictions:
    rules:
      - name: "Logging Standards"
        forbid:
          - from: "**/*.go"
            to: "fmt:Print*"      # Use structured logging
          - from: "**/*.go"
            to: "log:Print*"      # Instead of direct printing

MIGRATION FROM .ARCHUNIT:

  .ARCHUNIT Format (Simple):
    !internal/                    # Deny internal package access
    fmt:!Println                  # Deny fmt.Println method
    [*_test.go] +testing         # Allow testing in test files
  
  arch-unit.yaml Equivalent:
    rules:
      - name: "Internal Access"
        forbid:
          - from: "**"
            to: "internal/**"
      - name: "Direct Printing"
        forbid:
          - from: "**/*.go"
            to: "fmt:Println"
      - name: "Test Files"
        allow:
          - from: "**/*_test.go"
            to: "testing"

CONFIGURATION EXAMPLES:

  Basic Go Project:
    arch-unit config                    # Generate complete config template
    arch-unit config -o my-config.yaml  # Save to custom file
    
  Python Project:
    languages:
      python:
        includes: ["src/**/*.py", "tests/**/*.py"]
        excludes: ["**/__pycache__/**", "*.pyc"]
    linters:
      ruff:
        enabled: true
        args: ["--select=E,W,F", "--ignore=E501"]
  
  Multi-language Project:
    languages:
      go:
        includes: ["**/*.go"]
      python:
        includes: ["**/*.py"]  
      javascript:
        includes: ["**/*.js", "**/*.jsx"]
    linters:
      golangci-lint: { enabled: true }
      ruff: { enabled: true }
      eslint: { enabled: true }

ADVANCED OPTIONS:

  Performance Tuning:
    performance:
      workers: 8                  # Parallel analysis workers
      smart_debounce: true        # Prevent rapid re-runs
  
  CI/CD Integration:
    integrations:
      ci:
        fail_on_violations: true  # Exit code 1 on violations
        exit_codes:
          violations_found: 1
          analysis_error: 2
  
  Output Customization:
    output:
      relative_paths: true        # Show relative paths
      group_by: "file"           # Group violations by file
      max_violations_per_file: 10 # Limit output per file

EXAMPLES:

  Generate Configuration:
    arch-unit config                              # Display full template
    arch-unit config -o arch-unit.yaml          # Save to file
    arch-unit config -o /path/config.yaml       # Custom path
    arch-unit config -o config.yaml --force     # Overwrite existing

  View Current Configuration:
    cat arch-unit.yaml                          # View current config
    arch-unit check --linters=none             # Test architecture rules only
    arch-unit check --linters=golangci-lint    # Test specific linter config`,
	RunE: runConfig,
}

var (
	configOutputPath string
	configForce      bool
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.Flags().StringVarP(&configOutputPath, "output", "o", "-", "Output path for config file ('-' for stdout, default: arch-unit.yaml)")
	configCmd.Flags().BoolVarP(&configForce, "force", "f", false, "Overwrite existing config file")
}

func runConfig(cmd *cobra.Command, args []string) error {
	// Generate config content
	content := generateDefaultConfig()

	// Handle stdout output
	if configOutputPath == "-" {
		fmt.Print(content)
		return nil
	}

	// Determine output path
	outputPath := "arch-unit.yaml"
	if configOutputPath != "" {
		outputPath = configOutputPath
	}

	// Make absolute path
	absPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err == nil && !configForce {
		return fmt.Errorf("config file already exists at %s (use --force to overwrite)", absPath)
	}

	// Create directory if needed
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write file
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✓ Generated default configuration file: %s\n", absPath)
	fmt.Println()
	fmt.Println("The configuration includes:")
	fmt.Println("  • All available linters with built-in defaults marked as 'BUILTIN'")
	fmt.Println("  • Example architecture rules for common scenarios")
	fmt.Println("  • Comprehensive documentation for all options")
	fmt.Println()
	fmt.Println("Edit the file to customize includes/excludes for your project.")
	fmt.Println("Built-in patterns are always applied in addition to your custom patterns.")

	return nil
}

func generateDefaultConfig() string {
	var sb strings.Builder

	// Header with explanation
	sb.WriteString(`# arch-unit.yaml - Architecture Analysis Configuration
#
# This file configures arch-unit for analyzing your codebase.
# Built-in defaults are marked with 'BUILTIN' comments - these are always
# applied in addition to any custom patterns you specify.

# Global configuration
version: "v1"

# Architecture rules - define forbidden dependencies and patterns
rules:
  # Example: Prevent direct database access from controllers
  - name: "No DB in Controllers"
    description: "Controllers should not directly access database"
    forbid:
      - from: "**/*controller*"
        to: "database/**"
        
  # Example: Prevent circular dependencies between packages
  - name: "No Circular Dependencies"
    description: "Prevent circular imports between core packages"
    forbid:
      - from: "pkg/models/**"
        to: "pkg/services/**"
      - from: "pkg/services/**" 
        to: "pkg/controllers/**"
        
  # Example: Logging restrictions
  - name: "No Direct Logging"
    description: "Use structured logging instead of fmt package"
    forbid:
      - from: "**/*.go"
        to: "fmt:Print*"
      - from: "**/*.go"
        to: "log:Print*"

# Language-specific configuration
languages:
  go:
    includes:
      - "**/*.go"      # BUILTIN: Go source files
      # Add your custom Go patterns here
    excludes:
      # Built-in Go exclusions (vendor/**, .git/**, examples/**, hack/**) are automatic
      - "**/*_gen.go"       # Generated Go files
      - "**/*.pb.go"        # Protocol buffer files
      - "**/testdata/**"    # Go test data directories
      # Add your custom Go exclusions here
    
  python:
    includes:
      - "**/*.py"      # BUILTIN: Python source files
      - "**/*.pyi"     # BUILTIN: Python stub files
      # Add your custom Python patterns here
    excludes:
      # Built-in Python exclusions (__pycache__/**, .venv/**, examples/**, hack/**) are automatic
      - "*.pyc"             # Compiled Python files
      - "*.pyo"             # Optimized Python files
      - "*.egg-info/**"     # Python package metadata
      # Add your custom Python exclusions here
    
  javascript:
    includes:
      - "**/*.js"      # BUILTIN: JavaScript files
      - "**/*.jsx"     # BUILTIN: React JSX files
      - "**/*.mjs"     # BUILTIN: ES modules
      - "**/*.cjs"     # BUILTIN: CommonJS modules
      # Add your custom JavaScript patterns here
    excludes:
      # Built-in JavaScript exclusions (node_modules/**, build/**, dist/**, examples/**, hack/**) are automatic
      - "bower_components/**" # Bower packages (legacy)
      - "jspm_packages/**"    # JSPM packages (legacy)
      - "public/**"           # Public assets
      - ".cache/**"           # Cache directories
      # Add your custom JavaScript exclusions here
      
  typescript:
    includes:
      - "**/*.ts"      # BUILTIN: TypeScript files
      - "**/*.tsx"     # BUILTIN: TypeScript JSX files
      # Add your custom TypeScript patterns here
    excludes:
      # Built-in TypeScript exclusions (node_modules/**, build/**, dist/**, examples/**, hack/**) are automatic
      - "*.d.ts"            # TypeScript declaration files (generated)
      # Add your custom TypeScript exclusions here
      
  markdown:
    includes:
      - "**/*.md"      # BUILTIN: Markdown files
      - "**/*.mdx"     # BUILTIN: MDX files
      - "**/*.markdown" # BUILTIN: Markdown variant
      # Add your custom Markdown patterns here
    excludes:
      # Built-in Markdown exclusions (node_modules/**, examples/**, hack/**) are automatic
      - "*.min.md"          # Minified markdown files
      - "LICENSE*"          # License files (different writing style)
      - "CHANGELOG*"        # Changelog files (different writing style)
      # Add your custom Markdown exclusions here

# Global excludes (applied to all languages)
# Note: Built-in patterns are automatically included and don't need to be specified here
global_excludes:
  - "**/test/**"          # Example: Exclude test directories
  - "**/*_test.*"         # Example: Exclude test files
  # Add your custom global exclusions here
  # Built-in patterns like .git/**, examples/**, hack/**, node_modules/** are automatically excluded

# Linter configurations
linters:
`)

	// Generate linter configurations with built-in defaults
	linters := []struct {
		name     string
		instance interface {
			Name() string
			DefaultIncludes() []string
			DefaultExcludes() []string
		}
		enabled     bool
		description string
		languages   []string
	}{
		{"golangci-lint", golangci.NewGolangciLint("."), true, "Go static analysis and linting", []string{"go"}},
		{"ruff", ruff.NewRuff("."), false, "Fast Python linter", []string{"python"}},
		{"pyright", pyright.NewPyright("."), false, "Python and TypeScript type checker", []string{"python", "typescript"}},
		{"eslint", eslint.NewESLint("."), false, "JavaScript/TypeScript linter", []string{"javascript", "typescript"}},
		{"markdownlint", markdownlint.NewMarkdownlint("."), false, "Markdown style checker", []string{"markdown"}},
		{"vale", vale.NewVale("."), false, "Prose and documentation linter", []string{"markdown"}},
	}

	for _, linter := range linters {
		sb.WriteString(fmt.Sprintf("  %s:\n", linter.name))
		sb.WriteString(fmt.Sprintf("    # %s\n", linter.description))
		sb.WriteString(fmt.Sprintf("    enabled: %t\n", linter.enabled))

		// Add languages
		sb.WriteString("    languages:\n")
		for _, lang := range linter.languages {
			sb.WriteString(fmt.Sprintf("      - %s\n", lang))
		}

		sb.WriteString("    # Language patterns are inherited from the languages section above\n")
		sb.WriteString("    # Additional linter-specific patterns:\n")
		sb.WriteString("    includes:\n")

		// Add any linter-specific built-in includes that aren't language-based
		builtinIncludes := linter.instance.DefaultIncludes()
		if len(builtinIncludes) > 0 {
			sb.WriteString("      # BUILTIN linter-specific patterns (in addition to language patterns):\n")
			for _, include := range builtinIncludes {
				sb.WriteString(fmt.Sprintf("      # - \"%s\"  # BUILTIN: %s specific\n", include, linter.name))
			}
		}
		sb.WriteString("      # Add your custom includes here\n")

		sb.WriteString("    excludes:\n")

		// Add linter-specific built-in excludes
		builtinExcludes := linter.instance.DefaultExcludes()
		if len(builtinExcludes) > 0 {
			sb.WriteString("      # BUILTIN linter-specific patterns (in addition to language patterns):\n")
			for _, exclude := range builtinExcludes {
				sb.WriteString(fmt.Sprintf("      - \"%s\"  # BUILTIN: %s specific\n", exclude, linter.name))
			}
		}
		sb.WriteString("      # Add your custom excludes here\n")

		// Add common configuration options
		switch linter.name {
		case "golangci-lint":
			sb.WriteString("    args:\n")
			sb.WriteString("      # - \"--config=.golangci.yml\"  # Custom golangci-lint config\n")
			sb.WriteString("      # - \"--enable=gosec,gocritic\" # Enable specific linters\n")

		case "ruff":
			sb.WriteString("    args:\n")
			sb.WriteString("      # - \"--select=E,W,F\"    # Select specific rule categories\n")
			sb.WriteString("      # - \"--ignore=E501\"     # Ignore specific rules\n")

		case "pyright":
			sb.WriteString("    args:\n")
			sb.WriteString("      # - \"--project=pyrpoject.toml\"  # Custom pyproject.toml config\n")

		case "eslint":
			sb.WriteString("    args:\n")
			sb.WriteString("      # - \"--config=.eslintrc.js\"     # Custom ESLint config\n")
			sb.WriteString("      # - \"--ext=.js,.jsx,.ts,.tsx\"   # File extensions\n")

		case "markdownlint":
			sb.WriteString("    args:\n")
			sb.WriteString("      # - \"--config=.markdownlint.json\"  # Custom config file\n")
			sb.WriteString("      # - \"--disable MD013\"             # Disable specific rules\n")

		case "vale":
			sb.WriteString("    args:\n")
			sb.WriteString("      # - \"--config=.vale.ini\"     # Custom Vale config\n")
			sb.WriteString("      # - \"--minAlertLevel=error\"  # Minimum alert level\n")
		}

		sb.WriteString("\n")
	}

	// Add footer with additional information
	sb.WriteString(`
# Advanced Configuration Options

# Cache configuration (optional)
cache:
  # Enable/disable violation caching for faster subsequent runs
  enabled: true
  
  # Cache directory (default: ~/.cache/arch-unit)
  # dir: "/custom/cache/path"

# Output formatting options
output:
  # Show relative paths instead of absolute paths
  relative_paths: true
  
  # Group violations by file or by rule
  group_by: "file"  # Options: "file", "rule", "linter"
  
  # Maximum violations to display per file (0 = unlimited)
  max_violations_per_file: 0

# Performance settings
performance:
  # Number of parallel workers for analysis
  workers: 4
  
  # Enable intelligent debouncing to prevent rapid re-runs
  smart_debounce: true

# Integration settings
integrations:
  # GitHub Actions integration
  github_actions:
    enabled: false
    # annotate_pr: true  # Add PR comments for violations
  
  # CI/CD settings  
  ci:
    # Fail build on any violations
    fail_on_violations: true
    
    # Exit codes for different scenarios
    exit_codes:
      violations_found: 1
      analysis_error: 2
      config_error: 3

# Development settings
development:
  # Enable debug logging
  debug: false
  
  # Show timing information
  show_timing: false
  
  # Validate configuration on startup
  validate_config: true
`)

	return sb.String()
}
