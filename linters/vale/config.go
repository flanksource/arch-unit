package vale

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

// GenerateValeConfig generates a temporary .vale.ini configuration file
// that respects the all_language_excludes macro
func GenerateValeConfig(workDir string, config *models.Config, language string, linterDefaults []string) (string, error) {
	// Create a temporary directory for Vale config
	tempDir := filepath.Join(workDir, ".arch-unit-vale-temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir for Vale config: %w", err)
	}

	configPath := filepath.Join(tempDir, ".vale.ini")

	// Get effective excludes using the all_language_excludes macro
	var excludes []string
	if config != nil {
		excludes = config.GetAllLanguageExcludes(language, linterDefaults)
	} else {
		// Fallback to built-in excludes
		excludes = append(models.GetBuiltinExcludePatterns(), linterDefaults...)
	}

	// Build Vale configuration content
	var configContent strings.Builder
	configContent.WriteString("# Auto-generated Vale configuration by arch-unit\n")
	configContent.WriteString("# This file respects the all_language_excludes macro\n\n")
	
	// Basic Vale settings - use built-in styles if no custom styles installed
	configContent.WriteString("# Vale requires at least minimal configuration\n")
	configContent.WriteString("StylesPath = .vale-styles\n")
	configContent.WriteString("MinAlertLevel = suggestion\n\n")
	
	// Format-specific settings
	configContent.WriteString("[*.md]\n")
	configContent.WriteString("# Markdown-specific settings\n")
	configContent.WriteString("BasedOnStyles = Vale\n\n")

	// Add exclusions as glob patterns
	if len(excludes) > 0 {
		configContent.WriteString("# Exclusion patterns from all_language_excludes macro\n")
		configContent.WriteString("[*]\n")
		configContent.WriteString("SkippedScopes = code, tt\n")
		configContent.WriteString("IgnoredScopes = code, tt\n\n")
		
		// Vale uses BlockIgnores for file patterns
		configContent.WriteString("BlockIgnores = ")
		for i, pattern := range excludes {
			if i > 0 {
				configContent.WriteString(", ")
			}
			// Convert patterns to Vale format
			valePattern := convertToValePattern(pattern)
			configContent.WriteString(valePattern)
		}
		configContent.WriteString("\n\n")
	}

	// Add formats section
	configContent.WriteString("[formats]\n")
	configContent.WriteString("md = md\n")
	configContent.WriteString("markdown = md\n")
	configContent.WriteString("mdx = md\n")
	configContent.WriteString("txt = md\n")
	configContent.WriteString("rst = rst\n")
	configContent.WriteString("adoc = asciidoc\n")
	configContent.WriteString("html = html\n")
	configContent.WriteString("xml = xml\n")

	// Write config file
	if err := os.WriteFile(configPath, []byte(configContent.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write Vale config: %w", err)
	}

	return configPath, nil
}

// convertToValePattern converts arch-unit patterns to Vale glob patterns
func convertToValePattern(pattern string) string {
	// Vale uses standard glob patterns but with some differences
	// Convert ** patterns to single * for Vale
	pattern = strings.ReplaceAll(pattern, "**", "*")
	
	// Remove trailing slash patterns
	pattern = strings.TrimSuffix(pattern, "/")
	
	// Vale patterns are relative to the working directory
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
	}
	
	return pattern
}

// CleanupValeConfig removes temporary Vale configuration
func CleanupValeConfig(workDir string) {
	tempDir := filepath.Join(workDir, ".arch-unit-vale-temp")
	os.RemoveAll(tempDir)
}