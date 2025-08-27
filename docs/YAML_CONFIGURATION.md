# YAML Configuration (arch-unit.yaml)

arch-unit now supports a structured YAML configuration format that provides more powerful features than the legacy `.ARCHUNIT` files, including linter integrations, file-specific debounce settings, and consolidated violation reporting.

## Migration from .ARCHUNIT

The new `arch-unit.yaml` format replaces `.ARCHUNIT` files with a more structured approach:

### Legacy (.ARCHUNIT)
```
!internal/
!fmt:Println
!testing
[*_test.go] +testing
[*_test.go] +fmt:Println
```

### New (arch-unit.yaml)
```yaml
version: "1.0"
debounce: "30s"
rules:
  "**":
    imports:
      - "!internal/"
      - "!fmt:Println" 
      - "!testing"
  "**/*_test.go":
    imports:
      - "+testing"
      - "+fmt:Println"
linters:
  golangci-lint:
    enabled: true
    debounce: "60s"
    output_format: "json"
```

## Configuration Structure

### Global Settings
```yaml
version: "1.0"           # Configuration version
debounce: "30s"          # Global debounce period
```

### Rules Section
Rules are organized by path patterns that determine which files they apply to:

```yaml
rules:
  # Global rules (apply to all files)
  "**":
    imports:
      - "!internal/"
      - "!fmt:Println"
    debounce: "30s"
    
  # File-specific rules  
  "**/*_test.go":
    imports:
      - "+testing"
      - "+fmt:Println"
    debounce: "10s"
    
  # Path-specific rules
  "cmd/*/main.go":
    imports:
      - "+os:Exit"
```

### Pattern Matching

| Pattern | Description | Example Files |
|---------|-------------|---------------|
| `**` | All files | `service.go`, `cmd/main.go` |
| `*_test.go` | Test files in any directory | `user_test.go` |
| `**/*_test.go` | Test files recursively | `api/user_test.go` |
| `cmd/*/main.go` | Main files in cmd subdirs | `cmd/app/main.go` |
| `internal/**` | All files in internal | `internal/config/db.go` |

### Import Rules

Import rules follow the same syntax as legacy `.ARCHUNIT`:

| Rule | Description |
|------|-------------|
| `package` | Allow package (default) |
| `!package` | Deny package |
| `+package` | Override parent denial |
| `package:method` | Allow specific method |
| `package:!method` | Deny specific method |
| `*:method` | Apply to all packages |

### Linter Integration

Configure external linters to run alongside arch-unit:

```yaml
linters:
  golangci-lint:
    enabled: true
    debounce: "60s"
    args: ["--fast", "--timeout=5m"]
    output_format: "json"
  
  ruff:
    enabled: true  
    debounce: "30s"
    args: ["--select=E,W,F"]
    output_format: "json"
    
  "make lint":
    enabled: true
    debounce: "45s"
    output_format: "text"
```

#### Supported Linters
- **golangci-lint**: Go linting with JSON output parsing
- **ruff**: Python linting with JSON output parsing  
- **eslint**: JavaScript linting with JSON output parsing
- **black**: Python formatting
- **mypy**: Python type checking
- **make targets**: Custom make commands

## File-Specific Configuration

Apply different rules and settings to different file patterns:

```yaml
rules:
  # Strict rules for production code
  "**":
    imports:
      - "!fmt:Print*"
      - "!log:Print*"
    debounce: "60s"
    linters:
      golangci-lint:
        args: ["--enable=gosec,gocritic"]
  
  # Relaxed rules for tests
  "**/*_test.go":
    imports:
      - "+testing"
      - "+fmt:Print*"
    debounce: "10s"
    linters:
      golangci-lint:
        args: ["--disable=gosec"]
        
  # Special rules for main files
  "cmd/*/main.go":
    imports:
      - "+os:Exit"
      - "+fmt:Println"
    linters:
      golangci-lint:
        args: ["--disable=forbidigo"]
```

## Debounce Settings

Configure debounce periods at multiple levels:

1. **Global**: Applied to all operations
2. **Rule-level**: Override global for specific patterns
3. **Linter-level**: Override for specific linters
4. **CLI**: Override everything with `--debounce` flag

```yaml
debounce: "30s"  # Global default

rules:
  "**/*_test.go":
    debounce: "10s"  # Faster checks for tests
    linters:
      golangci-lint:
        debounce: "5s"  # Even faster for golangci-lint on tests
```

## CLI Usage

### Basic Usage
```bash
# Use arch-unit.yaml (default)
arch-unit check

# Force legacy .ARCHUNIT format  
arch-unit check --legacy

# Run only linters
arch-unit check --linters-only

# Skip linters
arch-unit check --linters=false
```

### Initialization
```bash
# Create arch-unit.yaml
arch-unit init

# Create legacy .ARCHUNIT  
arch-unit init --legacy
```

## Output Formats

### JSON Output with Consolidation
When linters are enabled, arch-unit produces consolidated JSON output:

```json
{
  "summary": {
    "files_analyzed": 15,
    "rules_applied": 42, 
    "linters_run": 2,
    "linters_successful": 2,
    "total_violations": 3,
    "arch_violations": 1,
    "linter_violations": 2
  },
  "arch_unit": {
    "violations": [...]
  },
  "linters": [
    {
      "linter": "golangci-lint", 
      "success": true,
      "violations": [...]
    }
  ],
  "violations": [...] // All violations combined
}
```

### Standard Output
```
📋 Architecture Violations
├── service.go (2 violations)
│   ├── !fmt:Println (arch-unit)
│   └── unused variable (golangci-lint)

✗ Found 2 total violation(s)
  - 1 architecture violation(s)
  - 1 linter violation(s)
  Analyzed 15 file(s) with 42 rule(s)
  Ran 2 linter(s) (2 successful)
```

## Advanced Examples

### Microservice Architecture
```yaml
version: "1.0"
rules:
  # Domain layer - no external dependencies
  "domain/**":
    imports:
      - "!net/http"
      - "!database/sql"
      - "!encoding/json"
      
  # Application layer - can use domain
  "application/**": 
    imports:
      - "+domain/*"
      - "!infrastructure/*"
      
  # Infrastructure layer - can access anything
  "infrastructure/**":
    imports:
      - "+database/sql"
      - "+net/http"
      
  # Interfaces/API layer
  "interfaces/**":
    imports:
      - "+net/http"
      - "+application/*"
      - "!domain/*"  # Must go through application
```

### Python Django Project
```yaml
version: "1.0"
rules:
  "**/*.py":
    imports:
      - "!django.db:models.Model"  # Use abstract models
    linters:
      ruff:
        enabled: true
        args: ["--select=E,W,F,DJ"]
      black:
        enabled: true
      mypy:
        enabled: true
        
  "*/models.py":
    imports:
      - "+django.db"
      
  "*/tests.py":
    imports:
      - "+django.test"
      - "+unittest"
    linters:
      ruff:
        args: ["--ignore=S101"]  # Allow assert
```

## Troubleshooting

### Configuration Validation
```bash
# Check configuration syntax
arch-unit check --dry-run

# Validate specific patterns
arch-unit check --include="*_test.go" --dry-run
```

### Pattern Debugging
Use the `--verbose` flag to see which patterns match which files:

```bash
arch-unit check --verbose
```

### Linter Issues
- Ensure linters are installed and in PATH
- Check linter-specific arguments are correct
- Use `output_format: "json"` for better violation parsing
- Set appropriate debounce periods for performance

## Best Practices

1. **Start Simple**: Begin with basic rules and add complexity gradually
2. **Use Hierarchical Rules**: Define global defaults, then override for specific patterns
3. **Test Patterns**: Use small test projects to validate pattern matching
4. **Linter Integration**: Enable JSON output for better violation consolidation
5. **Debounce Tuning**: Use shorter periods for tests, longer for production code
6. **Version Control**: Commit `arch-unit.yaml` to share configuration across team