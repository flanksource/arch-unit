package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigFileName = ".arch-ast.yaml"
	AlternateConfigFileName = ".arch-ast.yml"
)

// ConfigLoader loads AST configuration from files
type ConfigLoader struct {
	workingDir string
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(workingDir string) *ConfigLoader {
	return &ConfigLoader{
		workingDir: workingDir,
	}
}

// LoadConfig loads configuration from the default location or specified file
func (cl *ConfigLoader) LoadConfig(configPath string) (*ASTConfig, error) {
	var configFile string

	if configPath != "" {
		// Use specified config file
		if filepath.IsAbs(configPath) {
			configFile = configPath
		} else {
			configFile = filepath.Join(cl.workingDir, configPath)
		}
	} else {
		// Look for default config files
		configFile = cl.findDefaultConfig()
		if configFile == "" {
			// No config file found, return default config
			return DefaultConfig(), nil
		}
	}

	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if configPath != "" {
			// Specified config file doesn't exist
			return nil, fmt.Errorf("config file not found: %s", configFile)
		}
		// Default config file doesn't exist, return default config
		return DefaultConfig(), nil
	}

	// Load and parse the config file
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
	}

	var config ASTConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configFile, err)
	}

	// Validate the configuration
	if err := cl.validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration in %s: %w", configFile, err)
	}

	return &config, nil
}

// findDefaultConfig looks for default configuration files in the working directory
func (cl *ConfigLoader) findDefaultConfig() string {
	candidates := []string{
		filepath.Join(cl.workingDir, DefaultConfigFileName),
		filepath.Join(cl.workingDir, AlternateConfigFileName),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

// validateConfig validates the loaded configuration
func (cl *ConfigLoader) validateConfig(config *ASTConfig) error {
	if config.Version == "" {
		return fmt.Errorf("version is required")
	}

	if config.Version != "1.0" {
		return fmt.Errorf("unsupported configuration version: %s", config.Version)
	}

	for i, analyzer := range config.Analyzers {
		if err := cl.validateAnalyzerConfig(analyzer, i); err != nil {
			return fmt.Errorf("analyzer %d: %w", i, err)
		}
	}

	return nil
}

// validateAnalyzerConfig validates an individual analyzer configuration
func (cl *ConfigLoader) validateAnalyzerConfig(analyzer AnalyzerConfig, index int) error {
	if analyzer.Path == "" {
		return fmt.Errorf("path is required")
	}

	if analyzer.Analyzer == "" {
		return fmt.Errorf("analyzer type is required")
	}

	switch analyzer.Analyzer {
	case "sql":
		return cl.validateSQLOptions(analyzer.GetSQLOptions())
	case "openapi":
		return cl.validateOpenAPIOptions(analyzer.GetOpenAPIOptions())
	case "custom":
		return cl.validateCustomOptions(analyzer.GetCustomOptions())
	default:
		return fmt.Errorf("unsupported analyzer type: %s", analyzer.Analyzer)
	}
}

// validateSQLOptions validates SQL analyzer options
func (cl *ConfigLoader) validateSQLOptions(opts *SQLOptions) error {
	if opts.Dialect == "" {
		return fmt.Errorf("sql analyzer requires dialect option")
	}

	supportedDialects := []string{"postgresql", "mysql", "sqlite", "sqlserver", "oracle"}
	dialectSupported := false
	for _, supported := range supportedDialects {
		if opts.Dialect == supported {
			dialectSupported = true
			break
		}
	}

	if !dialectSupported {
		return fmt.Errorf("unsupported SQL dialect: %s", opts.Dialect)
	}

	return nil
}

// validateOpenAPIOptions validates OpenAPI analyzer options
func (cl *ConfigLoader) validateOpenAPIOptions(opts *OpenAPIOptions) error {
	if opts.Version == "" {
		// Default to 3.0 if not specified
		opts.Version = "3.0"
	}

	supportedVersions := []string{"3.0", "3.1"}
	versionSupported := false
	for _, supported := range supportedVersions {
		if opts.Version == supported {
			versionSupported = true
			break
		}
	}

	if !versionSupported {
		return fmt.Errorf("unsupported OpenAPI version: %s", opts.Version)
	}

	return nil
}

// validateCustomOptions validates custom analyzer options
func (cl *ConfigLoader) validateCustomOptions(opts *CustomOptions) error {
	if opts.Command == "" {
		return fmt.Errorf("custom analyzer requires command option")
	}

	return nil
}

// SaveConfig saves configuration to the specified file
func (cl *ConfigLoader) SaveConfig(config *ASTConfig, configPath string) error {
	var configFile string

	if filepath.IsAbs(configPath) {
		configFile = configPath
	} else {
		configFile = filepath.Join(cl.workingDir, configPath)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}