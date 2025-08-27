package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/clicky"
	"github.com/spf13/cobra"
)

var (
	astNoCache    bool
	astCacheTTL   string
	astMaxWorkers int
	astLanguages  []string
)

var astAnalyzeCmd = &cobra.Command{
	Use:   "analyze [path]",
	Short: "Analyze AST for source files",
	Long: `Analyze Abstract Syntax Trees for source files in the specified directory.

This command builds and updates the AST cache for your codebase, which can then be
queried using other ast commands. Progress is shown using task tracking.

Examples:
  # Analyze current directory
  arch-unit ast analyze

  # Analyze specific directory
  arch-unit ast analyze ./src

  # Force re-analysis (ignore cache)
  arch-unit ast analyze --no-cache

  # Analyze only specific languages
  arch-unit ast analyze --languages go,python`,
	Args: cobra.MaximumNArgs(1),
	RunE: runASTAnalyze,
}

func init() {
	astCmd.AddCommand(astAnalyzeCmd)
	astAnalyzeCmd.Flags().BoolVar(&astNoCache, "no-cache", false, "Disable caching and force re-analysis of all files")
	astAnalyzeCmd.Flags().StringVar(&astCacheTTL, "cache-ttl", "4h", "Cache time-to-live (e.g., 1h, 30m, 24h)")
	astAnalyzeCmd.Flags().StringSliceVar(&astLanguages, "languages", nil, "Filter to specific languages (e.g., go,python,javascript)")
	astAnalyzeCmd.Flags().IntVar(&astMaxWorkers, "max-workers", 0, "Maximum number of parallel workers (0 = auto)")
}

func runASTAnalyze(cmd *cobra.Command, args []string) error {
	// Determine target directory
	workDir := "."
	if len(args) > 0 {
		workDir = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if directory exists
	if info, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Parse cache TTL
	var cacheTTL time.Duration
	if !astNoCache && astCacheTTL != "" {
		duration, err := time.ParseDuration(astCacheTTL)
		if err != nil {
			return fmt.Errorf("invalid cache-ttl format: %w", err)
		}
		cacheTTL = duration
	}

	// Get cache singleton
	astCache, err := cache.NewASTCache()
	if err != nil {
		return fmt.Errorf("failed to get cache: %w", err)
	}
	defer astCache.Close()

	// Create coordinator options
	opts := ast.CoordinatorOptions{
		NoCache:    astNoCache,
		CacheTTL:   cacheTTL,
		Languages:  astLanguages,
		MaxWorkers: astMaxWorkers,
	}

	// Create coordinator
	coordinator := ast.NewCoordinator(astCache, absPath, opts)

	// Create root task
	rootTask := clicky.StartGlobalTask("AST Analysis")

	// Log configuration
	rootTask.Infof("Analyzing directory: %s", absPath)
	if astNoCache {
		rootTask.Infof("Cache: disabled")
	} else {
		rootTask.Infof("Cache: enabled (TTL: %v)", cacheTTL)
	}
	if astMaxWorkers > 0 {
		rootTask.Infof("Workers: %d", astMaxWorkers)
	} else {
		rootTask.Infof("Workers: auto (CPU count)")
	}
	if len(astLanguages) > 0 {
		rootTask.Infof("Languages: %v", astLanguages)
	} else {
		rootTask.Infof("Languages: all supported")
	}

	// Run analysis
	startTime := time.Now()
	results, err := coordinator.AnalyzeDirectory(rootTask, absPath)
	if err != nil {
		rootTask.Errorf("Analysis failed: %v", err)
		rootTask.Failed()
		return err
	}

	duration := time.Since(startTime)
	rootTask.Infof("Analysis completed in %v", duration)
	
	// Count some basic statistics for the final log
	totalFiles := len(results)
	successCount := 0
	errorCount := 0
	for _, result := range results {
		if result.Error != nil {
			errorCount++
		} else {
			successCount++
		}
	}
	
	rootTask.Infof("Analyzed %d files: %d successful, %d errors", totalFiles, successCount, errorCount)
	
	if errorCount > 0 {
		rootTask.Warning()
	} else {
		rootTask.Success()
	}

	// Wait for clicky tasks to complete
	_ = clicky.WaitForGlobalCompletionSilent()
	
	return nil
}