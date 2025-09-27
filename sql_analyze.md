# SQL Analysis Plan: AST Analysis with Path-Based Configuration & dbtpl Integration

## Overview
Extend arch-unit AST analysis to support SQL databases and OpenAPI/Spring REST with:
1. **Phase 1**: Path-based configuration system + reanalysis detection
2. **Phase 2**: SQL (using dbtpl) and OpenAPI AST extractors with direct analysis capability

## Phase 1: Data Model & Configuration Infrastructure

### 1.1 Data Model Extension (Reusing Existing Fields)

**New NodeType Constants** (in `models/ast.go`):
```go
// SQL Database node types (as sub-types)
NodeTypeTypeTable        NodeType = "type_table"        // Tables as sub-type of "type"
NodeTypeTypeView         NodeType = "type_view"         // Views as sub-type of "type"
NodeTypeMethodStoredProc NodeType = "method_stored_proc" // Stored procedures as sub-type of "method"
NodeTypeMethodFunction   NodeType = "method_function"   // SQL functions as sub-type of "method"
NodeTypeFieldColumn      NodeType = "field_column"      // Columns as sub-type of "field"

// HTTP/REST node types (as sub-types)
NodeTypeMethodHTTPGet    NodeType = "method_http_get"    // GET endpoints as sub-type of "method"
NodeTypeMethodHTTPPost   NodeType = "method_http_post"   // POST endpoints as sub-type of "method"
NodeTypeMethodHTTPPut    NodeType = "method_http_put"    // PUT endpoints as sub-type of "method"
NodeTypeMethodHTTPDelete NodeType = "method_http_delete" // DELETE endpoints as sub-type of "method"
NodeTypeFieldHTTPParam   NodeType = "field_http_param"   // Parameters as sub-type of "field"
NodeTypeTypeHTTPSchema   NodeType = "type_http_schema"   // Schemas as sub-type of "type"
```

### 1.2 Path-Based Configuration System

**Configuration File Format** (`.arch-ast.yaml`):
```yaml
version: "1.0"
analyzers:
  - path: "**/*.sql"
    analyzer: "sql"
    options:
      dialect: "postgresql"

  - path: "migrations/**/*.sql"
    analyzer: "sql"
    options:
      dialect: "mysql"

  - path: "api/openapi.yaml"
    analyzer: "openapi"
    options:
      version: "3.0"

  - path: "**/*-api.yaml"
    analyzer: "openapi"
    options:
      version: "3.1"

  - path: "proto/schema.proto"
    analyzer: "custom"
    options:
      command: "protoc --ast-dump={{.Path}} --format=json"
      field_mappings:
        package: "PackageName"
        service: "TypeName"
        rpc: "MethodName"
        field: "FieldName"
```

**Configuration Structure** (`analysis/config/config.go`):
```go
type ASTConfig struct {
    Version   string             `yaml:"version"`
    Analyzers []AnalyzerConfig   `yaml:"analyzers"`
}

type AnalyzerConfig struct {
    Path     string                 `yaml:"path"`     // Glob pattern or specific path
    Analyzer string                 `yaml:"analyzer"` // "sql", "openapi", "custom"
    Options  map[string]interface{} `yaml:"options"`  // Analyzer-specific options
}
```

### 1.3 Enhanced CLI Commands

**New AST Analyze Commands**:
```bash
# Direct SQL database analysis using dbtpl (creates virtual path)
arch-unit ast analyze sql --connection "postgres://user:pass@host/db" --output db_schema.json

# Direct OpenAPI analysis
arch-unit ast analyze openapi --url "https://api.example.com/openapi.json" --output api_schema.json

# File-based analysis (existing)
arch-unit ast analyze --config .arch-ast.yaml

# Single file analysis
arch-unit ast analyze --file api/openapi.yaml --analyzer openapi
```

### 1.4 Reanalysis Detection (No File Watching)

**Source Change Detection** (`analysis/reanalysis_detector.go`):
```go
type ReanalysisDetector struct {
    cache *cache.ASTCache
}

func (r *ReanalysisDetector) NeedsReanalysis(source AnalysisSource) (bool, error) {
    switch source.Type {
    case "file":
        return r.checkFileChange(source.Path)
    case "sql_connection":
        return r.checkSQLSchemaChange(source.ConnectionString)
    case "openapi_url":
        return r.checkURLChange(source.URL)
    }
}

func (r *ReanalysisDetector) checkSQLSchemaChange(connStr string) (bool, error) {
    // Use dbtpl to get schema metadata hash/timestamp
    schemaHash := calculateSchemaHashViaDbtpl(connStr)
    lastHash, err := r.cache.GetSourceHash("sql:" + connStr)
    return lastHash != schemaHash, err
}
```

### 1.5 Virtual Path Creation for Direct Analysis

**Virtual Path Manager** (`analysis/virtual_paths.go`):
```go
type VirtualPathManager struct{}

func (v *VirtualPathManager) CreateVirtualPath(source AnalysisSource, outputPath string) string {
    switch source.Type {
    case "sql_connection":
        // Create: "virtual://sql/postgresql_localhost_mydb"
        return fmt.Sprintf("virtual://sql/%s", sanitizeConnectionString(source.ConnectionString))

    case "openapi_url":
        // Create: "virtual://openapi/api_example_com"
        return fmt.Sprintf("virtual://openapi/%s", sanitizeURL(source.URL))

    case "custom_output":
        // Use provided output path as virtual path
        return outputPath
    }
}
```

## Phase 2: AST Implementation

### 2.1 SQL Database AST Extractor (using dbtpl)

**SQL Extractor with dbtpl Integration** (`analysis/sql/sql_ast_extractor.go`):
```go
import "github.com/xo/dbtpl"

type SQLASTExtractor struct {
    dialect string
}

func (e *SQLASTExtractor) ExtractFromConnection(connStr string) (*types.ASTResult, error) {
    // Parse connection string to get database details
    dbURL, err := url.Parse(connStr)
    if err != nil {
        return nil, err
    }

    // Use dbtpl to introspect database schema
    loader, err := dbtpl.NewLoader(
        dbtpl.WithDatabase(dbURL.Scheme),
        dbtpl.WithDSN(connStr),
    )
    if err != nil {
        return nil, err
    }

    // Load schema information using dbtpl
    schema, err := loader.LoadSchema()
    if err != nil {
        return nil, err
    }

    // Convert dbtpl schema to AST nodes
    result := types.NewASTResult(virtualPath, "sql")

    // Map dbtpl structures to ASTNode using existing fields:
    for _, table := range schema.Tables {
        // Table as type node
        tableNode := &models.ASTNode{
            PackageName: schema.Name,           // Schema name
            TypeName:    table.Name,            // Table name
            NodeType:    "type_table",
            FilePath:    virtualPath,
            // ... other fields
        }
        result.AddNode(tableNode)

        // Columns as field nodes
        for _, column := range table.Columns {
            columnNode := &models.ASTNode{
                PackageName: schema.Name,       // Schema name
                TypeName:    table.Name,        // Table name (parent)
                FieldName:   column.Name,       // Column name
                NodeType:    "field_column",
                FilePath:    virtualPath,
                // ... other fields
            }
            result.AddNode(columnNode)
        }
    }

    // Handle views, procedures, functions using similar mapping
    for _, view := range schema.Views {
        viewNode := &models.ASTNode{
            PackageName: schema.Name,
            TypeName:    view.Name,
            NodeType:    "type_view",
            FilePath:    virtualPath,
        }
        result.AddNode(viewNode)
    }

    // Handle stored procedures/functions
    for _, proc := range schema.Procedures {
        procNode := &models.ASTNode{
            PackageName: schema.Name,
            MethodName:  proc.Name,
            NodeType:    "method_stored_proc",
            FilePath:    virtualPath,
            // Extract parameters from proc.Parameters
            Parameters:  convertDbtplParams(proc.Parameters),
        }
        result.AddNode(procNode)
    }

    return result, nil
}

func (e *SQLASTExtractor) ExtractFromFile(filepath string, content []byte) (*types.ASTResult, error) {
    // Parse DDL statements from SQL files
    // Could potentially use dbtpl's parsing capabilities for DDL if supported
    // Otherwise implement basic DDL parsing for CREATE TABLE, CREATE VIEW, etc.
}

// Helper function to convert dbtpl parameters to AST parameters
func convertDbtplParams(dbtplParams []dbtpl.Parameter) []models.Parameter {
    params := make([]models.Parameter, len(dbtplParams))
    for i, p := range dbtplParams {
        params[i] = models.Parameter{
            Name: p.Name,
            Type: p.Type,
            NameLength: len(p.Name),
        }
    }
    return params
}
```

**Dependencies** (`go.mod`):
```go
require (
    github.com/xo/dbtpl v0.8.0  // For database schema introspection
)
```

**SQL CLI Integration**:
```bash
# Direct database analysis using dbtpl
arch-unit ast analyze sql --connection "postgres://localhost/mydb"
# Creates virtual path: "virtual://sql/postgres_localhost_mydb"
# Uses dbtpl to introspect schema and convert to AST nodes
```

### 2.2 OpenAPI AST Extractor

**OpenAPI Extractor** (`analysis/openapi/openapi_ast_extractor.go`):
```go
type OpenAPIExtractor struct {
    version string
}

func (e *OpenAPIExtractor) ExtractFromURL(url string) (*types.ASTResult, error) {
    // Fetch OpenAPI spec from URL
    // Parse endpoints, schemas, parameters
    // Map to ASTNode using existing fields:
    //   - PackageName: API version/namespace
    //   - TypeName: schema/resource name
    //   - MethodName: endpoint path
    //   - FieldName: parameter/field name
    //   - NodeType: method_http_get, type_http_schema, field_http_param, etc.
}

func (e *OpenAPIExtractor) ExtractFromFile(filepath string, content []byte) (*types.ASTResult, error) {
    // Parse local OpenAPI file
}
```

### 2.3 Custom Extractor Support

**Command Extractor** (`analysis/extractors/command_extractor.go`):
```go
type CommandExtractor struct {
    Command       string
    FieldMappings map[string]string
}

func (e *CommandExtractor) ExtractFile(cache cache.ReadOnlyCache, filepath string, content []byte) (*types.ASTResult, error) {
    // Execute command with filepath
    // Parse JSON/YAML output
    // Map fields to ASTNode using configuration
}
```

## Implementation Benefits

✅ **dbtpl Integration**: Robust, mature database schema introspection
✅ **Multi-Database Support**: dbtpl supports PostgreSQL, MySQL, SQLite, SQL Server, Oracle
✅ **No File Watching**: Simpler architecture, no background processes
✅ **Reanalysis Detection**: Smart change detection prevents unnecessary work
✅ **Virtual Paths**: Unified interface for files and external sources
✅ **Direct Analysis**: Connect directly to databases and APIs
✅ **Path-Based Config**: Simple, flexible configuration format
✅ **Reuses Existing Fields**: No database schema changes

## File Structure
```
analysis/
├── config/
│   ├── config.go            # Configuration structure
│   ├── config_loader.go     # Load .arch-ast.yaml
│   └── path_matcher.go      # Glob pattern matching
├── sql/
│   ├── sql_ast_extractor.go # SQL database extractor using dbtpl
│   └── dbtpl_converter.go   # Convert dbtpl structures to AST nodes
├── openapi/
│   └── openapi_extractor.go # OpenAPI specification extractor
├── extractors/
│   ├── command_extractor.go # External command wrapper
│   └── yaml_extractor.go    # YAML AST parser
├── reanalysis_detector.go   # Change detection
├── virtual_paths.go         # Virtual path management
└── extractor_registry.go    # Enhanced registry
```

## Field Mapping Strategy

### SQL Database Mapping
- `PackageName` → Database schema name
- `TypeName` → Table/view name
- `MethodName` → Stored procedure/function name
- `FieldName` → Column name
- `NodeType` → "type_table", "method_stored_proc", "field_column", etc.

### OpenAPI/REST Mapping
- `PackageName` → API namespace/version (e.g., "v1", "users")
- `TypeName` → Resource/schema name (e.g., "User", "Product")
- `MethodName` → Endpoint path (e.g., "/users/{id}")
- `FieldName` → Parameter name, response field name
- `NodeType` → "method_http_get", "field_http_param", "type_http_schema", etc.

This approach leverages the mature dbtpl library for robust database schema introspection while maintaining the path-based configuration and virtual path architecture.