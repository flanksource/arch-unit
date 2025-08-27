# arch-unit

Architecture linter for Go and Python projects that enforces dependency rules through `.ARCHUNIT` files.

## Features

- 🔍 **AST-based analysis** for Go (1.23+) and Python (3.11+)
- 📁 **Hierarchical rules** with directory-scoped `.ARCHUNIT` files
- 🎯 **Fine-grained control** over package and method access
- 🔄 **Rule inheritance and overrides** for flexible architecture enforcement
- 📁 **File-specific overrides** to apply rules selectively to matching files
- ⏱️ **Debounce support** to prevent rapid re-runs in file watchers and IDEs
- 📊 **Multiple output formats**: table, JSON, CSV, HTML, Markdown
- 🚀 **Fast and efficient** analysis with minimal false positives

## Installation

### Using Go

```bash
go install github.com/flanksource/arch-unit@latest
```

### From Source

```bash
git clone https://github.com/flanksource/arch-unit
cd arch-unit
make install
```

## Quick Start

1. Initialize a `.ARCHUNIT` file in your project:

```bash
arch-unit init
```

2. Edit the `.ARCHUNIT` file to define your architecture rules:

```bash
# Prevent access to internal packages
!internal/

# Prevent test utilities in production
!*_test
!testing

# Prevent direct database access outside repository layer  
!database/sql

# Method-specific rules
fmt:!Println  # No fmt.Println, use logger
*:!Test*      # No test methods in production
```

3. Check your codebase for violations:

```bash
arch-unit check
```

## Rule Syntax

### Basic Patterns

| Pattern | Description | Example |
|---------|-------------|---------|
| `pattern` | Allow access (default) | `utils/` |
| `!pattern` | Deny access | `!internal/` |
| `+pattern` | Override parent rules | `+internal/` |

### Package/Method Rules

| Pattern | Description | Example |
|---------|-------------|---------|
| `package:method` | Method-specific rule | `fmt:Println` |
| `package:!method` | Deny specific method | `fmt:!Printf` |
| `*:method` | Apply to all packages | `*:!Test*` |

### Pattern Matching

| Pattern | Description | Example |
|---------|-------------|---------|
| `internal/` | Match package/folder | `!internal/` |
| `*.test` | Wildcard suffix | `!*.test` |
| `api.*` | Match package and sub-packages | `api.*` |
| `*/private/*` | Match path segment | `!*/private/*` |

## Examples

### Example 1: Layered Architecture

Root `.ARCHUNIT`:
```bash
# Domain layer should not depend on infrastructure
!infrastructure/

# Application layer should not depend on presentation
!presentation/
```

Infrastructure layer `.ARCHUNIT`:
```bash
# Infrastructure can access domain
+domain/

# But not application or presentation
!application/
!presentation/
```

### Example 2: Test Isolation

Root `.ARCHUNIT`:
```bash
# No test packages in production
!testing
!*_test

# No test methods
*:!Test*
*:!test*
```

Test directory `.ARCHUNIT`:
```bash
# Tests can access everything
+testing
+*_test
```

### Example 3: API Boundaries

```bash
# Public API only through specific packages
!internal/
!private/

# Database access only through repository
!database/sql
!gorm
!mongo

# HTTP client access controlled
!net/http:Get
!net/http:Post
```

## AST Analysis and Pattern Matching

The `ast` command provides powerful code analysis capabilities using Abstract Syntax Tree parsing, allowing you to search and analyze your codebase with sophisticated pattern matching.

### Sample Project Structure

To understand pattern matching, consider this example project structure:

```
myproject/
├── controllers/
│   ├── UserController.go
│   │   ├── type UserController struct
│   │   ├── func (c *UserController) GetUser(id string) (*User, error)
│   │   ├── func (c *UserController) CreateUser(data UserDTO) (*User, error)
│   │   └── func (c *UserController) UpdateUser(id string, data UserDTO) error
│   └── ProductController.go
│       ├── type ProductController struct
│       ├── func (c *ProductController) ListProducts(filters Filter) ([]Product, error)
│       └── func (c *ProductController) GetProduct(id string) (*Product, error)
├── services/
│   ├── UserService.go
│   │   ├── type UserService struct
│   │   │   ├── repo UserRepository
│   │   │   └── cache CacheService
│   │   ├── func (s *UserService) ValidateUser(user *User) error
│   │   └── func (s *UserService) ProcessUserBatch(users []User) error
│   └── EmailService.go
│       └── func (s *EmailService) SendWelcomeEmail(user *User) error
├── models/
│   ├── User.go
│   │   ├── type User struct
│   │   │   ├── ID string
│   │   │   ├── Name string
│   │   │   └── Email string
│   └── Product.go
│       └── type Product struct
│           ├── ID string
│           ├── Name string
│           └── Price float64
└── utils/
    └── helpers.go
        ├── func FormatDate(t time.Time) string
        └── func ValidateEmail(email string) bool
```

### AST Command Usage

```bash
# Basic usage - show all nodes in current directory
arch-unit ast

# Analyze specific directory
arch-unit ast --cwd /path/to/project

# Search with patterns
arch-unit ast "controllers:*"           # All types in controllers package
arch-unit ast "services.UserService"    # Specific type (dot notation)
arch-unit ast "*:*:Get*"               # All Get methods in any type

# Query with metrics
arch-unit ast --query "lines(*) > 100"          # Find large code blocks
arch-unit ast --query "cyclomatic(*) >= 10"     # Find complex methods
arch-unit ast --query "len(*) > 40"             # Find nodes with long names
arch-unit ast --query "imports(*) > 10"         # Find modules with many imports
arch-unit ast --query "calls(*) > 5"            # Find methods with many external calls

# Output formats
arch-unit ast "models:*" --format table        # Table view (default)
arch-unit ast "models:*" --format json         # JSON output
arch-unit ast "models:*" --format tree         # Tree hierarchy

# Verbose mode for debugging patterns
arch-unit ast "services.*" -vvv                # Show pattern parsing details
```

### Pattern Syntax Guide

Patterns use the format: `package:type:method:field` (colon notation) or `package.type.method.field` (dot notation)

**Smart Pattern Recognition:**
- Single strings are automatically recognized as methods, types, or packages based on naming conventions
- `GetUser` → searches for GetUser method in any type/package (`*:*:GetUser`)
- `UserController` → searches for UserController type in any package (`*:UserController`)
- `UserController:GetUser` → searches for GetUser method in UserController type (`*:UserController:GetUser`)

| Pattern Type | Example Pattern | What It Matches | Sample Matches from Structure |
|-------------|-----------------|-----------------|--------------------------------|
| **All Nodes** | `*` | Everything in the codebase | All packages, types, methods, and fields |
| **Package Patterns** | | | |
| Simple package | `controllers` | All nodes in controllers package | `UserController`, `ProductController`, all their methods |
| Package wildcard | `*Service` | Packages ending with "Service" | Would match if there was a package named "UserService" |
| All in package | `services:*` | All types in services package | `UserService`, `EmailService` |
| **Type Patterns** | | | |
| Specific type | `controllers:UserController` | The UserController type | Just the `UserController` struct |
| Type with dot notation | `controllers.UserController` | Same as above (convenience) | Just the `UserController` struct |
| Wildcard type | `*:*Controller` | All types ending with Controller | `UserController`, `ProductController` |
| All types in package | `models:*` | All types in models package | `User`, `Product` |
| **Method Patterns** | | | |
| Specific method | `controllers:UserController:GetUser` | GetUser method | Just the `GetUser` method |
| Dot notation method | `controllers.UserController.GetUser` | Same as above | Just the `GetUser` method |
| Method wildcard | `controllers:*:Get*` | All Get methods in controllers | `GetUser`, `GetProduct` |
| Any Get method | `*:*:Get*` | All Get methods anywhere | `GetUser`, `GetProduct` |
| All methods of type | `services:UserService:*` | All UserService methods | `ValidateUser`, `ProcessUserBatch` |
| **Field Patterns** | | | |
| Specific field | `models:User:ID` | ID field of User | Just the `ID` field |
| All fields | `models:User:*` | All User fields | `ID`, `Name`, `Email` |
| Field in any type | `*:*:Email` | Email field in any type | `User.Email` |
| Wildcard field | `models:*:*:*` | All fields in all models types | All fields in `User` and `Product` |
| **Complex Patterns** | | | |
| Service methods | `services:*:*` | All methods in all service types | All methods in `UserService` and `EmailService` |
| Controller Get methods | `controllers:*:Get*` | Get methods in controllers | `GetUser`, `GetProduct` |
| User-related | `*:User*:*` | All methods in User-related types | All methods in `UserController`, `UserService` |
| **Smart Patterns** | | | |
| Single method name | `GetUser` | GetUser method anywhere | All `GetUser` methods in any type/package |
| Single type name | `UserController` | UserController type anywhere | All `UserController` types in any package |
| Type:Method shorthand | `UserController:GetUser` | Specific method in type | `GetUser` in `UserController` (any package) |
| Method with prefix | `Get*` | All Get methods anywhere | All methods starting with `Get` |
| Type with suffix | `*Controller` | All Controller types | All types ending with `Controller` |

### Metric Queries

Use `--query` flag with metric conditions to find code quality issues.

#### Metric Query Syntax
Use function notation: `metric(pattern) operator value`

| Query | Purpose | Example Results |
|-------|---------|-----------------|
| `lines(*) > 100` | Find large functions/classes | Methods or types with >100 lines |
| `cyclomatic(*) >= 10` | Find complex methods | `ProcessUserBatch` if complexity ≥ 10 |
| `lines(*:*:*) > 50` | Large methods only | Any method exceeding 50 lines |
| `cyclomatic(services:*) > 5` | Complex service methods | Service methods with high complexity |
| `params(*) > 3` | Methods with many parameters | Methods taking >3 parameters |
| `returns(controllers:*) != 2` | Non-standard returns | Controller methods not returning (value, error) |
| `lines(*) < 5` | Find very small functions | Potentially trivial methods |
| `len(*) > 40` | Find nodes with long names | Methods/types with names exceeding 40 characters |
| `imports(*) > 10` | Find modules with many imports | Nodes importing more than 10 dependencies |
| `calls(*) > 5` | Find methods with many external calls | Methods calling outside their package frequently |

#### Available Metrics
- **lines**: Line count of the node
- **cyclomatic**: Cyclomatic complexity (control flow complexity)
- **parameters** / **params**: Number of method parameters
- **returns**: Number of return values
- **len**: Length of the node's full name (package:type:method:field)
- **imports**: Number of import relationships
- **calls**: Number of external call relationships (calls outside the package)



### Working Directory Control

The `--cwd` flag allows you to analyze different directories without changing your current location:

```bash
# Analyze a different project
arch-unit ast --cwd ~/projects/backend "controllers:*"

# Analyze multiple projects
arch-unit ast --cwd ~/projects/service1 "*.lines > 100"
arch-unit ast --cwd ~/projects/service2 "*.lines > 100"

# Use in scripts
PROJECT_DIR="/var/app"
arch-unit ast --cwd "$PROJECT_DIR" --query "*.cyclomatic > 15"
```

### Real-World Examples

```bash
# Smart pattern matching - single strings work intuitively
arch-unit ast "GetUser"              # Find all GetUser methods
arch-unit ast "UserController"       # Find all UserController types
arch-unit ast "processData"          # Find all processData methods

# Type:Method shorthand - assumes any package
arch-unit ast "UserController:GetUser"    # Find GetUser in UserController
arch-unit ast "Service:ValidateUser"      # Find ValidateUser in Service types

# Traditional patterns still work
arch-unit ast "*Repository"          # Find all database repository types
arch-unit ast "*:*:Test*"           # Find all test methods (often have Test prefix)
arch-unit ast "handlers:*:Handle*"  # Find all HTTP handlers (often have Handle prefix)

# Find large classes that might need refactoring
arch-unit ast --query "lines(*) > 500" --format table

# Find highly complex methods that need simplification
arch-unit ast --query "cyclomatic(*) >= 20" --format table

# Find methods with very long names (might need renaming)
arch-unit ast --query "len(*:*:*) > 50" --format table

# Find modules that import too many dependencies
arch-unit ast --query "imports(*) > 15" --format json

# Find methods that make too many external calls
arch-unit ast --query "calls(*:*:*) > 8" --format table

# Analyze a microservice for API endpoints
arch-unit ast --cwd ./user-service "controllers:*:*"

# Find all fields named ID across all types
arch-unit ast "*:*:*:ID"

# Get overview statistics
arch-unit ast --format tree

# File filtering examples - analyze only specific files
arch-unit ast --include "*.go" --exclude "*_test.go"        # Only Go files, no tests
arch-unit ast --include "src/**/*.py" --include "lib/**/*.py" # Python files in specific dirs
arch-unit ast --exclude "vendor/**" --exclude "node_modules/**" # Exclude vendor dirs
arch-unit ast "Controller*" --include "internal/**/*.go"    # Controllers in internal packages only
arch-unit ast --query "lines(*) > 100" --include "pkg/**/*.go" --exclude "*_test.go" # Large functions in pkg, no tests
```

## Command Line Usage

### Check Command

```bash
# Check current directory
arch-unit check

# Check specific directory
arch-unit check ./src

# Output formats
arch-unit check -j                    # JSON output
arch-unit check --csv -o report.csv   # CSV file
arch-unit check --html -o report.html # HTML report
arch-unit check --markdown            # Markdown table

# Fail on violations (exit code 1)
arch-unit check --fail-on-violation

# Filter files
arch-unit check --include "*.go" --exclude "*_test.go"

# Debounce to prevent rapid re-runs
arch-unit check --debounce=30s
```

### Init Command

```bash
# Initialize in current directory
arch-unit init

# Initialize in specific directory
arch-unit init ./src

# Force overwrite existing file
arch-unit init --force
```

## How It Works

1. **Rule Discovery**: Walks the directory tree to find all `.ARCHUNIT` files
2. **AST Parsing**: Parses Go and Python source files into Abstract Syntax Trees
3. **Call Analysis**: Identifies all method calls, field accesses, and imports
4. **Rule Matching**: Applies rules based on file location and inheritance
5. **Violation Reporting**: Reports violations with detailed location information

## Rule Precedence

Rules are applied with the following precedence:

1. Most specific directory rules first (deepest in tree)
2. Override rules (`+pattern`) can relax parent restrictions
3. Deny rules (`!pattern`) take precedence over allow rules
4. Method-specific rules override package-level rules

## Integration

### CI/CD Pipeline

```yaml
# GitHub Actions
- name: Check Architecture
  run: |
    arch-unit check --fail-on-violation
```

### Pre-commit Hook

```bash
#!/bin/sh
arch-unit check --fail-on-violation
```

### Docker

```dockerfile
FROM golang:1.23-alpine
RUN go install github.com/flanksource/arch-unit@latest
WORKDIR /app
COPY . .
RUN arch-unit check --fail-on-violation
```

## Development

```bash
# Install dependencies
make mod

# Run tests
make test

# Lint code
make lint

# Build binary
make build

# Run locally
make run
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

Apache License 2.0