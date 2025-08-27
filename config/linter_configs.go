package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/commons/logger"
)

// LinterConfigPatterns defines the configuration files each linter looks for
var LinterConfigPatterns = map[string][]string{
	"golangci-lint": {
		".golangci.yml",
		".golangci.yaml", 
		".golangci.toml",
		".golangci.json",
	},
	"ruff": {
		"ruff.toml",
		".ruff.toml",
		"pyproject.toml", // Special case - needs [tool.ruff] section
	},
	"eslint": {
		"eslint.config.js",
		"eslint.config.mjs",
		"eslint.config.cjs",
		".eslintrc.js",
		".eslintrc.cjs",
		".eslintrc.json",
		".eslintrc.yml",
		".eslintrc.yaml",
		".eslintrc", // Can be JSON or INI format
	},
	"vale": {
		".vale.ini",
		"_vale.ini",
	},
	"markdownlint": {
		".markdownlint.json",
		".markdownlint.jsonc",
		".markdownlint.yaml",
		".markdownlint.yml",
		".markdownlintrc", // Can be JSON or INI format
	},
	"pyright": {
		"pyrightconfig.json",
		"pyproject.toml", // Special case - needs [tool.pyright] section
	},
}

// DetectLinterConfigs scans the project directory for linter configuration files
// Returns a map of linter names to whether their config was found
func DetectLinterConfigs(rootDir string) (map[string]bool, error) {
	configsFound := make(map[string]bool)
	
	// Initialize all linters as not found
	for linter := range LinterConfigPatterns {
		configsFound[linter] = false
	}
	
	// Check for each linter's config files
	for linter, patterns := range LinterConfigPatterns {
		for _, pattern := range patterns {
			configPath := filepath.Join(rootDir, pattern)
			
			// Check if file exists
			if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
				// Special handling for pyproject.toml
				if pattern == "pyproject.toml" {
					if hasToolSection(configPath, linter) {
						configsFound[linter] = true
						logger.Debugf("Found %s config in %s", linter, pattern)
						break
					}
				} else {
					configsFound[linter] = true
					logger.Debugf("Found %s config: %s", linter, pattern)
					break
				}
			}
		}
	}
	
	// Also check parent directories for some configs (like .markdownlintrc)
	// that can be inherited from parent directories
	if !configsFound["markdownlint"] {
		if found := searchParentDirs(rootDir, ".markdownlintrc"); found {
			configsFound["markdownlint"] = true
			logger.Debugf("Found markdownlint config in parent directory")
		}
	}
	
	return configsFound, nil
}

// hasToolSection checks if a pyproject.toml file has a [tool.linter] section
func hasToolSection(pyprojectPath string, linter string) bool {
	data, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return false
	}
	
	// Convert content to string for simple searching
	content := string(data)
	
	// Map linter names to their tool sections
	toolSections := map[string][]string{
		"ruff": {"[tool.ruff]", "[tool.ruff."},
		"pyright": {"[tool.pyright]", "[tool.pyright."},
	}
	
	if sections, ok := toolSections[linter]; ok {
		for _, section := range sections {
			if strings.Contains(content, section) {
				return true
			}
		}
	}
	
	return false
}

// searchParentDirs searches for a file in parent directories
func searchParentDirs(startDir string, filename string) bool {
	currentDir, err := filepath.Abs(startDir)
	if err != nil {
		return false
	}
	
	for {
		configPath := filepath.Join(currentDir, filename)
		if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
			return true
		}
		
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached root directory
			break
		}
		currentDir = parent
	}
	
	return false
}

// GetLinterConfigInfo returns human-readable information about found configs
func GetLinterConfigInfo(configs map[string]bool) string {
	var found []string
	var notFound []string
	
	for linter, hasConfig := range configs {
		if hasConfig {
			found = append(found, linter)
		} else {
			notFound = append(notFound, linter)
		}
	}
	
	info := ""
	if len(found) > 0 {
		info += "Found configs for: " + strings.Join(found, ", ")
	}
	if len(notFound) > 0 {
		if info != "" {
			info += "\n"
		}
		info += "No configs found for: " + strings.Join(notFound, ", ")
	}
	
	return info
}

// ShouldEnableLinter determines if a linter should be enabled based on:
// 1. Whether its config file exists
// 2. Whether it's explicitly enabled in arch-unit.yaml
// 3. Whether it supports the detected languages
func ShouldEnableLinter(linterName string, hasConfig bool, explicitlyEnabled *bool, supportsLanguage bool) bool {
	// If explicitly configured in arch-unit.yaml, respect that setting
	if explicitlyEnabled != nil {
		return *explicitlyEnabled
	}
	
	// Otherwise, only enable if:
	// 1. Config file exists, AND
	// 2. The linter supports one of the detected languages
	return hasConfig && supportsLanguage
}

// MergeLinterConfigs merges detected config-based enablement with explicit config
func MergeLinterConfigs(detected map[string]bool, explicit map[string]bool) map[string]bool {
	result := make(map[string]bool)
	
	// Start with detected configs
	for linter, hasConfig := range detected {
		result[linter] = hasConfig
	}
	
	// Override with explicit settings
	for linter, enabled := range explicit {
		result[linter] = enabled
	}
	
	return result
}