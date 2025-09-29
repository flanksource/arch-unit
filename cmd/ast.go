package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	astFormat         string
	astTemplate       string
	astShowCalls      bool
	astShowLibraries  bool
	astShowComplexity bool
	astShowFields     bool
	astCachedOnly     bool
	astRebuildCache   bool
	astThreshold      int
	astDepth          int
	astQuery          string
	astAll            bool

	// New display configuration flags
	astShowDirs      bool
	astShowFiles     bool
	astShowPackages  bool
	astShowTypes     bool
	astShowMethods   bool
	astShowParams    bool
	astShowImports   bool
	astShowLineNo    bool
	astShowFileStats bool
)

var astCmd = &cobra.Command{
	Use:   "ast [pattern]",
	Short: "Print cached AST nodes matching a pattern (alias for 'ast print')",
	Long: `Print cached Abstract Syntax Tree nodes matching a pattern.

This command displays previously analyzed AST data from the cache without performing
new analysis. It is an alias for 'ast print' and provides the same functionality.

To analyze files and build the cache, use 'ast analyze' first.

Print cached Abstract Syntax Tree nodes of your codebase.

The ast command provides detailed information about code structure, relationships,
complexity metrics, and dependencies using powerful pattern matching.

PATTERN SYNTAX:
  AST Patterns use the format: package:type:method:field
  File Path Patterns use doublestar glob syntax with @ prefix or path() function
  Combined Patterns: @file_pattern:ast_pattern or path(file_pattern) AND ast_pattern
  - Use wildcards (*) for any component
  - Omit components to match any value
  - Components are matched hierarchically

PATTERN EXAMPLES:
  Basic AST Patterns:
    "*"                    # All nodes
    "Controller*"          # Types starting with "Controller"
    "*Service"             # Types ending with "Service"
    "models.*"             # All nodes in "models" package

  File Path Patterns:
    "@**/*.go"             # All Go files in any directory
    "@src/**/*.go"         # Go files in src directory and subdirectories
    "@controllers/*.go"    # Go files directly in controllers directory
    "path(**/*.py)"        # All Python files (function syntax)

  Combined File Path + AST Patterns:
    "@**/*.go:Service*"                    # Service types in any Go file
    "@controllers/**/*.go:*Controller*"    # Controller types in controllers directory
    "@src/**/*.go:api:UserService"         # UserService in api package, src directory
    "path(internal/**/*.go) AND Service*"  # Service types in internal directory

  Package Patterns:
    "controllers"          # All nodes in controllers package
    "pkg.*"                # All nodes in packages starting with "pkg"
    "api:*"                # All types in api package

  Type Patterns:
    "controllers:User*"    # Types starting with "User" in controllers
    "*:Controller"         # All Controller types in any package
    "models:User"          # Specific User type in models package

  Method Patterns:
    "controllers:UserController:Get*"     # Methods starting with "Get"
    "*:*:CreateUser"                      # CreateUser methods in any type
    "service:UserService:*"               # All methods in UserService

  Field Patterns:
    "models:User:Name"                    # Specific field
    "models:User:*"                       # All fields in User type
    "*:*:*:ID"                           # All ID fields

  Metric Queries (use with --query flag):
    "lines(*) > 100"              # Find all nodes with more than 100 lines
    "cyclomatic(*) >= 10"         # Find methods with high complexity
    "params(*:*:Get*) > 3"        # Find Get methods with many parameters
    "lines(Service*) < 50"        # Find small Service implementations
    "returns(*Controller*) != 1"  # Find controllers not returning single value
    "len(*) > 40"                 # Find nodes with long names
    "imports(*) > 10"             # Find modules with many imports
    "calls(*) > 5"                # Find methods with many external calls

  File Path + Metric Queries:
    "lines(@src/**/*.go:Service*) > 100"      # Large Service classes in src directory
    "cyclomatic(@**/*.go:*Handler*) >= 15"   # Complex handlers in any Go file
    "params(@controllers/**/*.go:*) > 5"     # Methods with many params in controllers
    "lines(@internal/**/*.go:*) < 20"       # Small functions in internal packages

AVAILABLE NODE TYPES:
  - package: Package declarations
  - type: Classes, structs, interfaces
  - method: Functions, methods
  - field: Struct fields, class properties
  - variable: Variables, constants

AVAILABLE METRICS:
  - lines: Line count of the node
  - cyclomatic: Cyclomatic complexity (control flow complexity)
  - parameters/params: Number of method parameters (params is an alias)
  - returns: Number of return values for methods
  - len: Length of the node's full name (package:type:method:field)
  - imports: Number of import relationships
  - calls: Number of external call relationships (calls outside the package)

TEMPLATE VARIABLES (for --format template):
  - {{.Package}}: Package name
  - {{.Class}}: Type/class name (same as {{.Type}})
  - {{.Type}}: Type/class name
  - {{.Method}}: Method/function name
  - {{.Field}}: Field name
  - {{.File}}: Relative file path
  - {{.Lines}}: Source lines of code (SLOC)
  - {{.Complexity}}: Cyclomatic complexity
  - {{.Params}}: Number of parameters
  - {{.Returns}}: Number of return values
  - {{.NodeType}}: Node type (package, type, method, field, variable)
  - {{.StartLine}}: Starting line number
  - {{.EndLine}}: Ending line number

COMMAND EXAMPLES:
  # Show all Controller types with verbose pattern matching
  arch-unit ast "Controller*" -v

  # Show specific method with call relationships
  arch-unit ast "controllers:UserController:GetUser" --calls

  # Find complex methods above threshold
  arch-unit ast "Service*" --threshold=10

  # Show external library dependencies
  arch-unit ast "pkg.*" --libraries

  # Output as JSON for programmatic use
  arch-unit ast "Controller*" --format=json

  # Show all fields in User models
  arch-unit ast "models:User:*:*" --format=table

  # Custom template output format
  arch-unit ast "*" --format=template --template "{{.Package}}.{{.Class}}.{{.Method}} ({{.Lines}} SLOC)"
  arch-unit ast "*Service*" --format=template --template "Service: {{.Method}} - Complexity: {{.Complexity}}"

  # Execute metric queries for code analysis
  arch-unit ast --query "lines(*) > 100"
  arch-unit ast --query "cyclomatic(*) >= 15" --format table
  arch-unit ast --query "params(*Service*) > 5" -v
  arch-unit ast --query "len(*) > 40" --format json
  arch-unit ast --query "imports(*) > 10"
  arch-unit ast --query "calls(*:*:*) > 5"

  # File path pattern examples
  arch-unit ast "@**/*.go:Service*"                          # All Service types in Go files
  arch-unit ast "@src/**/*.go:*Controller*" --format table   # Controllers in src directory
  arch-unit ast "@internal/**/*.go:api:UserService"          # UserService in api package, internal dir
  arch-unit ast "path(controllers/**/*.go) AND *Handler*"    # Handler types in controllers directory

  # File path + metric queries
  arch-unit ast --query "lines(@src/**/*.go:Service*) > 100"      # Large services in src
  arch-unit ast --query "cyclomatic(@**/*.go:*Handler*) >= 15"   # Complex handlers
  arch-unit ast --query "params(@controllers/**/*.go:*) > 5"     # Methods with many params

  # File filtering examples (legacy - use file path patterns instead)
  arch-unit ast --include "*.go" --exclude "*_test.go"        # Only Go files, no tests
  arch-unit ast --include "src/**/*.py" --include "lib/**/*.py" # Python files in specific dirs
  arch-unit ast --exclude "vendor/**" --exclude "node_modules/**" # Exclude vendor dirs
  arch-unit ast "Controller*" --include "internal/**/*.go"    # Controllers in internal packages only

  # Rebuild AST cache
  arch-unit ast --rebuild-cache

VERBOSE MODE:
  Use -v flag to see detailed pattern matching information including:
  - Parsed pattern components
  - Generated SQL queries
  - Full pkg:class:method:field patterns for each match`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAST,
}

func init() {
	rootCmd.AddCommand(astCmd)

	// PERSISTENT FLAGS - These are inherited by all subcommands
	astCmd.PersistentFlags().StringVar(&astFormat, "format", "tree", "Output format: pretty, json, csv, markdown, html, excel, tree, table, template")
	astCmd.PersistentFlags().StringVar(&astTemplate, "template", "", "Template string for custom output format (used with --format template)")
	astCmd.PersistentFlags().BoolVar(&astShowCalls, "calls", false, "Show call relationships")
	astCmd.PersistentFlags().BoolVar(&astShowLibraries, "libraries", false, "Show external library dependencies")
	astCmd.PersistentFlags().BoolVar(&astShowComplexity, "complexity", false, "Show complexity metrics")
	astCmd.PersistentFlags().BoolVar(&astShowFields, "fields", false, "Show field nodes in AST output")
	astCmd.PersistentFlags().IntVar(&astThreshold, "threshold", 0, "Complexity threshold filter")
	astCmd.PersistentFlags().IntVar(&astDepth, "depth", 1, "Relationship traversal depth")
	astCmd.PersistentFlags().StringVar(&astQuery, "query", "", "AQL query to execute")
	astCmd.PersistentFlags().BoolVar(&astAll, "all", false, "Search all cached nodes including virtual paths (SQL, OpenAPI, etc.)")

	// Display control flags - inherited by all subcommands
	astCmd.PersistentFlags().BoolVar(&astShowDirs, "dirs", true, "Show directory structure in tree")
	astCmd.PersistentFlags().BoolVar(&astShowFiles, "files", true, "Show individual files in tree")
	astCmd.PersistentFlags().BoolVar(&astShowPackages, "packages", true, "Show package nodes in tree")
	astCmd.PersistentFlags().BoolVar(&astShowTypes, "types", true, "Show type definitions in tree")
	astCmd.PersistentFlags().BoolVar(&astShowMethods, "methods", true, "Show methods in tree")
	astCmd.PersistentFlags().BoolVar(&astShowParams, "params", false, "Show method parameters in tree")
	astCmd.PersistentFlags().BoolVar(&astShowImports, "imports", false, "Show import statements in tree")
	astCmd.PersistentFlags().BoolVar(&astShowLineNo, "line-no", true, "Show line numbers in tree")
	astCmd.PersistentFlags().BoolVar(&astShowFileStats, "file-stats", false, "Show file-level statistics")

	// ROOT COMMAND SPECIFIC FLAGS
	astCmd.Flags().BoolVar(&astCachedOnly, "cached-only", false, "Show only cached results, don't analyze new files (deprecated - root command is now cache-only by default)")
	astCmd.Flags().BoolVar(&astRebuildCache, "rebuild-cache", false, "Rebuild the entire AST cache (deprecated - use 'ast analyze --no-cache' instead)")
}

// Note: getDisplayConfigFromFlags is now in ast_display.go

func runAST(cmd *cobra.Command, args []string) error {
	// The root ast command is now an alias for ast print (cache-only)
	// Handle deprecated flags with warnings
	if astRebuildCache {
		logger.Warnf("--rebuild-cache flag is deprecated on root ast command. Use 'ast analyze --no-cache' instead.")
		return fmt.Errorf("use 'arch-unit ast analyze --no-cache' to rebuild cache")
	}

	if astCachedOnly {
		logger.Warnf("--cached-only flag is deprecated - root ast command is now cache-only by default")
	}

	// Validate template flags
	if astFormat == "template" && astTemplate == "" {
		return fmt.Errorf("--template flag is required when using --format template")
	}
	if astTemplate != "" && astFormat != "template" {
		return fmt.Errorf("--template flag can only be used with --format template")
	}

	// Initialize AST cache
	astCache := cache.MustGetASTCache()

	// Get working directory
	workingDir, err := GetWorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Handle AQL query
	if astQuery != "" {
		return executeAQLQueryPrint(astCache, astQuery, workingDir)
	}

	// Handle pattern-based queries
	if len(args) > 0 {
		return queryASTPatternPrint(astCache, args[0], workingDir)
	}

	// Show overview
	return showASTOverviewPrint(astCache, workingDir)
}

// Note: All AST implementation functions have been moved to ast_display.go and the new subcommands.
// The root AST command now delegates to the print command functions.
