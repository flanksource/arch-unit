package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	graphFormat   string
	graphNoColor  bool
	graphDepth    int
	graphShowLibs bool
	graphRootOnly bool
)

var astGraphCmd = &cobra.Command{
	Use:   "graph [pattern]",
	Short: "Generate and display call graphs for AST nodes",
	Long: `Generate and display call graphs showing the relationships between functions,
methods, and their dependencies.

The call graph shows:
- Direct function calls between AST nodes
- External library/framework calls
- Call depth and complexity metrics
- Root nodes (entry points) in the call graph

PATTERN EXAMPLES:
  arch-unit ast graph "Controller*"                   # Call graphs for all controllers
  arch-unit ast graph "UserService" --depth 3        # 3 levels deep from UserService
  arch-unit ast graph "*Service*" --format dot       # DOT format for Graphviz
  arch-unit ast graph "main:main" --show-libs        # Show library calls

OUTPUT FORMATS:
  - tree: Tree visualization of call relationships (default)
  - dot: DOT notation for Graphviz rendering
  - json: JSON structure for programmatic use

EXAMPLES:
  # Generate call graph for all services, 2 levels deep
  arch-unit ast graph "*Service*" --depth 2

  # Export to Graphviz format
  arch-unit ast graph "UserController" --format dot > callgraph.dot
  graphviz -Tpng callgraph.dot -o callgraph.png

  # Show only root entry points
  arch-unit ast graph "*" --root-only`,
	Args: cobra.MaximumNArgs(1),
	RunE: runASTGraph,
}

func init() {
	astCmd.AddCommand(astGraphCmd)

	astGraphCmd.Flags().StringVar(&graphFormat, "format", "tree", "Output format: tree, dot, json")
	astGraphCmd.Flags().BoolVar(&graphNoColor, "no-color", false, "Disable colored output")
	astGraphCmd.Flags().IntVar(&graphDepth, "depth", 3, "Maximum depth for call graph traversal")
	astGraphCmd.Flags().BoolVar(&graphShowLibs, "show-libs", true, "Show external library calls")
	astGraphCmd.Flags().BoolVar(&graphRootOnly, "root-only", false, "Show only root nodes (entry points)")
}

func runASTGraph(cmd *cobra.Command, args []string) error {
	// Get pattern from args
	pattern := "*"
	if len(args) > 0 {
		pattern = args[0]
	}

	// Initialize AST cache
	astCache := cache.MustGetASTCache()

	workingDir, err := GetWorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create analyzer
	analyzer := ast.NewAnalyzer(astCache, workingDir)

	// Analyze files if needed
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

	// Get all relationships for the call graph
	logger.Debugf("Fetching relationships...")
	relationships, err := analyzer.GetAllRelationships()
	if err != nil {
		return fmt.Errorf("failed to get relationships: %w", err)
	}

	var libraryRels []*models.LibraryRelationship
	if graphShowLibs {
		libraryRels, err = analyzer.GetLibraryRelationships()
		if err != nil {
			logger.Warnf("Failed to get library relationships: %v", err)
			// Continue without library relationships
			libraryRels = nil
		}
	}

	logger.Debugf("Found %d relationships and %d library relationships",
		len(relationships), len(libraryRels))

	// Build call graph
	graphBuilder := ast.NewGraphBuilder()
	var callGraph *ast.CallGraph

	if graphRootOnly {
		// Build full graph first to identify roots
		fullGraph := graphBuilder.BuildCallGraph(nodes, relationships, libraryRels)
		callGraph = &ast.CallGraph{
			Nodes:         fullGraph.RootNodes,
			Relationships: []*models.ASTRelationship{}, // No relationships for root-only
			LibraryRels:   []*models.LibraryRelationship{},
			RootNodes:     fullGraph.RootNodes,
		}
	} else {
		// Build call graph with depth limit from the matched nodes as roots
		callGraph = graphBuilder.BuildCallGraphFromRoots(nodes, relationships, libraryRels, graphDepth)
	}

	if len(callGraph.Nodes) == 0 {
		logger.Infof("No call graph nodes found")
		return nil
	}

	logger.Infof("Built call graph with %d nodes, %d relationships",
		len(callGraph.Nodes), len(callGraph.Relationships))

	// Format and display the call graph
	output, err := graphBuilder.FormatCallGraph(callGraph, graphFormat, graphDepth)
	if err != nil {
		return fmt.Errorf("failed to format call graph: %w", err)
	}

	fmt.Println(output)

	return nil
}
