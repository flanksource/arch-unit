package linters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
)

// RunnerV2 orchestrates execution of multiple linters with intelligent debouncing
type RunnerV2 struct {
	registry       *Registry
	violationCache *cache.ViolationCache
	linterStats    *cache.LinterStats
	config         *models.Config
	workDir        string
}

// NewRunnerV2 creates a new V2 linter runner with intelligent debouncing
func NewRunnerV2(config *models.Config, workDir string) (*RunnerV2, error) {
	violationCache, err := cache.NewViolationCache()
	if err != nil {
		return nil, fmt.Errorf("failed to create violation cache: %w", err)
	}

	linterStats, err := cache.NewLinterStats()
	if err != nil {
		return nil, fmt.Errorf("failed to create linter stats: %w", err)
	}

	return &RunnerV2{
		registry:       DefaultRegistry,
		violationCache: violationCache,
		linterStats:    linterStats,
		config:         config,
		workDir:        workDir,
	}, nil
}


// Close closes any resources held by the runner
func (r *RunnerV2) Close() error {
	var errs []error

	if r.violationCache != nil {
		if err := r.violationCache.Close(); err != nil {
			errs = append(errs, fmt.Errorf("violation cache close error: %w", err))
		}
	}

	if r.linterStats != nil {
		if err := r.linterStats.Close(); err != nil {
			errs = append(errs, fmt.Errorf("linter stats close error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// RunEnabledLinters runs all enabled linters with intelligent debouncing
func (r *RunnerV2) RunEnabledLinters() ([]LinterResult, error) {
	return r.RunEnabledLintersOnFiles(nil, false)
}

// RunEnabledLintersOnFiles runs enabled linters on specific files
func (r *RunnerV2) RunEnabledLintersOnFiles(specificFiles []string, fix bool) ([]LinterResult, error) {
	var results []LinterResult

	enabledLinters := r.config.GetEnabledLinters()
	logger.Infof("Running %d enabled linters: %v", len(enabledLinters), enabledLinters)

	ctx := context.Background()

	for _, linterName := range enabledLinters {
		result, err := r.RunWithIntelligentDebounce(ctx, linterName, specificFiles, fix)
		if err != nil {
			logger.Warnf("Failed to run linter %s: %v", linterName, err)
			results = append(results, LinterResult{
				Linter:  linterName,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		results = append(results, *result)
	}

	return results, nil
}

// RunWithIntelligentDebounce executes a linter with intelligent debouncing
func (r *RunnerV2) RunWithIntelligentDebounce(ctx context.Context, linterName string, files []string, fix bool) (*LinterResult, error) {
	start := time.Now()

	// Get linter from registry
	linter, ok := r.registry.Get(linterName)
	if !ok {
		return nil, fmt.Errorf("unknown linter: %s", linterName)
	}

	// Get configuration
	config := r.config.GetLinterConfig(linterName, r.workDir)

	// Check intelligent debounce first
	shouldSkip, actualDebounce, err := r.linterStats.ShouldSkipLinter(linterName, r.workDir, config.Debounce)
	if err != nil {
		logger.Warnf("Failed to check debounce for %s: %v", linterName, err)
	} else if shouldSkip {
		// Load cached violations and return
		return r.loadCachedResult(linterName, actualDebounce, nil, start)
	}

	// Start task and execute linter
	typedTask := clicky.StartTask[*LinterResult](r.buildCommandDisplay(linter, config, files), func(ctx2 flanksourceContext.Context, t *task.Task) (*LinterResult, error) {
		return r.executeLinter(ctx, linterName, linter, config, files, fix, start, t)
	})

	// Wait for task completion
	result := typedTask.WaitFor()
	if result.Error != nil {
		return nil, result.Error
	}
	
	// Get the actual result from the task
	linterResult, err := typedTask.GetResult()
	if err != nil {
		return nil, err
	}
	return linterResult, nil
}

// executeLinter executes a linter with proper error handling and caching
func (r *RunnerV2) executeLinter(ctx context.Context, linterName string, linter Linter, config *models.LinterConfig, files []string, fix bool, start time.Time, t *task.Task) (*LinterResult, error) {
	// Execute linter
	violations, err := linter.Run(ctx, RunOptions{
		WorkDir:    r.workDir,
		Files:      files,
		Config:     config,
		ArchConfig: r.config, // Pass full config for all_language_excludes macro
		ForceJSON:  config.OutputFormat == "json" || config.OutputFormat == "",
		Fix:        fix,
	})

	duration := time.Since(start)
	success := err == nil

	// Record execution stats
	if r.linterStats != nil {
		if statsErr := r.linterStats.RecordExecution(linterName, r.workDir, duration, len(violations), success); statsErr != nil {
			logger.Warnf("Failed to record execution stats for %s: %v", linterName, statsErr)
		}
	}

	// Cache violations if successful
	if success && len(violations) > 0 && r.violationCache != nil {
		r.cacheViolations(linterName, violations)
	}

	// Update task status
	if t != nil {
		r.updateTaskStatus(t, linterName, success, len(violations), err)
	}

	return &LinterResult{
		Linter:     linterName,
		Success:    success,
		Duration:   duration,
		Violations: violations,
		Error:      r.formatError(err),
	}, err
}

// buildCommandDisplay builds a user-friendly command display
func (r *RunnerV2) buildCommandDisplay(linter Linter, config *models.LinterConfig, files []string) string {
	args := append([]string{}, config.Args...)

	// Add JSON args if supported and enabled
	if (config.OutputFormat == "json" || config.OutputFormat == "") && linter.SupportsJSON() {
		jsonArgs := linter.JSONArgs()
		for _, jsonArg := range jsonArgs {
			if !r.hasArg(args, jsonArg) {
				args = append(args, jsonArg)
			}
		}
	}

	// Add files or default includes
	if len(files) > 0 {
		args = append(args, files...)
	} else if !r.hasPathArg(args) {
		args = append(args, ".")
	}

	return fmt.Sprintf("%s %s", linter.Name(), strings.Join(args, " "))
}

// loadCachedResult loads cached violations for debounced linters
func (r *RunnerV2) loadCachedResult(linterName string, debounce time.Duration, task *clicky.Task, start time.Time) (*LinterResult, error) {
	logger.Debugf("Skipping linter %s (debounced for %v)", linterName, debounce)

	var violations []models.Violation
	if r.violationCache != nil {
		cachedViolations, err := r.violationCache.GetViolationsBySource(linterName)
		if err != nil {
			logger.Warnf("Failed to load cached violations for %s: %v", linterName, err)
		} else {
			violations = cachedViolations
			logger.Debugf("Loaded %d cached violations for %s", len(violations), linterName)
		}
	}

	return &LinterResult{
		Linter:       linterName,
		Success:      true,
		Duration:     time.Since(start),
		Violations:   violations,
		Debounced:    true,
		DebounceUsed: debounce,
	}, nil
}

// cacheViolations stores violations in the cache
func (r *RunnerV2) cacheViolations(linterName string, violations []models.Violation) {
	if r.violationCache == nil {
		return
	}

	// Group violations by file
	fileViolations := make(map[string][]models.Violation)
	for _, v := range violations {
		fileViolations[v.File] = append(fileViolations[v.File], v)
	}

	// Store each file's violations
	for file, vList := range fileViolations {
		if err := r.violationCache.StoreViolations(file, vList); err != nil {
			logger.Debugf("Failed to cache linter violations for %s: %v", file, err)
		}
	}
}

// updateTaskStatus updates the task status
func (r *RunnerV2) updateTaskStatus(t *task.Task, linterName string, success bool, violationCount int, err error) {
	if success {
		if violationCount > 0 {
			t.SetName(fmt.Sprintf("%s (%d violations)", linterName, violationCount))
			t.Warning()
		} else {
			t.SetName(linterName)
			t.Success()
		}
	} else {
		t.SetName(fmt.Sprintf("%s (failed)", linterName))
		if err != nil {
			t.Errorf("Error: %v", err)
		}
		t.Failed()
	}
}

// hasArg checks if args contains a specific argument or argument prefix
func (r *RunnerV2) hasArg(args []string, argToFind string) bool {
	prefix := strings.Split(argToFind, "=")[0]
	for _, arg := range args {
		if arg == argToFind || strings.HasPrefix(arg, prefix+"=") {
			return true
		}
	}
	return false
}

// hasPathArg checks if the args already contain a path argument
func (r *RunnerV2) hasPathArg(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// formatError formats an error for display
func (r *RunnerV2) formatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetIntelligentDebounceForLinter returns the recommended debounce for a linter
func (r *RunnerV2) GetIntelligentDebounceForLinter(linterName string) (time.Duration, error) {
	if r.linterStats == nil {
		return 30 * time.Second, nil
	}
	return r.linterStats.GetIntelligentDebounce(linterName, r.workDir)
}

// GetStatsForLinter returns execution statistics for a linter
func (r *RunnerV2) GetStatsForLinter(linterName string) (*cache.ExecutionStats, error) {
	if r.linterStats == nil {
		return nil, fmt.Errorf("linter stats not available")
	}
	return r.linterStats.GetStats(linterName, r.workDir)
}
