package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/flanksource/arch-unit/config"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/linters"
	"github.com/flanksource/arch-unit/linters/eslint"
	"github.com/flanksource/arch-unit/linters/golangci"
	"github.com/flanksource/arch-unit/linters/markdownlint"
	"github.com/flanksource/arch-unit/linters/pyright"
	"github.com/flanksource/arch-unit/linters/ruff"
	"github.com/flanksource/arch-unit/linters/vale"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	failOnViolation bool
	includePattern  string
	excludePattern  string
	debounceFlag    string
	lintersFlag     string
)

var checkCmd = &cobra.Command{
	Use:   "check [path] [files...]",
	Short: "Check architecture violations in the codebase",
	Long: `Check for architecture violations by analyzing Go and Python files
against rules defined in arch-unit.yaml.

Examples:
  # Check current directory
  arch-unit check

  # Check specific directory
  arch-unit check ./src

  # Check specific files only
  arch-unit check . main.go service.go

  # Run with specific linters only
  arch-unit check --linters=golangci-lint,ruff

  # Run without linters
  arch-unit check --linters=none

  # Run all configured linters (default)
  arch-unit check --linters="*"

  # Check with JSON output
  arch-unit check -j

  # Check with debounce to prevent rapid re-runs
  arch-unit check --debounce=30s`,
	Args: cobra.ArbitraryArgs,
	RunE: runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)

	checkCmd.Flags().BoolVar(&failOnViolation, "fail-on-violation", true, "Exit with code 1 if violations are found")
	checkCmd.Flags().StringVar(&includePattern, "include", "", "Include files matching pattern (e.g., '*.go')")
	checkCmd.Flags().StringVar(&excludePattern, "exclude", "", "Exclude files matching pattern (e.g., '*_test.go')")
	checkCmd.Flags().StringVar(&debounceFlag, "debounce", "", "Debounce period to prevent frequent re-runs (e.g., '30s', '5m')")
	checkCmd.Flags().StringVar(&lintersFlag, "linters", "*", "Linters to run ('*' for all configured, 'none' to skip, or comma-separated list e.g., 'golangci-lint,ruff')")
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
	rootDir := "."
	var specificFiles []string
	
	if len(args) > 0 {
		firstArg := args[0]
		
		// Check if the first argument is a file or directory
		info, err := os.Stat(firstArg)
		if err == nil {
			if info.IsDir() {
				// It's a directory, use it as rootDir
				rootDir = firstArg
				// Any additional arguments are specific files within that directory
				if len(args) > 1 {
					for _, file := range args[1:] {
						// Convert to absolute paths
						absPath, err := filepath.Abs(filepath.Join(rootDir, file))
						if err != nil {
							return fmt.Errorf("invalid file path %s: %w", file, err)
						}
						specificFiles = append(specificFiles, absPath)
					}
					logger.Infof("Checking specific files in %s: %v", rootDir, specificFiles)
				}
			} else {
				// It's a file, so all arguments are specific files to check
				rootDir = "." // Use current directory as root
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
			rootDir = firstArg
			// Any additional arguments are specific files within that directory
			if len(args) > 1 {
				for _, file := range args[1:] {
					// Convert to absolute paths
					absPath, err := filepath.Abs(filepath.Join(rootDir, file))
					if err != nil {
						return fmt.Errorf("invalid file path %s: %w", file, err)
					}
					specificFiles = append(specificFiles, absPath)
				}
				logger.Infof("Checking specific files in %s: %v", rootDir, specificFiles)
			}
		}
	}

	// Create TaskManager for progress tracking (only when not in JSON/CSV/etc output mode)
	var taskManager *clicky.TaskManager
	shouldUseTaskManager := !json && !csv && !html && !excel && !markdown && !compact
	if shouldUseTaskManager {
		taskManager = clicky.NewTaskManager()
	}

	var archResult *models.AnalysisResult
	var linterResults []models.LinterResult
	var consolidatedResult *models.ConsolidatedResult
	
	// Load configuration
	configParser := config.NewParser(rootDir)
	archConfig, err := configParser.LoadConfig()
	if err != nil {
		// Use smart defaults based on detected languages
		logger.Infof("No arch-unit.yaml found, detecting languages and using smart defaults...")
		
		archConfig, err = config.CreateSmartDefaultConfig(rootDir)
		if err != nil {
			return fmt.Errorf("failed to create default configuration: %w", err)
		}
		
		// Log what was auto-detected
		enabledLinters := archConfig.GetEnabledLinters()
		if len(enabledLinters) > 0 {
			logger.Infof("Auto-enabled linters (based on config files): %v", enabledLinters)
		} else {
			logger.Infof("No linter configs detected. Create config files (e.g., .golangci.yml, .eslintrc.json) to enable linters.")
		}
	}
	
	if archConfig != nil {
		// Handle global debounce from config or CLI flag (skip if checking specific files)
		if len(specificFiles) == 0 {
			debounceStr := debounceFlag
			if debounceStr == "" {
				debounceStr = archConfig.Debounce
			}
			
			if debounceStr != "" {
				if shouldSkip, err := handleDebounce(rootDir, debounceStr); err != nil {
					return err
				} else if shouldSkip {
					return nil
				}
			}
		}

		// Parse linters flag to determine which linters to run
		requestedLinters, runLinters := parseLintersList(lintersFlag, archConfig)
		runArchUnit := lintersFlag != "none" || lintersFlag == "*"
		
		// Run arch-unit analysis if not disabled
		if runArchUnit && lintersFlag != "none" {
			var archTask *clicky.Task
			if taskManager != nil {
				archTask = taskManager.Start("Running arch-unit analysis")
			}
			
			result, err := runYAMLBasedCheck(rootDir, archConfig, specificFiles)
			if err != nil {
				if archTask != nil {
					archTask.Failed()
				}
				return err
			}
			
			if archTask != nil {
				if len(result.Violations) > 0 {
					archTask.SetStatus(fmt.Sprintf("arch-unit (%d violations)", len(result.Violations)))
					archTask.Warning()
				} else {
					archTask.SetStatus("arch-unit")
					archTask.Success()
				}
			}
			
			archResult = result
		}

		// Initialize linters registry
		linters.DefaultRegistry.Register(golangci.NewGolangciLint(rootDir))
		linters.DefaultRegistry.Register(ruff.NewRuff(rootDir))
		linters.DefaultRegistry.Register(pyright.NewPyright(rootDir))
		linters.DefaultRegistry.Register(eslint.NewESLint(rootDir))
		linters.DefaultRegistry.Register(markdownlint.NewMarkdownlint(rootDir))
		linters.DefaultRegistry.Register(vale.NewVale(rootDir))

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
			
			linterRunner, err := linters.NewRunnerV2(filteredConfig, rootDir)
			if err != nil {
				logger.Warnf("Failed to create linter runner: %v", err)
			} else {
				defer linterRunner.Close()
				if taskManager != nil {
					linterRunner.SetTaskManager(taskManager)
				}
				results, err := linterRunner.RunEnabledLintersOnFiles(specificFiles)
				if err != nil {
					logger.Warnf("Failed to run linters: %v", err)
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
						})
					}
				}
			}
		}
	}

	// Wait for all tasks to complete if using TaskManager
	var exitCode int
	if taskManager != nil {
		exitCode = taskManager.Wait()
		// Small delay to ensure TaskManager rendering has completely finished
		time.Sleep(50 * time.Millisecond)
	}

	// Create consolidated result by fetching all violations from the database
	violationCache, err := cache.NewViolationCache()
	if err != nil {
		logger.Warnf("Failed to open violation cache for reporting: %v", err)
		// Fall back to in-memory results
		if len(linterResults) > 0 {
			consolidatedResult = models.NewConsolidatedResult(archResult, linterResults)
		} else if archResult != nil {
			consolidatedResult = models.NewConsolidatedResult(archResult, nil)
		} else {
			consolidatedResult = models.NewConsolidatedResult(&models.AnalysisResult{}, nil)
		}
	} else {
		defer violationCache.Close()
		
		// Get all violations from the database
		allViolations, err := violationCache.GetAllViolations()
		if err != nil {
			logger.Warnf("Failed to get violations from cache: %v", err)
			// Fall back to in-memory results
			if len(linterResults) > 0 {
				consolidatedResult = models.NewConsolidatedResult(archResult, linterResults)
			} else if archResult != nil {
				consolidatedResult = models.NewConsolidatedResult(archResult, nil)
			} else {
				consolidatedResult = models.NewConsolidatedResult(&models.AnalysisResult{}, nil)
			}
		} else {
			// Use all violations from database (both arch-unit and linters)
			var violations []models.Violation
			if len(specificFiles) > 0 {
				// Filter violations to only requested files
				// Create a set of requested files in both absolute and relative forms
				requestedFiles := make(map[string]bool)
				
				for _, f := range specificFiles {
					requestedFiles[f] = true
					// Also add relative path from current working directory
					if cwd, err := os.Getwd(); err == nil {
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
						if cwd, err := os.Getwd(); err == nil {
							absPath := filepath.Join(cwd, v.File)
							if requestedFiles[absPath] {
								matched = true
							}
						}
					}
					
					// If violation file is absolute, try making it relative
					if !matched && filepath.IsAbs(v.File) {
						if cwd, err := os.Getwd(); err == nil {
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
				// Use all violations
				violations = allViolations
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

	// Display combined violation tree if using TaskManager
	if taskManager != nil {
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