package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/flanksource/arch-unit/config"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/linters"
	_ "github.com/flanksource/arch-unit/linters/aql"
	_ "github.com/flanksource/arch-unit/linters/archunit"

	// "github.com/flanksource/arch-unit/linters/comment" // Temporarily disabled
	_ "github.com/flanksource/arch-unit/linters/eslint"
	_ "github.com/flanksource/arch-unit/linters/golangci"
	_ "github.com/flanksource/arch-unit/linters/markdownlint"
	_ "github.com/flanksource/arch-unit/linters/pyright"
	_ "github.com/flanksource/arch-unit/linters/ruff"
	_ "github.com/flanksource/arch-unit/linters/vale"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	failOnViolation bool
	includePattern  string
	excludePattern  string
	lintersFlag     string
	fixFlag         bool
	noCacheFlag     bool
	taskMgrOptions  = clicky.DefaultTaskManagerOptions()
)

var checkCmd = &cobra.Command{
	Use:          "check [path] [files...]",
	Short:        "Check architecture violations in the codebase",
	SilenceUsage: true,
	Long: `Check for architecture violations using rules defined in .ARCHUNIT or arch-unit.yaml files.
Analyzes Go, Python, JavaScript, TypeScript, and Markdown files for architecture violations.

RULE FORMATS:

.ARCHUNIT FORMAT (Simple line-based rules):
  Basic Rules:
    pattern           Allow access (default)
    !pattern          Deny access
    +pattern          Override parent denial

  Package/Method Rules:
    package:method    Allow specific method
    package:!method   Deny specific method
    *:method          Apply to all packages

  File-Specific Rules:
    [pattern] rule    Apply rule only to matching files

  Examples:
    !internal/                   # Deny internal package access
    !fmt:Println                 # Deny fmt.Println usage
    *:!Test*                     # Deny test methods in all packages
    [*_test.go] +testing         # Allow testing package in test files
    [main.go] +fmt:Println       # Allow fmt.Println in main.go only

arch-unit.yaml FORMAT (Structured YAML rules):
  rules:
    "**":                        # All files
      imports:
        - "!internal/"           # Same rule syntax as .ARCHUNIT
        - "!fmt:Println"
        - "*:!Test*"
    "**/*_test.go":              # Test files only
      imports:
        - "+testing"             # Override denials for tests
        - "+fmt:Println"
    "cmd/*/main.go":             # Main files in cmd subdirectories
      imports:
        - "+fmt:Println"
        - "+os:Exit"

COMMON ARCHITECTURE PATTERNS:

  Layered Architecture:
    !internal/                   # No internal package access
    !database/sql                # Database access only via repository
    !gorm                        # ORM access only via repository
    !net/http:Get                # HTTP access only via client layer
    !net/http:Post

  Test Isolation:
    !testing                     # No test imports in production
    !*_test                      # No test package imports
    *:!Test*                     # No test methods
    [*_test.go] +testing         # Allow testing in test files
    [*_test.go] +*_test          # Allow test packages in test files

  Logging Standards:
    !fmt:Print*                  # No direct fmt printing
    !log:Print*                  # No direct log printing
    !os:Print*                   # Use structured logging instead
    [main.go] +fmt:Println       # Allow in main for demos
    [*_test.go] +fmt:Print*      # Allow in tests

  API Boundaries:
    !encoding/json:Unmarshal     # JSON handling via service layer
    !net/http:*                  # HTTP handling via client wrapper
    [*_handler.go] +net/http     # Allow HTTP in handlers only
    [*_client.go] +net/http      # Allow HTTP in client layer

LINTER CONTROL:

  --linters="*"                  # Run all configured linters (default)
  --linters=none                 # Skip all linters, architecture rules only
  --linters=golangci-lint        # Run specific linter only
  --linters="ruff,eslint"        # Run multiple specific linters
  --linters="arch-unit,ruff"     # Run architecture rules + specific linter

  Available linters: arch-unit, aql, comment-analysis, golangci-lint, ruff,
                     pyright, eslint, markdownlint, vale

  Note: Use 'arch-unit config --help' for linter configuration details.

FILE FILTERING:

  --include="*.go"               # Include only Go files
  --include="**/*_service.go"    # Include only service files
  --exclude="*_test.go"          # Exclude test files
  --exclude="vendor/**"          # Exclude vendor directory

EXAMPLES:

  Basic Usage:
    arch-unit check                      # Check current directory
    arch-unit check ./src                # Check specific directory
    arch-unit check . main.go service.go # Check specific files

  Architecture Rules Only:
    arch-unit check --linters=none       # Skip external linters
    arch-unit check --linters=arch-unit  # Only arch-unit rules

  With External Linters:
    arch-unit check --linters="*"                    # All configured linters
    arch-unit check --linters="golangci-lint,ruff"   # Specific linters
    arch-unit check --linters="arch-unit,eslint"     # Rules + specific linter

  File Filtering:
    arch-unit check --include="**/*.go" --exclude="*_test.go"
    arch-unit check --include="src/**" --linters=ruff

  Output Formats:
    arch-unit check --json                # JSON output
    arch-unit check --csv                 # CSV output
    arch-unit check --html -o report.html # HTML report

  Auto-fixing:
    arch-unit check --fix                 # Auto-fix violations where possible

  Performance:
    arch-unit check --no-cache             # Bypass cache and force re-analysis`,
	Args: cobra.ArbitraryArgs,
	RunE: runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)

	checkCmd.Flags().BoolVar(&failOnViolation, "fail-on-violation", true, "Exit with code 1 if violations are found")
	checkCmd.Flags().StringVar(&includePattern, "include", "", "Include files matching pattern (e.g., '*.go')")
	checkCmd.Flags().StringVar(&excludePattern, "exclude", "", "Exclude files matching pattern (e.g., '*_test.go')")
	checkCmd.Flags().StringVar(&lintersFlag, "linters", "*", "Linters to run ('*' for all configured, 'none' to skip, or comma-separated list e.g., 'golangci-lint,ruff,arch-unit')")
	checkCmd.Flags().BoolVar(&fixFlag, "fix", false, "Automatically fix violations where possible")
	checkCmd.Flags().BoolVar(&noCacheFlag, "no-cache", false, "Disable caching and force re-analysis of all files")

	// Bind TaskManager flags
	clicky.BindTaskManagerPFlags(checkCmd.Flags(), taskMgrOptions)
}

// isWithinWorkingDirectory checks if a file path is within the working directory or its subdirectories
func isWithinWorkingDirectory(filePath, workingDir string) bool {
	// Convert both paths to absolute paths for comparison
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return false
	}

	// Check if the file path starts with the working directory path
	rel, err := filepath.Rel(absWorkingDir, absFilePath)
	if err != nil {
		return false
	}

	// If the relative path doesn't start with "..", it's within the working directory
	return !strings.HasPrefix(rel, "..")
}

// parseLintersList parses the linters flag and returns which linters to run
func parseLintersList(lintersFlag string, archConfig *models.Config) (map[string]bool, bool) {
	// Handle special cases
	if lintersFlag == "none" || lintersFlag == "" {
		return map[string]bool{}, false // Skip all linters
	}

	if lintersFlag == "*" {
		// Run all configured linters
		enabledMap := make(map[string]bool)
		for name, cfg := range archConfig.Linters {
			enabledMap[name] = cfg.Enabled
		}
		return enabledMap, true
	}

	// Parse comma-separated list
	requestedLinters := make(map[string]bool)
	for _, linter := range strings.Split(lintersFlag, ",") {
		linter = strings.TrimSpace(linter)
		if linter != "" {
			requestedLinters[linter] = true
		}
	}

	return requestedLinters, true
}

func runCheck(cmd *cobra.Command, args []string) error {
	// Determine working directory - this is where analysis will be performed
	var workingDir string
	var specificFiles []string

	if len(args) > 0 {
		firstArg := args[0]

		// Check if the first argument is a file or directory
		info, err := os.Stat(firstArg)
		if err == nil {
			if info.IsDir() {
				// It's a directory, use it as workingDir
				workingDir = firstArg
				// Any additional arguments are specific files within that directory
				if len(args) > 1 {
					for _, file := range args[1:] {
						// Convert to absolute paths
						absPath, err := filepath.Abs(filepath.Join(workingDir, file))
						if err != nil {
							return fmt.Errorf("invalid file path %s: %w", file, err)
						}
						specificFiles = append(specificFiles, absPath)
					}
					logger.Infof("Checking specific files in %s: %v", workingDir, specificFiles)
				}
			} else {
				// It's a file, so all arguments are specific files to check
				// Use GetWorkingDir to respect --cwd flag
				if wd, err := GetWorkingDir(); err == nil {
					workingDir = wd
				} else {
					workingDir = "."
				}
				for _, file := range args {
					absPath, err := filepath.Abs(file)
					if err != nil {
						return fmt.Errorf("invalid file path %s: %w", file, err)
					}
					specificFiles = append(specificFiles, absPath)
				}
				logger.Infof("Checking specific files: %v", specificFiles)
			}
		} else {
			// Argument doesn't exist, assume it's a directory path
			workingDir = firstArg
			// Any additional arguments are specific files within that directory
			if len(args) > 1 {
				for _, file := range args[1:] {
					// Convert to absolute paths
					absPath, err := filepath.Abs(filepath.Join(workingDir, file))
					if err != nil {
						return fmt.Errorf("invalid file path %s: %w", file, err)
					}
					specificFiles = append(specificFiles, absPath)
				}
				logger.Infof("Checking specific files in %s: %v", workingDir, specificFiles)
			}
		}
	} else {
		// No arguments provided, use GetWorkingDir to respect --cwd flag
		if wd, err := GetWorkingDir(); err == nil {
			workingDir = wd
		} else {
			workingDir = "."
		}
	}

	// Determine output format for progress display
	currentFormat := getOutputFormat()

	var archResult *models.AnalysisResult
	var linterResults []models.LinterResult
	var consolidatedResult *models.ConsolidatedResult
	var requestedLinters map[string]bool
	var configDir string

	// Load configuration - search from current directory up to git root
	configParser := config.NewParser(workingDir)
	archConfig, err := configParser.LoadConfig()
	foundConfigDir := workingDir
	if err != nil {
		// Use smart defaults based on detected languages in working directory
		logger.Infof("No arch-unit.yaml found, detecting languages and using smart defaults...")

		archConfig, err = config.CreateSmartDefaultConfig(workingDir)
		if err != nil {
			return fmt.Errorf("failed to create default configuration: %w", err)
		}

		// Use working directory as config directory when no config found
		configDir = workingDir

		// Log what was auto-detected
		enabledLinters := archConfig.GetEnabledLinters()
		if len(enabledLinters) > 0 {
			logger.Infof("Auto-enabled linters (based on config files): %v", enabledLinters)
		} else {
			logger.Infof("No linter configs detected. Create config files (e.g., .golangci.yml, .eslintrc.json) to enable linters.")
		}
	} else {
		configDir = foundConfigDir
		logger.Infof("Using config from: %s", configDir)
	}

	if archConfig != nil {
		// Initialize linters registry using working directory for analysis
		// But some linters like ArchUnit might need the config directory for rules
		// TODO: Fix linter interface mismatch - linters have wrong Run method signature
		// linters.DefaultRegistry.Register(aql.NewAQLWithConfig(workingDir, archConfig))
		// linters.DefaultRegistry.Register(archunit.NewArchUnit(configDir))
		// linters.DefaultRegistry.Register(comment.NewCommentAnalysisLinter()) // Temporarily disabled
		// linters.DefaultRegistry.Register(golangci.NewGolangciLint(workingDir))
		// linters.DefaultRegistry.Register(ruff.NewRuff(workingDir))
		// linters.DefaultRegistry.Register(pyright.NewPyright(workingDir))
		// linters.DefaultRegistry.Register(eslint.NewESLint(workingDir))
		// linters.DefaultRegistry.Register(markdownlint.NewMarkdownlint(workingDir))
		// linters.DefaultRegistry.Register(vale.NewVale(workingDir))

		// Parse linters flag to determine which linters to run
		var runLinters bool
		requestedLinters, runLinters = parseLintersList(lintersFlag, archConfig)

		// Run linters if requested
		if runLinters {
			// Filter config to only run requested linters
			filteredConfig := &models.Config{
				Version:   archConfig.Version,
				Debounce:  archConfig.Debounce,
				Rules:     archConfig.Rules,
				Linters:   make(map[string]models.LinterConfig),
				Languages: archConfig.Languages,
			}

			// Add arch-unit as a linter if requested
			if lintersFlag == "*" || requestedLinters["arch-unit"] {
				filteredConfig.Linters["arch-unit"] = models.LinterConfig{
					Enabled: true,
				}
			}

			// Add AQL as a linter if requested and AQL rules are configured
			if (lintersFlag == "*" || requestedLinters["aql"]) && len(archConfig.AQLRules) > 0 {
				filteredConfig.Linters["aql"] = models.LinterConfig{
					Enabled: true,
				}

				// Store AQL rules in the filtered config for the linter to access
				filteredConfig.AQLRules = archConfig.AQLRules
			}

			// Copy only requested linters
			for name, cfg := range archConfig.Linters {
				if lintersFlag == "*" {
					// Include all enabled linters
					if cfg.Enabled {
						filteredConfig.Linters[name] = cfg
					}
				} else if requestedLinters[name] {
					// Include specifically requested linter
					cfg.Enabled = true
					filteredConfig.Linters[name] = cfg
				}
			}

			var linterRunner *linters.Runner
			if noCacheFlag {
				linterRunner, err = linters.NewRunnerWithOptions(filteredConfig, workingDir, linters.RunnerOptions{NoCache: true})
			} else {
				linterRunner, err = linters.NewRunner(filteredConfig, workingDir)
			}
			if err != nil {
				return fmt.Errorf("failed to create linter runner: %w", err)
			} else {
				defer func() { _ = linterRunner.Close() }()

				results, err := linterRunner.RunEnabledLintersOnFiles(specificFiles, fixFlag)
				if err != nil {
					return fmt.Errorf("failed to run linters: %w", err)
				} else {
					// Convert to models.LinterResult
					for _, result := range results {
						linterResults = append(linterResults, models.LinterResult{
							Linter:     result.Linter,
							Success:    result.Success,
							Duration:   result.Duration,
							Violations: result.Violations,
							RawOutput:  result.RawOutput,
							Error:      result.Error,
							FileCount:  result.FileCount,
							RuleCount:  result.RuleCount,
						})
					}
				}
			}
		} else {
			logger.Infof("Skipping all linters as per --linters flag")
		}
	} else {
		logger.Infof("ArchConfig is nil")
	}

	// Wait for all tasks to complete using global task manager
	exitCode := clicky.WaitForGlobalCompletion()
	// Small delay to ensure TaskManager rendering has completely finished
	time.Sleep(50 * time.Millisecond)

	// Create consolidated result by fetching all violations from the database
	// Skip cache access if --no-cache flag is set
	if noCacheFlag {
		// Use in-memory results only when cache is disabled
		if len(linterResults) > 0 {
			consolidatedResult = models.NewConsolidatedResult(archResult, linterResults)
		} else if archResult != nil {
			consolidatedResult = models.NewConsolidatedResult(archResult, nil)
		} else {
			consolidatedResult = models.NewConsolidatedResult(&models.AnalysisResult{}, nil)
		}
	} else {
		violationCache, err := cache.NewViolationCache()
		if err != nil {
			return fmt.Errorf("failed to open violation cache for reporting: %w", err)
		} else {
			defer func() { _ = violationCache.Close() }()

			// Get violations from the database filtered by requested linters
			var allViolations []models.Violation
			var err error

			if lintersFlag == "*" {
				// Get all violations
				allViolations, err = violationCache.GetAllViolations()
			} else if len(requestedLinters) > 0 {
				// Get violations only from requested linters
				sources := make([]string, 0, len(requestedLinters))
				for linter := range requestedLinters {
					sources = append(sources, linter)
				}
				allViolations, err = violationCache.GetViolationsBySources(sources)
			} else {
				// No linters requested, return empty
				allViolations = []models.Violation{}
			}

			if err != nil {
				return err
			}

			logger.Infof("Fetched %d total violations from database, files=%d", len(allViolations), len(specificFiles))
			// Use violations from database, but filter based on working directory
			var violations []models.Violation
			if len(specificFiles) > 0 {
				// Filter violations to only requested files
				// Create a set of requested files in both absolute and relative forms
				requestedFiles := make(map[string]bool)

				for _, f := range specificFiles {
					requestedFiles[f] = true
					// Also add relative path from working directory
					if cwd, err := GetWorkingDir(); err == nil {
						if rel, err := filepath.Rel(cwd, f); err == nil && !strings.HasPrefix(rel, "../") {
							requestedFiles[rel] = true
						}
					}
				}

				for _, v := range allViolations {
					// Check if violation file matches any requested file
					matched := false

					// Direct match
					if requestedFiles[v.File] {
						matched = true
					}

					// If violation file is relative, try making it absolute
					if !matched && !filepath.IsAbs(v.File) {
						if cwd, err := GetWorkingDir(); err == nil {
							absPath := filepath.Join(cwd, v.File)
							if requestedFiles[absPath] {
								matched = true
							}
						}
					}

					// If violation file is absolute, try making it relative
					if !matched && filepath.IsAbs(v.File) {
						if cwd, err := GetWorkingDir(); err == nil {
							if rel, err := filepath.Rel(cwd, v.File); err == nil && !strings.HasPrefix(rel, "../") {
								if requestedFiles[rel] {
									matched = true
								}
							}
						}
					}

					if matched {
						violations = append(violations, v)
					}
				}
			} else {
				// Filter violations to only those within the working directory
				for _, v := range allViolations {
					if isWithinWorkingDirectory(v.File, workingDir) {
						violations = append(violations, v)
					}
				}
			}

			// Create result with violations from database
			// Don't include linter results as they're already in the database
			fileCount := 0
			ruleCount := 0
			if archResult != nil {
				fileCount = archResult.FileCount
				ruleCount = archResult.RuleCount
			}
			dbResult := &models.AnalysisResult{
				Violations: violations,
				FileCount:  fileCount,
				RuleCount:  ruleCount,
			}
			consolidatedResult = models.NewConsolidatedResult(dbResult, nil)
		}
	}

	// Display results based on output format
	if currentFormat == "pretty" && !compact {
		// Display combined violation tree for pretty format
		displayCombinedViolations(consolidatedResult)

		// Exit with appropriate code
		if failOnViolation && (exitCode != 0 || consolidatedResult.HasFailures()) {
			os.Exit(1)
		}
	} else {
		// Output results in requested format (JSON, CSV, etc.)
		if err := outputConsolidatedResults(consolidatedResult); err != nil {
			return fmt.Errorf("failed to output consolidated results: %w", err)
		}

		// Exit with error if violations found and flag is set
		if failOnViolation && consolidatedResult.HasFailures() {
			os.Exit(1)
		}
	}

	return nil
}

// displayCombinedViolations displays all violations from arch-unit and linters in a tree format
func displayCombinedViolations(result *models.ConsolidatedResult) {
	if result == nil || len(result.Violations) == 0 {
		return
	}

	// Build violations tree
	tree := models.BuildViolationTree(result.Violations)

	// Format using clicky with tree format
	output, err := clicky.Format(tree, clicky.FormatOptions{Format: "tree"})
	if err != nil {
		logger.Errorf("Failed to format violations tree: %v", err)
		// Fallback to simple display
		fmt.Printf("\n📋 Combined Violations (%d total)\n", len(result.Violations))
		for _, v := range result.Violations {
			fmt.Printf("- %s\n", v.String())
		}
		return
	}

	fmt.Printf("\n%s\n", output)

	// Print summary
	fmt.Printf("\n%s Found %d total violation(s)\n",
		color.RedString("✗"),
		result.Summary.TotalViolations)
	if result.Summary.ArchViolations > 0 {
		fmt.Printf("  - %d architecture violation(s)\n", result.Summary.ArchViolations)
	}
	if result.Summary.LinterViolations > 0 {
		fmt.Printf("  - %d linter violation(s)\n", result.Summary.LinterViolations)
	}

	// Count and display fixable violations
	fixableCount := 0
	unsafeFixableCount := 0
	for _, v := range result.Violations {
		if v.Fixable {
			if v.FixApplicability == "unsafe" {
				unsafeFixableCount++
			} else {
				fixableCount++
			}
		}
	}

	if fixableCount > 0 || unsafeFixableCount > 0 {
		fmt.Printf("\n%s Fix Summary:\n", color.GreenString("🔧"))
		if fixableCount > 0 {
			fmt.Printf("  - %d violation(s) can be safely auto-fixed with %s\n",
				fixableCount, color.CyanString("arch-unit check --fix"))
		}
		if unsafeFixableCount > 0 {
			fmt.Printf("  - %d violation(s) can be auto-fixed but may be unsafe\n",
				unsafeFixableCount)
		}
	}
}

// outputConsolidatedResults outputs consolidated results in the requested format
func outputConsolidatedResults(result *models.ConsolidatedResult) error {
	// For now, just print a simple summary
	fmt.Printf("Total violations: %d\n", result.Summary.TotalViolations)
	return nil
}

func getOutputFormat() string {
	// First check if a format flag was provided via clicky
	format := clicky.Flags.FormatOptions.ResolveFormat()
	if format != "" && format != "pretty" {
		return format
	}

	// Fall back to detecting format from output file extension if no format flag
	if outputFile != "" {
		if strings.HasSuffix(outputFile, ".json") {
			return "json"
		} else if strings.HasSuffix(outputFile, ".csv") {
			return "csv"
		} else if strings.HasSuffix(outputFile, ".html") {
			return "html"
		} else if strings.HasSuffix(outputFile, ".md") {
			return "markdown"
		}
	}

	// Default to pretty format (tree display)
	return "pretty"
}
