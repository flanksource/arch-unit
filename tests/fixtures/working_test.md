# Working Test Fixtures

## Simple Pattern Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Find Any Nodes | examples/go-project | * | 31 | size(nodes) > 0 |
| Find Methods | examples/go-project | cyclomatic(*) >= 0 | 31 | size(nodes) == 31 |

## Simple CLI Tests  

| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| Help Command | . | --help | Analyze and inspect the Abstract Syntax Tree | output.contains("Analyze and inspect the Abstract Syntax Tree") |