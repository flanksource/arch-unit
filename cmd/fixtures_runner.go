package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/fixtures"
	_ "github.com/flanksource/arch-unit/fixtures/types" // Register fixture types
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

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

func runASTFixtures(cmd *cobra.Command, args []string) error {
	// Get working directory
	workingDir, err := GetWorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create runner with options
	runner, err := fixtures.NewRunner(fixtures.RunnerOptions{
		Paths:      args,
		Format:     clicky.Flags.Format,
		Filter:     "", // No filter by default
		NoColor:    clicky.Flags.FormatOptions.NoColor,
		WorkDir:    workingDir,
		MaxWorkers: clicky.Flags.MaxConcurrent,
		Logger:     logger.StandardLogger(),
	})
	if err != nil {
		return fmt.Errorf("failed to create fixture runner: %w", err)
	}

	// Run the fixtures
	return runner.Run()
}

func init() {

	rootCmd.AddCommand(astFixturesCmd)

}
