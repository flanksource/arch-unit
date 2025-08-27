
# Simple Test Fixtures

## Basic Pattern Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Find All Nodes | examples/go-project | * | 13 | true |
| Find Service Type | examples/go-project | *Service | 1 | nodes.all(n, n.type_name == "UserService") |
| Find All Methods | examples/go-project | *:*:* | 5 | nodes.all(n, n.node_type == "method") |

## CLI Output Tests

| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| List All Nodes | examples/go-project | * | nodes | stdout.contains("UserService") && exitCode == 0 |
| JSON Output | examples/go-project | * --format json | JSON output | stdout.contains("\"node_type\"") && exitCode == 0 |