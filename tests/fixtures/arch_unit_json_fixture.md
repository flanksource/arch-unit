---
# Build arch-unit before running tests
exec: "arch-unit"
cwd: examples
timeout: 60s
---

# Arch-Unit JSON Output and Filtering Tests

This fixture tests `arch-unit check` JSON output and filtering to return empty slices.
It also re-implements various arch-unit tests in fixture format.


Core arch-unit functionality tests:

| Test Name | CLI Args | CEL Validation |
|-----------|----------|----------------|
| Check Help | check --help | stdout.contains("Check for architecture violations")  |
| JSON Output | check --json | jq('.',stdout).JSONArray().size() > 0 |

