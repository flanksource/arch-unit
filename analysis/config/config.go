package config

// ASTConfig represents the configuration for AST analysis
type ASTConfig struct {
	Version   string             `yaml:"version"`
	Analyzers []AnalyzerConfig   `yaml:"analyzers"`
}

// AnalyzerConfig represents configuration for a specific analyzer
type AnalyzerConfig struct {
	Path     string                 `yaml:"path"`     // Glob pattern or specific path
	Analyzer string                 `yaml:"analyzer"` // "sql", "openapi", "custom"
	Options  map[string]interface{} `yaml:"options"`  // Analyzer-specific options
}

// SQLOptions represents options for SQL analyzer
type SQLOptions struct {
	Dialect        string `yaml:"dialect"`         // "postgresql", "mysql", "sqlite", etc.
	ConnectionString string `yaml:"connection"`     // Database connection string
	SchemaFilter   string `yaml:"schema_filter"`   // Filter for specific schemas
}

// OpenAPIOptions represents options for OpenAPI analyzer
type OpenAPIOptions struct {
	Version string `yaml:"version"` // "3.0", "3.1", etc.
	URL     string `yaml:"url"`     // URL to fetch OpenAPI spec from
}

// CustomOptions represents options for custom analyzer
type CustomOptions struct {
	Command       string            `yaml:"command"`        // Command to execute
	FieldMappings map[string]string `yaml:"field_mappings"` // Field mapping configuration
}

// DefaultConfig returns a default configuration
func DefaultConfig() *ASTConfig {
	return &ASTConfig{
		Version:   "1.0",
		Analyzers: []AnalyzerConfig{},
	}
}

// GetSQLOptions extracts SQL-specific options from an AnalyzerConfig
func (ac *AnalyzerConfig) GetSQLOptions() *SQLOptions {
	opts := &SQLOptions{}

	if ac.Options == nil {
		return opts
	}

	if dialect, ok := ac.Options["dialect"].(string); ok {
		opts.Dialect = dialect
	}

	if connection, ok := ac.Options["connection"].(string); ok {
		opts.ConnectionString = connection
	}

	if schemaFilter, ok := ac.Options["schema_filter"].(string); ok {
		opts.SchemaFilter = schemaFilter
	}

	return opts
}

// GetOpenAPIOptions extracts OpenAPI-specific options from an AnalyzerConfig
func (ac *AnalyzerConfig) GetOpenAPIOptions() *OpenAPIOptions {
	opts := &OpenAPIOptions{}

	if ac.Options == nil {
		return opts
	}

	if version, ok := ac.Options["version"].(string); ok {
		opts.Version = version
	}

	if url, ok := ac.Options["url"].(string); ok {
		opts.URL = url
	}

	return opts
}

// GetCustomOptions extracts custom analyzer options from an AnalyzerConfig
func (ac *AnalyzerConfig) GetCustomOptions() *CustomOptions {
	opts := &CustomOptions{
		FieldMappings: make(map[string]string),
	}

	if ac.Options == nil {
		return opts
	}

	if command, ok := ac.Options["command"].(string); ok {
		opts.Command = command
	}

	if fieldMappings, ok := ac.Options["field_mappings"].(map[string]interface{}); ok {
		for key, value := range fieldMappings {
			if strValue, ok := value.(string); ok {
				opts.FieldMappings[key] = strValue
			}
		}
	}

	return opts
}