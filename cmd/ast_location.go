package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var astLocationCmd = &cobra.Command{
	Use:   "location [pattern]",
	Short: "Show file locations and line numbers for AST nodes",
	Long: `Show file locations and line numbers for AST nodes matching a pattern.

This command displays location information for previously analyzed AST data from
the cache. It focuses on where nodes are defined in the codebase, showing file
paths and line numbers in a concise format.

PATTERN SYNTAX:
  AST Patterns use the format: package:type:method:field
  File Path Patterns use doublestar glob syntax with @ prefix or path() function
  Combined Patterns: @file_pattern:ast_pattern or path(file_pattern) AND ast_pattern
  - Use wildcards (*) for any component
  - Omit components to match any value
  - Components are matched hierarchically

PATTERN EXAMPLES:
  Basic AST Patterns:
    "*"                    # All cached nodes
    "Controller*"          # Types starting with "Controller"
    "*Service"             # Types ending with "Service"
    "models.*"             # All nodes in "models" package

  File Path Patterns:
    "@**/*.go"             # All Go files in any directory
    "@src/**/*.go"         # Go files in src directory and subdirectories
    "@controllers/*.go"    # Go files directly in controllers directory

  Combined File Path + AST Patterns:
    "@**/*.go:Service*"                    # Service types in any Go file
    "@controllers/**/*.go:*Controller*"    # Controller types in controllers directory
    "@src/**/*.go:api:UserService"         # UserService in api package, src directory

  Type and Method Patterns:
    "controllers:User*"              # Types starting with "User" in controllers
    "*:Controller"                   # All Controller types in any package
    "controllers:UserController:Get*" # Methods starting with "Get"
    "*:*:CreateUser"                 # CreateUser methods in any type

  Field Patterns:
    "models:User:Name"               # Specific field
    "models:User:*"                  # All fields in User type
    "*:*:*:ID"                      # All ID fields

OUTPUT FORMATS:
  Table (default): Shows location, type, name, and line range in tabular format
  JSON: Structured output with detailed location information including:
    - file: Relative file path
    - package, type, method, field: Node identifiers (when applicable)
    - node_type: Type of AST node (package, type, method, field, variable)
    - start_line, end_line: Line number range

COMMAND EXAMPLES:
  # Show locations of all Controller types
  arch-unit ast location "Controller*"

  # Show method locations with line numbers
  arch-unit ast location "*:*:Get*"

  # Show locations in specific directory
  arch-unit ast location "@controllers/**/*.go:*"

  # JSON output for programmatic use
  arch-unit ast location "models:User" --format json

  # Show all service method locations
  arch-unit ast location "*Service*:*"

  # Show field locations in models
  arch-unit ast location "models:*:*:*"

  # Show locations with filtering (inherited flags)
  arch-unit ast location "*" --types --no-methods

FILTERING:
  All display flags from the parent ast command are inherited:
  --types, --methods, --fields, --params, --dirs, --files, etc.

VERBOSE MODE:
  Use -v flag to see detailed pattern matching information.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runASTLocation,
}

func init() {
	astCmd.AddCommand(astLocationCmd)
}

func runASTLocation(cmd *cobra.Command, args []string) error {
	// Initialize AST cache
	astCache := cache.MustGetASTCache()

	// Get working directory
	workingDir, err := GetWorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Handle pattern-based queries
	if len(args) > 0 {
		return queryASTLocationPattern(astCache, args[0], workingDir)
	}

	// Show overview
	return showASTLocationOverview(astCache, workingDir)
}

// queryASTLocationPattern queries AST node locations matching a pattern
func queryASTLocationPattern(astCache *cache.ASTCache, pattern string, workingDir string) error {
	// Parse the pattern
	aqlPattern, err := models.ParsePattern(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	// Find matching nodes using a simple query approach
	query := "SELECT id, file_path, package_name, type_name, method_name, field_name, node_type, start_line, end_line, cyclomatic_complexity, parameter_count, return_count, line_count FROM ast_nodes WHERE file_path LIKE ?"
	workingDirPattern := workingDir + "/%"
	args := []interface{}{workingDirPattern}

	if aqlPattern.Package != "" && aqlPattern.Package != "*" {
		if strings.Contains(aqlPattern.Package, "*") {
			query += " AND package_name LIKE ?"
			args = append(args, strings.ReplaceAll(aqlPattern.Package, "*", "%"))
		} else {
			query += " AND package_name = ?"
			args = append(args, aqlPattern.Package)
		}
	}

	if aqlPattern.Type != "" && aqlPattern.Type != "*" {
		if strings.Contains(aqlPattern.Type, "*") {
			query += " AND type_name LIKE ?"
			args = append(args, strings.ReplaceAll(aqlPattern.Type, "*", "%"))
		} else {
			query += " AND type_name = ?"
			args = append(args, aqlPattern.Type)
		}
	}

	if aqlPattern.Method != "" && aqlPattern.Method != "*" {
		if strings.Contains(aqlPattern.Method, "*") {
			query += " AND method_name LIKE ?"
			args = append(args, strings.ReplaceAll(aqlPattern.Method, "*", "%"))
		} else {
			query += " AND method_name = ?"
			args = append(args, aqlPattern.Method)
		}
	}

	if aqlPattern.Field != "" && aqlPattern.Field != "*" {
		if strings.Contains(aqlPattern.Field, "*") {
			query += " AND field_name LIKE ?"
			args = append(args, strings.ReplaceAll(aqlPattern.Field, "*", "%"))
		} else {
			query += " AND field_name = ?"
			args = append(args, aqlPattern.Field)
		}
	}

	rows, err := astCache.QueryRaw(query, args...)
	if err != nil {
		return fmt.Errorf("failed to query AST nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nodes []*models.ASTNode
	for rows.Next() {
		var node models.ASTNode
		err := rows.Scan(&node.ID, &node.FilePath, &node.PackageName, &node.TypeName,
			&node.MethodName, &node.FieldName, &node.NodeType, &node.StartLine,
			&node.EndLine, &node.CyclomaticComplexity, &node.ParameterCount,
			&node.ReturnCount, &node.LineCount)
		if err != nil {
			return fmt.Errorf("failed to scan node: %w", err)
		}
		nodes = append(nodes, &node)
	}

	// Apply display config filtering (respect inherited flags)
	filteredNodes := applyDisplayConfigFiltering(nodes)

	// Display location information
	fmt.Printf("Locations for pattern: %s\n", pattern)
	fmt.Printf("Found %d locations:\n\n", len(filteredNodes))

	return OutputLocationNodes(filteredNodes, workingDir, astFormat)
}

// showASTLocationOverview shows an overview of locations in the AST cache
func showASTLocationOverview(astCache *cache.ASTCache, workingDir string) error {
	// Get basic statistics filtered by working directory
	query := "SELECT node_type, COUNT(*) FROM ast_nodes WHERE file_path LIKE ? GROUP BY node_type"
	workingDirPattern := workingDir + "/%"
	rows, err := astCache.QueryRaw(query, workingDirPattern)
	if err != nil {
		return fmt.Errorf("failed to get AST statistics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats := make(map[string]int)
	total := 0

	for rows.Next() {
		var nodeType string
		var count int
		if err := rows.Scan(&nodeType, &count); err != nil {
			return err
		}
		stats[nodeType] = count
		total += count
	}

	if total == 0 {
		logger.Infof("No AST data found in cache for %s", workingDir)
		logger.Infof("Run 'arch-unit ast analyze' first to build the cache.")
		return nil
	}

	if astFormat == "json" {
		return OutputJSON(map[string]interface{}{
			"working_dir":         workingDir,
			"location_statistics": stats,
			"total_locations":     total,
		})
	}

	fmt.Printf("AST Location Overview for %s\n", workingDir)
	fmt.Printf("──────────────────────────────\n\n")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "Node Type\tCount\n")
	_, _ = fmt.Fprintf(w, "─────────\t─────\n")

	for nodeType, count := range stats {
		_, _ = fmt.Fprintf(w, "%s\t%d\n", nodeType, count)
	}

	_, _ = fmt.Fprintf(w, "─────────\t─────\n")
	_, _ = fmt.Fprintf(w, "Total\t%d\n", total)
	_ = w.Flush()

	fmt.Printf("\nUse 'arch-unit ast location \"*\"' to see all locations.\n")

	return nil
}

// applyDisplayConfigFiltering filters nodes based on display configuration flags
func applyDisplayConfigFiltering(nodes []*models.ASTNode) []*models.ASTNode {
	config := GetDisplayConfigFromFlags()

	var filtered []*models.ASTNode
	for _, node := range nodes {
		switch node.NodeType {
		case models.NodeTypePackage:
			if config.ShowPackages {
				filtered = append(filtered, node)
			}
		case models.NodeTypeType:
			if config.ShowTypes {
				filtered = append(filtered, node)
			}
		case models.NodeTypeMethod:
			if config.ShowMethods {
				filtered = append(filtered, node)
			}
		case models.NodeTypeField:
			if config.ShowFields {
				filtered = append(filtered, node)
			}
		case models.NodeTypeVariable:
			// Variables are shown by default unless specifically filtered
			filtered = append(filtered, node)
		default:
			// Include unknown node types by default
			filtered = append(filtered, node)
		}
	}

	return filtered
}
