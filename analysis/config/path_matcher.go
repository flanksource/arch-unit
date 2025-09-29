package config

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// PathMatcher handles glob pattern matching for file paths
type PathMatcher struct {
	workingDir string
}

// NewPathMatcher creates a new path matcher
func NewPathMatcher(workingDir string) *PathMatcher {
	return &PathMatcher{
		workingDir: workingDir,
	}
}

// MatchAnalyzer finds the appropriate analyzer configuration for a file path
func (pm *PathMatcher) MatchAnalyzer(filePath string, analyzers []AnalyzerConfig) *AnalyzerConfig {
	// Convert absolute path to relative path for matching
	relPath, err := filepath.Rel(pm.workingDir, filePath)
	if err != nil {
		// If we can't get relative path, use the original path
		relPath = filePath
	}

	// Normalize path separators for consistent matching
	relPath = filepath.ToSlash(relPath)

	// Find the first matching analyzer
	for _, analyzer := range analyzers {
		if pm.matchPattern(relPath, analyzer.Path) {
			return &analyzer
		}
	}

	return nil
}

// MatchFiles finds all files that match the given analyzer configurations
func (pm *PathMatcher) MatchFiles(analyzers []AnalyzerConfig) (map[string][]string, error) {
	filesByAnalyzer := make(map[string][]string)

	for i, analyzer := range analyzers {
		analyzerKey := pm.getAnalyzerKey(analyzer, i)

		files, err := pm.findMatchingFiles(analyzer.Path)
		if err != nil {
			return nil, err
		}

		if len(files) > 0 {
			filesByAnalyzer[analyzerKey] = files
		}
	}

	return filesByAnalyzer, nil
}

// matchPattern checks if a file path matches a glob pattern
func (pm *PathMatcher) matchPattern(filePath, pattern string) bool {
	// Handle exact matches
	if pattern == filePath {
		return true
	}

	// Handle glob patterns
	matched, err := doublestar.Match(pattern, filePath)
	if err != nil {
		// If pattern is invalid, treat as literal string
		return pattern == filePath
	}

	return matched
}

// findMatchingFiles finds all files that match the given glob pattern
func (pm *PathMatcher) findMatchingFiles(pattern string) ([]string, error) {
	// If pattern is an absolute path, use it as-is
	var searchPattern string
	if filepath.IsAbs(pattern) {
		searchPattern = pattern
	} else {
		searchPattern = filepath.Join(pm.workingDir, pattern)
	}

	// Use doublestar to find matching files
	matches, err := doublestar.FilepathGlob(searchPattern)
	if err != nil {
		return nil, err
	}

	// Filter to only include regular files
	var files []string
	for _, match := range matches {
		if info, err := filepath.Abs(match); err == nil {
			if stat, err := filepath.Glob(info); err == nil && len(stat) > 0 {
				files = append(files, match)
			}
		}
	}

	return files, nil
}

// getAnalyzerKey creates a unique key for an analyzer configuration
func (pm *PathMatcher) getAnalyzerKey(analyzer AnalyzerConfig, index int) string {
	// Create a key that combines analyzer type and path
	key := analyzer.Analyzer
	if analyzer.Path != "" {
		key += "_" + strings.ReplaceAll(analyzer.Path, "/", "_")
		key = strings.ReplaceAll(key, "*", "wildcard")
	}
	return key
}

// IsVirtualPath checks if a path is a virtual path (e.g., for SQL connections or URLs)
func (pm *PathMatcher) IsVirtualPath(path string) bool {
	return strings.HasPrefix(path, "virtual://")
}

// ExtractVirtualPathType extracts the type from a virtual path
func (pm *PathMatcher) ExtractVirtualPathType(virtualPath string) string {
	if !pm.IsVirtualPath(virtualPath) {
		return ""
	}

	// Format: virtual://type/identifier
	parts := strings.Split(virtualPath, "/")
	if len(parts) >= 3 {
		return parts[2] // The type part
	}

	return ""
}

// MatchVirtualPath checks if a virtual path matches an analyzer pattern
func (pm *PathMatcher) MatchVirtualPath(virtualPath string, analyzer AnalyzerConfig) bool {
	if !pm.IsVirtualPath(virtualPath) {
		return false
	}

	virtualType := pm.ExtractVirtualPathType(virtualPath)

	// Match based on analyzer type
	switch analyzer.Analyzer {
	case "sql":
		return virtualType == "sql"
	case "openapi":
		return virtualType == "openapi"
	default:
		return false
	}
}

// NormalizePath normalizes a file path for consistent handling
func (pm *PathMatcher) NormalizePath(path string) string {
	// Convert to forward slashes for consistent handling
	normalized := filepath.ToSlash(path)

	// Make relative to working directory if possible
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(pm.workingDir, path); err == nil {
			normalized = filepath.ToSlash(rel)
		}
	}

	return normalized
}