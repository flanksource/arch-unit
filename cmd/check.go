package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	// "github.com/flanksource/arch-unit/linters/comment" // Temporarily disabled

	"github.com/flanksource/arch-unit/client"
	"github.com/flanksource/arch-unit/filters"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/output"
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
	Use:   "check [path] [files...]",
	Short: "Check architecture violations in the codebase",
	Long: `Check for architecture violations by analyzing Go and Python files
against rules defined in .ARCHUNIT files.

Examples:
  # Check current directory
  arch-unit check

  # Check specific directory
  arch-unit check ./src

  # Check specific files only
  arch-unit check . main.go service.go

  # Check with JSON output
  arch-unit check -j

  # Fail on violations (exit code 1)
  arch-unit check --fail-on-violation

  # Check with debounce to prevent rapid re-runs
  arch-unit check --debounce=30s

  # Run with verbose linter output
  arch-unit check -v

  # Run with very verbose linter output (includes response)
  arch-unit check -vv`,
	Args: cobra.ArbitraryArgs,
	RunE: runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)

	checkCmd.Flags().BoolVar(&failOnViolation, "fail-on-violation", false, "Exit with code 1 if violations are found")
	checkCmd.Flags().StringVar(&includePattern, "include", "", "Include files matching pattern (e.g., '*.go')")
	checkCmd.Flags().StringVar(&excludePattern, "exclude", "", "Exclude files matching pattern (e.g., '*_test.go')")
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

	// Load rules
	parser := filters.NewParser(rootDir)
	ruleSets, err := parser.LoadRules()
	if err != nil {
		return fmt.Errorf("failed to load rules: %w", err)
	}

	if len(ruleSets) == 0 {
		logger.Warnf("No .ARCHUNIT files found in %s", rootDir)
		return nil
	}

	logger.Infof("Loaded %d rule set(s)", len(ruleSets))

	// Find source files
	goFiles, pythonFiles, err := client.FindSourceFiles(rootDir)
	if err != nil {
		return fmt.Errorf("failed to find source files: %w", err)
	}

	logger.Infof("Found %d Go files and %d Python files", len(goFiles), len(pythonFiles))

	// Apply filters
	goFiles = filterFiles(goFiles, includePattern, excludePattern)
	pythonFiles = filterFiles(pythonFiles, includePattern, excludePattern)

	var result *models.AnalysisResult

	// Analyze Go files
	if len(goFiles) > 0 {
		goResult, err := client.AnalyzeGoFiles(rootDir, goFiles, ruleSets)
		if err != nil {
			return fmt.Errorf("failed to analyze Go files: %w", err)
		}
		result = goResult
	}

	// Analyze Python files
	if len(pythonFiles) > 0 {
		pyResult, err := client.AnalyzePythonFiles(rootDir, pythonFiles, ruleSets)
		if err != nil {
			logger.Warnf("Failed to analyze Python files: %v", err)
		} else {
			if result == nil {
				result = pyResult
			} else {
				result.Violations = append(result.Violations, pyResult.Violations...)
				result.FileCount += pyResult.FileCount
			}
		}
	}

	if result == nil {
		result = &models.AnalysisResult{}
	}

	// Output results
	outputManager := output.NewOutputManager(getOutputFormat())
	outputManager.SetOutputFile(outputFile)
	outputManager.SetCompact(compact)
	if err := outputManager.Output(result); err != nil {
		return fmt.Errorf("failed to output results: %w", err)
	}

	// Summary
	if !json && !csv && !html && !excel && !markdown {
		printSummary(result)
	}

	// Exit with error if violations found and flag is set
	if failOnViolation && len(result.Violations) > 0 {
		os.Exit(1)
	}

	return nil
}

// displayCombinedViolations displays all violations from arch-unit and linters in a tree format
func displayCombinedViolations(result *models.ConsolidatedResult) {
	if result == nil || len(result.Violations) == 0 {
		return
	}

	// Group violations by file
	fileMap := make(map[string][]models.Violation)
	for _, v := range result.Violations {
		fileMap[v.File] = append(fileMap[v.File], v)
	}

	// Sort files for consistent output
	var files []string
	for file := range fileMap {
		files = append(files, file)
	}
	sort.Strings(files)

	fmt.Println("\n📋 Combined Violations")
	fmt.Println(strings.Repeat("─", 80))

	for i, file := range files {
		violations := fileMap[file]

		// Get relative path for display
		relPath := file
		if cwd, err := GetWorkingDir(); err == nil {
			if rel, err := filepath.Rel(cwd, file); err == nil && !strings.HasPrefix(rel, "../") {
				relPath = rel
			}
		}

		// File header with violation count
		isLast := i == len(files)-1
		if isLast {
			fmt.Printf("└── %s (%d violations)\n",
				color.New(color.FgCyan, color.Bold).Sprint(relPath),
				len(violations))
		} else {
			fmt.Printf("├── %s (%d violations)\n",
				color.New(color.FgCyan, color.Bold).Sprint(relPath),
				len(violations))
		}

		// Group violations by source (arch-unit vs linters)
		sourceMap := make(map[string][]models.Violation)
		for _, v := range violations {
			source := "arch-unit"
			if v.Source != "" {
				source = v.Source
			}
			sourceMap[source] = append(sourceMap[source], v)
		}

		// Sort sources for consistent output
		var sources []string
		for source := range sourceMap {
			sources = append(sources, source)
		}
		sort.Strings(sources)

		prefix := "│   "
		if isLast {
			prefix = "    "
		}

		for j, source := range sources {
			sourceViolations := sourceMap[source]
			isLastSource := j == len(sources)-1

			// Source header
			sourceColor := color.FgYellow
			if source == "arch-unit" {
				sourceColor = color.FgMagenta
			}

			if isLastSource {
				fmt.Printf("%s└── %s\n", prefix, color.New(sourceColor).Sprint(source))
			} else {
				fmt.Printf("%s├── %s\n", prefix, color.New(sourceColor).Sprint(source))
			}

			sourcePrefix := prefix + "│   "
			if isLastSource {
				sourcePrefix = prefix + "    "
			}

			// Display violations for this source
			for k, v := range sourceViolations {
				isLastViolation := k == len(sourceViolations)-1

				// Format violation message
				var violationMsg string
				if v.Rule != nil {
					violationMsg = v.Rule.String()
				} else if v.Message != "" {
					violationMsg = v.Message
				} else {
					violationMsg = fmt.Sprintf("%s.%s", v.CalledPackage, v.CalledMethod)
				}

				// Add fixable indicator if applicable
				fixableIndicator := ""
				if v.Fixable {
					if v.FixApplicability == "unsafe" {
						fixableIndicator = color.New(color.FgYellow).Sprint(" 🔧⚠")
					} else {
						fixableIndicator = color.New(color.FgGreen).Sprint(" 🔧")
					}
				}

				lineInfo := fmt.Sprintf("line %d", v.Line)
				if v.Column > 0 {
					lineInfo = fmt.Sprintf("line %d:%d", v.Line, v.Column)
				}

				if isLastViolation {
					fmt.Printf("%s└── %s%s %s\n",
						sourcePrefix,
						color.RedString(violationMsg),
						fixableIndicator,
						color.New(color.FgHiBlack).Sprint("("+lineInfo+")"))
				} else {
					fmt.Printf("%s├── %s%s %s\n",
						sourcePrefix,
						color.RedString(violationMsg),
						fixableIndicator,
						color.New(color.FgHiBlack).Sprint("("+lineInfo+")"))
				}
			}
		}

		if !isLast {
			fmt.Println("│")
		}
	}

	fmt.Println(strings.Repeat("─", 80))

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

func filterFiles(files []string, include, exclude string) []string {
	if include == "" && exclude == "" {
		return files
	}

	var filtered []string
	for _, file := range files {
		if include != "" {
			matched, _ := filepath.Match(include, filepath.Base(file))
			if !matched {
				continue
			}
		}

		if exclude != "" {
			matched, _ := filepath.Match(exclude, filepath.Base(file))
			if matched {
				continue
			}
		}

		filtered = append(filtered, file)
	}

	return filtered
}

func getOutputFormat() string {
	globalFormatOpts := GetFormatOptions()
	if globalFormatOpts != nil && globalFormatOpts.Format != "" {
		// Map "pretty" to "table" for backward compatibility
		if globalFormatOpts.Format == "pretty" {
			return "table"
		}
		return globalFormatOpts.Format
	}
	return "table"
}

func filterFiles(files []string, include, exclude string) []string {
	if include == "" && exclude == "" {
		return files
	}

	var filtered []string
	for _, file := range files {
		if include != "" {
			matched, _ := filepath.Match(include, filepath.Base(file))
			if !matched {
				continue
			}
		}

		if exclude != "" {
			matched, _ := filepath.Match(exclude, filepath.Base(file))
			if matched {
				continue
			}
		}

		filtered = append(filtered, file)
	}

	return filtered
}

func getOutputFormat() string {
	if json {
		return "json"
	}
	if csv {
		return "csv"
	}
	if html {
		return "html"
	}
	if excel {
		return "excel"
	}
	if markdown {
		return "markdown"
	}
	return "table"
}

func printSummary(result *models.AnalysisResult) {
	fmt.Println()

	if len(result.Violations) == 0 {
		color.Green("✓ No architecture violations found!")
		fmt.Printf("  Analyzed %d file(s) with %d rule(s)\n", result.FileCount, result.RuleCount)
	} else {
		color.Red("✗ Found %d architecture violation(s)", len(result.Violations))
		fmt.Printf("  Analyzed %d file(s) with %d rule(s)\n", result.FileCount, result.RuleCount)
	}
}
