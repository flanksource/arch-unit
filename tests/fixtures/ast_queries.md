# AST Query Test Fixtures

## Pattern Matching Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Find All Services | examples/go-project | *Service* | 5 | nodes.all(n, n.type_name.endsWith("Service") || n.method_name.contains("Service")) |
| Find User Types | examples/go-project | *User* | 7 | nodes.all(n, n.type_name.contains("User") || n.method_name.contains("User")) |
| Find All Methods | examples/go-project | *:*:* | 31 | nodes.all(n, n.node_type == "method") |
| Find Package Methods | examples/go-project/pkg/service | service:*:* | 4 | nodes.all(n, n.package_name == "service") |
| Find Specific Method | examples/go-project | *:*:GetUser | 1 | nodes.exists(n, n.method_name == "GetUser") |
| Find All Fields | examples/go-project | *:*:*:* | 8 | nodes.all(n, n.node_type == "field") |
| Match Service Methods | examples/go-project | service:*Service:* | 4 | nodes.all(n, n.package_name == "service") |
| Match Service Pattern | examples/yaml-config | *Service* | 1 | nodes.all(n, n.type_name.contains("Service")) |

## Complexity Analysis Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| High Complexity Methods | examples/go-project | cyclomatic(*) > 5 | 0 | nodes.all(n, n.cyclomatic_complexity > 5) |
| Low Complexity Methods | examples/go-project | cyclomatic(*) <= 2 | 31 | nodes.all(n, n.cyclomatic_complexity <= 2) |
| Medium Complexity | examples/yaml-config | cyclomatic(*) == 3 | 0 | nodes.all(n, n.cyclomatic_complexity == 3) |
| Complex Service Methods | examples/go-project | cyclomatic(*Service*) >= 1 | 3 | nodes.all(n, n.type_name.contains("Service") && n.cyclomatic_complexity >= 1) |

## Line Count Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Large Methods | examples/go-project | lines(*) > 100 | 0 | nodes.all(n, n.line_count > 100) |
| Small Methods | examples/go-project | lines(*) < 50 | 31 | nodes.all(n, n.line_count < 50) |
| Exact Line Count | examples/go-project | lines(*) == 17 | 0 | true |
| Service Line Check | examples/go-project | lines(*Service*) < 100 | 5 | nodes.all(n, n.type_name.contains("Service") && n.line_count < 100) |

## Parameter Count Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Many Parameters | examples/go-project | params(*) > 3 | 0 | nodes.all(n, n.parameter_count > 3) |
| No Parameters | examples/go-project | params(*) == 0 | 21 | nodes.all(n, n.parameter_count == 0) |
| Single Parameter | examples/go-project | params(*) == 1 | 9 | nodes.all(n, n.parameter_count == 1) |
| Service Parameters | examples/go-project | parameters(*Service*) >= 0 | 5 | nodes.all(n, n.type_name.contains("Service")) |

## Name Length Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Long Names | examples/go-project | len(*) > 40 | 1 | nodes.all(n, size(n.package_name + ":" + n.type_name + ":" + n.method_name) > 40) |
| Short Names | examples/go-project | len(*) < 20 | 0 | nodes.all(n, size(n.package_name + ":" + n.type_name + ":" + n.method_name) < 20) |
| Medium Names | examples/yaml-config | len(*) > 10 | 2 | nodes.all(n, size(n.method_name) > 10 || size(n.type_name) > 10) |

## Import Count Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Many Imports | examples/go-project | imports(*) > 5 | 0 | nodes.all(n, n.import_count > 5) |
| No Imports | examples/go-project | imports(*) == 0 | 31 | nodes.all(n, n.import_count == 0) |
| Some Imports | examples/go-project | imports(*) >= 1 | 0 | nodes.all(n, n.import_count >= 1) |

## Call Count Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Many Calls | examples/go-project | calls(*) > 5 | 0 | nodes.all(n, n.call_count > 5) |
| No Calls | examples/go-project | calls(*) == 0 | 31 | nodes.all(n, n.call_count == 0) |
| Some Calls | examples/go-project | calls(*) >= 1 | 0 | nodes.all(n, n.call_count >= 1) |

## Combined Metric Tests

| Test Name | CWD | Query | Expected Count | CEL Validation |
|-----------|-----|-------|----------------|----------------|
| Complex Large Methods | examples/go-project | cyclomatic(*) > 5 && lines(*) > 50 | 0 | nodes.all(n, n.cyclomatic_complexity > 5 && n.line_count > 50) |
| Simple Small Methods | examples/go-project | cyclomatic(*) <= 2 && lines(*) < 20 | 31 | nodes.all(n, n.cyclomatic_complexity <= 2 && n.line_count < 20) |