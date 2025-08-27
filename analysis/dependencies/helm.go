package dependencies

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/models"
	"gopkg.in/yaml.v3"
)

// HelmDependencyScanner scans Helm chart dependencies
type HelmDependencyScanner struct {
	*analysis.BaseDependencyScanner
	resolver *analysis.ResolutionService
}

// NewHelmDependencyScanner creates a new Helm dependency scanner
func NewHelmDependencyScanner() *HelmDependencyScanner {
	scanner := &HelmDependencyScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner("helm",
			[]string{"Chart.yaml", "Chart.lock", "requirements.yaml", "requirements.lock", "values.yaml", "*values.yaml", "values-*.yaml"}),
	}

	// Register with the global registry
	analysis.DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// NewHelmDependencyScannerWithResolver creates a new Helm dependency scanner with a resolution service
func NewHelmDependencyScannerWithResolver(resolver *analysis.ResolutionService) *HelmDependencyScanner {
	scanner := &HelmDependencyScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner("helm",
			[]string{"Chart.yaml", "Chart.lock", "requirements.yaml", "requirements.lock", "values.yaml", "*values.yaml", "values-*.yaml"}),
		resolver: resolver,
	}

	// Register with the global registry
	analysis.DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// ScanFile scans a Helm chart file and extracts dependencies
func (s *HelmDependencyScanner) ScanFile(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	filename := path.Base(strings.ToLower(filepath))

	switch {
	case filename == "chart.yaml":
		return s.scanChartYaml(ctx, filepath, content)
	case filename == "chart.lock":
		return s.scanChartLock(ctx, filepath, content)
	case filename == "requirements.yaml":
		return s.scanRequirementsYaml(ctx, filepath, content)
	case filename == "requirements.lock":
		return s.scanRequirementsLock(ctx, filepath, content)
	case strings.Contains(filename, "values") && strings.HasSuffix(filename, ".yaml"):
		return s.scanValuesYaml(ctx, filepath, content)
	default:
		return nil, fmt.Errorf("unsupported Helm file: %s", filepath)
	}
}

// scanChartYaml scans Chart.yaml for dependencies (Helm v3)
func (s *HelmDependencyScanner) scanChartYaml(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Helm chart from %s", filepath)

	var chart struct {
		APIVersion   string `yaml:"apiVersion"`
		Name         string `yaml:"name"`
		Version      string `yaml:"version"`
		Dependencies []struct {
			Name       string   `yaml:"name"`
			Version    string   `yaml:"version"`
			Repository string   `yaml:"repository"`
			Condition  string   `yaml:"condition"`
			Tags       []string `yaml:"tags"`
			Enabled    *bool    `yaml:"enabled"`
			Alias      string   `yaml:"alias"`
		} `yaml:"dependencies"`
	}

	if err := yaml.Unmarshal(content, &chart); err != nil {
		return nil, fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}

	var dependencies []*models.Dependency

	for i, dep := range chart.Dependencies {
		// Skip disabled dependencies
		if dep.Enabled != nil && !*dep.Enabled {
			continue
		}

		sourceInfo := fmt.Sprintf("%s:%d", path.Base(filepath), s.findDependencyLine(content, dep.Name, i))
		dependency := &models.Dependency{
			Name:    dep.Name,
			Version: dep.Version,
			Type:    models.DependencyTypeHelm,
			Source:  sourceInfo,
			Depth:   0, // Direct dependencies
		}

		// Use alias if provided
		if dep.Alias != "" {
			dependency.Name = dep.Alias
			dependency.Package = []string{dep.Name} // Store original name in packages
		}

		// Use resolver if available, otherwise fall back to heuristics
		if s.resolver != nil {
			if gitURL, err := s.resolver.ResolveGitURL(dep.Name, "helm"); err == nil && gitURL != "" {
				dependency.Git = gitURL
			} else if dep.Repository != "" {
				// If resolver didn't find anything, try the heuristics as fallback
				dependency.Git = s.parseHelmRepository(dep.Repository, dep.Name)
			}
		} else if dep.Repository != "" {
			// No resolver available, use heuristics
			dependency.Git = s.parseHelmRepository(dep.Repository, dep.Name)
		}

		dependencies = append(dependencies, dependency)
		ctx.Debugf("Found Helm dependency: %s@%s from %s at %s", dep.Name, dep.Version, dep.Repository, dependency.Source)
	}

	ctx.Debugf("Found %d Helm dependencies", len(dependencies))
	return dependencies, nil
}

// scanChartLock scans Chart.lock for resolved dependencies (Helm v3)
func (s *HelmDependencyScanner) scanChartLock(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Helm lock file from %s", filepath)

	var lock struct {
		Dependencies []struct {
			Name       string `yaml:"name"`
			Version    string `yaml:"version"`
			Repository string `yaml:"repository"`
			Digest     string `yaml:"digest"`
		} `yaml:"dependencies"`
		Digest    string `yaml:"digest"`
		Generated string `yaml:"generated"`
	}

	if err := yaml.Unmarshal(content, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse Chart.lock: %w", err)
	}

	var dependencies []*models.Dependency

	for _, dep := range lock.Dependencies {
		dependency := &models.Dependency{
			Name:    dep.Name,
			Version: dep.Version,
			Type:    models.DependencyTypeHelm,
			Depth:   0, // Direct dependencies
		}

		// Parse repository URL
		if dep.Repository != "" {
			dependency.Git = s.parseHelmRepository(dep.Repository, dep.Name)
		}

		// Store digest as part of version info if present
		if dep.Digest != "" {
			dependency.Version = fmt.Sprintf("%s (digest: %s)", dep.Version, dep.Digest[:12])
		}

		dependencies = append(dependencies, dependency)
		ctx.Debugf("Found locked Helm dependency: %s@%s", dep.Name, dep.Version)
	}

	ctx.Debugf("Found %d locked Helm dependencies", len(dependencies))
	return dependencies, nil
}

// scanRequirementsYaml scans requirements.yaml for dependencies (Helm v2)
func (s *HelmDependencyScanner) scanRequirementsYaml(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Helm v2 requirements from %s", filepath)

	var requirements struct {
		Dependencies []struct {
			Name       string   `yaml:"name"`
			Version    string   `yaml:"version"`
			Repository string   `yaml:"repository"`
			Condition  string   `yaml:"condition"`
			Tags       []string `yaml:"tags"`
			Enabled    *bool    `yaml:"enabled"`
			Alias      string   `yaml:"alias"`
		} `yaml:"dependencies"`
	}

	if err := yaml.Unmarshal(content, &requirements); err != nil {
		return nil, fmt.Errorf("failed to parse requirements.yaml: %w", err)
	}

	var dependencies []*models.Dependency

	for _, dep := range requirements.Dependencies {
		// Skip disabled dependencies
		if dep.Enabled != nil && !*dep.Enabled {
			continue
		}

		dependency := &models.Dependency{
			Name:    dep.Name,
			Version: dep.Version,
			Type:    models.DependencyTypeHelm,
			Depth:   0, // Direct dependencies
		}

		// Use alias if provided
		if dep.Alias != "" {
			dependency.Name = dep.Alias
			dependency.Package = []string{dep.Name} // Store original name in packages
		}

		// Parse repository URL
		if dep.Repository != "" {
			dependency.Git = s.parseHelmRepository(dep.Repository, dep.Name)
		}

		dependencies = append(dependencies, dependency)
		ctx.Debugf("Found Helm v2 dependency: %s@%s", dep.Name, dep.Version)
	}

	ctx.Debugf("Found %d Helm v2 dependencies", len(dependencies))
	return dependencies, nil
}

// scanRequirementsLock scans requirements.lock for resolved dependencies (Helm v2)
func (s *HelmDependencyScanner) scanRequirementsLock(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Helm v2 lock file from %s", filepath)

	var lock struct {
		Dependencies []struct {
			Name       string `yaml:"name"`
			Version    string `yaml:"version"`
			Repository string `yaml:"repository"`
			Digest     string `yaml:"digest"`
		} `yaml:"dependencies"`
		Digest    string `yaml:"digest"`
		Generated string `yaml:"generated"`
	}

	if err := yaml.Unmarshal(content, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse requirements.lock: %w", err)
	}

	var dependencies []*models.Dependency

	for _, dep := range lock.Dependencies {
		dependency := &models.Dependency{
			Name:    dep.Name,
			Version: dep.Version,
			Type:    models.DependencyTypeHelm,
			Depth:   0, // Direct dependencies
		}

		// Parse repository URL
		if dep.Repository != "" {
			dependency.Git = s.parseHelmRepository(dep.Repository, dep.Name)
		}

		// Store digest as part of version info if present
		if dep.Digest != "" {
			dependency.Version = fmt.Sprintf("%s (digest: %s)", dep.Version, dep.Digest[:12])
		}

		dependencies = append(dependencies, dependency)
		ctx.Debugf("Found locked Helm v2 dependency: %s@%s", dep.Name, dep.Version)
	}

	ctx.Debugf("Found %d locked Helm v2 dependencies", len(dependencies))
	return dependencies, nil
}

// scanValuesYaml scans values.yaml files for Docker image references
func (s *HelmDependencyScanner) scanValuesYaml(ctx *analysis.ScanContext, filepath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Docker images from Helm values file %s", filepath)

	var valuesData interface{}
	if err := yaml.Unmarshal(content, &valuesData); err != nil {
		return nil, fmt.Errorf("failed to parse values YAML: %w", err)
	}

	var dependencies []*models.Dependency

	// Extract global registry configuration first
	global := s.extractGlobalConfig(valuesData)

	// Recursively scan for image references
	s.scanForImages(valuesData, "", filepath, global, &dependencies)

	ctx.Debugf("Found %d Docker images in values file", len(dependencies))
	return dependencies, nil
}

// GlobalConfig holds global image registry configuration
type GlobalConfig struct {
	Registry string
	Prefix   string
}

// extractGlobalConfig extracts global image registry configuration
func (s *HelmDependencyScanner) extractGlobalConfig(data interface{}) GlobalConfig {
	config := GlobalConfig{}

	if dataMap, ok := data.(map[string]interface{}); ok {
		if globalData, exists := dataMap["global"]; exists {
			if globalMap, ok := globalData.(map[string]interface{}); ok {
				if registry, ok := globalMap["imageRegistry"].(string); ok {
					config.Registry = registry
				}
				if prefix, ok := globalMap["imagePrefix"].(string); ok {
					config.Prefix = prefix
				}
			}
		}
	}

	return config
}

// scanForImages recursively scans YAML data for image references
func (s *HelmDependencyScanner) scanForImages(data interface{}, path, filepath string, global GlobalConfig, dependencies *[]*models.Dependency) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			currentPath := s.buildPath(path, key)

			// Check for image patterns at this level
			if s.isImageKey(key) {
				s.processImageValue(value, currentPath, filepath, global, dependencies)
			} else if key == "image" {
				// Handle both direct image strings and nested image objects
				if imageStr, ok := value.(string); ok && imageStr != "" {
					// Direct image string
					s.processImageValue(value, currentPath, filepath, global, dependencies)
				} else {
					// Nested image object (image.repository, image.tag)
					s.processImageObject(value, currentPath, filepath, global, dependencies)
				}
			} else {
				// Recurse into nested structures
				s.scanForImages(value, currentPath, filepath, global, dependencies)
			}
		}
	case []interface{}:
		for i, item := range v {
			currentPath := s.buildPath(path, fmt.Sprintf("[%d]", i))
			s.scanForImages(item, currentPath, filepath, global, dependencies)
		}
	}
}

// isImageKey checks if a key likely contains a Docker image
func (s *HelmDependencyScanner) isImageKey(key string) bool {
	key = strings.ToLower(key)
	// Skip global configuration keys and other non-image keys
	if strings.Contains(key, "imagepullpolicy") || strings.Contains(key, "imagepullsecret") ||
		strings.Contains(key, "imageregistry") || strings.Contains(key, "imageprefix") {
		return false
	}
	// Only match keys that clearly indicate a direct image reference (but not the nested "image" object)
	return strings.HasSuffix(key, "image") && key != "image"
}

// processImageValue processes a direct image value (e.g., "nginx:1.21")
func (s *HelmDependencyScanner) processImageValue(value interface{}, path, filepath string, global GlobalConfig, dependencies *[]*models.Dependency) {
	if imageStr, ok := value.(string); ok && imageStr != "" {
		// Skip template variables and empty values
		if strings.Contains(imageStr, "{{") || imageStr == "" {
			return
		}

		image := s.resolveImageWithGlobal(imageStr, global)
		dep := s.createDockerDependency(image, filepath, path)
		*dependencies = append(*dependencies, dep)
	}
}

// processImageObject processes an image object with repository/tag structure
func (s *HelmDependencyScanner) processImageObject(value interface{}, path, filepath string, global GlobalConfig, dependencies *[]*models.Dependency) {
	if imageMap, ok := value.(map[string]interface{}); ok {
		repository := ""
		tag := ""

		// Extract repository and tag
		if repo, ok := imageMap["repository"].(string); ok {
			repository = repo
		}
		if tagValue, ok := imageMap["tag"].(string); ok {
			tag = tagValue
		}

		if repository != "" {
			// Skip template variables
			if strings.Contains(repository, "{{") {
				return
			}

			// Build full image reference
			image := repository
			if tag != "" && !strings.Contains(tag, "{{") {
				image = repository + ":" + tag
			} else if !strings.Contains(repository, ":") {
				image = repository + ":latest"
			}

			// Apply global configuration
			image = s.resolveImageWithGlobal(image, global)

			// Use repository path for source tracking
			sourcePath := s.buildPath(path, "repository")
			dep := s.createDockerDependency(image, filepath, sourcePath)
			*dependencies = append(*dependencies, dep)
		}
	}
}

// resolveImageWithGlobal applies global registry and prefix configuration
func (s *HelmDependencyScanner) resolveImageWithGlobal(image string, global GlobalConfig) string {
	// If image already has a registry, don't modify it
	if strings.Contains(image, "/") && (strings.Contains(image, ".") || strings.Contains(image, ":")) {
		return image
	}

	// Apply global prefix and registry
	if global.Prefix != "" {
		image = global.Prefix + "/" + image
	}

	if global.Registry != "" {
		image = global.Registry + "/" + image
	}

	return image
}

// createDockerDependency creates a Docker dependency from an image reference
func (s *HelmDependencyScanner) createDockerDependency(image, filepath, sourcePath string) *models.Dependency {
	dep := &models.Dependency{
		Name:   image,
		Type:   models.DependencyTypeDocker,
		Source: fmt.Sprintf("%s:%s", path.Base(filepath), sourcePath),
		Depth:  0, // Direct dependencies
	}

	// Parse image for version information
	if strings.Contains(image, ":") && !strings.Contains(image, "://") {
		parts := strings.SplitN(image, ":", 2)
		dep.Name = parts[0]
		dep.Version = parts[1]
	} else {
		dep.Version = "latest"
	}

	// Use resolver if available for Git URL detection
	if s.resolver != nil {
		if gitURL, err := s.resolver.ResolveGitURL(dep.Name, "docker"); err == nil && gitURL != "" {
			dep.Git = gitURL
		}
	}

	return dep
}

// buildPath builds a YAML path string for source tracking
func (s *HelmDependencyScanner) buildPath(currentPath, key string) string {
	if currentPath == "" {
		return key
	}
	return currentPath + "." + key
}

// parseHelmRepository converts Helm repository URLs to Git URLs where possible
func (s *HelmDependencyScanner) parseHelmRepository(repository, chartName string) string {
	// Handle file:// URLs (local charts)
	if strings.HasPrefix(repository, "file://") {
		return ""
	}

	// Handle OCI registries
	if strings.HasPrefix(repository, "oci://") {
		return s.detectGitHubFromOCI(repository)
	}

	// Try to detect GitHub repository using various strategies
	return s.detectGitHubRepository(repository, chartName)
}

// detectGitHubFromOCI extracts GitHub repository from OCI URLs
func (s *HelmDependencyScanner) detectGitHubFromOCI(ociURL string) string {
	ociURL = strings.TrimPrefix(ociURL, "oci://")

	// GitHub Container Registry
	if strings.HasPrefix(ociURL, "ghcr.io/") {
		parts := strings.Split(strings.TrimPrefix(ociURL, "ghcr.io/"), "/")
		if len(parts) >= 2 {
			return fmt.Sprintf("https://github.com/%s/%s", parts[0], parts[1])
		}
	}

	// Return the OCI URL as-is for other registries
	return "oci://" + ociURL
}

// detectGitHubRepository uses heuristics to detect GitHub repositories for Helm charts
func (s *HelmDependencyScanner) detectGitHubRepository(repository, chartName string) string {
	// First try to extract GitHub URL directly
	if githubRepo := s.extractGitHubRepoFromURL(repository); githubRepo != "" {
		return githubRepo
	}

	// Try common Helm repository patterns
	repoLower := strings.ToLower(repository)

	// GitHub Pages pattern: https://USERNAME.github.io/REPO
	if strings.Contains(repoLower, ".github.io") {
		re := regexp.MustCompile(`https?://([^/]+)\.github\.io/([^/]+)`)
		matches := re.FindStringSubmatch(repository)
		if len(matches) >= 3 {
			org := matches[1]
			repo := matches[2]

			// If this is a charts repository, try individual chart repo first
			if repo == "charts" || repo == "helm-charts" {
				// Try individual chart repository first
				individualRepo := fmt.Sprintf("https://github.com/%s/%s", org, chartName)

				// Use resolver to validate if available, otherwise try direct check
				if s.resolver != nil {
					if valid, finalURL, err := s.resolver.ValidateGitURL(individualRepo); err == nil && valid {
						return finalURL // Return the redirected URL if any
					}
				}

				// Fall back to the charts monorepo
				return fmt.Sprintf("https://github.com/%s/%s", org, repo)
			}

			// For non-chart repositories, return as-is
			return fmt.Sprintf("https://github.com/%s/%s", matches[1], matches[2])
		}
	}

	// Try to infer from known patterns
	return s.tryCommonNamingConventions(repository, chartName)
}

// extractGitHubRepoFromURL extracts a GitHub repository URL from various URL formats
func (s *HelmDependencyScanner) extractGitHubRepoFromURL(url string) string {
	if url == "" {
		return ""
	}

	// Normalize the URL
	normalizedURL := strings.TrimSpace(url)
	normalizedURL = strings.TrimSuffix(normalizedURL, "/")

	// Handle different GitHub URL formats
	patterns := []string{
		// Standard HTTPS URLs
		`^https?://github\.com/([^/]+/[^/]+?)(?:\.git)?/?$`,
		`^https?://github\.com/([^/]+/[^/]+?)(?:\.git)?/.*$`,
		// SSH URLs
		`^git@github\.com:([^/]+/[^/]+?)(?:\.git)?$`,
		`^ssh://git@github\.com/([^/]+/[^/]+?)(?:\.git)?$`,
		// Git protocol URLs
		`^git://github\.com/([^/]+/[^/]+?)(?:\.git)?$`,
		// URLs without protocol
		`^github\.com/([^/]+/[^/]+?)(?:\.git)?/?$`,
		`^github\.com/([^/]+/[^/]+?)(?:\.git)?/.*$`,
		// Raw/user content URLs
		`^https?://raw\.githubusercontent\.com/([^/]+/[^/]+)/.*$`,
		// GitHub Pages URLs
		`^https?://([^/]+)\.github\.io/([^/]+)/?.*$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(normalizedURL)
		if len(matches) >= 2 {
			repoPath := matches[1]
			// For GitHub Pages, combine org and repo
			if strings.Contains(pattern, "github\\.io") && len(matches) >= 3 {
				repoPath = matches[1] + "/" + matches[2]
			}
			// Remove any trailing .git
			repoPath = strings.TrimSuffix(repoPath, ".git")
			return fmt.Sprintf("https://github.com/%s", repoPath)
		}
	}

	return ""
}

// tryCommonNamingConventions tries to guess GitHub repositories based on common naming patterns
func (s *HelmDependencyScanner) tryCommonNamingConventions(repository, chartName string) string {
	// Common patterns for Helm chart repositories
	commonPatterns := map[string]string{
		"bitnami":           "bitnami/charts",
		"prometheus":        "prometheus-community/helm-charts",
		"grafana":           "grafana/helm-charts",
		"elastic":           "elastic/helm-charts",
		"ingress-nginx":     "kubernetes/ingress-nginx",
		"cert-manager":      "cert-manager/cert-manager",
		"jetstack":          "cert-manager/cert-manager",
		"fluxcd":            "fluxcd/charts",
		"argo":              "argoproj/argo-helm",
		"gitlab":            "gitlab-org/charts",
		"hashicorp":         fmt.Sprintf("hashicorp/%s", chartName),
		"kubernetes-charts": "helm/charts",
		"stable":            "helm/charts",
		"incubator":         "helm/charts",
	}

	repoLower := strings.ToLower(repository)
	for key, githubPath := range commonPatterns {
		if strings.Contains(repoLower, key) {
			return fmt.Sprintf("https://github.com/%s", githubPath)
		}
	}

	// If nothing matches, return the original repository URL
	return repository
}

// findDependencyLine attempts to find the line number where a dependency is declared
func (s *HelmDependencyScanner) findDependencyLine(content []byte, dependencyName string, fallbackIdx int) int {
	lines := strings.Split(string(content), "\n")
	inDependencies := false
	depCount := 0

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Check if we're entering the dependencies section
		if trimmedLine == "dependencies:" {
			inDependencies = true
			continue
		}

		// If we're in dependencies section and find the dependency name
		if inDependencies {
			// Check if we've left the dependencies section
			if !strings.HasPrefix(trimmedLine, "-") && !strings.HasPrefix(trimmedLine, " ") && trimmedLine != "" && !strings.HasPrefix(trimmedLine, "name:") && !strings.HasPrefix(trimmedLine, "version:") && !strings.HasPrefix(trimmedLine, "repository:") && !strings.HasPrefix(trimmedLine, "condition:") {
				break
			}

			// Check if this line contains our dependency name
			if strings.Contains(trimmedLine, fmt.Sprintf("name: %s", dependencyName)) || strings.Contains(trimmedLine, fmt.Sprintf("- name: %s", dependencyName)) {
				return i + 1 // Line numbers start at 1
			}

			// If this is a new dependency entry, increment counter
			if strings.HasPrefix(trimmedLine, "- name:") {
				if depCount == fallbackIdx {
					return i + 1
				}
				depCount++
			}
		}
	}

	// Fallback: estimate based on dependencies section start + index
	return 10 + fallbackIdx*4 // Rough estimate assuming dependencies start around line 10 and each takes ~4 lines
}

func init() {
	// Auto-register the scanner
	NewHelmDependencyScanner()
}
