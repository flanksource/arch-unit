package config

import (
	"github.com/flanksource/arch-unit/languages"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
)

// DefaultLintersByLanguage maps languages to their recommended linters
var DefaultLintersByLanguage = map[string][]string{
	"go":         {"golangci-lint"},
	"python":     {"ruff", "pyright"},
	"java":       {"checkstyle", "spotbugs"},
	"javascript": {"eslint"},
	"typescript": {"eslint"},
	"rust":       {"rustfmt", "clippy"},
	"ruby":       {"rubocop"},
	"markdown":   {"markdownlint", "vale"},
}

// CreateSmartDefaultConfig creates a configuration based on detected languages and linter configs
func CreateSmartDefaultConfig(rootDir string) (*models.Config, error) {
	// Detect languages in the directory
	detectedLanguages, err := languages.DetectLanguagesInDirectory(rootDir)
	if err != nil {
		logger.Warnf("Failed to detect languages: %v", err)
		// Fall back to a minimal default config
		return CreateMinimalDefaultConfig(), nil
	}

	if len(detectedLanguages) == 0 {
		logger.Infof("No supported languages detected, using minimal configuration")
		return CreateMinimalDefaultConfig(), nil
	}

	logger.Infof("Detected languages: %v", detectedLanguages)

	// Detect linter configuration files
	linterConfigs, err := DetectLinterConfigs(rootDir)
	if err != nil {
		logger.Warnf("Failed to detect linter configs: %v", err)
		linterConfigs = make(map[string]bool)
	}

	config := &models.Config{
		Version:   "1.0",
		Debounce:  "30s",
		Languages: make(map[string]models.LanguageConfig),
		Linters:   make(map[string]models.LinterConfig),
		Rules:     make(map[string]models.RuleConfig),
	}

	// Track which linters we're enabling
	enabledLinters := make(map[string]bool)

	// Add language configs and enable appropriate linters based on config detection
	for _, lang := range detectedLanguages {
		// Add language configuration
		config.Languages[lang] = models.LanguageConfig{
			Includes: languages.GetDefaultIncludesForLanguage(lang),
		}

		// Check which linters to enable for this language
		if linters, ok := DefaultLintersByLanguage[lang]; ok {
			for _, linterName := range linters {
				// Only enable if config file exists
				if hasConfig, found := linterConfigs[linterName]; found && hasConfig {
					if !enabledLinters[linterName] {
						config.Linters[linterName] = models.LinterConfig{
							Enabled: true,
							// No args or output_format - let the system handle JSON internally
						}
						enabledLinters[linterName] = true
						logger.Debugf("Enabling %s (config found)", linterName)
					}
				} else {
					logger.Debugf("Not enabling %s (no config found)", linterName)
				}
			}
		}
	}

	// Add a default rule configuration for all files
	config.Rules["**"] = models.RuleConfig{
		// Rules can be added by users as needed
	}

	return config, nil
}

// CreateMinimalDefaultConfig creates a minimal default configuration when no languages are detected
func CreateMinimalDefaultConfig() *models.Config {
	return &models.Config{
		Version:   "1.0",
		Debounce:  "30s",
		Rules:     make(map[string]models.RuleConfig),
		Linters:   make(map[string]models.LinterConfig),
		Languages: map[string]models.LanguageConfig{
			// Minimal config - users can add languages as needed
		},
	}
}
