package analysis

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"golang.org/x/mod/modfile"
)

// GoDependencyScanner scans Go module dependencies from go.mod files
type GoDependencyScanner struct {
	*BaseDependencyScanner
	resolver *ResolutionService
}

// NewGoDependencyScanner creates a new Go dependency scanner
func NewGoDependencyScanner() *GoDependencyScanner {
	scanner := &GoDependencyScanner{
		BaseDependencyScanner: NewBaseDependencyScanner("go", []string{"go.mod", "go.sum"}),
	}

	// Register with the global registry
	DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// NewGoDependencyScannerWithResolver creates a new Go dependency scanner with a resolution service
func NewGoDependencyScannerWithResolver(resolver *ResolutionService) *GoDependencyScanner {
	scanner := &GoDependencyScanner{
		BaseDependencyScanner: NewBaseDependencyScanner("go", []string{"go.mod", "go.sum"}),
		resolver:              resolver,
	}

	// Register with the global registry
	DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// ScanFile scans a go.mod file and extracts dependencies
func (s *GoDependencyScanner) ScanFile(ctx *ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	if !strings.HasSuffix(filePath, "go.mod") {
		return s.scanGoSum(ctx, filePath, content)
	}

	ctx.Debugf("Scanning Go dependencies from %s", filePath)

	// Parse go.mod file
	modFile, err := modfile.Parse(filePath, content, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	var dependencies []*models.Dependency

	// Extract module dependencies
	for lineIdx, require := range modFile.Require {
		dep := &models.Dependency{
			Name:     require.Mod.Path,
			Version:  require.Mod.Version,
			Type:     s.determineDependencyType(require.Mod.Path),
			Source:   fmt.Sprintf("%s:%d", filepath.Base(filePath), s.findRequireLine(content, require.Mod.Path, lineIdx)),
			Indirect: require.Indirect,
			Depth:    0, // Direct dependencies from go.mod have depth 0
			Package:  []string{require.Mod.Path},
		}

		if !ctx.Matches(dep) {
			continue
		}
		// Use resolver to get Git URL if available
		if s.resolver != nil {
			if gitURL, err := s.resolver.ResolveGitURL(ctx, require.Mod.Path, "go"); err == nil && gitURL != "" {
				dep.Git = gitURL
			}
		}

		dependencies = append(dependencies, dep)
		ctx.Debugf("Found dependency: %s@%s at %s", dep.Name, dep.Version, dep.Source)
	}

	// Extract replace directives as they affect actual dependencies
	for _, replace := range modFile.Replace {
		// Find the dependency being replaced
		for _, dep := range dependencies {
			if dep.Name == replace.Old.Path {
				// Update with replacement info
				if replace.New.Version != "" {
					dep.Version = replace.New.Version
				}
				if replace.New.Path != replace.Old.Path {
					// For local replacements (relative/absolute paths), preserve the original name
					// and indicate the local path in the version
					if isLocalPath(replace.New.Path) {
						dep.Version = "local:" + replace.New.Path
						// Keep original name: dep.Name stays replace.Old.Path
					} else {
						// For URL replacements, update the name
						dep.Name = replace.New.Path
					}
				}
				ctx.Debugf("Replaced %s with %s@%s",
					replace.Old.Path, replace.New.Path, replace.New.Version)
				break
			}
		}
	}

	ctx.Debugf("Found %d Go dependencies", len(dependencies))
	return dependencies, nil
}

// isLocalPath checks if a path is a local file system path (relative or absolute)
func isLocalPath(path string) bool {
	// Check for relative paths (starts with ./ or ../)
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		return true
	}
	// Check for absolute paths (starts with /)
	if strings.HasPrefix(path, "/") {
		return true
	}
	// Check for Windows paths (starts with C:\ etc)
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	// If it looks like a URL or module path, it's not local
	return false
}

// scanGoSum extracts dependency information from go.sum file
func (s *GoDependencyScanner) scanGoSum(ctx *ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	if !strings.HasSuffix(filePath, "go.sum") {
		return nil, fmt.Errorf("not a go.sum file: %s", filePath)
	}

	ctx.Debugf("Scanning Go checksums from %s", filePath)

	// go.sum contains checksums but not full dependency info
	// We'll extract unique modules for reference
	dependencies := make(map[string]*models.Dependency)

	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		// Format: module version hash
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		module := parts[0]
		version := parts[1]

		// Skip /go.mod entries
		if strings.HasSuffix(version, "/go.mod") {
			version = strings.TrimSuffix(version, "/go.mod")
		}

		// Only keep the first occurrence of each module
		if _, exists := dependencies[module]; !exists {
			dep := &models.Dependency{
				Name:    module,
				Version: version,
				Type:    s.determineDependencyType(module),
				Source:  fmt.Sprintf("%s:%d", filepath.Base(filePath), lineNum),
				Package: []string{module},
			}

			if !ctx.Matches(dep) {
				continue
			}

			// Use resolver to get Git URL if available
			if s.resolver != nil {
				if gitURL, err := s.resolver.ResolveGitURL(ctx, module, "go"); err == nil && gitURL != "" {
					dep.Git = gitURL
				}
			}

			dependencies[module] = dep
		}
	}

	// Convert map to slice
	result := make([]*models.Dependency, 0, len(dependencies))
	for _, dep := range dependencies {
		result = append(result, dep)
	}

	ctx.Debugf("Found %d unique modules in go.sum", len(result))
	return result, nil
}

// determineDependencyType determines the appropriate dependency type for a Go module
func (s *GoDependencyScanner) determineDependencyType(modulePath string) models.DependencyType {
	if strings.HasPrefix(modulePath, "golang.org/x/") {
		return models.DependencyTypeStdlib
	}
	return models.DependencyTypeGo
}

// findRequireLine attempts to find the line number where a dependency is declared
func (s *GoDependencyScanner) findRequireLine(content []byte, modulePath string, fallbackIdx int) int {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.Contains(line, modulePath) && (strings.Contains(line, "require") || strings.TrimSpace(line) == modulePath || strings.Contains(strings.TrimSpace(line), modulePath+" ")) {
			return i + 1 // Line numbers start at 1
		}
	}
	// Fallback: estimate line based on position in requires slice
	// This is approximate but better than no line number
	return fallbackIdx + 10 // Rough estimate assuming require block starts around line 10
}
