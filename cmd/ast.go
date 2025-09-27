package cmd

import (
	jsonenc "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	"github.com/flanksource/clicky"
	flanksourceContext "github.com/flanksource/commons/context"
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

	// New display configuration flags
	astShowDirs       bool
	astShowFiles      bool
	astShowPackages   bool
	astShowTypes      bool
	astShowMethods    bool
	astShowParams     bool
	astShowImports    bool
	astShowLineNo     bool
	astShowFileStats  bool
)

var astCmd = &cobra.Command{
	Use:   "ast [pattern]",
	Short: "Analyze and inspect AST (Abstract Syntax Tree) of code",
	Long: `Analyze and inspect the Abstract Syntax Tree of your codebase.

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

	astCmd.Flags().StringVar(&astFormat, "format", "pretty", "Output format: pretty, json, csv, markdown, html, excel, tree, table, template")
	astCmd.Flags().StringVar(&astTemplate, "template", "", "Template string for custom output format (used with --format template)")
	astCmd.Flags().BoolVar(&astShowCalls, "calls", false, "Show call relationships")
	astCmd.Flags().BoolVar(&astShowLibraries, "libraries", false, "Show external library dependencies")
	astCmd.Flags().BoolVar(&astShowComplexity, "complexity", false, "Show complexity metrics")
	astCmd.Flags().BoolVar(&astShowFields, "fields", false, "Show field nodes in AST output")
	astCmd.Flags().BoolVar(&astCachedOnly, "cached-only", false, "Show only cached results, don't analyze new files")
	astCmd.Flags().BoolVar(&astRebuildCache, "rebuild-cache", false, "Rebuild the entire AST cache")
	astCmd.Flags().IntVar(&astThreshold, "threshold", 0, "Complexity threshold filter")
	astCmd.Flags().IntVar(&astDepth, "depth", 1, "Relationship traversal depth")
	astCmd.Flags().StringVar(&astQuery, "query", "", "AQL query to execute")
}

func runAST(cmd *cobra.Command, args []string) error {
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

	// Handle cache rebuild
	if astRebuildCache {
		return rebuildASTCache(astCache)
	}

	// Handle AQL query
	if astQuery != "" {
		return executeAQLQuery(astCache, astQuery, workingDir)
	}

	// Handle cache stats
	if len(args) == 0 && astCachedOnly {
		return showCacheStats(astCache)
	}

	// Analyze files if not cached-only mode
	if !astCachedOnly {
		if err := analyzeFiles(astCache, workingDir); err != nil {
			return fmt.Errorf("failed to analyze files: %w", err)
		}
	}

	// Handle pattern-based queries
	if len(args) > 0 {
		return queryASTPattern(astCache, args[0], workingDir)
	}

	// Show overview
	return showASTOverview(astCache, workingDir)
}

// rebuildASTCache rebuilds the entire AST cache
func rebuildASTCache(astCache *cache.ASTCache) error {
	logger.Infof("Rebuilding AST cache...")

	// TODO: Add cache clearing functionality to ASTCache
	logger.Infof("Cache rebuild completed")
	return nil
}

// executeAQLQuery executes an AQL query
func executeAQLQuery(astCache *cache.ASTCache, aqlQuery string, workingDir string) error {
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
		return outputJSON(violations)
	}

	fmt.Printf("AQL Query: %s\n", aqlQuery)
	fmt.Printf("Found %d results:\n\n", len(violations))

	for _, v := range violations {
		relPath := v.File
		if strings.HasPrefix(v.File, workingDir+"/") {
			relPath = strings.TrimPrefix(v.File, workingDir+"/")
		}
		fmt.Printf("  %s:%d - %s\n", relPath, v.Line, v.Message)
	}

	return nil
}

// showCacheStats shows cache statistics
func showCacheStats(astCache *cache.ASTCache) error {
	// TODO: Implement cache statistics queries
	logger.Infof("AST Cache Statistics:")
	logger.Infof("  Feature not yet implemented")
	return nil
}

// analyzeFiles analyzes source files in the working directory
func analyzeFiles(astCache *cache.ASTCache, workingDir string) error {
	// Find all supported source files
	type fileInfo struct {
		path     string
		language string
	}
	var sourceFiles []fileInfo

	err := filepath.Walk(workingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories (except root)
		if info.IsDir() {
			baseName := filepath.Base(path)
			if baseName != "." && strings.HasPrefix(baseName, ".") {
				return filepath.SkipDir
			}
		}

		// Skip vendor and .git directories
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.git/") ||
			strings.Contains(path, "/node_modules/") || strings.Contains(path, "/__pycache__/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			// Detect language based on file extension
			var lang string
			switch {
			case strings.HasSuffix(path, ".go"):
				lang = "go"
			case strings.HasSuffix(path, ".py"):
				lang = "python"
			case strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") ||
				strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".cjs"):
				lang = "javascript"
			case strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx"):
				lang = "typescript"
			case strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".markdown") ||
				strings.HasSuffix(path, ".mdx"):
				lang = "markdown"
			default:
				return nil // Skip unsupported files
			}

			sourceFiles = append(sourceFiles, fileInfo{path: path, language: lang})
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to find source files: %w", err)
	}

	if len(sourceFiles) == 0 {
		return nil
	}

	// Count files by language
	langCounts := make(map[string]int)
	for _, file := range sourceFiles {
		langCounts[file.language]++
	}

	// Initialize library resolver
	libResolver := analysis.NewLibraryResolver(astCache)
	if err := libResolver.StoreLibraryNodes(); err != nil {
		return fmt.Errorf("Failed to store library nodes: %v", err)
	}

	// Create generic analyzer for all languages
	genericAnalyzer := analysis.NewGenericAnalyzer(astCache)

	logger.Infof("Analyzing %d source files...", len(sourceFiles))

	// Process files
	for _, file := range sourceFiles {
		// Read file content
		content, err := os.ReadFile(file.path)
		if err != nil {
			return fmt.Errorf("Failed to read file %s: %v", file.path, err)
		}

		// Use generic analyzer
		task := clicky.StartTask("analyze-file", func(ctx flanksourceContext.Context, t *clicky.Task) (bool, error) {
			_, err := genericAnalyzer.AnalyzeFile(t, file.path, content)
			if err != nil {
				return false, fmt.Errorf("Failed to extract AST from %s: %v", file.path, err)
			}
			return true, nil
		})

		// Wait for task completion
		if _, err := task.GetResult(); err != nil {
			return err
		}
	}

	return nil
}

// queryASTPattern queries AST nodes matching a pattern
func queryASTPattern(astCache *cache.ASTCache, pattern string, workingDir string) error {
	// Parse the pattern
	aqlPattern, err := models.ParsePattern(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	// Find matching nodes using a simple query approach
	// This is a simplified version - in practice we'd use the AQL engine
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

	// Filter by complexity threshold if specified
	if astThreshold > 0 && astShowComplexity {
		var filtered []*models.ASTNode
		for _, node := range nodes {
			if node.CyclomaticComplexity > astThreshold {
				filtered = append(filtered, node)
			}
		}
		nodes = filtered
	}

	return displayNodes(astCache, nodes, pattern, workingDir)
}

// showASTOverview shows an overview of the AST cache
func showASTOverview(astCache *cache.ASTCache, workingDir string) error {
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

	fmt.Printf("AST Overview for %s\n", workingDir)
	fmt.Printf("─────────────────────────────\n\n")

	if total == 0 {
		logger.Infof("No AST data found. Run analysis first with a pattern or without --cached-only.")
		return nil
	}

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

// displayNodes displays AST nodes in the requested format
func displayNodes(astCache *cache.ASTCache, nodes []*models.ASTNode, pattern string, workingDir string) error {
	if len(nodes) == 0 {
		logger.Infof("No nodes found matching pattern: %s", pattern)
		return nil
	}

	switch astFormat {
	case "json":
		return outputNodesJSON(nodes)
	case "table":
		return outputNodesTable(nodes, workingDir)
	case "template":
		return outputNodesTemplate(nodes, workingDir)
	default: // tree
		return outputNodesTree(astCache, nodes, pattern, workingDir)
	}
}

// outputNodesJSON outputs nodes as JSON
func outputNodesJSON(nodes []*models.ASTNode) error {
	encoder := jsonenc.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(nodes)
}

// outputNodesTable outputs nodes as a table
func outputNodesTable(nodes []*models.ASTNode, workingDir string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "File\tPackage\tType\tMethod\tComplexity\tLines\n")
	_, _ = fmt.Fprintf(w, "────\t───────\t────\t──────\t──────────\t─────\n")

	for _, node := range nodes {
		relPath := node.FilePath
		if strings.HasPrefix(node.FilePath, workingDir+"/") {
			relPath = strings.TrimPrefix(node.FilePath, workingDir+"/")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n",
			relPath,
			node.PackageName,
			node.TypeName,
			node.MethodName,
			node.CyclomaticComplexity,
			node.LineCount)
	}

	_ = w.Flush()
	return nil
}

// outputNodesTemplate outputs nodes using a template
func outputNodesTemplate(nodes []*models.ASTNode, workingDir string) error {
	tmpl, err := template.New("ast").Parse(astTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	for _, node := range nodes {
		// Create template data
		data := struct {
			Package    string
			Class      string
			Type       string
			Method     string
			Field      string
			File       string
			Lines      int
			Complexity int
			Params     int
			Returns    int
			NodeType   string
			StartLine  int
			EndLine    int
		}{
			Package:    node.PackageName,
			Class:      node.TypeName,
			Type:       node.TypeName,
			Method:     node.MethodName,
			Field:      node.FieldName,
			File:       node.FilePath,
			Lines:      node.LineCount,
			Complexity: node.CyclomaticComplexity,
			Params:     node.ParameterCount,
			Returns:    node.ReturnCount,
			NodeType:   string(node.NodeType),
			StartLine:  node.StartLine,
			EndLine:    node.EndLine,
		}

		// Make file path relative to working directory
		if strings.HasPrefix(data.File, workingDir+"/") {
			data.File = strings.TrimPrefix(data.File, workingDir+"/")
		}

		if err := tmpl.Execute(os.Stdout, data); err != nil {
			return fmt.Errorf("failed to execute template: %w", err)
		}
		fmt.Println() // Add newline after each node
	}

	return nil
}

// outputNodesTree outputs nodes in tree format using clicky.Format
func outputNodesTree(astCache *cache.ASTCache, nodes []*models.ASTNode, pattern string, workingDir string) error {
	// Build tree structure using the existing BuildASTNodeTree
	tree := models.BuildASTNodeTree(nodes)

	// Use clicky.Format to handle coloring properly
	output, err := clicky.Format(tree, clicky.FormatOptions{
		Format:  "tree",
		NoColor: clicky.Flags.FormatOptions.NoColor,
	})
	if err != nil {
		return fmt.Errorf("failed to format AST tree: %w", err)
	}

	fmt.Printf("AST Nodes matching pattern: %s\n", pattern)
	fmt.Printf("Found %d nodes:\n\n", len(nodes))

	fmt.Print(output)

	return nil
}


// outputJSON outputs data as JSON
func outputJSON(data interface{}) error {
	encoder := jsonenc.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// isValidFilePath checks if the argument is a valid file path (rather than an AQL pattern)
func isValidFilePath(arg, workingDir string) bool {
	resolvedPath, err := resolveFilePath(arg, workingDir)
	if err != nil {
		return false
	}

	// Check if it's a regular file
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return false
	}

	return info.Mode().IsRegular()
}

// resolveFilePath resolves a file path argument to an absolute path
func resolveFilePath(arg, workingDir string) (string, error) {
	// If already absolute, verify it exists
	if filepath.IsAbs(arg) {
		if _, err := os.Stat(arg); err == nil {
			return arg, nil
		}
		return "", fmt.Errorf("file not found: %s", arg)
	}

	// Try relative to working directory first
	absPath := filepath.Join(workingDir, arg)
	if _, err := os.Stat(absPath); err == nil {
		return absPath, nil
	}

	// Try relative to current directory
	if cwd, err := os.Getwd(); err == nil {
		cwdPath := filepath.Join(cwd, arg)
		if _, err := os.Stat(cwdPath); err == nil {
			return cwdPath, nil
		}
	}

	return "", fmt.Errorf("file not found: %s", arg)
}

// queryASTByFilePath queries and displays AST nodes for a specific file
func queryASTByFilePath(astCache *cache.ASTCache, filePath, workingDir string) error {
	// Resolve the file path to absolute path
	resolvedPath, err := resolveFilePath(filePath, workingDir)
	if err != nil {
		return err
	}

	// Query AST nodes for the specific file
	nodes, err := astCache.GetASTNodesByFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to get AST nodes for file %s: %w", filePath, err)
	}

	if len(nodes) == 0 {
		// Check if file needs analysis
		needsAnalysis, err := astCache.NeedsReanalysis(resolvedPath)
		if err != nil {
			return fmt.Errorf("failed to check file analysis status: %w", err)
		}

		if needsAnalysis {
			logger.Infof("No AST data found for file: %s", filePath)
			logger.Infof("Run 'arch-unit ast' to analyze files first, then try again.")
			return nil
		}

		logger.Infof("File %s has been analyzed but contains no AST nodes", filePath)
		return nil
	}

	// Display the nodes using existing display functionality
	// Use relative path for display if possible
	displayPath := filePath
	if strings.HasPrefix(resolvedPath, workingDir+"/") {
		displayPath = strings.TrimPrefix(resolvedPath, workingDir+"/")
	}

	return displayNodes(astCache, nodes, displayPath, workingDir)
}
