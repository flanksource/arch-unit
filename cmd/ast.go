package cmd

import (
	jsonenc "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	astFormat        string
	astShowCalls     bool
	astShowLibraries bool
	astShowComplexity bool
	astCachedOnly    bool
	astRebuildCache  bool
	astThreshold     int
	astDepth         int
	astQuery         string
)

var astCmd = &cobra.Command{
	Use:   "ast [pattern]",
	Short: "Analyze and inspect AST (Abstract Syntax Tree) of code",
	Long: `Analyze and inspect the Abstract Syntax Tree of your codebase.

The ast command provides detailed information about code structure, relationships,
complexity metrics, and dependencies using powerful pattern matching.

PATTERN SYNTAX:
  Patterns use the format: package:type:method:field
  - Use wildcards (*) for any component
  - Omit components to match any value
  - Components are matched hierarchically

PATTERN EXAMPLES:
  Basic Patterns:
    "*"                    # All nodes
    "Controller*"          # Types starting with "Controller"
    "*Service"             # Types ending with "Service"
    "models.*"             # All nodes in "models" package

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
  
  # File filtering examples
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

var astFixturesCmd = &cobra.Command{
	Use:   "fixtures [fixture-files...]",
	Short: "Run fixture-based tests from markdown tables",
	Long: `Run fixture-based tests defined in markdown table format.

This command provides a declarative testing framework for AST queries and CLI commands
using markdown tables with CEL (Common Expression Language) validation. Tests are 
organized hierarchically by file and section, making it easy to maintain large test suites.

FIXTURE FILE FORMAT:
  Markdown files containing tables with test cases. Files can include optional
  YAML front-matter for configuration:

  ---
  build: "go build -o myapp"      # Build command to run before tests
  exec: "./myapp"                  # Base executable for exec tests
  base_exec: "arch-unit ast"       # Default command prefix
  env:                             # Environment variables
    DEBUG: "true"
    LOG_LEVEL: "debug"
  ---

TEST TYPES:

  1. Query Tests (AST pattern and AQL queries):
     Test the AST query engine directly without invoking the CLI.

     Table Columns:
     - Test Name: Descriptive name for the test (required)
     - CWD: Working directory relative to fixture file (optional, default: ".")
     - Query: AST pattern or AQL query to execute (required)
     - Expected Count: Expected number of results (optional)
     - CEL Validation: CEL expression to validate results (required)

     Example:
     | Test Name | CWD | Query | Expected Count | CEL Validation |
     |-----------|-----|-------|----------------|----------------|
     | Find Controllers | examples/go | *Controller* | 2 | nodes.all(n, n.type_name.endsWith("Controller")) |
     | Complex Methods | . | cyclomatic(*) > 10 | - | nodes.exists(n, n.cyclomatic_complexity > 15) |

  2. Exec Tests (CLI command execution):
     Test the full CLI interface including output formatting.

     Table Columns:
     - Test Name: Descriptive name for the test (required)
     - CWD: Working directory relative to fixture file (optional)
     - CLI Args: Command line arguments (required if no Exec)
     - Exec: Full command to execute (overrides base_exec + CLI Args)
     - Exit Code: Expected exit code (optional, default: 0)
     - Expected Output: Text that should appear in stdout (optional)
     - CEL Validation: CEL expression for validation (optional)

     Exit Code Validation:
     - The Exit Code column specifies the expected exit code (default: 0)
     - If the actual exit code doesn't match, the test fails immediately
     - No other validations (CEL, output) are performed on exit code mismatch
     - Error message includes the exit code and cleaned stderr/stdout

     Example:
     | Test Name | CWD | CLI Args | Exit Code | Expected Output | CEL Validation |
     |-----------|-----|----------|-----------|-----------------|----------------|
     | JSON Output | examples | * --format json | 0 | - | stdout.contains("node_type") |
     | Invalid Flag | . | --invalid | 1 | - | stderr.contains("unknown flag") |
     | List Services | . | *Service --format table | 0 | UserService | true |

CEL VALIDATION REFERENCE:

  Available Variables:
    Query Tests:
      - nodes: List of AST nodes returned by the query
      - Node properties: type_name, method_name, package_name, node_type,
        file_path, line_count, cyclomatic_complexity, parameter_count,
        return_count, start_line, end_line

    Exec Tests:
      - stdout: Command standard output as string
      - stderr: Command standard error as string
      - exitCode: Command exit code as integer
      - duration: Execution time in milliseconds

  Built-in Functions:
    String Functions:
      - str.contains(substring): Check if string contains substring
      - str.startsWith(prefix): Check if string starts with prefix
      - str.endsWith(suffix): Check if string ends with suffix
      - str.matches(regex): Check if string matches regex pattern

    List Functions:
      - list.all(item, predicate): All items match predicate
      - list.exists(item, predicate): At least one item matches
      - list.filter(item, predicate): Filter items by predicate
      - list.map(item, expression): Transform items
      - list.unique(): Get unique values
      - size(list): Get list size

  CEL Expression Examples:
    # Validate all nodes are methods with low complexity
    nodes.all(n, n.node_type == "method" && n.cyclomatic_complexity < 10)

    # Check for specific method in results
    nodes.exists(n, n.method_name == "GetUser" && n.package_name == "service")

    # Validate JSON output structure
    stdout.contains('"node_type"') && stdout.contains('"file_path"')

    # Check command succeeded with expected output
    exitCode == 0 && stdout.contains("PASS") && !stderr.contains("ERROR")

    # Complex validation with multiple conditions
    nodes.filter(n, n.node_type == "method").size() > 5 && 
    nodes.filter(n, n.cyclomatic_complexity > 10).size() == 0

    # Validate execution time
    duration < 1000 && exitCode == 0

EXAMPLES:
  # Run all fixture files in a directory
  arch-unit ast fixtures tests/fixtures/*.md
  
  # Run specific fixture file with verbose output
  arch-unit ast fixtures tests/fixtures/ast_queries.md --verbose
  
  # Run tests in parallel with 4 workers
  arch-unit ast fixtures tests/fixtures/*.md --max-concurrent 4
  
  # Filter tests by name pattern (supports commons/MatchItems syntax)
  arch-unit ast fixtures tests/fixtures/*.md --filter "*Service*"
  
  # Multiple patterns (comma-separated)
  arch-unit ast fixtures tests/*.md --filter "*Controller*,*Service*"
  
  # Exclude patterns with negation (!)
  arch-unit ast fixtures tests/*.md --filter "*Test*,!*Skip*"
  
  # Output results as JSON for CI/CD integration
  arch-unit ast fixtures tests/fixtures/*.md --format json
  
  # Run with custom working directory
  arch-unit ast fixtures tests/*.md --cwd /path/to/project

OUTPUT FORMATS:
  - tree: Hierarchical tree view showing file/section/test structure (default)
  - table: Tabular display of test results with pass/fail status
  - json: JSON output for programmatic processing and CI/CD integration
  - yaml: YAML format for configuration management tools
  - csv: CSV format for spreadsheet analysis

EXECUTION OPTIONS:
  --filter: Filter tests by name using commons/MatchItems syntax
            - Simple wildcards: "*Controller*" matches any test containing "Controller"
            - Multiple patterns: "Pattern1,Pattern2" (comma-separated)
            - Negation: "!SkipThis" excludes tests matching the pattern
            - Combined: "*Test*,!*Old*" includes all tests except those with "Old"
  --max-concurrent: Number of parallel workers (default: 1, use 0 for CPU count)
  --verbose: Show detailed execution information and debug output
  --no-color: Disable colored output for CI/CD environments
  --format: Output format (tree, table, json, yaml, csv)

TROUBLESHOOTING:
  Common Issues:
  - "No fixtures found": Check file paths and glob patterns
  - "CEL evaluation failed": Verify CEL syntax and available variables
  - "Command not found": Ensure executables are in PATH or use absolute paths
  - "Working directory not found": Verify CWD exists relative to fixture file

  Debugging Tips:
  - Use --verbose to see detailed query execution and CEL evaluation
  - Start with simple CEL expressions and gradually add complexity
  - Test patterns with 'arch-unit ast' command directly first
  - Check that test data exists in specified working directories
  - Use --format json to inspect the full structure of results

FILE ORGANIZATION:
  Organize fixtures by feature or component:
    tests/fixtures/
    ├── ast_patterns.md     # AST pattern matching tests
    ├── aql_queries.md      # AQL query language tests  
    ├── cli_output.md       # CLI output format tests
    ├── performance.md      # Performance regression tests
    └── integration/        # Integration test fixtures
        ├── go_analysis.md
        └── python_analysis.md`,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runASTFixtures,
	SilenceUsage: true, // Don't print usage on fixture failures
}

func init() {
	rootCmd.AddCommand(astCmd)

	astCmd.Flags().StringVar(&astFormat, "format", "pretty", "Output format: pretty, json, csv, markdown, html, excel, tree, table, template")
	astCmd.Flags().StringVar(&astTemplate, "template", "", "Template string for custom output format (used with --format template)")
	astCmd.Flags().BoolVar(&astShowCalls, "calls", false, "Show call relationships")
	astCmd.Flags().BoolVar(&astShowLibraries, "libraries", false, "Show external library dependencies")
	astCmd.Flags().BoolVar(&astShowComplexity, "complexity", false, "Show complexity metrics")
	astCmd.Flags().BoolVar(&astCachedOnly, "cached-only", false, "Show only cached results, don't analyze new files")
	astCmd.Flags().BoolVar(&astRebuildCache, "rebuild-cache", false, "Rebuild the entire AST cache")
	astCmd.Flags().IntVar(&astThreshold, "threshold", 0, "Complexity threshold filter")
	astCmd.Flags().IntVar(&astDepth, "depth", 1, "Relationship traversal depth")
	astCmd.Flags().StringVar(&astQuery, "query", "", "AQL query to execute")
}

func runAST(cmd *cobra.Command, args []string) error {
	// Initialize AST cache
	astCache, err := cache.NewASTCache()
	if err != nil {
		return fmt.Errorf("failed to create AST cache: %w", err)
	}
	defer astCache.Close()

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

// formatASTQueryResults formats and outputs AST query results with clicky support
func formatASTQueryResults(nodes []*models.ASTNode, pattern string) error {
	// Get working directory for relative paths
	workingDir, _ := GetWorkingDir()
	
	// Create the query result with relative paths
	result := ast.CreateQueryResult(nodes, pattern, workingDir)
	
	// Create format manager
	formatManager := formatters.NewFormatManager()
	
	// Get global format options
	globalFormatOpts := GetFormatOptions()
	
	// Handle template format specially
	if globalFormatOpts.Format == "template" && astTemplate != "" {
		// For template format, we need custom handling
		// For now, just use JSON format as fallback
		globalFormatOpts.Format = "json"
	}
	
	// Handle table and tree formats
	if globalFormatOpts.Format == "table" || globalFormatOpts.Format == "tree" {
		// These formats are supported by pretty formatter
		globalFormatOpts.Format = "pretty"
	}
	
	// Format and output
	return formatManager.FormatToFile(*globalFormatOpts, result)
}



// formatASTQueryResults formats and outputs AST query results with clicky support
func formatASTQueryResults(nodes []*models.ASTNode, pattern string) error {
	// Determine output format - use global flags or astFormat
	format := getASTQueryOutputFormat()
	
	// Handle different output formats
	switch format {
	case "json":
		// For JSON, use simple structure
		result := ast.CreateQueryResult(nodes, pattern)
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		
		if outputFile != "" {
			return os.WriteFile(outputFile, data, 0644)
		}
		fmt.Println(string(data))
		return nil
		
	case "csv":
		// CSV format
		fmt.Println("File,Package,Type,Method,Field,NodeType,Line,Complexity,Parameters")
		for _, node := range nodes {
			fileName := node.FilePath
			if idx := strings.LastIndex(fileName, "/"); idx >= 0 {
				fileName = fileName[idx+1:]
			}
			
			var params []string
			for _, p := range node.Parameters {
				params = append(params, fmt.Sprintf("%s %s", p.Name, p.Type))
			}
			
			fmt.Printf("%s,%s,%s,%s,%s,%s,%d,%d,\"%s\"\n",
				fileName, node.PackageName, node.TypeName, node.MethodName, 
				node.FieldName, node.NodeType, node.StartLine, 
				node.CyclomaticComplexity, strings.Join(params, ", "))
		}
		return nil
		
	case "markdown":
		// Use clicky's markdown formatter
		markdownFormatter := formatters.NewMarkdownFormatter()
		output, err := ast.FormatQueryWithClicky(nodes, pattern, markdownFormatter)
		if err != nil {
			return fmt.Errorf("failed to format as markdown: %w", err)
		}
		
		if outputFile != "" {
			return os.WriteFile(outputFile, []byte(output), 0644)
		}
		fmt.Print(output)
		return nil
		
	case "html":
		// Use clicky's HTML formatter
		htmlFormatter := formatters.NewHTMLFormatter()
		output, err := ast.FormatQueryWithClicky(nodes, pattern, htmlFormatter)
		if err != nil {
			return fmt.Errorf("failed to format as HTML: %w", err)
		}
		
		if outputFile != "" {
			return os.WriteFile(outputFile, []byte(output), 0644)
		}
		fmt.Println(output)
		return nil
		
	case "excel":
		// For Excel, use CSV format
		csvFormatter := formatters.NewCSVFormatter()
		output, err := ast.FormatQueryWithClicky(nodes, pattern, csvFormatter)
		if err != nil {
			return fmt.Errorf("failed to format as CSV for Excel: %w", err)
		}
		
		if outputFile == "" {
			return fmt.Errorf("--output file is required for Excel format")
		}
		
		if !strings.HasSuffix(outputFile, ".csv") && !strings.HasSuffix(outputFile, ".xlsx") {
			outputFile += ".csv"
		}
		
		return os.WriteFile(outputFile, []byte(output), 0644)
		
	case "pretty":
		// Use clicky's pretty formatter for colored terminal output
		prettyFormatter := formatters.NewPrettyFormatter()
		if astNoColor {
			// TODO: Add no-color support to clicky formatter
		}
		output, err := ast.FormatQueryWithClicky(nodes, pattern, prettyFormatter)
		if err != nil {
			return fmt.Errorf("failed to format as pretty: %w", err)
		}
		fmt.Print(output)
		return nil
		
	default:
		// Fall back to legacy formatter for tree, table, template formats
		formatter := ast.NewFormatter(astFormat, astTemplate, astNoColor, "")
		output, err := formatter.FormatNodes(nodes, pattern)
		if err != nil {
			return fmt.Errorf("failed to format with legacy formatter: %w", err)
		}
		fmt.Println(output)
		return nil
	}
}

// getASTQueryOutputFormat determines the output format for AST queries
func getASTQueryOutputFormat() string {
	// Check global flags first (from root.go)
	if json {
		return "json"
	}
	if csv {
		return "csv"
	}
	if markdown {
		return "markdown"
	}
	if html {
		return "html"
	}
	if excel {
		return "excel"
	}
	
	// Use astFormat flag
	return astFormat
}

// executeAQLQuery executes an AQL query
func executeAQLQuery(astCache *cache.ASTCache, aqlQuery string, workingDir string) error {
	// Wrap the query in a temporary rule
	ruleText := fmt.Sprintf(`RULE "temp" { LIMIT(%s) }`, aqlQuery)
	
	// Parse the AQL
	ruleSet, err := parser.ParseAQLFile(ruleText)
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
	fmt.Println("AST Cache Statistics:")
	fmt.Println("  Feature not yet implemented")
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
		logger.Infof("No supported source files found in %s", workingDir)
		return nil
	}

	// Count files by language
	langCounts := make(map[string]int)
	for _, file := range sourceFiles {
		langCounts[file.language]++
	}
	
	logger.Infof("Found %d source files:", len(sourceFiles))
	for lang, count := range langCounts {
		logger.Infof("  %s: %d files", lang, count)
	}

	// Initialize library resolver
	libResolver := analysis.NewLibraryResolver(astCache)
	if err := libResolver.StoreLibraryNodes(); err != nil {
		logger.Warnf("Failed to store library nodes: %v", err)
	}

	// Create extractors for each language
	goExtractor := analysis.NewGoASTExtractor(astCache)
	pythonExtractor := analysis.NewPythonASTExtractor(astCache)
	jsExtractor := analysis.NewJavaScriptASTExtractor(astCache)
	tsExtractor := analysis.NewTypeScriptASTExtractor(astCache)
	mdExtractor := analysis.NewMarkdownASTExtractor(astCache)

	logger.Infof("Analyzing %d source files...", len(sourceFiles))

	// Process files
	for i, file := range sourceFiles {
		if i%10 == 0 {
			logger.Infof("Progress: %d/%d files", i, len(sourceFiles))
		}

		var err error
		switch file.language {
		case "go":
			err = goExtractor.ExtractFile(file.path)
		case "python":
			err = pythonExtractor.ExtractFile(file.path)
		case "javascript":
			err = jsExtractor.ExtractFile(file.path)
		case "typescript":
			err = tsExtractor.ExtractFile(file.path)
		case "markdown":
			err = mdExtractor.ExtractFile(file.path)
		}
		
		if err != nil {
			logger.Warnf("Failed to extract AST from %s: %v", file.path, err)
			continue
		}
	}

	logger.Infof("AST analysis completed")
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
	defer rows.Close()

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
	defer rows.Close()

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
		fmt.Println("No AST data found. Run analysis first with a pattern or without --cached-only.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Node Type\tCount\n")
	fmt.Fprintf(w, "─────────\t─────\n")
	
	for nodeType, count := range stats {
		fmt.Fprintf(w, "%s\t%d\n", nodeType, count)
	}
	
	fmt.Fprintf(w, "─────────\t─────\n")
	fmt.Fprintf(w, "Total\t%d\n", total)
	w.Flush()

	return nil
}

// displayNodes displays AST nodes in the requested format
func displayNodes(astCache *cache.ASTCache, nodes []*models.ASTNode, pattern string, workingDir string) error {
	if len(nodes) == 0 {
		fmt.Printf("No nodes found matching pattern: %s\n", pattern)
		return nil
	}

	switch astFormat {
	case "json":
		return outputNodesJSON(nodes)
	case "table":
		return outputNodesTable(nodes, workingDir)
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
	fmt.Fprintf(w, "File\tPackage\tType\tMethod\tComplexity\tLines\n")
	fmt.Fprintf(w, "────\t───────\t────\t──────\t──────────\t─────\n")
	
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
	
	w.Flush()
	return nil
}

// outputNodesTree outputs nodes in tree format grouped by file and class
func outputNodesTree(astCache *cache.ASTCache, nodes []*models.ASTNode, pattern string, workingDir string) error {
	fmt.Printf("AST Nodes matching pattern: %s\n", pattern)
	fmt.Printf("Found %d nodes:\n\n", len(nodes))

	// Group nodes by file -> class -> node type
	fileGroups := make(map[string]map[string]map[string][]*models.ASTNode)
	
	for _, node := range nodes {
		relPath := node.FilePath
		if strings.HasPrefix(node.FilePath, workingDir+"/") {
			relPath = strings.TrimPrefix(node.FilePath, workingDir+"/")
		}
		
		if fileGroups[relPath] == nil {
			fileGroups[relPath] = make(map[string]map[string][]*models.ASTNode)
		}
		
		className := node.TypeName
		if className == "" {
			className = "package-level"
		}
		
		if fileGroups[relPath][className] == nil {
			fileGroups[relPath][className] = make(map[string][]*models.ASTNode)
		}
		
		nodeType := string(node.NodeType)
		fileGroups[relPath][className][nodeType] = append(fileGroups[relPath][className][nodeType], node)
	}

	// Display grouped output
	fileCount := 0
	totalFiles := len(fileGroups)
	
	for fileName, classGroups := range fileGroups {
		fileCount++
		isLastFile := fileCount == totalFiles
		filePrefix := "├── "
		if isLastFile {
			filePrefix = "└── "
		}
		
		fmt.Printf("%s📁 %s\n", filePrefix, fileName)
		
		classCount := 0
		totalClasses := len(classGroups)
		
		for className, nodeTypeGroups := range classGroups {
			classCount++
			isLastClass := classCount == totalClasses
			
			classPrefix := "│   ├── "
			childPrefix := "│   │   "
			if isLastFile {
				classPrefix = "    ├── "
				childPrefix = "    │   "
			}
			if isLastClass {
				classPrefix = strings.Replace(classPrefix, "├── ", "└── ", 1)
				childPrefix = strings.Replace(childPrefix, "│   ", "    ", 1)
			}
			
			if className == "package-level" {
				fmt.Printf("%s📦 %s\n", classPrefix, className)
			} else {
				fmt.Printf("%s🏗️  %s\n", classPrefix, className)
			}
			
			// Display methods, fields, types, variables in compact format
			for nodeType, nodeList := range nodeTypeGroups {
				if len(nodeList) == 0 {
					continue
				}
				
				var items []string
				for _, node := range nodeList {
					name := node.MethodName
					if name == "" {
						name = node.FieldName
					}
					if name == "" {
						name = node.TypeName
					}
					
					item := fmt.Sprintf("%s:%d", name, node.StartLine)
					if astShowComplexity && node.CyclomaticComplexity > 0 {
						item += fmt.Sprintf("(c:%d)", node.CyclomaticComplexity)
					}
					items = append(items, item)
				}
				
				if len(items) > 0 {
					icon := "🔧"
					switch nodeType {
					case "method":
						icon = "⚡"
					case "field":
						icon = "📊"
					case "type":
						icon = "🏷️"
					case "variable":
						icon = "📝"
					}
					
					fmt.Printf("%s%s %-8s %s\n", childPrefix, icon, nodeType+":", strings.Join(items, ", "))
				}
			}
		}
		
		if !isLastFile {
			fmt.Println("│")
		}
	}

	return nil
}

// displayNodeRelationships displays call relationships for a node
func displayNodeRelationships(astCache *cache.ASTCache, node *models.ASTNode, prefix string) error {
	relationships, err := astCache.GetASTRelationships(node.ID, models.RelationshipCall)
	if err != nil {
		return err
	}

	if len(relationships) == 0 {
		return nil
	}

	fmt.Printf("%s📞 Calls (%d):\n", prefix, len(relationships))
	
	for i, rel := range relationships {
		isLastRel := i == len(relationships)-1
		relPrefix := "├── "
		if isLastRel {
			relPrefix = "└── "
		}

		if rel.ToASTID != nil {
			toNode, err := astCache.GetASTNode(*rel.ToASTID)
			if err != nil {
				continue
			}
			fmt.Printf("%s    %s%s (line %d)\n", prefix, relPrefix, toNode.GetFullName(), rel.LineNo)
		} else {
			fmt.Printf("%s    %s%s (line %d)\n", prefix, relPrefix, rel.Text, rel.LineNo)
		}
	}

	return nil
}

// displayNodeLibraries displays library dependencies for a node
func displayNodeLibraries(astCache *cache.ASTCache, node *models.ASTNode, prefix string) error {
	libRels, err := astCache.GetLibraryRelationships(node.ID, models.RelationshipCall)
	if err != nil {
		return err
	}

	if len(libRels) == 0 {
		return nil
	}

	fmt.Printf("%s📚 Libraries (%d):\n", prefix, len(libRels))
	
	for i, libRel := range libRels {
		isLastRel := i == len(libRels)-1
		relPrefix := "├── "
		if isLastRel {
			relPrefix = "└── "
		}

		fmt.Printf("%s    %s%s (%s, line %d)\n", prefix, relPrefix, 
			libRel.LibraryNode.GetFullName(), libRel.LibraryNode.Framework, libRel.LineNo)
	}

	return nil
}

// outputJSON outputs data as JSON
func outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}