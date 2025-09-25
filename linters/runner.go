package linters

import (
	"context"
	"fmt"
	"time"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
	commonsCtx "github.com/flanksource/commons/context"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
)

// Runner orchestrates execution of multiple linters with intelligent debouncing
type Runner struct {
	registry       *Registry
	violationCache *cache.ViolationCache
	linterStats    *cache.LinterStats
	config         *models.Config
	workDir        string
	noCache        bool
}

// RunnerOptions configures the runner behavior
type RunnerOptions struct {
	NoCache bool // Disable caching
}

// NewRunner creates a new linter runner with intelligent debouncing
func NewRunner(config *models.Config, workDir string) (*Runner, error) {
	return NewRunnerWithOptions(config, workDir, RunnerOptions{})
}

// NewRunnerWithOptions creates a new linter runner with custom options
func NewRunnerWithOptions(config *models.Config, workDir string, opts RunnerOptions) (*Runner, error) {
	var violationCache *cache.ViolationCache
	var linterStats *cache.LinterStats
	var err error

	if !opts.NoCache {
		violationCache, err = cache.NewViolationCache()
		if err != nil {
			return nil, fmt.Errorf("failed to create violation cache: %w", err)
		}

		linterStats, err = cache.NewLinterStats()
		if err != nil {
			return nil, fmt.Errorf("failed to create linter stats: %w", err)
		}
	}

	return &Runner{
		registry:       DefaultRegistry,
		violationCache: violationCache,
		linterStats:    linterStats,
		config:         config,
		workDir:        workDir,
		noCache:        opts.NoCache,
	}, nil
}

// Close closes any resources held by the runner
func (r *Runner) Close() error {
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
func (r *Runner) RunEnabledLinters() ([]LinterResult, error) {
	return r.RunEnabledLintersOnFiles(nil, false)
}

// RunEnabledLintersOnFiles runs enabled linters on specific files
func (r *Runner) RunEnabledLintersOnFiles(specificFiles []string, fix bool) ([]LinterResult, error) {
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
		logger.Infof(result.Pretty().ANSI())

		results = append(results, *result)
	}

	return results, nil
}

// RunWithIntelligentDebounce executes a linter with intelligent debouncing
func (r *Runner) RunWithIntelligentDebounce(ctx context.Context, linterName string, files []string, fix bool) (*LinterResult, error) {

	// Get linter from registry
	linter, ok := r.registry.Get(linterName)
	if !ok {
		return nil, fmt.Errorf("unknown linter: %s", linterName)
	}

	// Get configuration
	config := r.config.GetLinterConfig(linterName, r.workDir)

	// Check intelligent debounce (only if cache is enabled)
	if r.linterStats != nil {
		shouldSkip, actualDebounce, err := r.linterStats.ShouldSkipLinter(linterName, r.workDir, config.Debounce)
		if err != nil {
			logger.Warnf("Failed to check debounce for %s: %v", linterName, err)
		} else if shouldSkip {
			logger.Debugf("Skipping %s due to intelligent debounce (%v)", linterName, actualDebounce)
			// Load cached violations and return
			return r.loadCachedResult(linterName, actualDebounce)
		}
	}

	opts := RunOptions{
		WorkDir:    r.workDir,
		Files:      files,
		Config:     config,
		ArchConfig: r.config, // Pass full config for all_language_excludes macro
		ForceJSON:  config.OutputFormat == "json" || config.OutputFormat == "",
		Fix:        fix,
		NoCache:    r.noCache,
	}

	if mixin, ok := linter.(OptionsMixin); ok {
		mixin.SetOptions(opts)
	}

	task := clicky.StartTask[[]models.Violation](fmt.Sprintf("Running %s", linterName), func(fCtx flanksourceContext.Context, t *task.Task) ([]models.Violation, error) {
		return linter.Run(context.Background(), opts)
	})
	violations, err := task.GetResult()

	// Record execution stats
	if r.linterStats != nil {
		if statsErr := r.linterStats.RecordExecution(linterName, r.workDir, task.Duration(), len(violations), task.Error() == nil); statsErr != nil {
			logger.Warnf("Failed to record execution stats for %s: %v", linterName, statsErr)
		}
	}

	// Cache violations if successful
	if task.Error() == nil && len(violations) > 0 && r.violationCache != nil {
		r.cacheViolations(linterName, violations)
	}

	r.updateTaskStatus(task.Task, linterName, task.IsOk(), len(violations), task.Error())

	result := &LinterResult{
		Linter:     linterName,
		Success:    task.IsOk(),
		Duration:   task.Duration(),
		Violations: violations,
		Error:      r.formatError(err),
	}

	// If the linter provides metadata, include it in the result
	if metadata, ok := linter.(MetadataProvider); ok {
		result.FileCount = metadata.GetFileCount()
		result.RuleCount = metadata.GetRuleCount()
	}

	return result, nil
}

// loadCachedResult loads cached violations for debounced linters
func (r *Runner) loadCachedResult(linterName string, debounce time.Duration) (*LinterResult, error) {

	task := clicky.StartTask[*LinterResult](fmt.Sprintf("Skipping %s (debounced for %v)", linterName, debounce), func(ctx commonsCtx.Context, t *clicky.Task) (
		*LinterResult, error) {
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

		result := &LinterResult{
			Linter:       linterName,
			Success:      true,
			Duration:     t.Duration(),
			Violations:   violations,
			Debounced:    true,
			DebounceUsed: debounce,
		}

		// For cached results, we can't provide file/rule counts since we didn't actually analyze
		// TODO: Consider caching metadata along with violations

		return result, nil

	})

	return task.GetResult()
}

// cacheViolations stores violations in the cache
func (r *Runner) cacheViolations(linterName string, violations []models.Violation) {
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

// updateTaskStatus updates the task manager status
func (r *Runner) updateTaskStatus(task *clicky.Task, linterName string, success bool, violationCount int, err error) {
	if success {
		if violationCount > 0 {
			task.SetName(fmt.Sprintf("%s (%d violations)", linterName, violationCount))
			task.Warning()
		} else {
			task.SetName(linterName)
			task.Success()
		}
	} else {
		task.SetName(fmt.Sprintf("%s (failed)", linterName))
		if err != nil {
			task.Errorf("Error: %v", err)
		}
		task.Failed()
	}
}

// hasArg checks if args contains a specific argument or argument prefix
// hasPathArg checks if the args already contain a path argument
// formatError formats an error for display
func (r *Runner) formatError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GetIntelligentDebounceForLinter returns the recommended debounce for a linter
func (r *Runner) GetIntelligentDebounceForLinter(linterName string) (time.Duration, error) {
	if r.linterStats == nil {
		return 30 * time.Second, nil
	}
	return r.linterStats.GetIntelligentDebounce(linterName, r.workDir)
}

// GetStatsForLinter returns execution statistics for a linter
func (r *Runner) GetStatsForLinter(linterName string) (*cache.ExecutionStats, error) {
	if r.linterStats == nil {
		return nil, fmt.Errorf("linter stats not available")
	}
	return r.linterStats.GetStats(linterName, r.workDir)
}
