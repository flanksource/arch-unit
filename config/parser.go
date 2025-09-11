package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
	"gopkg.in/yaml.v3"
)

const ConfigFileName = "arch-unit.yaml"

type Parser struct {
	rootDir string
}

func NewParser(rootDir string) *Parser {
	return &Parser{
		rootDir: rootDir,
	}
}

// findGitRoot finds the git root directory by walking up from startDir
func findGitRoot(startDir string) string {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory, no git repo found
			return startDir
		}
		dir = parent
	}
}

// findConfigFile searches for a config file by walking up the directory tree
func (p *Parser) findConfigFile(startDir, fileName string) (string, error) {
	gitRoot := findGitRoot(startDir)
	dir := startDir
	
	for {
		configPath := filepath.Join(dir, fileName)
		if _, err := os.Stat(configPath); err == nil {
			logger.Debugf("Found config file: %s", configPath)
			return configPath, nil
		}
		
		// Don't go above git root
		if dir == gitRoot {
			break
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory
			break
		}
		dir = parent
	}
	
	return "", fmt.Errorf("configuration file %s not found in directory tree from %s to %s", fileName, startDir, gitRoot)
}

// LoadConfig loads the arch-unit.yaml configuration file
func (p *Parser) LoadConfig() (*models.Config, error) {
	// Try to find config file by walking up the directory tree
	configPath, err := p.findConfigFile(p.rootDir, ConfigFileName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	var config models.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML configuration: %w", err)
	}

	// Validate configuration
	if err := p.validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// validateConfig performs basic validation on the configuration
func (p *Parser) validateConfig(config *models.Config) error {
	if config.Version == "" {
		config.Version = "1.0" // Default version
	}

	// Validate debounce duration if specified
	if config.Debounce != "" {
		if _, err := config.GetDebounceDuration(); err != nil {
			return fmt.Errorf("invalid global debounce duration '%s': %w", config.Debounce, err)
		}
	}

	// Validate rule configs
	for pattern, ruleConfig := range config.Rules {
		if err := p.validateRuleConfig(pattern, &ruleConfig); err != nil {
			return fmt.Errorf("invalid rule config for pattern '%s': %w", pattern, err)
		}
	}

	// Validate linter configs
	for name, linterConfig := range config.Linters {
		if err := p.validateLinterConfig(name, &linterConfig); err != nil {
			return fmt.Errorf("invalid linter config for '%s': %w", name, err)
		}
	}

	return nil
}

// validateRuleConfig validates a rule configuration
func (p *Parser) validateRuleConfig(pattern string, config *models.RuleConfig) error {
	// Validate debounce duration if specified
	if config.Debounce != "" {
		if _, err := config.GetDebounceDuration(); err != nil {
			return fmt.Errorf("invalid debounce duration '%s': %w", config.Debounce, err)
		}
	}

	// Validate import rules format
	for i, importRule := range config.Imports {
		if err := p.validateImportRule(importRule); err != nil {
			return fmt.Errorf("invalid import rule #%d '%s': %w", i+1, importRule, err)
		}
	}

	// Validate nested linter configs
	for name, linterConfig := range config.Linters {
		if err := p.validateLinterConfig(name, &linterConfig); err != nil {
			return fmt.Errorf("invalid nested linter config for '%s': %w", name, err)
		}
	}

	return nil
}

// validateImportRule validates an import rule format
func (p *Parser) validateImportRule(rule string) error {
	if rule == "" {
		return fmt.Errorf("empty import rule")
	}

	// Remove prefix for validation
	cleanRule := rule
	if cleanRule[0] == '+' || cleanRule[0] == '!' {
		cleanRule = cleanRule[1:]
	}

	if cleanRule == "" {
		return fmt.Errorf("rule cannot be just a prefix")
	}

	// Basic format validation - could be enhanced
	return nil
}

// validateLinterConfig validates a linter configuration
func (p *Parser) validateLinterConfig(name string, config *models.LinterConfig) error {
	// Validate debounce duration if specified
	if config.Debounce != "" {
		if _, err := config.GetDebounceDuration(); err != nil {
			return fmt.Errorf("invalid debounce duration '%s': %w", config.Debounce, err)
		}
	}

	// Validate output format
	if config.OutputFormat != "" {
		validFormats := []string{"json", "text", "xml", "junit"}
		isValid := false
		for _, format := range validFormats {
			if config.OutputFormat == format {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid output format '%s', must be one of: %v", config.OutputFormat, validFormats)
		}
	}

	return nil
}

// GetRulesForFile returns the applicable rules for a given file path
func (p *Parser) GetRulesForFile(filePath string, config *models.Config) (*models.RuleSet, error) {
	return config.GetRulesForFile(filePath)
}

// FindConfigFile searches for arch-unit.yaml in the directory tree
func FindConfigFile(startDir string) (string, error) {
	currentDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	// Search up the directory tree
	for {
		configPath := filepath.Join(currentDir, ConfigFileName)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached root directory
			break
		}
		currentDir = parent
	}

	return "", fmt.Errorf("no %s found in directory tree starting from %s", ConfigFileName, startDir)
}

// CreateDefaultConfig creates a default arch-unit.yaml configuration
func CreateDefaultConfig() *models.Config {
	return &models.Config{
		Version:  "1.0",
		Debounce: "30s",
		Rules: map[string]models.RuleConfig{
			"**": {

				Linters: map[string]models.LinterConfig{
					"golangci-lint": {
						Enabled:      true,
						Args:         []string{"--timeout=5m"},
						OutputFormat: "json",
					},
				},
			},
		},
		Linters: map[string]models.LinterConfig{
			"golangci-lint": {
				Enabled:      true,
				Debounce:     "60s",
				Args:         []string{"--timeout=10m", "--issues-exit-code=0"},
				OutputFormat: "json",
			},
			"ruff": {
				Enabled:      true,
				Debounce:     "30s",
				Args:         []string{"--format=json"},
				OutputFormat: "json",
			},
		},
	}
}
