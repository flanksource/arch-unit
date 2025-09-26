package javascript

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/dependencies"
	"github.com/flanksource/arch-unit/models"
	"gopkg.in/yaml.v3"
)

// NodeDependencyScanner scans Node.js/JavaScript dependencies
type NodeDependencyScanner struct {
	*dependencies.BaseDependencyScanner
}

// NewNodeDependencyScanner creates a new Node.js dependency scanner
func NewNodeDependencyScanner() *NodeDependencyScanner {
	scanner := &NodeDependencyScanner{
		BaseDependencyScanner: dependencies.NewBaseDependencyScanner("node",
			[]string{"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml"}),
	}

	// Register with the global registry
	analysis.DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// ScanFile scans a Node.js dependency file and extracts dependencies
func (s *NodeDependencyScanner) ScanFile(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	filename := strings.ToLower(filepath)

	switch {
	case strings.HasSuffix(filename, "package.json"):
		return s.scanPackageJson(ctx, filepath, content)
	case strings.HasSuffix(filename, "package-lock.json"):
		return s.scanPackageLockJson(ctx, filepath, content)
	case strings.HasSuffix(filename, "yarn.lock"):
		return s.scanYarnLock(ctx, filepath, content)
	case strings.HasSuffix(filename, "pnpm-lock.yaml"):
		return s.scanPnpmLock(ctx, filepath, content)
	default:
		return nil, fmt.Errorf("unsupported Node.js dependency file: %s", filepath)
	}
}

// scanPackageJson scans package.json files
func (s *NodeDependencyScanner) scanPackageJson(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Node.js dependencies from %s", filepath)

	var packageJson struct {
		Name                 string            `json:"name"`
		Version              string            `json:"version"`
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}

	if err := json.Unmarshal(content, &packageJson); err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	var dependencies []*models.Dependency

	// Process regular dependencies
	for name, version := range packageJson.Dependencies {
		dep := s.createNodeDependency(name, version)
		dependencies = append(dependencies, dep)
		ctx.Debugf("Found dependency: %s@%s", name, version)
	}

	dependencies = ctx.Filter(dependencies)

	ctx.Debugf("Found %d Node.js dependencies", len(dependencies))
	return dependencies, nil
}

// scanPackageLockJson scans package-lock.json files
func (s *NodeDependencyScanner) scanPackageLockJson(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Node.js lock file from %s", filepath)

	var lockFile struct {
		Dependencies map[string]struct {
			Version   string `json:"version"`
			Resolved  string `json:"resolved"`
			Integrity string `json:"integrity"`
			Dev       bool   `json:"dev"`
		} `json:"dependencies"`
		Packages map[string]struct {
			Version   string `json:"version"`
			Resolved  string `json:"resolved"`
			Integrity string `json:"integrity"`
			Dev       bool   `json:"dev"`
		} `json:"packages"`
	}

	if err := json.Unmarshal(content, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse package-lock.json: %w", err)
	}

	var dependencies []*models.Dependency

	// NPM v7+ uses "packages" field
	if len(lockFile.Packages) > 0 {
		for path, info := range lockFile.Packages {
			// Skip the root package
			if path == "" {
				continue
			}

			// Extract package name from path (e.g., "node_modules/@babel/core" -> "@babel/core")
			name := strings.TrimPrefix(path, "node_modules/")
			if name == path {
				continue // Not a dependency path
			}

			dep := &models.Dependency{
				Name:     name,
				Version:  info.Version,
				Type:     models.DependencyTypeNpm,
				Homepage: fmt.Sprintf("https://www.npmjs.com/package/%s", name),
			}

			dependencies = append(dependencies, dep)
		}
	} else {
		// NPM v6 and earlier use "dependencies" field
		for name, info := range lockFile.Dependencies {
			dep := &models.Dependency{
				Name:     name,
				Version:  info.Version,
				Type:     models.DependencyTypeNpm,
				Homepage: fmt.Sprintf("https://www.npmjs.com/package/%s", name),
			}

			dependencies = append(dependencies, dep)
		}
	}

	ctx.Debugf("Found %d Node.js dependencies in lock file", len(dependencies))
	return dependencies, nil
}

// scanYarnLock scans yarn.lock files
func (s *NodeDependencyScanner) scanYarnLock(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Yarn lock file from %s", filepath)

	// Yarn lock files have a custom format, we'll do basic parsing
	var dependencies []*models.Dependency
	seen := make(map[string]bool)

	lines := strings.Split(string(content), "\n")
	var currentPackage string
	var currentVersion string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Package declaration line (e.g., "package-name@version:")
		if !strings.HasPrefix(line, " ") && strings.Contains(line, "@") && strings.HasSuffix(line, ":") {
			// Parse package name and version range
			line = strings.TrimSuffix(line, ":")
			parts := strings.Split(line, ", ")

			for _, part := range parts {
				// Extract package name (before last @)
				lastAt := strings.LastIndex(part, "@")
				if lastAt > 0 {
					currentPackage = part[:lastAt]
					// Remove quotes if present
					currentPackage = strings.Trim(currentPackage, "\"")
					break
				}
			}
		}

		// Version line (e.g., "  version \"1.2.3\"")
		if strings.HasPrefix(line, "  version ") {
			versionPart := strings.TrimPrefix(line, "  version ")
			currentVersion = strings.Trim(versionPart, "\"")

			// Create dependency if we have both package and version
			if currentPackage != "" && currentVersion != "" {
				key := currentPackage + "@" + currentVersion
				if !seen[key] {
					seen[key] = true
					dep := &models.Dependency{
						Name:     currentPackage,
						Version:  currentVersion,
						Type:     models.DependencyTypeNpm,
						Homepage: fmt.Sprintf("https://www.npmjs.com/package/%s", currentPackage),
					}
					dependencies = append(dependencies, dep)
					ctx.Debugf("Found Yarn dependency: %s@%s", currentPackage, currentVersion)
				}
			}
		}
	}

	dependencies = ctx.Filter(dependencies)

	ctx.Debugf("Found %d unique Node.js dependencies in yarn.lock", len(dependencies))
	return dependencies, nil
}

// scanPnpmLock scans pnpm-lock.yaml files
func (s *NodeDependencyScanner) scanPnpmLock(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning pnpm lock file from %s", filepath)

	var lockFile struct {
		Dependencies    map[string]string `yaml:"dependencies"`
		DevDependencies map[string]string `yaml:"devDependencies"`
		Packages        map[string]struct {
			Resolution struct {
				Integrity string `yaml:"integrity"`
				Tarball   string `yaml:"tarball"`
			} `yaml:"resolution"`
			Dependencies    map[string]string `yaml:"dependencies"`
			DevDependencies map[string]string `yaml:"devDependencies"`
		} `yaml:"packages"`
	}

	if err := yaml.Unmarshal(content, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse pnpm-lock.yaml: %w", err)
	}

	var dependencies []*models.Dependency
	seen := make(map[string]bool)

	// Process packages section
	for path := range lockFile.Packages {
		// Extract package name and version from path
		// Format: "/@babel/core/7.20.0" or "/package-name/1.2.3"
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

		var name, version string
		if strings.HasPrefix(path, "/@") && len(parts) >= 3 {
			// Scoped package (e.g., @babel/core)
			name = "@" + parts[1] + "/" + parts[2]
			if len(parts) > 3 {
				version = parts[3]
			}
		} else if len(parts) >= 2 {
			// Regular package
			name = parts[0]
			version = parts[1]
		} else {
			continue
		}

		if name != "" && version != "" {
			key := name + "@" + version
			if !seen[key] {
				seen[key] = true
				dep := &models.Dependency{
					Name:    name,
					Version: version,
					Type:    models.DependencyTypeNpm,
					Git:     fmt.Sprintf("https://www.npmjs.com/package/%s", name),
				}
				dependencies = append(dependencies, dep)
				ctx.Debugf("Found pnpm dependency: %s@%s", name, version)
			}
		}
	}

	// Fallback to top-level dependencies if packages section is empty
	if len(dependencies) == 0 {
		for name, version := range lockFile.Dependencies {
			dep := s.createNodeDependency(name, version)
			dependencies = append(dependencies, dep)
		}

		for name, version := range lockFile.DevDependencies {
			dep := s.createNodeDependency(name, version)
			dependencies = append(dependencies, dep)
		}
	}

	dependencies = ctx.Filter(dependencies)

	ctx.Debugf("Found %d Node.js dependencies in pnpm-lock.yaml", len(dependencies))
	return dependencies, nil
}

// createNodeDependency creates a Node.js dependency object
func (s *NodeDependencyScanner) createNodeDependency(name, version string) *models.Dependency {
	// Clean version specifiers
	version = strings.TrimPrefix(version, "^")
	version = strings.TrimPrefix(version, "~")
	version = strings.TrimPrefix(version, ">=")
	version = strings.TrimPrefix(version, ">")

	// Handle version ranges (take the first version)
	if strings.Contains(version, " ") {
		parts := strings.Fields(version)
		if len(parts) > 0 {
			version = parts[0]
		}
	}

	dep := &models.Dependency{
		Name:    name,
		Version: version,
		Type:    models.DependencyTypeNpm,
	}

	// Add NPM registry URL
	dep.Homepage = fmt.Sprintf("https://www.npmjs.com/package/%s", name)

	// For GitHub dependencies (e.g., "user/repo")
	if strings.Contains(version, "github:") || strings.Contains(version, "/") {
		if strings.HasPrefix(version, "github:") {
			repo := strings.TrimPrefix(version, "github:")
			dep.Git = fmt.Sprintf("https://github.com/%s", repo)
		} else if strings.Contains(version, "github.com") {
			dep.Git = version
		}
	}

	return dep
}

func init() {
	// Auto-register the scanner
	NewNodeDependencyScanner()
}
