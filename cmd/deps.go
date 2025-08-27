package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/spf13/cobra"
)

var (
	depsFilters    []string
	depsIndirect   bool
	depsDepth      int
	depsNoCache    bool
	depsGitCacheDir string
	depsShowConflicts bool
)

var depsCmd = &cobra.Command{
	Use:   "deps [path]",
	Short: "Analyze and visualize project dependencies",
	Long: `Scan dependency files (go.mod, package.json, requirements.txt, etc.) 
and show dependency tree with versions.

Supported dependency files:
  - Go: go.mod, go.sum
  - JavaScript/TypeScript: package.json, package-lock.json, yarn.lock
  - Python: requirements.txt, Pipfile, pyproject.toml, poetry.lock
  - Helm: Chart.yaml
  - Docker: Dockerfile`,
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

	// Common flags
	depsCmd.PersistentFlags().StringVarP(&depsFormat, "format", "f", "tree", "Output format: tree, json, yaml")
	depsCmd.PersistentFlags().StringVarP(&depsLanguage, "language", "l", "", "Filter by language: go, javascript, python, helm, docker")
	depsCmd.PersistentFlags().BoolVar(&depsNoCache, "no-cache", false, "Disable cache and force re-scan")
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
	
	fmt.Printf("DEBUG: Starting scan for path: %s, depth: %d\n", path, depsDepth)

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Find dependency files
	depFiles, err := findDependencyFiles(absPath, depsLanguage)
	if err != nil {
		return fmt.Errorf("failed to find dependency files: %w", err)
	}

	if len(depFiles) == 0 {
		fmt.Printf("No dependency files found in %s\n", absPath)
		return nil
	}

	fmt.Printf("Found %d dependency files\n", len(depFiles))

	// Create cache (unless disabled)
	var astCache *cache.ASTCache
	if !depsNoCache {
		astCache, err = cache.NewASTCache()
		if err != nil {
			fmt.Printf("Warning: Failed to create cache: %v\n", err)
			// Continue without cache
		} else {
			defer astCache.Close()
		}
	}

	// Process each dependency file
	allDeps := make([]*models.Dependency, 0)
	for _, depFile := range depFiles {
		fmt.Printf("Scanning %s...\n", depFile)
		
		// Read file content
		content, err := os.ReadFile(depFile)
		if err != nil {
			fmt.Printf("  Failed to read file: %v\n", err)
			continue
		}

		// Determine scanner based on file type
		scanner := getScanner(depFile)
		if scanner == nil {
			fmt.Printf("  No scanner available for %s\n", depFile)
			continue
		}

		// Scan dependencies
		deps, err := scanner.ScanFile(nil, depFile, content)
		if err != nil {
			fmt.Printf("  Failed to scan: %v\n", err)
			continue
		}

		allDeps = append(allDeps, deps...)
		fmt.Printf("  Found %d dependencies\n", len(deps))

		// Store in cache if available
		if astCache != nil {
			if err := astCache.StoreDependencies(depFile, deps); err != nil {
				fmt.Printf("  Warning: Failed to cache dependencies: %v\n", err)
			}
		}
	}

	// Display summary
	fmt.Printf("Total dependencies found: %d\n", len(allDeps))
	
	// Display dependencies based on format
	return displayDependencies(allDeps, depsFormat)
}

func runDepsTree(cmd *cobra.Command, args []string) error {
	// Tree and list commands use the same implementation
	return runDepsScan(cmd, args)
}

func runDepsList(cmd *cobra.Command, args []string) error {
	// Tree and list commands use the same implementation  
	return runDepsScan(cmd, args)
}

func runDepsTreeOld(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Get dependencies from cache or scan
	deps, err := getDependencies(path)
	if err != nil {
		return err
	}

	// Build and display tree
	return displayDependencyTree(deps)
}

func runDepsList(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Get dependencies from cache or scan
	deps, err := getDependencies(path)
	if err != nil {
		return err
	}

	fmt.Printf("DEBUG: runDepsList got %d dependencies\n", len(deps))
	fmt.Printf("DEBUG: clicky.Flags.FormatOptions = %+v\n", clicky.Flags.FormatOptions)

	// Use clicky.MustFormat to respect global format flags
	fmt.Println(clicky.MustFormat(deps))
	return nil
}

// findDependencyFiles finds all dependency files in the given path
func findDependencyFiles(path string, language string) ([]string, error) {
	var patterns []string
	
	if language != "" {
		// Get patterns for specific language from registry
		if scanner, ok := analysis.DefaultDependencyRegistry.Get(language); ok {
			patterns = scanner.SupportedFiles()
		} else {
			return nil, fmt.Errorf("unsupported language: %s", language)
		}
	} else {
		// Get all supported patterns from registry
		patterns = analysis.DefaultDependencyRegistry.GetAllSupportedFiles()
	}

	var files []string
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip common non-source directories
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || 
			   name == ".venv" || name == "venv" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file matches any pattern
		fileName := info.Name()
		for _, pattern := range patterns {
			matched, _ := filepath.Match(pattern, fileName)
			if matched {
				files = append(files, filePath)
				break
			}
		}

		return nil
	})

	return files, err
}

// getScanner returns the appropriate scanner for a file
func getScanner(filepath string) analysis.DependencyScanner {
	scanner, found := analysis.DefaultDependencyRegistry.GetScannerForFile(filepath)
	if found {
		return scanner
	}
	
	return nil
}

// getDependencies retrieves dependencies from cache or performs a scan
func getDependencies(path string) ([]*models.Dependency, error) {
	if !depsNoCache {
		// Try to get from cache first
		astCache, err := cache.NewASTCache()
		if err == nil {
			defer astCache.Close()
			deps, err := astCache.GetAllDependencies()
			if err == nil && len(deps) > 0 {
				return deps, nil
			}
		}
	}

	// Perform scan
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	depFiles, err := findDependencyFiles(absPath, depsLanguage)
	if err != nil {
		return nil, err
	}

	allDeps := make([]*models.Dependency, 0)
	for _, depFile := range depFiles {
		content, err := os.ReadFile(depFile)
		if err != nil {
			continue
		}

		scanner := getScanner(depFile)
		if scanner == nil {
			continue
		}

		deps, err := scanner.ScanFile(nil, depFile, content)
		if err != nil {
			continue
		}

		allDeps = append(allDeps, deps...)
	}

	return allDeps, nil
}

// displayDependencies displays dependencies in the specified format
func displayDependencies(deps []*models.Dependency, format string) error {
	switch format {
	case "tree":
		return displayDependencyTree(deps)
	case "list", "table":
		return displayDependenciesList(deps)
	case "json":
		// Simple JSON output
		fmt.Println("[")
		for i, dep := range deps {
			fmt.Printf("  {\"name\": \"%s\", \"version\": \"%s\", \"type\": \"%s\"}", 
				dep.Name, dep.Version, dep.Type)
			if i < len(deps)-1 {
				fmt.Println(",")
			} else {
				fmt.Println("")
			}
		}
		fmt.Println("]")
	case "yaml":
		// Simple YAML output
		for _, dep := range deps {
			fmt.Printf("- name: %s\n  version: %s\n  type: %s\n", 
				dep.Name, dep.Version, dep.Type)
		}
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
	return nil
}

// displayDependencyTree displays dependencies in a tree format
func displayDependencyTree(deps []*models.Dependency) error {
	if len(deps) == 0 {
		fmt.Println("No dependencies found")
		return nil
	}

	// Group by type
	byType := make(map[string][]*models.Dependency)
	for _, dep := range deps {
		byType[string(dep.Type)] = append(byType[string(dep.Type)], dep)
	}

	// Display each type
	for depType, typeDeps := range byType {
		color.New(color.FgCyan, color.Bold).Printf("\n%s Dependencies (%d)\n", strings.ToUpper(depType), len(typeDeps))
		fmt.Println(strings.Repeat("─", 40))
		
		for _, dep := range typeDeps {
			// Use Pretty method for consistent formatting
			prettyDep := dep.Pretty()
			fmt.Printf("  %s %s\n", 
				color.GreenString("├──"),
				prettyDep.Content)
			
			// Display additional info if available
			if dep.Git != "" {
				fmt.Printf("      %s %s\n", 
					color.HiBlackString("└─ git:"),
					color.HiBlackString(dep.Git))
			}
		}
	}

	return nil
}

// displayDependenciesList displays dependencies in a list format
func displayDependenciesList(deps []*models.Dependency) error {
	if len(deps) == 0 {
		fmt.Println("No dependencies found")
		return nil
	}

	// Print header
	fmt.Printf("%-12s %-40s %-15s %s\n", "TYPE", "NAME", "VERSION", "LANGUAGE")
	fmt.Println(strings.Repeat("-", 80))
	
	// Print dependencies
	for _, dep := range deps {
		prettyDep := dep.Pretty()
		fmt.Printf("%-12s %-50s %s\n", 
			string(dep.Type), 
			prettyDep.Content,
			dep.Language)
	}
	
	return nil
}