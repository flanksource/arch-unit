package dependencies

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/models"
	"github.com/pelletier/go-toml/v2"
)

// PythonDependencyScanner scans Python dependencies from various file formats
type PythonDependencyScanner struct {
	*analysis.BaseDependencyScanner
}

// NewPythonDependencyScanner creates a new Python dependency scanner
func NewPythonDependencyScanner() *PythonDependencyScanner {
	scanner := &PythonDependencyScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner("python", 
			[]string{"requirements.txt", "requirements*.txt", "Pipfile", "Pipfile.lock", 
				"pyproject.toml", "setup.py", "setup.cfg", "poetry.lock"}),
	}
	
	// Register with the global registry
	analysis.DefaultDependencyRegistry.Register(scanner)
	
	return scanner
}

// ScanFile scans a Python dependency file and extracts dependencies
func (s *PythonDependencyScanner) ScanFile(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	filename := strings.ToLower(filepath)
	
	switch {
	case strings.Contains(filename, "requirements"):
		return s.scanRequirementsTxt(ctx, filepath, content)
	case strings.HasSuffix(filename, "pipfile"):
		return s.scanPipfile(ctx, filepath, content)
	case strings.HasSuffix(filename, "pipfile.lock"):
		return s.scanPipfileLock(ctx, filepath, content)
	case strings.HasSuffix(filename, "pyproject.toml"):
		return s.scanPyprojectToml(ctx, filepath, content)
	case strings.HasSuffix(filename, "poetry.lock"):
		return s.scanPoetryLock(ctx, filepath, content)
	case strings.HasSuffix(filename, "setup.py"):
		return s.scanSetupPy(ctx, filepath, content)
	case strings.HasSuffix(filename, "setup.cfg"):
		return s.scanSetupCfg(ctx, filepath, content)
	default:
		return nil, fmt.Errorf("unsupported Python dependency file: %s", filepath)
	}
}

// scanRequirementsTxt scans requirements.txt format files
func (s *PythonDependencyScanner) scanRequirementsTxt(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	s.LogProgress(ctx, "Scanning Python requirements from %s", filepath)
	
	var dependencies []*models.Dependency
	scanner := bufio.NewScanner(bytes.NewReader(content))
	
	// Regex patterns for parsing requirements
	packagePattern := regexp.MustCompile(`^([a-zA-Z0-9_\-\.]+)\s*([=<>!~]+.*)?`)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Skip -r (include) and -e (editable) directives for now
		if strings.HasPrefix(line, "-r ") || strings.HasPrefix(line, "-e ") {
			continue
		}
		
		// Parse package and version
		matches := packagePattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			dep := &models.Dependency{
				Name:     matches[1],
				Type:     models.DependencyTypePip,
	
			}
			
			// Parse version if present
			if len(matches) >= 3 && matches[2] != "" {
				version := strings.TrimSpace(matches[2])
				// Simplify version specification (e.g., "==1.2.3" -> "1.2.3")
				version = strings.TrimPrefix(version, "==")
				version = strings.TrimPrefix(version, ">=")
				version = strings.TrimPrefix(version, "~=")
				dep.Version = version
			}
			
			// Git field left empty for PyPI packages unless source URL is provided
			
			dependencies = append(dependencies, dep)
			s.LogDebug(ctx, "Found Python dependency: %s@%s", dep.Name, dep.Version)
		}
	}
	
	s.LogProgress(ctx, "Found %d Python dependencies", len(dependencies))
	return dependencies, nil
}

// scanPipfile scans Pipfile format
func (s *PythonDependencyScanner) scanPipfile(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	s.LogProgress(ctx, "Scanning Pipfile from %s", filepath)
	
	var pipfile struct {
		Packages    map[string]interface{} `toml:"packages"`
		DevPackages map[string]interface{} `toml:"dev-packages"`
	}
	
	if err := toml.Unmarshal(content, &pipfile); err != nil {
		return nil, fmt.Errorf("failed to parse Pipfile: %w", err)
	}
	
	var dependencies []*models.Dependency
	
	// Process regular packages
	for name, spec := range pipfile.Packages {
		dep := &models.Dependency{
			Name:     name,
			Type:     models.DependencyTypePip,
			Language: "python",
			Git:      fmt.Sprintf("https://pypi.org/project/%s/", name),
		}
		
		// Extract version from spec
		switch v := spec.(type) {
		case string:
			dep.Version = strings.TrimPrefix(v, "==")
		case map[string]interface{}:
			if version, ok := v["version"].(string); ok {
				dep.Version = strings.TrimPrefix(version, "==")
			}
		}
		
		dependencies = append(dependencies, dep)
	}
	
	// Process dev packages (marked as dev dependencies)
	for name, spec := range pipfile.DevPackages {
		dep := &models.Dependency{
			Name:     name,
			Type:     models.DependencyTypePip,
			Language: "python",
			Git:      fmt.Sprintf("https://pypi.org/project/%s/", name),
		}
		
		// Extract version from spec
		switch v := spec.(type) {
		case string:
			dep.Version = strings.TrimPrefix(v, "==")
		case map[string]interface{}:
			if version, ok := v["version"].(string); ok {
				dep.Version = strings.TrimPrefix(version, "==")
			}
		}
		
		dependencies = append(dependencies, dep)
	}
	
	s.LogProgress(ctx, "Found %d Python dependencies in Pipfile", len(dependencies))
	return dependencies, nil
}

// scanPipfileLock scans Pipfile.lock format
func (s *PythonDependencyScanner) scanPipfileLock(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	s.LogProgress(ctx, "Scanning Pipfile.lock from %s", filepath)
	
	var lockfile struct {
		Default map[string]struct {
			Version string `json:"version"`
			Hashes  []string `json:"hashes"`
		} `json:"default"`
		Develop map[string]struct {
			Version string `json:"version"`
			Hashes  []string `json:"hashes"`
		} `json:"develop"`
	}
	
	if err := json.Unmarshal(content, &lockfile); err != nil {
		return nil, fmt.Errorf("failed to parse Pipfile.lock: %w", err)
	}
	
	var dependencies []*models.Dependency
	
	// Process default packages
	for name, info := range lockfile.Default {
		dep := &models.Dependency{
			Name:     name,
			Version:  strings.TrimPrefix(info.Version, "=="),
			Type:     models.DependencyTypePip,
			Language: "python",
			Git:      fmt.Sprintf("https://pypi.org/project/%s/", name),
		}
		dependencies = append(dependencies, dep)
	}
	
	// Process develop packages
	for name, info := range lockfile.Develop {
		dep := &models.Dependency{
			Name:     name,
			Version:  strings.TrimPrefix(info.Version, "=="),
			Type:     models.DependencyTypePip,
			Language: "python",
			Git:      fmt.Sprintf("https://pypi.org/project/%s/", name),
		}
		dependencies = append(dependencies, dep)
	}
	
	s.LogProgress(ctx, "Found %d Python dependencies in Pipfile.lock", len(dependencies))
	return dependencies, nil
}

// scanPyprojectToml scans pyproject.toml format
func (s *PythonDependencyScanner) scanPyprojectToml(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	s.LogProgress(ctx, "Scanning pyproject.toml from %s", filepath)
	
	var pyproject struct {
		Tool struct {
			Poetry struct {
				Dependencies    map[string]interface{} `toml:"dependencies"`
				DevDependencies map[string]interface{} `toml:"dev-dependencies"`
			} `toml:"poetry"`
		} `toml:"tool"`
		Project struct {
			Dependencies         []string `toml:"dependencies"`
			OptionalDependencies map[string][]string `toml:"optional-dependencies"`
		} `toml:"project"`
	}
	
	if err := toml.Unmarshal(content, &pyproject); err != nil {
		return nil, fmt.Errorf("failed to parse pyproject.toml: %w", err)
	}
	
	var dependencies []*models.Dependency
	
	// Process Poetry dependencies
	for name, spec := range pyproject.Tool.Poetry.Dependencies {
		if name == "python" {
			continue // Skip Python version specification
		}
		
		dep := &models.Dependency{
			Name:     name,
			Type:     models.DependencyTypePip,
			Language: "python",
			Git:      fmt.Sprintf("https://pypi.org/project/%s/", name),
		}
		
		// Extract version from spec
		switch v := spec.(type) {
		case string:
			dep.Version = strings.TrimPrefix(v, "^")
			dep.Version = strings.TrimPrefix(dep.Version, "~")
		case map[string]interface{}:
			if version, ok := v["version"].(string); ok {
				dep.Version = strings.TrimPrefix(version, "^")
				dep.Version = strings.TrimPrefix(dep.Version, "~")
			}
		}
		
		dependencies = append(dependencies, dep)
	}
	
	// Process Poetry dev dependencies
	for name, spec := range pyproject.Tool.Poetry.DevDependencies {
		dep := &models.Dependency{
			Name:     name,
			Type:     models.DependencyTypePip,
			Language: "python",
			Git:      fmt.Sprintf("https://pypi.org/project/%s/", name),
		}
		
		// Extract version from spec
		switch v := spec.(type) {
		case string:
			dep.Version = strings.TrimPrefix(v, "^")
			dep.Version = strings.TrimPrefix(dep.Version, "~")
		case map[string]interface{}:
			if version, ok := v["version"].(string); ok {
				dep.Version = strings.TrimPrefix(version, "^")
				dep.Version = strings.TrimPrefix(dep.Version, "~")
			}
		}
		
		dependencies = append(dependencies, dep)
	}
	
	// Process standard project dependencies (PEP 621)
	for _, depStr := range pyproject.Project.Dependencies {
		// Parse dependency string (e.g., "requests>=2.28.0")
		parts := regexp.MustCompile(`([a-zA-Z0-9_\-\.]+)\s*([=<>!~]+.*)?`).FindStringSubmatch(depStr)
		if len(parts) >= 2 {
			dep := &models.Dependency{
				Name:     parts[1],
				Type:     models.DependencyTypePip,

				Git:      fmt.Sprintf("https://pypi.org/project/%s/", parts[1]),
			}
			
			if len(parts) >= 3 && parts[2] != "" {
				version := strings.TrimSpace(parts[2])
				version = strings.TrimPrefix(version, ">=")
				version = strings.TrimPrefix(version, "==")
				dep.Version = version
			}
			
			dependencies = append(dependencies, dep)
		}
	}
	
	s.LogProgress(ctx, "Found %d Python dependencies in pyproject.toml", len(dependencies))
	return dependencies, nil
}

// scanPoetryLock scans poetry.lock format
func (s *PythonDependencyScanner) scanPoetryLock(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	s.LogProgress(ctx, "Scanning poetry.lock from %s", filepath)
	
	var lockfile struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
			Source  struct {
				Type      string `toml:"type"`
				URL       string `toml:"url"`
				Reference string `toml:"reference"`
			} `toml:"source"`
		} `toml:"package"`
	}
	
	if err := toml.Unmarshal(content, &lockfile); err != nil {
		return nil, fmt.Errorf("failed to parse poetry.lock: %w", err)
	}
	
	var dependencies []*models.Dependency
	
	for _, pkg := range lockfile.Package {
		dep := &models.Dependency{
			Name:     pkg.Name,
			Version:  pkg.Version,
			Type:     models.DependencyTypePip,
			Language: "python",
		}
		
		// Set Git URL based on source
		if pkg.Source.Type == "git" {
			dep.Git = pkg.Source.URL
		}
		// Git field left empty for regular PyPI packages
		
		dependencies = append(dependencies, dep)
	}
	
	s.LogProgress(ctx, "Found %d Python dependencies in poetry.lock", len(dependencies))
	return dependencies, nil
}

// scanSetupPy scans setup.py files (basic parsing)
func (s *PythonDependencyScanner) scanSetupPy(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	s.LogProgress(ctx, "Scanning setup.py from %s", filepath)
	
	// This is a simplified parser - setup.py is Python code and can be complex
	// Look for install_requires and extras_require patterns
	var dependencies []*models.Dependency
	
	// Regex to find install_requires or similar lists
	installRequiresPattern := regexp.MustCompile(`install_requires\s*=\s*\[([\s\S]*?)\]`)
	matches := installRequiresPattern.FindSubmatch(content)
	
	if len(matches) > 1 {
		requiresList := string(matches[1])
		// Extract quoted strings from the list
		stringPattern := regexp.MustCompile(`["']([^"']+)["']`)
		deps := stringPattern.FindAllStringSubmatch(requiresList, -1)
		
		for _, match := range deps {
			if len(match) > 1 {
				depStr := match[1]
				// Parse dependency string
				parts := regexp.MustCompile(`([a-zA-Z0-9_\-\.]+)\s*([=<>!~]+.*)?`).FindStringSubmatch(depStr)
				if len(parts) >= 2 {
					dep := &models.Dependency{
						Name:     parts[1],
						Type:     models.DependencyTypePip,
						Language: "python",
						Git:      fmt.Sprintf("https://pypi.org/project/%s/", parts[1]),
					}
					
					if len(parts) >= 3 && parts[2] != "" {
						version := strings.TrimSpace(parts[2])
						version = strings.TrimPrefix(version, ">=")
						version = strings.TrimPrefix(version, "==")
						dep.Version = version
					}
					
					dependencies = append(dependencies, dep)
				}
			}
		}
	}
	
	s.LogProgress(ctx, "Found %d Python dependencies in setup.py", len(dependencies))
	return dependencies, nil
}

// scanSetupCfg scans setup.cfg files
func (s *PythonDependencyScanner) scanSetupCfg(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	s.LogProgress(ctx, "Scanning setup.cfg from %s", filepath)
	
	var dependencies []*models.Dependency
	scanner := bufio.NewScanner(bytes.NewReader(content))
	
	inOptionsSection := false
	inInstallRequires := false
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Check for [options] section
		if strings.HasPrefix(line, "[") {
			inOptionsSection = strings.HasPrefix(line, "[options")
			inInstallRequires = false
			continue
		}
		
		if !inOptionsSection {
			continue
		}
		
		// Check for install_requires
		if strings.HasPrefix(line, "install_requires") {
			inInstallRequires = true
			// Check if dependencies are on the same line
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					line = strings.TrimSpace(parts[1])
				} else {
					continue
				}
			} else {
				continue
			}
		}
		
		if inInstallRequires && line != "" && !strings.HasPrefix(line, "#") {
			// Check if this line starts a new config option
			if strings.Contains(line, "=") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inInstallRequires = false
				continue
			}
			
			// Parse dependency
			parts := regexp.MustCompile(`([a-zA-Z0-9_\-\.]+)\s*([=<>!~]+.*)?`).FindStringSubmatch(line)
			if len(parts) >= 2 {
				dep := &models.Dependency{
					Name:     parts[1],
					Type:     models.DependencyTypePip,
					Language: "python",
					Git:      fmt.Sprintf("https://pypi.org/project/%s/", parts[1]),
				}
				
				if len(parts) >= 3 && parts[2] != "" {
					version := strings.TrimSpace(parts[2])
					version = strings.TrimPrefix(version, ">=")
					version = strings.TrimPrefix(version, "==")
					dep.Version = version
				}
				
				dependencies = append(dependencies, dep)
			}
		}
	}
	
	s.LogProgress(ctx, "Found %d Python dependencies in setup.cfg", len(dependencies))
	return dependencies, nil
}

func init() {
	// Auto-register the scanner
	NewPythonDependencyScanner()
}