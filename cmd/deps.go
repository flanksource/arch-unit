package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/dependencies"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	depsFilters       []string
	depsIndirect      bool
	depsDepth         int
	depsNoCache       bool
	depsGitCacheDir   string
	depsShowConflicts bool
)

var depsCmd = &cobra.Command{
	Use:   "deps [path-or-git-url]",
	Short: "Analyze and visualize project dependencies",
	Long: `Scan dependency files (go.mod, package.json, requirements.txt, etc.)
and show dependency tree with versions.

Path can be a local directory or git URL:
  - Local:  ".", "/path/to/project"
  - Git:    "github.com/user/repo@v1.0.0", "https://github.com/user/repo@main"

Supported dependency files:
  - Go: go.mod, go.sum
  - JavaScript/TypeScript: package.json, package-lock.json, yarn.lock
  - Python: requirements.txt, Pipfile, pyproject.toml, poetry.lock
  - Helm: Chart.yaml
  - Docker: Dockerfile

Use --depth > 0 to enable git repository traversal and version conflict detection.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeps,
}

var depsScanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan and cache dependencies",
	Long:  `Scan dependency files in the specified path and cache the results`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDepsScan,
}

var depsTreeCmd = &cobra.Command{
	Use:   "tree [path]",
	Short: "Show dependency tree",
	Long:  `Display dependencies in a tree format`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDepsTree,
}

var depsListCmd = &cobra.Command{
	Use:   "list [path]",
	Short: "List all dependencies",
	Long:  `List all dependencies found in the project`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDepsList,
}

func init() {
	rootCmd.AddCommand(depsCmd)
	depsCmd.AddCommand(depsScanCmd)
	depsCmd.AddCommand(depsTreeCmd)
	depsCmd.AddCommand(depsListCmd)
	depsCmd.PersistentFlags().BoolVar(&depsIndirect, "indirect", true, "Include indirect dependencies")
	depsCmd.PersistentFlags().IntVar(&depsDepth, "depth", 0, "Maximum dependency depth to traverse (0 for local only, >0 for git traversal)")
	depsCmd.PersistentFlags().StringSliceVar(&depsFilters, "filter", []string{}, "Filter dependencies (e.g., '!go', '*flanksource*', 'github.com/spf13/*')")
	depsCmd.PersistentFlags().BoolVar(&depsNoCache, "no-cache", false, "Bypass cache for Git URL resolution")
	depsCmd.PersistentFlags().StringVar(&depsGitCacheDir, "git-cache-dir", ".cache/arch-unit/repositories", "Directory for git repository cache")
	depsCmd.PersistentFlags().BoolVar(&depsShowConflicts, "show-conflicts", false, "Show version conflicts in output")
}

func runDeps(cmd *cobra.Command, args []string) error {
	// Default to scan if no subcommand specified
	return runDepsScan(cmd, args)
}

func runDepsScan(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	result := task.StartTask(fmt.Sprintf("Dependency Scan: %s", path), func(ctx clicky.Context, t *clicky.Task) (*models.ScanResult, error) {
		return performDependencyScan(ctx, t, path)
	})

	// Wait for the specific task to complete, not all global tasks
	deps, err := result.GetResult()
	if err != nil {
		return err
	}

	// Defensive check: ensure result is not nil
	if deps == nil {
		logger.Infof("No dependencies found")
		return nil
	}

	if len(deps.Dependencies) == 0 {
		logger.Infof("No dependencies found")
		return nil
	}

	// Display results
	if depsShowConflicts && len(deps.Conflicts) > 0 {
		logger.Warnf("Version conflicts detected (%d)", len(deps.Conflicts))
		for _, conflict := range deps.Conflicts {
			logger.Warnf("📦 %s:", conflict.DependencyName)
			for _, version := range conflict.Versions {
				logger.Warnf("  - %s (depth %d from %s)", version.Version, version.Depth, version.Source)
			}
			logger.Warnf("  Resolution: %s", conflict.ResolutionStrategy)
		}
	}

	fmt.Println(clicky.MustFormat(deps.Dependencies))
	return nil
}

func performDependencyScan(ctx clicky.Context, t *clicky.Task, path string) (*models.ScanResult, error) {

	// Configure resolution service TTL based on cache flag
	if depsNoCache {
		analysis.SetResolutionServiceTTL(0) // TTL of 0 means no caching
	}

	// Create resolution service EARLY to avoid lazy initialization deadlock in parallel tasks
	resolver, err := analysis.GetResolutionService()
	if err != nil {
		return nil, fmt.Errorf("failed to get resolution service: %w", err)
	}

	// Create a custom registry with all existing scanners plus the resolver-enabled Go scanner
	registry := analysis.NewDependencyRegistry()

	// Copy all existing scanners from the default registry
	defaultRegistry := analysis.GetDefaultRegistry()
	for _, lang := range defaultRegistry.List() {
		if lang != "go" && lang != "helm" && lang != "docker" { // Skip go, helm, and docker, we'll add our enhanced versions
			if existingScanner, ok := defaultRegistry.Get(lang); ok {
				registry.Register(existingScanner)
			}
		}
	}

	// Add our enhanced Go scanner with resolver
	goScanner := analysis.NewGoDependencyScannerWithResolver(resolver)
	registry.Register(goScanner)

	// Add enhanced Helm scanner with resolver
	helmScanner := dependencies.NewHelmDependencyScannerWithResolver(resolver)
	registry.Register(helmScanner)

	// Add enhanced Docker scanner with resolver
	dockerScanner := dependencies.NewDockerDependencyScannerWithResolver(resolver)
	registry.Register(dockerScanner)

	// Create scanner with custom registry
	scanner := dependencies.NewScannerWithRegistry(registry)

	// Set git cache directory if depth > 0
	if depsDepth > 0 {
		scanner.SetupGitSupport(depsGitCacheDir)
	}

	// Create scan context with all configuration
	scanCtx := analysis.NewScanContext(t, path).
		WithDepth(depsDepth).
		WithIndirect(depsIndirect)
	// TODO: Apply filters in post-processing

	// Phase 2: Scanning
	if depsDepth > 0 {
		t.Infof("Scanning dependencies with depth %d in %s", depsDepth, path)
	} else {
		t.Infof("Scanning local dependencies in %s", path)
	}

	// Use ScanWithContext method
	result, err := scanner.ScanWithContext(scanCtx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to scan dependencies: %w", err)
	}

	// Ensure result is never nil
	if result == nil {
		result = &models.ScanResult{
			Dependencies: []*models.Dependency{},
			Conflicts:    []models.VersionConflict{},
			Metadata: models.ScanMetadata{
				ScanType:          "local",
				MaxDepth:          depsDepth,
				RepositoriesFound: 0,
				TotalDependencies: 0,
				ConflictsFound:    0,
			},
		}
	}

	// Mark task as successful
	if t != nil {
		t.Success()
	}
	return result, nil
}

func runDepsTree(cmd *cobra.Command, args []string) error {
	// Tree and list commands use the same implementation
	return runDepsScan(cmd, args)
}

func runDepsList(cmd *cobra.Command, args []string) error {
	// Tree and list commands use the same implementation
	return runDepsScan(cmd, args)
}
