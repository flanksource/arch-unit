# File-Specific Override Rules

arch-unit now supports file-specific override rules, allowing you to apply different architectural constraints to different files or file patterns within your project.

## Syntax

File-specific rules use square brackets to specify the file pattern:

```
[file-pattern] rule
```

Where:
- `file-pattern` is a glob pattern that matches files
- `rule` is any standard arch-unit rule

## Examples

### Basic File Patterns

```bash
# Allow test files to use testing packages
[*_test.go] +testing
[*_test.go] +github.com/stretchr/testify

# Allow main.go files to use os.Exit
[cmd/*/main.go] os:Exit

# Prevent service files from direct database access
[*_service.go] !database/sql
```

### Complex Scenarios

```bash
# Base rule: Deny fmt.Println everywhere
!fmt:Println

# Override: Allow it in test files
[*_test.go] +fmt:Println

# Override: Allow it in debug files
[*_debug.go] +fmt:Println
[debug/*.go] +fmt:Println
```

### Repository Pattern Example

```bash
# Deny database access by default
!database/sql
!github.com/lib/pq

# Allow repository files to use database
[*_repository.go] +database/sql
[*_repository.go] +github.com/lib/pq
[repository/*.go] +database/sql
```

## Pattern Matching

File patterns support standard glob syntax:
- `*` matches any sequence of characters (except path separator)
- `?` matches any single character
- `[...]` matches any character in the set
- `**` is not supported (use `*/` for directory traversal)

Patterns are matched against:
1. The filename only (e.g., `*_test.go`)
2. The full relative path (e.g., `cmd/*/main.go`)
3. Path segments for complex patterns

## Rule Priority

Rules are evaluated in order, with later rules overriding earlier ones:
1. Global rules apply to all files
2. File-specific rules override global rules for matching files
3. More specific patterns take precedence

## Python Support

File-specific overrides work with Python files too:

```bash
# Python: Allow test files to use unittest/pytest
[test_*.py] +unittest
[*_test.py] +pytest
[tests/*.py] +pytest

# Python: Restrict SQLAlchemy to models/repository only
!sqlalchemy
[models/*.py] +sqlalchemy
[repository/*.py] +sqlalchemy
```

## Best Practices

1. **Start with restrictive global rules**: Deny packages by default, then allow specific files
2. **Use clear naming conventions**: Consistent file naming makes pattern matching easier
3. **Document your patterns**: Add comments explaining why certain overrides exist
4. **Test your patterns**: Use `arch-unit check` to verify rules work as expected

## Debugging

To debug file-specific rules:

1. Run with JSON output to see which rules are applied:
   ```bash
   arch-unit check --json | jq '.violations'
   ```

2. Check the violation message for file-specific indicators:
   ```
   Call to fmt.Println violates file-specific rule [*_service.go]
   ```

3. Verify your patterns match expected files using glob testing tools

## Migration Guide

If you're migrating from global rules to file-specific rules:

1. Identify files that legitimately need exceptions
2. Add file-specific override rules for those files
3. Tighten global rules to be more restrictive
4. Test thoroughly to ensure no unintended side effects