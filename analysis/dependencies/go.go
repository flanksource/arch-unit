package dependencies

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/models"
	"golang.org/x/mod/modfile"
)

// GoDependencyScanner scans Go module dependencies from go.mod files
type GoDependencyScanner struct {
	*analysis.BaseDependencyScanner
}

// NewGoDependencyScanner creates a new Go dependency scanner
func NewGoDependencyScanner() *GoDependencyScanner {
	scanner := &GoDependencyScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner("go", []string{"go.mod", "go.sum"}),
	}

	// Register with the global registry
	analysis.DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// ScanFile scans a go.mod file and extracts dependencies
func (s *GoDependencyScanner) ScanFile(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	if !strings.HasSuffix(filepath, "go.mod") {
		return s.scanGoSum(ctx, filepath, content)
	}

	ctx.Debugf("Scanning Go dependencies from %s", filepath)

	// Parse go.mod file
	modFile, err := modfile.Parse(filepath, content, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	var dependencies []*models.Dependency

	// Extract module dependencies
	for _, require := range modFile.Require {
		// Skip indirect dependencies if needed
		dep := &models.Dependency{
			Name:    require.Mod.Path,
			Version: require.Mod.Version,
			Type:    models.DependencyTypeGo,
		}

		// Extract organization and project from module path
		// e.g., github.com/org/project -> org/project
		if strings.HasPrefix(require.Mod.Path, "github.com/") {
			parts := strings.Split(require.Mod.Path, "/")
			if len(parts) >= 3 {
				dep.Git = fmt.Sprintf("https://%s", require.Mod.Path)
				// Provide packages based on the module structure
				dep.Package = []string{require.Mod.Path}
			}
		} else if strings.HasPrefix(require.Mod.Path, "golang.org/x/") {
			// Standard extended library
			dep.Git = fmt.Sprintf("https://github.com/golang/%s",
				strings.TrimPrefix(require.Mod.Path, "golang.org/x/"))
		}

		dependencies = append(dependencies, dep)
		ctx.Debugf("Found dependency: %s@%s", dep.Name, dep.Version)
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
					// Track the original name
					dep.Name = replace.New.Path
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

// scanGoSum extracts dependency information from go.sum file
func (s *GoDependencyScanner) scanGoSum(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	if !strings.HasSuffix(filepath, "go.sum") {
		return nil, fmt.Errorf("not a go.sum file: %s", filepath)
	}

	ctx.Debugf("Scanning Go checksums from %s", filepath)

	// go.sum contains checksums but not full dependency info
	// We'll extract unique modules for reference
	dependencies := make(map[string]*models.Dependency)

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
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

		// Create dependency entry
		dep := &models.Dependency{
			Name:    module,
			Version: version,
			Type:    models.DependencyTypeGo,
		}

		if strings.HasPrefix(module, "github.com/") {
			dep.Git = fmt.Sprintf("https://%s", module)
			dep.Package = []string{module}
		} else if strings.HasPrefix(module, "golang.org/x/") {
			// Standard extended library - consistent with go.mod scanner
			dep.Git = fmt.Sprintf("https://github.com/golang/%s",
				strings.TrimPrefix(module, "golang.org/x/"))
		}

		// Only keep the first occurrence or merge if better info available
		if existing, exists := dependencies[module]; !exists {
			dependencies[module] = dep
		} else {
			// Merge: prefer entry with more information
			if existing.Git == "" && dep.Git != "" {
				existing.Git = dep.Git
			}
			if existing.Version == "" && dep.Version != "" {
				existing.Version = dep.Version
			}
			if len(existing.Package) == 0 && len(dep.Package) > 0 {
				existing.Package = dep.Package
			}
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

func init() {
	// Auto-register the scanner
	NewGoDependencyScanner()
}
