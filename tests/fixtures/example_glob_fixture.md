---
files: "**/*.go"
exec: echo "Processing {{.file}} - filename={{.filename}}, dir={{.dir}}, ext={{.ext}}"
---

# Example Glob Fixture

This fixture demonstrates the new glob pattern functionality.
When files is specified in the frontmatter, the fixture will be executed once for each matching file.

The following template variables are available:
- `{{.file}}` - Relative path to the matched file
- `{{.filename}}` - Filename without extension
- `{{.dir}}` - Directory containing the file
- `{{.absfile}}` - Absolute path to file
- `{{.absdir}}` - Absolute directory path
- `{{.basename}}` - Full filename with extension
- `{{.ext}}` - File extension

## Test Cases

| Test Name | Expected Output | CEL Validation |
|-----------|-----------------|----------------|
| Process Go Files | Processing | stdout.contains("Processing") |