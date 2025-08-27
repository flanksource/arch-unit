# AST Fixture-Based Testing System

This directory contains a fixture-based testing system for the arch-unit AST module that uses markdown tables to define test cases and CEL (Common Expression Language) for validation.

## Overview

The fixture system allows you to:
- Define test cases in readable markdown tables
- Use CEL expressions for flexible validation
- Easily add new test cases without writing Go code
- Test both the AST query engine and CLI commands

## File Structure

```
tests/fixtures/
├── README.md                 # This documentation
├── ast_queries.md           # AST query test fixtures
├── ast_cli.md              # CLI command test fixtures
├── parser.go               # Markdown table parser
├── cel_evaluator.go        # CEL expression evaluator
├── runner_test.go          # Test runner using Ginkgo
└── fixtures_suite_test.go  # Ginkgo test suite
```

## Writing Test Fixtures

### Query Test Fixtures

Query fixtures test the AST query engine directly. Create tables in markdown files with these columns:

| Column | Description | Example |
|--------|-------------|---------|
| Test Name | Descriptive name for the test | "Find All Controllers" |
| CWD | Working directory relative to project root | "examples/go-project" |
| Query | AST query to execute | "*Controller*" or "cyclomatic(*) > 5" |
| Expected Count | Expected number of results (optional) | 5 |
| CEL Validation | CEL expression to validate results | `nodes.all(n, n.type_name.endsWith("Controller"))` |

Example:
```markdown
| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Find Controllers | examples/go-project | *Controller* | 1 | nodes.all(n, n.type_name.endsWith("Controller")) |
```

### CLI Test Fixtures

CLI fixtures test the command-line interface. Use these columns:

| Column | Description | Example |
|--------|-------------|---------|
| Test Name | Descriptive name for the test | "JSON Format Output" |
| CWD | Working directory | "examples/go-project" |
| CLI Args | Command arguments | "* --format json" |
| Expected Output | Text that should appear in output | "node_type" |
| CEL Validation | CEL expression for output validation | `output.contains("file_path")` |

Example:
```markdown
| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| JSON Format | examples/go-project | * --format json | JSON | output.contains("file_path") |
```

## CEL Expression Reference

### Available Variables

In query tests (`nodes` variable):
- `nodes` - List of AST nodes returned by the query
- Each node has properties: `type_name`, `method_name`, `package_name`, `node_type`, `line_count`, `cyclomatic_complexity`, etc.

In CLI tests (`output` variable):
- `output` - String containing the command output

### Available Functions

#### String Functions
- `string.endsWith(suffix)` - Check if string ends with suffix
- `string.contains(substring)` - Check if string contains substring  
- `string.startsWith(prefix)` - Check if string starts with prefix

#### List Functions
- `nodes.all(n, predicate)` - Returns true if all nodes match the predicate
- `nodes.exists(n, predicate)` - Returns true if any node matches the predicate
- `nodes.filter(n, predicate)` - Returns filtered list of nodes
- `list.unique()` - Returns list with unique values
- `size(list)` - Returns the size of a list

### Example CEL Expressions

```cel
# Check all nodes are controllers
nodes.all(n, n.type_name.endsWith("Controller"))

# Check at least one method named GetUser exists
nodes.exists(n, n.method_name == "GetUser")

# Check all methods have low complexity
nodes.all(n, n.cyclomatic_complexity < 10)

# Check output contains specific text
output.contains("AST Overview")

# Complex validation with multiple conditions
nodes.filter(n, n.node_type == "method").size() > 5 && 
nodes.exists(n, n.line_count > 100)

# Check specific properties
nodes.all(n, n.package_name == "service" && n.parameter_count <= 3)
```

## Adding New Test Cases

1. Choose the appropriate markdown file or create a new one
2. Add your test case as a new row in a table
3. Use meaningful test names and clear CEL expressions
4. Run tests to verify they pass

### Tips for Writing Good Fixtures

1. **Use descriptive test names** - Make it clear what is being tested
2. **Keep CEL expressions simple** - Complex expressions are harder to debug
3. **Test one thing at a time** - Each fixture should validate a specific behavior
4. **Use appropriate CWD** - Ensure test data exists in the specified directory
5. **Document complex validations** - Add comments above complex CEL expressions

## Running the Tests

Run all fixture tests:
```bash
go test ./tests/fixtures/...
```

Run with verbose output:
```bash
go test -v ./tests/fixtures/...
```

Run specific test suites:
```bash
ginkgo run ./tests/fixtures
```

## Debugging Failed Tests

When a test fails, the output will show:
1. The test name from the fixture
2. The query or command that was executed
3. The actual vs expected results
4. The CEL expression that failed (if applicable)

To debug:
1. Check if the test data exists in the specified CWD
2. Verify the query/command syntax is correct
3. Test the CEL expression with simpler predicates
4. Use the verbose flag to see detailed output

## Extending the System

### Adding New Table Columns

Edit `parser.go` to handle new columns in the `parseTableRow` function.

### Adding Custom CEL Functions

Edit `cel_evaluator.go` to add new functions in the `NewCELEvaluator` function.

### Supporting New Test Types

Edit `runner_test.go` to add new test contexts for different types of tests.

## Benefits

- **Maintainable**: Test cases are defined declaratively in markdown
- **Readable**: Tables are easy to understand and review
- **Flexible**: CEL provides powerful validation capabilities
- **Extensible**: Easy to add new test types and validations
- **No Compilation**: Add tests without recompiling Go code