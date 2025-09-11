package archunit

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/flanksource/arch-unit/config"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/internal/files"
	"github.com/flanksource/arch-unit/linters"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	commonsCtx "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
)

// ArchUnit implements the Linter interface for arch-unit rules
type ArchUnit struct {
	linters.RunOptions
	fileCount int
	ruleCount int
}

// NewArchUnit creates a new arch-unit linter
func NewArchUnit(workDir string) *ArchUnit {
	return &ArchUnit{RunOptions: linters.RunOptions{WorkDir: workDir}}
}

// Name returns the linter name
func (a *ArchUnit) Name() string {
	return "arch-unit"
}

// DefaultIncludes returns default file patterns this linter should process
func (a *ArchUnit) DefaultIncludes() []string {
	return []string{"**/*.go", "**/*.py"}
}

// DefaultExcludes returns patterns this linter should ignore by default
func (a *ArchUnit) DefaultExcludes() []string {
	return []string{
		"vendor/**",
		"node_modules/**",
		".git/**",
		"**/*_test.go",
		"**/test_*.py",
	}
}

// SupportsJSON returns true if linter supports JSON output
func (a *ArchUnit) SupportsJSON() bool {
	return true // arch-unit natively outputs structured data
}

// JSONArgs returns additional args needed for JSON output
func (a *ArchUnit) JSONArgs() []string {
	return []string{} // No additional args needed
}

// SupportsFix returns true if linter supports auto-fixing violations
func (a *ArchUnit) SupportsFix() bool {
	return false // ArchUnit rules can't be auto-fixed
}

// FixArgs returns additional args needed for fix mode
func (a *ArchUnit) FixArgs() []string {
	return []string{} // No fix args since not supported
}

// ValidateConfig validates linter-specific configuration
func (a *ArchUnit) ValidateConfig(config *models.LinterConfig) error {
	// arch-unit doesn't need specific validation
	return nil
}

// GetFileCount returns the number of files analyzed by the last run
func (a *ArchUnit) GetFileCount() int {
	return a.fileCount
}

// GetRuleCount returns the number of rules applied by the last run
func (a *ArchUnit) GetRuleCount() int {
	return a.ruleCount
}

// Run executes the arch-unit analysis and returns violations
func (a *ArchUnit) Start(ctx commonsCtx.Context, task *clicky.Task) ([]models.Violation, error) {
	return a.Run(ctx, a.RunOptions)

}

// Run executes the arch-unit analysis and returns violations
func (a *ArchUnit) Run(ctx context.Context, opts linters.RunOptions) ([]models.Violation, error) {
	// If specific files are provided, filter for Go and Python files
	var goFiles, pythonFiles []string

	if len(opts.Files) > 0 {
		for _, file := range opts.Files {
			ext := filepath.Ext(file)
			if ext == ".go" {
				goFiles = append(goFiles, file)
			} else if ext == ".py" {
				pythonFiles = append(pythonFiles, file)
			}
		}
	} else {
		// Find all source files in the work directory
		var err error
		goFiles, pythonFiles, err = files.FindSourceFiles(opts.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("failed to find source files: %w", err)
		}
	}

	// Load configuration - start from the directory containing the files being analyzed
	searchDir := opts.WorkDir
	if len(opts.Files) > 0 {
		// Use the directory of the first file for config search
		searchDir = filepath.Dir(opts.Files[0])
	}
	
	configParser := config.NewParser(searchDir)
	archConfig, err := configParser.LoadConfig()
	if err != nil {
		// Use smart defaults if no config found
		archConfig, err = config.CreateSmartDefaultConfig(searchDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create default configuration: %w", err)
		}
	}

	// Open violation cache (unless disabled)
	var violationCache *cache.ViolationCache
	if !opts.NoCache {
		var err error
		violationCache, err = cache.NewViolationCache()
		if err != nil {
			logger.Warnf("Failed to open violation cache: %v", err)
			// Continue without cache
		}
		defer func() {
			if violationCache != nil {
				violationCache.Close()
			}
		}()
	}

	var allViolations []models.Violation
	var totalFiles, totalRules int

	// Analyze Go files
	if len(goFiles) > 0 {
		goResult, err := analyzeGoFilesWithCache(opts.WorkDir, goFiles, archConfig, violationCache)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze Go files: %w", err)
		}
		allViolations = append(allViolations, goResult.Violations...)
		totalFiles += goResult.FileCount
		totalRules += goResult.RuleCount
	}

	// TODO: Implement Python analysis using new architecture
	// Analyze Python files
	if len(pythonFiles) > 0 {
		logger.Warnf("Python analysis temporarily disabled during refactoring")
		// pyResult, err := analyzePythonFilesWithCache(opts.WorkDir, pythonFiles, archConfig, violationCache)
		// if err != nil {
		// 	logger.Warnf("Failed to analyze Python files: %v", err)
		// } else {
		// 	allViolations = append(allViolations, pyResult.Violations...)
		// }
	}

	// Set source to "arch-unit" for all violations
	for i := range allViolations {
		allViolations[i].Source = "arch-unit"
	}

	// Store counts for later retrieval
	a.fileCount = totalFiles
	a.ruleCount = totalRules

	return allViolations, nil
}

// analyzeGoFilesWithCache analyzes Go files with caching support
func analyzeGoFilesWithCache(rootDir string, files []string, config *models.Config, violationCache *cache.ViolationCache) (*models.AnalysisResult, error) {
	checker := NewViolationChecker()
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

		violations, err := checker.CheckViolations(file, rules)
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

		// Also add to result for immediate return
		result.Violations = append(result.Violations, violations...)
	}

	return result, nil
}

// TODO: Refactor Python analysis to use new architecture
// analyzePythonFilesWithCache analyzes Python files with caching support
func analyzePythonFilesWithCache(rootDir string, files []string, config *models.Config, violationCache *cache.ViolationCache) (*models.AnalysisResult, error) {
	logger.Warnf("Python analysis temporarily disabled during refactoring")
	return &models.AnalysisResult{
		FileCount:  len(files),
		RuleCount:  0,
		Violations: []models.Violation{},
	}, nil
}

