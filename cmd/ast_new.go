package cmd

import (
	"fmt"
	"os"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	astFormat         string
	astShowCalls      bool
	astShowLibraries  bool
	astShowComplexity bool
	astCachedOnly     bool
	astRebuildCache   bool
	astThreshold      int
	astDepth          int
	astQuery          string
	astNoColor        bool
)

var astCmd = &cobra.Command{
	Use:   "ast [pattern]",
	Short: "Analyze and inspect AST (Abstract Syntax Tree) of code",
	Long: `Analyze and inspect the Abstract Syntax Tree of your codebase.
	
The ast command provides detailed information about code structure, relationships,
complexity metrics, and dependencies.

Examples:
  # Show AST for all Controller types
  arch-unit ast "Controller*"
  
  # Show AST with call relationships
  arch-unit ast "pkg.UserService:CreateUser" --calls
  
  # Show complexity metrics above threshold
  arch-unit ast "Service*" --complexity --threshold=10
  
  # Show external library dependencies
  arch-unit ast "pkg.*" --libraries
  
  # Output as JSON
  arch-unit ast "Controller*" --format=json
  
  # Execute AQL query
  arch-unit ast --query "*.cyclomatic > 15"
  
  # Rebuild AST cache
  arch-unit ast --rebuild-cache`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAST,
}

func init() {
	rootCmd.AddCommand(astCmd)

	astCmd.Flags().StringVar(&astFormat, "format", "tree", "Output format: tree, json, table")
	astCmd.Flags().BoolVar(&astShowCalls, "calls", false, "Show call relationships")
	astCmd.Flags().BoolVar(&astShowLibraries, "libraries", false, "Show external library dependencies")
	astCmd.Flags().BoolVar(&astShowComplexity, "complexity", false, "Show complexity metrics")
	astCmd.Flags().BoolVar(&astCachedOnly, "cached-only", false, "Show only cached results, don't analyze new files")
	astCmd.Flags().BoolVar(&astRebuildCache, "rebuild-cache", false, "Rebuild the entire AST cache")
	astCmd.Flags().IntVar(&astThreshold, "threshold", 0, "Complexity threshold filter")
	astCmd.Flags().IntVar(&astDepth, "depth", 1, "Relationship traversal depth")
	astCmd.Flags().StringVar(&astQuery, "query", "", "AQL query to execute")
	astCmd.Flags().BoolVar(&astNoColor, "no-color", false, "Disable colored output")
}

func runAST(cmd *cobra.Command, args []string) error {
	// 1. Setup: Initialize cache and get working directory
	astCache, err := cache.NewASTCache()
	if err != nil {
		return fmt.Errorf("failed to create AST cache: %w", err)
	}
	defer astCache.Close()

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// 2. Create analyzer and formatter
	analyzer := ast.NewAnalyzer(astCache, workingDir)
	formatter := ast.NewFormatter(astFormat, astNoColor, workingDir)

	// 3. Handle rebuild cache request
	if astRebuildCache {
		logger.Infof("Rebuilding AST cache...")
		if err := analyzer.RebuildCache(); err != nil {
			return fmt.Errorf("failed to rebuild cache: %w", err)
		}
		logger.Infof("Cache rebuild complete")
		return nil
	}

	// 4. Handle AQL query
	if astQuery != "" {
		logger.Debugf("Executing AQL query: %s", astQuery)
		nodes, err := analyzer.ExecuteAQLQuery(astQuery)
		if err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}

		output, err := formatter.FormatNodes(nodes, astQuery)
		if err != nil {
			return fmt.Errorf("failed to format results: %w", err)
		}

		fmt.Println(output)
		return nil
	}

	// 5. Handle pattern query
	pattern := "*"
	if len(args) > 0 {
		pattern = args[0]
	}

	// Show overview if no pattern and cached-only
	if pattern == "" && astCachedOnly {
		overview, err := analyzer.GetOverview()
		if err != nil {
			return fmt.Errorf("failed to get overview: %w", err)
		}

		output, err := formatter.FormatOverview(overview)
		if err != nil {
			return fmt.Errorf("failed to format overview: %w", err)
		}

		fmt.Println(output)
		return nil
	}

	// Analyze files if not cached-only and no specific pattern
	if !astCachedOnly && pattern == "*" {
		logger.Infof("Analyzing source files...")
		if err := analyzer.AnalyzeFiles(); err != nil {
			return fmt.Errorf("failed to analyze files: %w", err)
		}
	}

	// Query nodes by pattern
	logger.Debugf("Querying pattern: %s", pattern)
	nodes, err := analyzer.QueryPattern(pattern)
	if err != nil {
		return fmt.Errorf("failed to query pattern: %w", err)
	}

	// Apply filters
	if astThreshold > 0 {
		nodes = ast.FilterByComplexity(nodes, astThreshold)
		logger.Debugf("Filtered to %d nodes with complexity >= %d", len(nodes), astThreshold)
	}

	// Show relationships if requested
	if astShowCalls && len(nodes) > 0 {
		// For tree format, relationships are included in the display
		// For other formats, we'd need to fetch and append them
		if astFormat != "tree" {
			logger.Warnf("--calls flag is currently only supported with tree format")
		}
	}

	// Show libraries if requested
	if astShowLibraries && len(nodes) > 0 {
		// Similar to relationships
		if astFormat != "tree" {
			logger.Warnf("--libraries flag is currently only supported with tree format")
		}
	}

	// Format and output results
	output, err := formatter.FormatNodes(nodes, pattern)
	if err != nil {
		return fmt.Errorf("failed to format nodes: %w", err)
	}

	fmt.Println(output)

	// Show cache stats if verbose
	if logger.IsLevelEnabled(3) { // Debug level
		stats, err := analyzer.GetCacheStats()
		if err == nil {
			statsOutput, err := formatter.FormatCacheStats(stats)
			if err == nil {
				fmt.Println("\n" + statsOutput)
			}
		}
	}

	return nil
}