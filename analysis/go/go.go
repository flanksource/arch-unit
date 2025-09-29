package _go

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/languages"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
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

	return scanner
}

// ScanFile scans a go.mod file and extracts dependencies
func (s *GoDependencyScanner) ScanFile(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	if !strings.HasSuffix(filepath, "go.mod") {
		return s.scanGoSum(ctx, filepath, content)
	}

	if ctx != nil {
		ctx.Debugf("Scanning Go dependencies from %s", filepath)
	}

	// Parse go.mod file
	modFile, err := modfile.Parse(filepath, content, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	var dependencies []*models.Dependency

	// Extract module dependencies
	for lineNo, require := range modFile.Require {
		// Determine dependency type
		depType := models.DependencyTypeGo
		if strings.HasPrefix(require.Mod.Path, "golang.org/x/") {
			depType = models.DependencyTypeStdlib
		}

		dep := &models.Dependency{
			Name:    require.Mod.Path,
			Version: require.Mod.Version,
			Type:    depType,
			Source:  fmt.Sprintf("go.mod:%d", lineNo+1), // Line numbers are 1-based
		}

		// Note: Git URL resolution should be handled by a resolver service, not here
		// This follows the pattern where dependency scanners extract dependency info
		// and resolvers handle URL resolution
		if strings.HasPrefix(require.Mod.Path, "github.com/") {
			// Provide packages based on the module structure
			dep.Package = []string{require.Mod.Path}
		}

		if ctx != nil && !ctx.Matches(dep) {
			continue
		}
		dependencies = append(dependencies, dep)
		if ctx != nil {
			ctx.Debugf("Found dependency: %s@%s", dep.Name, dep.Version)
		}
	}

	// Extract replace directives as they affect actual dependencies
	for _, replace := range modFile.Replace {
		// Find the dependency being replaced
		for _, dep := range dependencies {
			if dep.Name == replace.Old.Path {
				// Update with replacement info
				if replace.New.Version != "" {
					dep.Version = replace.New.Version
				} else if replace.New.Path != replace.Old.Path {
					// Local path replacement (no version specified)
					dep.Version = fmt.Sprintf("local:%s", replace.New.Path)
				}
				if replace.New.Path != replace.Old.Path && replace.New.Version != "" {
					// Only change name if it's a different module with a version
					// For local paths, keep the original name
					dep.Name = replace.New.Path
				}
				if ctx != nil {
					ctx.Debugf("Replaced %s with %s@%s",
						replace.Old.Path, replace.New.Path, replace.New.Version)
				}
				break
			}
		}
	}

	if ctx != nil {
		ctx.Debugf("Found %d Go dependencies", len(dependencies))
	}
	return dependencies, nil
}

// scanGoSum extracts dependency information from go.sum file
func (s *GoDependencyScanner) scanGoSum(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	if !strings.HasSuffix(filepath, "go.sum") {
		return nil, fmt.Errorf("not a go.sum file: %s", filepath)
	}

	if ctx != nil {
		ctx.Debugf("Scanning Go checksums from %s", filepath)
	}

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
			dep.Package = []string{module}
		}
		// Note: Git URL resolution should be handled by a resolver service, not here

		if ctx != nil && ctx.Matches(dep) {
			continue
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

	if ctx != nil {
		ctx.Debugf("Found %d unique modules in go.sum", len(result))
	}
	return result, nil
}

// goAnalyzerAdapter adapts the GoASTExtractor to the languages.ASTAnalyzer interface
type goAnalyzerAdapter struct {
	extractor *GoASTExtractor
}

func (a *goAnalyzerAdapter) AnalyzeFile(task interface{}, filepath string, content []byte) (interface{}, error) {
	// Type assert task to *clicky.Task
	clickyTask, ok := task.(*clicky.Task)
	if !ok {
		// For backward compatibility, create a minimal adapter if not the right type
		return nil, nil
	}

	// Use the generic analyzer to handle the Go extractor
	// This delegates to the existing analysis framework
	genericAnalyzer := languages.GetGenericAnalyzerAdapter()
	return genericAnalyzer.AnalyzeFile(clickyTask, filepath, content)
}

func init() {
	// Register Go dependency scanner
	goDependencyScanner := NewGoDependencyScanner()
	analysis.RegisterDependencyScanner(goDependencyScanner)

	// Register Go AST extractor with DefaultExtractorRegistry
	goExtractor := NewGoASTExtractor()
	analysis.DefaultExtractorRegistry.Register("go", goExtractor)

	// Register Go analyzer with unified registry
	goAnalyzer := &goAnalyzerAdapter{extractor: goExtractor}
	languages.SetAnalyzer("go", goAnalyzer)
}
