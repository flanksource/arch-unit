package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	viewFormat   string
	viewNoColor  bool
	viewContext  int
)

var astViewCmd = &cobra.Command{
	Use:   "view [pattern]",
	Short: "View and pretty print source code of AST nodes",
	Long: `View and pretty print the source code of AST nodes matching the given pattern.

This command uses clicky for syntax highlighting and pretty printing of source code.
It shows the source code with context lines and highlighting for the matched AST nodes.

PATTERN EXAMPLES:
  arch-unit ast view "Controller*"                    # All Controller types
  arch-unit ast view "UserService:GetUser"           # Specific method
  arch-unit ast view "models:User:*"                 # All fields in User type
  arch-unit ast view "*Service*" --format json       # JSON output
  
OUTPUT FORMATS:
  - tree: Tree view with syntax highlighting (default)
  - plain: Plain text with line numbers  
  - json: JSON structure for programmatic use`,
	Args: cobra.MaximumNArgs(1),
	RunE: runASTView,
}

func init() {
	astCmd.AddCommand(astViewCmd)

	astViewCmd.Flags().StringVar(&viewFormat, "format", "tree", "Output format: tree, plain, json")
	astViewCmd.Flags().BoolVar(&viewNoColor, "no-color", false, "Disable colored output")
	astViewCmd.Flags().IntVar(&viewContext, "context", 2, "Number of context lines to show around each node")
}

func runASTView(cmd *cobra.Command, args []string) error {
	// Get pattern from args
	pattern := "*"
	if len(args) > 0 {
		pattern = args[0]
	}

	// Setup: Initialize cache and get working directory
	astCache, err := cache.NewASTCache()
	if err != nil {
		return fmt.Errorf("failed to create AST cache: %w", err)
	}
	defer astCache.Close()

	workingDir, err := GetWorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create analyzer
	analyzer := ast.NewAnalyzer(astCache, workingDir)

	// Analyze files if needed (check cache first)
	logger.Infof("Analyzing source files...")
	if err := analyzer.AnalyzeFiles(); err != nil {
		return fmt.Errorf("failed to analyze files: %w", err)
	}

	// Query nodes by pattern
	logger.Debugf("Querying pattern: %s", pattern)
	nodes, err := analyzer.QueryPattern(pattern)
	if err != nil {
		return fmt.Errorf("failed to query pattern: %w", err)
	}

	if len(nodes) == 0 {
		logger.Infof("No nodes found matching pattern: %s", pattern)
		return nil
	}

	logger.Infof("Found %d nodes matching pattern", len(nodes))

	// Create source viewer
	viewer := ast.NewSourceViewer(workingDir, viewNoColor)

	// View source for all nodes
	views, err := viewer.ViewMultipleNodes(nodes)
	if err != nil {
		return fmt.Errorf("failed to view node sources: %w", err)
	}

	if len(views) == 0 {
		logger.Warnf("No source code could be retrieved for the matched nodes")
		return nil
	}

	// Format and display the views
	output, err := viewer.FormatMultipleViews(views, viewFormat)
	if err != nil {
		return fmt.Errorf("failed to format source views: %w", err)
	}

	fmt.Println(output)

	return nil
}