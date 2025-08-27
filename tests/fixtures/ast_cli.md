build: make build
exec: arch-unit
---
# AST CLI Test Fixtures

## Basic CLI Commands

| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| Help Command | . | --help | "Analyze and inspect AST" | output.contains("Analyze and inspect AST") |
| Version Info | . | --version | "arch-unit" | output.contains("arch-unit") |
| Empty Directory | /tmp/empty | | "No Go files found" | output.contains("No Go files found") |
| Basic Analysis | examples/go-project | | "AST Overview" | output.contains("AST Overview") || output.contains("Analyzing") |
| Cached Only | examples/go-project | --cached-only | "AST Overview" | !output.contains("Analyzing") |

## Format Output Tests

| Test Name | CWD | CLI Args | Expected Format | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| JSON Format | examples/go-project | * --format json | file_path | output.contains("\"file_path\"") && output.contains("\"node_type\"") |
| Table Format | examples/go-project | * --format table | Table | output.contains("File") && output.contains("Package") && output.contains("Method") |
| Tree Format | examples/go-project | * --format tree | Tree | output.contains("📁") || output.contains("└─") |
| Template Format | examples/go-project | * --format template --template "{{.Package}}.{{.Method}}" | Custom | output.contains(".") && !output.contains("{{") |

## Pattern Matching CLI Tests

| Test Name | CWD | CLI Args | Expected Matches | CEL Validation |
|-----------|-----|----------|------------------|----------------|
| Service Pattern | examples/go-project | *Service* | UserService | output.contains("Service") |
| User Pattern | examples/go-project | *User* | User | output.contains("User") |
| All Pattern | examples/go-project | * | Multiple | output.contains("method") || output.contains("type") |
| Package Pattern | examples/go-project | service:* | service package | output.contains("service") |
| Method Pattern | examples/go-project | *:*:GetUser | GetUser method | output.contains("GetUser") |

## Query Flag Tests

| Test Name | CWD | CLI Args | Expected Results | CEL Validation |
|-----------|-----|----------|------------------|----------------|
| Complexity Query | examples/go-project | --query "cyclomatic(*) > 5" | High complexity | output.contains("AQL Query:") |
| Lines Query | examples/go-project | --query "lines(*) > 100" | Large methods | output.contains("AQL Query:") |
| Params Query | examples/go-project | --query "params(*) > 3" | Many parameters | output.contains("AQL Query:") |
| Length Query | examples/go-project | --query "len(*) > 40" | Long names | output.contains("AQL Query:") |

## Complexity Flag Tests

| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| Show Complexity | examples/go-project | * --complexity | Complexity info | output.contains("(c:") || output.contains("complexity") |
| High Threshold | examples/go-project | * --complexity --threshold 10 | Complex only | !output.contains("(c:1)") && !output.contains("(c:2)") |
| Low Threshold | examples/go-project | * --complexity --threshold 1 | All methods | output.contains("(c:") || output.contains("complexity") |

## Library and Call Analysis

| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| Show Libraries | examples/go-project | * --libraries | Import info | output.contains("import") || output.contains("library") || output.contains("External") |
| Show Calls | examples/go-project | * --calls | Call info | output.contains("call") || output.contains("->") || output.contains("Calls") |

## File Filtering Tests

| Test Name | CWD | CLI Args | Expected Files | CEL Validation |
|-----------|-----|----------|----------------|----------------|
| Include Go Files | examples/go-project | --include "*.go" | Only .go files | !output.contains(".py") && !output.contains(".java") |
| Exclude Test Files | examples/go-project | --exclude "*_test.go" | No test files | !output.contains("_test.go") |
| Include Specific Dir | examples/go-project | --include "pkg/**/*.go" | Only pkg dir | output.contains("pkg/") || output.contains("pkg\\\\") |
| Exclude Vendor | examples/go-project | --exclude "vendor/**" | No vendor | !output.contains("vendor/") && !output.contains("vendor\\\\") |

## Template Variable Tests

| Test Name | CWD | CLI Args | Template Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| Package Template | examples/go-project | * --format template --template "{{.Package}}" | Package names | !output.contains("{{") && output.contains("service") |
| Method Template | examples/go-project | * --format template --template "{{.Method}}" | Method names | !output.contains("{{") && (output.contains("GetUser") || output.contains("main")) |
| Complex Template | examples/go-project | * --format template --template "{{.Package}}.{{.Type}}.{{.Method}} ({{.Lines}} lines)" | Full info | output.contains("lines)") && output.contains(".") |
| Node Type Template | examples/go-project | * --format template --template "{{.NodeType}}: {{.Method}}" | Type prefix | output.contains("method:") || output.contains("type:") |

## Error Handling Tests

| Test Name | CWD | CLI Args | Expected Error | CEL Validation |
|-----------|-----|----------|----------------|----------------|
| Invalid Query | examples/go-project | --query "invalid syntax" | Error message | output.contains("error") || output.contains("invalid") |
| Template Without Format | examples/go-project | --template "{{.Package}}" | Error message | output.contains("--template flag can only be used with --format template") |
| Format Without Template | examples/go-project | --format template | Error message | output.contains("--template flag is required when using --format template") |
| Invalid Pattern | examples/go-project | :::: | No match or error | output.contains("No nodes found") || output.contains("error") |
