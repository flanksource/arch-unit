package cmd

import (
	stdjson "encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flanksource/arch-unit/client"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/output"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
)

// runYAMLBasedCheck runs the new arch-unit.yaml based check
func runYAMLBasedCheck(rootDir string, config *models.Config, specificFiles []string) (*models.AnalysisResult, error) {
	var goFiles, pythonFiles []string

	if len(specificFiles) > 0 {
		// Run only on specific files
		for _, file := range specificFiles {
			if filepath.Ext(file) == ".go" {
				goFiles = append(goFiles, file)
			} else if filepath.Ext(file) == ".py" {
				pythonFiles = append(pythonFiles, file)
			}
		}
		logger.Infof("Analyzing %d specific files", len(specificFiles))
	} else {
		// Find all source files
		var err error
		goFiles, pythonFiles, err = client.FindSourceFiles(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to find source files: %w", err)
		}
		logger.Infof("Found %d Go files and %d Python files", len(goFiles), len(pythonFiles))

		// Apply filters
		goFiles = filterFiles(goFiles, includePattern, excludePattern)
		pythonFiles = filterFiles(pythonFiles, includePattern, excludePattern)
	}

	// Open violation cache
	violationCache, err := cache.NewViolationCache()
	if err != nil {
		logger.Warnf("Failed to open violation cache: %v", err)
		// Continue without cache
	}
	defer func() {
		if violationCache != nil {
			violationCache.Close()
		}
	}()

	// Track analyzed files for result
	fileCount := len(goFiles) + len(pythonFiles)
	ruleCount := 0

	// Analyze Go files
	if len(goFiles) > 0 {
		goResult, err := analyzeGoFilesWithCache(rootDir, goFiles, config, violationCache)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze Go files: %w", err)
		}
		ruleCount += goResult.RuleCount
	}

	// Analyze Python files
	if len(pythonFiles) > 0 {
		pyResult, err := analyzePythonFilesWithCache(rootDir, pythonFiles, config, violationCache)
		if err != nil {
			logger.Warnf("Failed to analyze Python files: %v", err)
		} else {
			ruleCount += pyResult.RuleCount
		}
	}

	// Get ALL violations from the database (not just what we analyzed)
	var violations []models.Violation
	if violationCache != nil {
		// If specific files were requested, get violations only for those files
		if len(specificFiles) > 0 {
			for _, file := range specificFiles {
				fileViolations, err := violationCache.GetCachedViolations(file)
				if err != nil {
					logger.Warnf("Failed to get violations for %s: %v", file, err)
				} else {
					violations = append(violations, fileViolations...)
				}
			}
		} else {
			// Get only arch-unit violations from the database
			archViolations, err := violationCache.GetViolationsBySource("arch-unit")
			if err != nil {
				logger.Warnf("Failed to get arch-unit violations from cache: %v", err)
			} else {
				violations = archViolations
			}
		}

		// Log cache statistics
		if stats, err := violationCache.GetStats(); err == nil {
			logger.Debugf("Cache stats: %v", stats)
		}
	}

	result := &models.AnalysisResult{
		Violations: violations,
		FileCount:  fileCount,
		RuleCount:  ruleCount,
	}

	return result, nil
}

// analyzeGoFilesWithCache analyzes Go files with caching support
func analyzeGoFilesWithCache(rootDir string, files []string, config *models.Config, violationCache *cache.ViolationCache) (*models.AnalysisResult, error) {
	analyzer := client.NewGoAnalyzer()
	result := &models.AnalysisResult{
		FileCount: len(files),
	}

	var filesToAnalyze []string

	// Check cache for each file
	for _, file := range files {
		if violationCache != nil {
			needsRescan, err := violationCache.NeedsRescan(file)
			if err != nil {
				logger.Debugf("Error checking cache for %s: %v", file, err)
				filesToAnalyze = append(filesToAnalyze, file)
				continue
			}

			if !needsRescan {
				logger.Debugf("Using cached violations for %s", file)
				// Still count rules for cached files
				if rules, err := config.GetRulesForFile(file); err == nil && rules != nil {
					result.RuleCount += len(rules.Rules)
				}
			} else {
				filesToAnalyze = append(filesToAnalyze, file)
			}
		} else {
			filesToAnalyze = append(filesToAnalyze, file)
		}
	}

	logger.Infof("Analyzing %d files (using cache for %d files)", len(filesToAnalyze), len(files)-len(filesToAnalyze))

	// Analyze files that need scanning
	for _, file := range filesToAnalyze {
		rules, err := config.GetRulesForFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to get rules for file %s: %w", file, err)
		}

		if rules != nil {
			result.RuleCount += len(rules.Rules)
		}

		violations, err := analyzer.AnalyzeFile(file, rules)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze %s: %w", file, err)
		}

		// Store in cache
		if violationCache != nil {
			// Set source for violations
			for i := range violations {
				violations[i].Source = "arch-unit"
			}
			if err := violationCache.StoreViolations(file, violations); err != nil {
				logger.Debugf("Failed to cache violations for %s: %v", file, err)
			}
		}
	}

	// Don't return violations here - they will be fetched from DB
	return result, nil
}

// analyzePythonFilesWithCache analyzes Python files with caching support
func analyzePythonFilesWithCache(rootDir string, files []string, config *models.Config, violationCache *cache.ViolationCache) (*models.AnalysisResult, error) {
	analyzer := client.NewPythonAnalyzer(rootDir)
	result := &models.AnalysisResult{
		FileCount: len(files),
	}

	var filesToAnalyze []string

	// Check cache for each file
	for _, file := range files {
		if violationCache != nil {
			needsRescan, err := violationCache.NeedsRescan(file)
			if err != nil {
				logger.Debugf("Error checking cache for %s: %v", file, err)
				filesToAnalyze = append(filesToAnalyze, file)
				continue
			}

			if !needsRescan {
				logger.Debugf("Using cached violations for %s", file)
				// Still count rules for cached files
				if rules, err := config.GetRulesForFile(file); err == nil && rules != nil {
					result.RuleCount += len(rules.Rules)
				}
			} else {
				filesToAnalyze = append(filesToAnalyze, file)
			}
		} else {
			filesToAnalyze = append(filesToAnalyze, file)
		}
	}

	logger.Infof("Analyzing %d Python files (using cache for %d files)", len(filesToAnalyze), len(files)-len(filesToAnalyze))

	// Analyze files that need scanning
	for _, file := range filesToAnalyze {
		rules, err := config.GetRulesForFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to get rules for file %s: %w", file, err)
		}

		if rules != nil {
			result.RuleCount += len(rules.Rules)
		}

		violations, err := analyzer.AnalyzeFile(file, rules)
		if err != nil {
			// Skip files with errors for Python
			continue
		}

		// Store in cache
		if violationCache != nil {
			// Set source for violations
			for i := range violations {
				violations[i].Source = "arch-unit"
			}
			if err := violationCache.StoreViolations(file, violations); err != nil {
				logger.Debugf("Failed to cache violations for %s: %v", file, err)
			}
		}
	}

	// Don't return violations here - they will be fetched from DB
	return result, nil
}

// outputConsolidatedResults outputs consolidated results from arch-unit and linters
func outputConsolidatedResults(result *models.ConsolidatedResult) error {
	outputFormat := getOutputFormat()

	if outputFormat == "json" {
		data, err := stdjson.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}

		if outputFile != "" {
			return os.WriteFile(outputFile, data, 0644)
		}
		fmt.Println(string(data))
		return nil
	}

	// For non-JSON output, use the standard output manager with just violations
	archResult := &models.AnalysisResult{
		Violations: result.Violations,
		FileCount:  result.Summary.FilesAnalyzed,
		RuleCount:  result.Summary.RulesApplied,
	}

	outputManager := output.NewOutputManager(outputFormat)
	outputManager.SetOutputFile(outputFile)
	outputManager.SetCompact(compact)
	if err := outputManager.Output(archResult); err != nil {
		return fmt.Errorf("failed to output results: %w", err)
	}

	// Print consolidated summary for table/pretty format
	if outputFormat == "table" || outputFormat == "pretty" {
		printConsolidatedSummary(result)
	}

	return nil
}

// printConsolidatedSummary prints a summary of consolidated results
// ViolationSummary represents the violation summary with pretty formatting
type ViolationSummary struct {
	Status           string   `json:"status" pretty:"label=Status,style=text-green-600 font-bold,color=red>0"`
	TotalViolations  int      `json:"total_violations" pretty:"label=Total Violations,color=red>0,green=0"`
	ArchViolations   int      `json:"arch_violations,omitempty" pretty:"label=Architecture Violations,color=red>0"`
	LinterViolations int      `json:"linter_violations,omitempty" pretty:"label=Linter Violations,color=yellow>0"`
	FilesAnalyzed    int      `json:"files_analyzed" pretty:"label=Files Analyzed,color=blue"`
	RulesApplied     int      `json:"rules_applied" pretty:"label=Rules Applied,color=cyan"`
	LintersRun       int      `json:"linters_run" pretty:"label=Linters Run"`
	LintersSuccess   int      `json:"linters_successful" pretty:"label=Linters Successful,color=green"`
	FailedLinters    []string `json:"failed_linters,omitempty" pretty:"label=Failed Linters,style=text-red-600"`
	Duration         string   `json:"duration" pretty:"label=Duration,style=text-gray-600"`
}

func printConsolidatedSummary(result *models.ConsolidatedResult) {
	// Build the summary struct
	summary := ViolationSummary{
		TotalViolations:  result.Summary.TotalViolations,
		ArchViolations:   result.Summary.ArchViolations,
		LinterViolations: result.Summary.LinterViolations,
		FilesAnalyzed:    result.Summary.FilesAnalyzed,
		RulesApplied:     result.Summary.RulesApplied,
		LintersRun:       result.Summary.LintersRun,
		LintersSuccess:   result.Summary.LintersSuccessful,
		Duration:         result.Summary.Duration.String(),
	}

	if result.Summary.TotalViolations == 0 {
		summary.Status = "✓ No violations found!"
	} else {
		summary.Status = fmt.Sprintf("✗ Found %d total violation(s)", result.Summary.TotalViolations)
	}

	if failedLinters := result.GetFailedLinters(); len(failedLinters) > 0 {
		summary.FailedLinters = failedLinters
	}

	// Format and print using clicky
	output, err := clicky.Format(summary)
	if err != nil {
		// Fallback to original output
		fmt.Println()
		if result.Summary.TotalViolations == 0 {
			fmt.Printf("✓ No violations found!\n")
			fmt.Printf("  Analyzed %d file(s) with %d rule(s)\n",
				result.Summary.FilesAnalyzed, result.Summary.RulesApplied)
			fmt.Printf("  Ran %d linter(s) (%d successful)\n",
				result.Summary.LintersRun, result.Summary.LintersSuccessful)
		} else {
			fmt.Printf("✗ Found %d total violation(s)\n", result.Summary.TotalViolations)
			if result.Summary.ArchViolations > 0 {
				fmt.Printf("  - %d architecture violation(s)\n", result.Summary.ArchViolations)
			}
			if result.Summary.LinterViolations > 0 {
				fmt.Printf("  - %d linter violation(s)\n", result.Summary.LinterViolations)
			}
			fmt.Printf("  Analyzed %d file(s) with %d rule(s)\n",
				result.Summary.FilesAnalyzed, result.Summary.RulesApplied)
			fmt.Printf("  Ran %d linter(s) (%d successful)\n",
				result.Summary.LintersRun, result.Summary.LintersSuccessful)
		}
		if len(summary.FailedLinters) > 0 {
			fmt.Printf("  ⚠️  Failed linters: %v\n", summary.FailedLinters)
		}
		fmt.Printf("  Duration: %v\n", result.Summary.Duration)
	} else {
		fmt.Println()
		fmt.Print(output)
	}
}
