---
build: echo "Building..." && go build -o /tmp/test-arch-unit
exec: /tmp/test-arch-unit
---
# Exec Fixture Tests

## Basic Command Tests

| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| Echo Test | . | --version | version | exitCode == 0 |
| Help Output | . | --help | help | stdout.contains("Usage") && exitCode == 0 |

## AST Command Tests

| Test Name | CWD | CLI Args | Expected Output | CEL Validation |
|-----------|-----|----------|-----------------|----------------|
| AST Help | . | ast --help | AST help | stdout.contains("Analyze") && exitCode == 0 |
| Empty Query | examples/go-project | ast * | Results | exitCode == 0 |