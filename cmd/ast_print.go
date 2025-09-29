package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var astPrintCmd = &cobra.Command{
	Use:   "print [pattern]",
	Short: "Print cached AST nodes matching a pattern",
	Long: `Print Abstract Syntax Tree nodes from cache without performing analysis.

This command displays previously analyzed AST data from the cache. It will not
trigger file analysis - use 'ast analyze' first to build the cache.

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
  # Show all cached Controller types
  arch-unit ast print "Controller*"

  # Show specific method with JSON format
  arch-unit ast print "controllers:UserController:GetUser" --format json

  # Show all fields in User models as table
  arch-unit ast print "models:User:*:*" --format table

  # Custom template output format
  arch-unit ast print "*Service*" --format template --template "{{.Package}}.{{.Type}}.{{.Method}} ({{.Lines}} SLOC)"

  # File path pattern examples
  arch-unit ast print "@**/*.go:Service*"                          # All Service types in Go files
  arch-unit ast print "@src/**/*.go:*Controller*" --format table   # Controllers in src directory
  arch-unit ast print "@internal/**/*.go:api:UserService"          # UserService in api package, internal dir
  arch-unit ast print "path(controllers/**/*.go) AND *Handler*"    # Handler types in controllers directory

  # Tree output with custom filtering
  arch-unit ast print "*" --methods --no-fields --format tree

VERBOSE MODE:
  Use -v flag to see detailed pattern matching information including:
  - Parsed pattern components
  - Generated SQL queries
  - Full pkg:class:method:field patterns for each match`,
	Args: cobra.MaximumNArgs(1),
	RunE: runASTPrint,
}

func init() {
	astCmd.AddCommand(astPrintCmd)
}

func runASTPrint(cmd *cobra.Command, args []string) error {
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

// executeAQLQueryPrint executes an AQL query for print command
func executeAQLQueryPrint(astCache *cache.ASTCache, aqlQuery string, workingDir string) error {
	// Wrap the query in a temporary rule
	ruleText := fmt.Sprintf(`RULE "temp" { LIMIT(%s) }`, aqlQuery)

	// Parse the AQL - support both YAML and legacy formats
	var ruleSet *models.AQLRuleSet
	var err error
	if parser.IsLegacyAQLFormat(ruleText) {
		// Legacy AQL format
		ruleSet, err = parser.ParseAQL(ruleText)
	} else {
		// YAML format
		ruleSet, err = parser.LoadAQLFromYAML(ruleText)
	}

	if err != nil {
		return fmt.Errorf("failed to parse AQL query: %w", err)
	}

	// Execute the query
	engine := query.NewAQLEngine(astCache)
	allViolations, err := engine.ExecuteRuleSet(ruleSet)
	if err != nil {
		return fmt.Errorf("failed to execute AQL query: %w", err)
	}

	// Filter violations to only show files within working directory
	var violations []*models.Violation
	for _, v := range allViolations {
		if strings.HasPrefix(v.File, workingDir+"/") {
			violations = append(violations, v)
		}
	}

	// Display results
	if astFormat == "json" {
		return OutputJSON(violations)
	}

	fmt.Printf("AQL Query: %s\n", aqlQuery)
	fmt.Printf("Found %d results:\n\n", len(violations))

	for _, v := range violations {
		relPath := MakeRelativePath(v.File, workingDir)
		fmt.Printf("  %s:%d - %s\n", relPath, v.Line, v.Message)
	}

	return nil
}

// queryASTPatternPrint queries AST nodes matching a pattern for print command
func queryASTPatternPrint(astCache *cache.ASTCache, pattern string, workingDir string) error {
	// Parse the pattern
	aqlPattern, err := models.ParsePattern(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	logger.V(4).Infof("Parsed pattern '%s': Language=%s, Package=%s, Type=%s, Method=%s, FilePath=%s",
		pattern, aqlPattern.Language, aqlPattern.Package, aqlPattern.Type, aqlPattern.Method, aqlPattern.FilePath)

	// Find matching nodes using a simple query approach
	query := "SELECT id, file_path, package_name, type_name, method_name, field_name, node_type, start_line, end_line, cyclomatic_complexity, parameter_count, return_count, line_count, summary, language, field_type, default_value, parent_id FROM ast_nodes"
	var args []interface{}

	// Apply file path filter unless --all flag is used
	if !astAll {
		query += " WHERE file_path LIKE ?"
		workingDirPattern := workingDir + "/%"
		args = append(args, workingDirPattern)
	}

	// Helper to add WHERE or AND based on whether we already have conditions
	addCondition := func(condition string, value interface{}) {
		if strings.Contains(query, "WHERE") {
			query += " AND " + condition
		} else {
			query += " WHERE " + condition
		}
		args = append(args, value)
	}

	// Add language filter if specified
	if aqlPattern.Language != "" {
		logger.Debugf("Adding language filter: language = '%s'", aqlPattern.Language)
		addCondition("language = ?", aqlPattern.Language)
	}

	if aqlPattern.Package != "" && aqlPattern.Package != "*" {
		if strings.Contains(aqlPattern.Package, "*") {
			addCondition("package_name LIKE ?", strings.ReplaceAll(aqlPattern.Package, "*", "%"))
		} else {
			addCondition("package_name = ?", aqlPattern.Package)
		}
	}

	if aqlPattern.Type != "" && aqlPattern.Type != "*" {
		if strings.Contains(aqlPattern.Type, "*") {
			addCondition("type_name LIKE ?", strings.ReplaceAll(aqlPattern.Type, "*", "%"))
		} else {
			addCondition("type_name = ?", aqlPattern.Type)
		}
	}

	if aqlPattern.Method != "" && aqlPattern.Method != "*" {
		if strings.Contains(aqlPattern.Method, "*") {
			addCondition("method_name LIKE ?", strings.ReplaceAll(aqlPattern.Method, "*", "%"))
		} else {
			addCondition("method_name = ?", aqlPattern.Method)
		}
	}

	logger.V(4).Infof("Executing AST query: %s", query)
	logger.V(4).Infof("Query arguments: %+v", args)

	rows, err := astCache.QueryRaw(query, args...)
	if err != nil {
		return fmt.Errorf("failed to query AST nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nodes []*models.ASTNode
	for rows.Next() {
		var node models.ASTNode
		var parentID *int64
		err := rows.Scan(&node.ID, &node.FilePath, &node.PackageName, &node.TypeName,
			&node.MethodName, &node.FieldName, &node.NodeType, &node.StartLine,
			&node.EndLine, &node.CyclomaticComplexity, &node.ParameterCount,
			&node.ReturnCount, &node.LineCount, &node.Summary, &node.Language,
			&node.FieldType, &node.DefaultValue, &parentID)
		if err != nil {
			return fmt.Errorf("failed to scan node: %w", err)
		}
		node.ParentID = parentID
		nodes = append(nodes, &node)
	}

	logger.Debugf("Query returned %d AST nodes for pattern '%s'", len(nodes), pattern)
	if len(nodes) > 0 {
		logger.V(4).Infof("Sample node types: %s", getSampleNodeTypes(nodes))
	}

	// Build parent-child relationships for proper tree hierarchy
	models.PopulateNodeHierarchy(nodes)

	// Get display options from flags
	opts := GetDisplayOptionsFromFlags()

	return DisplayNodes(astCache, nodes, pattern, workingDir, opts)
}

// showASTOverviewPrint shows an overview of the AST cache for print command
func showASTOverviewPrint(astCache *cache.ASTCache, workingDir string) error {
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
			"working_dir": workingDir,
			"statistics":  stats,
			"total":       total,
		})
	}

	fmt.Printf("AST Cache Overview for %s\n", workingDir)
	fmt.Printf("─────────────────────────────\n\n")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "Node Type\tCount\n")
	_, _ = fmt.Fprintf(w, "─────────\t─────\n")

	for nodeType, count := range stats {
		_, _ = fmt.Fprintf(w, "%s\t%d\n", nodeType, count)
	}

	_, _ = fmt.Fprintf(w, "─────────\t─────\n")
	_, _ = fmt.Fprintf(w, "Total\t%d\n", total)
	_ = w.Flush()

	return nil
}

// getSampleNodeTypes returns a sample of node types for debugging
func getSampleNodeTypes(nodes []*models.ASTNode) string {
	typeMap := make(map[string]int)
	for _, node := range nodes {
		typeMap[node.NodeType]++
		if len(typeMap) >= 5 { // Limit to 5 types for brevity
			break
		}
	}

	var types []string
	for nodeType, count := range typeMap {
		types = append(types, fmt.Sprintf("%s(%d)", nodeType, count))
	}
	return strings.Join(types, ", ")
}
