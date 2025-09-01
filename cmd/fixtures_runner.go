package cmd

import (
	"fmt"
	"os"

	"github.com/flanksource/arch-unit/fixtures"
	_ "github.com/flanksource/arch-unit/fixtures/types" // Register fixture types
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var astFixturesCmd = &cobra.Command{
	Use:   "fixtures [fixture-files...]",
	Short: "Run fixture-based tests from markdown tables and command blocks",
	Long: `Run fixture-based tests defined in markdown table format or command block format.

This command provides a declarative testing framework for AST queries and CLI commands
using markdown tables or command blocks with CEL (Common Expression Language) validation.
Tests are organized hierarchically by file and section, making it easy to maintain large test suites.

FIXTURE FILE FORMATS:

  1. MARKDOWN TABLES (Traditional format)
  2. COMMAND BLOCKS (New expressive format)
  3. MIXED FORMAT (Both tables and command blocks in same file)

YAML FRONT-MATTER:
  Files can include optional YAML front-matter for configuration:

  ---
  build: "go build -o myapp"      # Build command to run before tests
  exec: "./myapp"                  # Base executable for exec tests
  base_exec: "arch-unit ast"       # Default command prefix
  env:                             # Environment variables
    DEBUG: "true"
    LOG_LEVEL: "debug"
  ---

FORMAT 1: MARKDOWN TABLES (Traditional)

  Query Tests (AST pattern and AQL queries):
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

  Exec Tests (CLI command execution):
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

FORMAT 2: COMMAND BLOCKS (New Expressive Format)

  Command blocks provide a more readable and maintainable way to define complex tests
  with multiple validations, environment variables, and configurations.

  Syntax:
    ### command: test-name

    ` + "```bash" + `
    command to execute
    ` + "```" + `

    ` + "```frontmatter" + `
    cwd: /path/to/working/directory
    exitCode: 0
    env:
      VAR1: value1
      VAR2: value2
    timeout: 30s
    ` + "```" + `

    Validations:
    * cel: stdout.contains("expected")
    * cel: exitCode == 0
    * contains: simple text check
    * regex: pattern.*matching
    * not: contains: unwanted text

  Command Block Components:
    - Command Name: Descriptive name after "command:" (required)
    - Bash Block: Command to execute with proper syntax highlighting
    - Frontmatter Block: YAML configuration for test execution
    - Validations: Bullet-point list of validation rules

  Frontmatter Options:
    - cwd: Working directory (relative to fixture file)
    - exitCode: Expected exit code (default: 0)
    - env: Environment variables map
    - timeout: Command timeout (e.g., "30s", "2m")
    - stdin: Standard input for command (optional)

  Validation Types:
    - cel: CEL expression (full power of CEL language)
    - contains: Simple string contains check (converted to stdout.contains)
    - regex: Regular expression matching (converted to stdout.matches)
    - not: Negation of any validation type
    - not contains: Negative string check
    - json: JSON path validation (for JSON output)

  Example:
    ### command: complex json analysis

    ` + "```bash" + `
    ast --query "cyclomatic(*) > 10" --format json --complexity
    ` + "```" + `

    ` + "```frontmatter" + `
    cwd: ./examples/go-project
    exitCode: 0
    env:
      LOG_LEVEL: debug
      OUTPUT_FORMAT: json
    timeout: 30s
    ` + "```" + `

    Validations:
    * cel: stdout.contains('"node_type"')
    * cel: json.results.size() > 0
    * regex: .*"cyclomatic_complexity":\s*[0-9]+.*
    * contains: complexity
    * not contains: ERROR
    * cel: duration < 5000

FORMAT 3: MIXED FORMAT (Tables and Command Blocks)

  You can use both table and command block formats in the same markdown file.
  This allows gradual migration from tables to command blocks or mixing simple
  table tests with complex command block tests as needed.

  Example:
    # Mixed Format Fixture File

    ## Table-Based Tests (Simple)
    | Test Name | CLI Args | CEL Validation |
    |-----------|----------|----------------|
    | Help Test | --help | stdout.contains("Usage") |

    ## Command Block Tests (Complex)

    ### command: json output validation

    ` + "```bash" + `
    ast * --format json --complexity
    ` + "```" + `

    ` + "```frontmatter" + `
    cwd: ./examples
    exitCode: 0
    ` + "```" + `

    Validations:
    * cel: stdout.contains("node_type")
    * cel: json.length > 0
    * not contains: ERROR

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

  Running Fixtures:
    # Run all fixture files in a directory (supports both tables and command blocks)
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

  Migration from Tables to Command Blocks:
    # Start with existing table-based tests
    | Test Name | CLI Args | CEL Validation |
    |-----------|----------|----------------|
    | Simple Test | --help | stdout.contains("Usage") |

    # Migrate complex tests to command blocks for better readability
    ### command: complex validation test

    ` + "```bash" + `
    ast --query "complexity > 5" --format json
    ` + "```" + `

    ` + "```frontmatter" + `
    cwd: ./src
    exitCode: 0
    env:
      DEBUG: "true"
    ` + "```" + `

    Validations:
    * cel: stdout.contains("node_type")
    * cel: json.results.size() > 0
    * not contains: ERROR

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
  - "Command block parsing failed": Check command block syntax (### command:)
  - "Frontmatter parsing failed": Verify YAML syntax in frontmatter blocks
  - "Validation type not recognized": Use supported types (cel, contains, regex, not)

  Debugging Tips:
  - Use --verbose to see detailed query execution and CEL evaluation
  - Start with simple CEL expressions and gradually add complexity
  - Test patterns with 'arch-unit ast' command directly first
  - Check that test data exists in specified working directories
  - Use --format json to inspect the full structure of results
  - For command blocks, verify bash and frontmatter code block syntax
  - Test validation expressions individually before combining them

  Format-Specific Tips:
    Tables:
    - Ensure all table rows have the same number of columns as headers
    - Use "-" for empty cells instead of leaving them blank

    Command Blocks:
    - Use proper markdown code block syntax with language identifiers
    - Separate bash commands and YAML frontmatter into different code blocks
    - Use bullet points (*) for validation lists
    - Quote strings in frontmatter YAML when they contain special characters

FILE ORGANIZATION:
  Organize fixtures by feature or component:
    tests/fixtures/
    ├── ast_patterns.md     # AST pattern matching tests (tables/command blocks)
    ├── aql_queries.md      # AQL query language tests (tables)
    ├── cli_output.md       # CLI output format tests (command blocks)
    ├── performance.md      # Performance regression tests (command blocks)
    ├── mixed_format.md     # Mixed table and command block tests
    └── integration/        # Integration test fixtures
        ├── go_analysis.md      # Go-specific tests (command blocks)
        └── python_analysis.md  # Python-specific tests (tables)`,
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

	// Get current executable path
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create runner with options
	runner, err := fixtures.NewRunner(fixtures.RunnerOptions{
		Paths:          args,
		Format:         clicky.Flags.ResolveFormat(),
		Filter:         "", // No filter by default
		NoColor:        clicky.Flags.FormatOptions.NoColor,
		WorkDir:        workingDir,
		MaxWorkers:     clicky.Flags.MaxConcurrent,
		Logger:         logger.StandardLogger(),
		ExecutablePath: executablePath,
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
