package languages

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

// DetectLanguagesInDirectory scans a directory and detects all programming languages present
func DetectLanguagesInDirectory(rootDir string) ([]string, error) {
	languageSet := make(map[string]bool)

	// Get built-in exclusion patterns
	excludePatterns := models.GetBuiltinExcludePatterns()

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Get relative path for pattern matching
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			relPath = path
		}

		// Check if path should be excluded
		if shouldExclude(relPath, excludePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Detect language for non-directory files
		if !info.IsDir() {
			lang := DetectLanguage(path)
			if lang != "unknown" {
				languageSet[lang] = true
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Convert set to slice
	var languages []string
	for lang := range languageSet {
		languages = append(languages, lang)
	}

	return languages, nil
}

// shouldExclude checks if a path matches any exclusion pattern
func shouldExclude(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Simple pattern matching (could be enhanced with proper glob matching)
		if matchesPattern(path, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern performs simple pattern matching
func matchesPattern(path, pattern string) bool {
	// Handle directory patterns like "examples/**"
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		if strings.HasPrefix(path, prefix+"/") || path == prefix {
			return true
		}
	}

	// Handle specific directory patterns like ".git/**"
	if strings.Contains(pattern, "/") {
		if strings.HasPrefix(path, strings.TrimSuffix(pattern, "/**")+"/") {
			return true
		}
	}

	// Handle file patterns like "*.min.*"
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	// Handle exact matches
	if path == pattern {
		return true
	}

	// Check if any parent directory matches
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if matchesSimplePattern(part, pattern) {
			return true
		}
	}

	return false
}

// matchesSimplePattern matches simple patterns without paths
func matchesSimplePattern(name, pattern string) bool {
	// Remove /** suffix if present
	pattern = strings.TrimSuffix(pattern, "/**")

	// Direct match
	if name == pattern {
		return true
	}

	// Handle patterns like "__pycache__"
	if pattern == name {
		return true
	}

	return false
}

// GetDefaultIncludesForLanguage returns the default include patterns for a language
func GetDefaultIncludesForLanguage(language string) []string {
	switch language {
	case "go":
		return []string{"**/*.go"}
	case "python":
		return []string{"**/*.py", "**/*.pyi"}
	case "java":
		return []string{"**/*.java"}
	case "javascript":
		return []string{"**/*.js", "**/*.jsx", "**/*.mjs", "**/*.cjs"}
	case "typescript":
		return []string{"**/*.ts", "**/*.tsx"}
	case "rust":
		return []string{"**/*.rs"}
	case "ruby":
		return []string{"**/*.rb"}
	case "markdown":
		return []string{"**/*.md", "**/*.mdx", "**/*.markdown"}
	default:
		return []string{}
	}
}
