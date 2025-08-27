package dependencies

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/models"
	"gopkg.in/yaml.v3"
)

// DockerDependencyScanner scans Docker and container-related dependencies
type DockerDependencyScanner struct {
	*analysis.BaseDependencyScanner
	resolver *analysis.ResolutionService
}

// NewDockerDependencyScanner creates a new Docker dependency scanner
func NewDockerDependencyScanner() *DockerDependencyScanner {
	scanner := &DockerDependencyScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner("docker",
			[]string{"Dockerfile", "Dockerfile.*", "*.dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}),
	}

	// Register with the global registry
	analysis.DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// NewDockerDependencyScannerWithResolver creates a new Docker dependency scanner with a resolution service
func NewDockerDependencyScannerWithResolver(resolver *analysis.ResolutionService) *DockerDependencyScanner {
	scanner := &DockerDependencyScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner("docker",
			[]string{"Dockerfile", "Dockerfile.*", "*.dockerfile", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}),
		resolver: resolver,
	}

	// Register with the global registry
	analysis.DefaultDependencyRegistry.Register(scanner)

	return scanner
}

// ScanFile scans a Docker-related file and extracts dependencies
func (s *DockerDependencyScanner) ScanFile(ctx *analysis.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	filename := strings.ToLower(filePath)

	if strings.Contains(filename, "dockerfile") || strings.HasSuffix(filename, ".dockerfile") {
		return s.scanDockerfile(ctx, filePath, content)
	} else if strings.Contains(filename, "compose") && (strings.HasSuffix(filename, ".yml") || strings.HasSuffix(filename, ".yaml")) {
		return s.scanDockerCompose(ctx, filePath, content)
	}

	return nil, fmt.Errorf("unsupported Docker file: %s", filePath)
}

// makeRelativePath attempts to make a path relative to the scan root directory
func makeRelativePath(path string, scanRoot string) string {
	if scanRoot == "" {
		// Fallback to current working directory if no scan root
		cwd, err := os.Getwd()
		if err != nil {
			return path
		}
		scanRoot = cwd
	}

	relPath, err := filepath.Rel(scanRoot, path)
	if err != nil {
		return path // Return original if we can't make it relative
	}

	return relPath
}

// scanDockerfile scans Dockerfile for base images and other dependencies
func (s *DockerDependencyScanner) scanDockerfile(ctx *analysis.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Docker dependencies from %s", filePath)

	// Make path relative for source tracking
	scanRoot := ""
	if ctx != nil {
		scanRoot = ctx.ScanRoot
	}
	relPath := makeRelativePath(filePath, scanRoot)

	var dependencies []*models.Dependency
	scanner := bufio.NewScanner(bytes.NewReader(content))

	// Regex patterns for Dockerfile instructions
	fromPattern := regexp.MustCompile(`(?i)^FROM\s+(?:--platform=[^\s]+\s+)?([^\s]+)(?:\s+AS\s+[^\s]+)?`)
	copyFromPattern := regexp.MustCompile(`(?i)^COPY\s+--from=([^\s]+)`)
	argPattern := regexp.MustCompile(`(?i)^ARG\s+([^=\s]+)(?:=([^\s]+))?`)

	// Track ARG values for variable substitution
	argValues := make(map[string]string)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle line continuation
		for strings.HasSuffix(line, "\\") && scanner.Scan() {
			lineNum++
			line = strings.TrimSuffix(line, "\\") + " " + strings.TrimSpace(scanner.Text())
		}

		// Parse ARG instructions
		if matches := argPattern.FindStringSubmatch(line); len(matches) > 1 {
			argName := matches[1]
			argValue := ""
			if len(matches) > 2 {
				argValue = matches[2]
			}
			argValues[argName] = argValue
			ctx.Debugf("Found ARG %s=%s", argName, argValue)
			continue
		}

		// Parse FROM instructions
		if matches := fromPattern.FindStringSubmatch(line); len(matches) > 1 {
			image := matches[1]

			// Substitute ARG variables if present
			image = s.substituteVariables(image, argValues)

			// Skip scratch image
			if image == "scratch" {
				continue
			}

			dep := s.parseDockerImage(image)
			dep.Source = fmt.Sprintf("%s:%d", relPath, lineNum)
			dependencies = append(dependencies, dep)
			ctx.Debugf("Found base image: %s at line %d", image, lineNum)
		}

		// Parse COPY --from instructions (multi-stage builds)
		if matches := copyFromPattern.FindStringSubmatch(line); len(matches) > 1 {
			source := matches[1]

			// Skip if copying from a build stage (not an external image)
			if !strings.Contains(source, "/") && !strings.Contains(source, ":") {
				continue
			}

			// Substitute ARG variables if present
			source = s.substituteVariables(source, argValues)

			dep := s.parseDockerImage(source)
			dep.Source = fmt.Sprintf("%s:%d", relPath, lineNum)
			dependencies = append(dependencies, dep)
			ctx.Debugf("Found COPY --from image: %s at line %d", source, lineNum)
		}
	}

	ctx.Debugf("Found %d Docker dependencies", len(dependencies))
	return dependencies, nil
}

// scanDockerCompose scans docker-compose files for service images
func (s *DockerDependencyScanner) scanDockerCompose(ctx *analysis.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	ctx.Debugf("Scanning Docker Compose file from %s", filePath)

	// Make path relative for source tracking
	scanRoot := ""
	if ctx != nil {
		scanRoot = ctx.ScanRoot
	}
	relPath := makeRelativePath(filePath, scanRoot)

	var compose struct {
		Version  string `yaml:"version"`
		Services map[string]struct {
			Image       string      `yaml:"image"`
			Build       interface{} `yaml:"build"`
			Environment interface{} `yaml:"environment"` // Can be []string or map[string]interface{}
			EnvFile     interface{} `yaml:"env_file"`    // Can be string or []string
		} `yaml:"services"`
	}

	if err := yaml.Unmarshal(content, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose file: %w", err)
	}

	// Also scan line by line to get line numbers for source tracking
	lineMap := s.buildImageLineMap(content)

	var dependencies []*models.Dependency
	seen := make(map[string]bool)

	for serviceName, service := range compose.Services {
		if service.Image == "" {
			// Service is built locally, not an external dependency
			continue
		}

		// Skip if we've already seen this image
		if seen[service.Image] {
			continue
		}
		seen[service.Image] = true

		dep := s.parseDockerImage(service.Image)

		// Add source location
		if lineNum, ok := lineMap[service.Image]; ok {
			dep.Source = fmt.Sprintf("%s:%d", relPath, lineNum)
		} else {
			// Fallback to just filename if we couldn't find the line
			dep.Source = relPath
		}

		ctx.Debugf("Found service %s using image: %s at %s", serviceName, service.Image, dep.Source)
		dependencies = append(dependencies, dep)
	}

	ctx.Debugf("Found %d Docker dependencies in compose file", len(dependencies))
	return dependencies, nil
}

// buildImageLineMap scans content to find line numbers for image definitions
func (s *DockerDependencyScanner) buildImageLineMap(content []byte) map[string]int {
	lineMap := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(content))

	// Pattern to match image: value lines
	// Matches lines like "    image: nginx:latest" or "image: postgres:15"
	imagePattern := regexp.MustCompile(`^\s*image:\s*["']?([^"'\s]+)["']?`)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check if this line defines an image
		if matches := imagePattern.FindStringSubmatch(line); len(matches) > 1 {
			image := matches[1]
			// Store the line number for this image
			// If the same image appears multiple times, the last occurrence wins
			lineMap[image] = lineNum
		}
	}

	return lineMap
}

// parseDockerImage parses a Docker image reference into a dependency
func (s *DockerDependencyScanner) parseDockerImage(image string) *models.Dependency {
	dep := &models.Dependency{
		Type: models.DependencyTypeDocker,
	}

	// Parse image reference: [registry/]namespace/name[:tag|@digest]
	// Examples:
	// - nginx:latest
	// - docker.io/library/nginx:1.21
	// - gcr.io/project/image:v1.0.0
	// - node:18-alpine
	// - myregistry.com:5000/myimage@sha256:abc123...

	// Handle digest references
	if strings.Contains(image, "@") {
		parts := strings.SplitN(image, "@", 2)
		image = parts[0]
		// Store digest in version field with @ prefix
		if len(parts) > 1 {
			dep.Version = "@" + parts[1]
		}
	}

	// Extract tag if present
	if strings.Contains(image, ":") {
		lastColon := strings.LastIndex(image, ":")
		if lastColon > 0 {
			afterColon := image[lastColon+1:]
			beforeColon := image[:lastColon]

			// Check if this is a registry port (e.g., myregistry.com:5000/image)
			// It's a port if there's a slash after the digits and the part before contains a dot (domain)
			isPort := false
			if strings.Contains(afterColon, "/") {
				// Split on slash to get potential port and rest of path
				parts := strings.SplitN(afterColon, "/", 2)
				potentialPort := parts[0]

				// Check if the potential port is all digits
				allDigits := true
				for _, c := range potentialPort {
					if c < '0' || c > '9' {
						allDigits = false
						break
					}
				}

				// It's a registry port if it's all digits and before colon looks like a domain
				if allDigits && strings.Contains(beforeColon, ".") {
					isPort = true
				}
			}

			// If it's not a port and we don't already have a version (from digest)
			if !isPort && dep.Version == "" {
				dep.Version = afterColon
				image = image[:lastColon]
			}
		}
	}

	// If no version specified, default to "latest"
	if dep.Version == "" {
		dep.Version = "latest"
	}

	// Set the name to the full image reference (without tag)
	dep.Name = image

	// Use resolver if available, otherwise fall back to heuristics
	if s.resolver != nil {
		if gitURL, err := s.resolver.ResolveGitURL(image, "docker"); err == nil && gitURL != "" {
			dep.Git = gitURL
		} else {
			// Resolver didn't find anything, use existing heuristics
			dep.Git = s.fallbackGitURL(image)
		}
	} else {
		// No resolver available, use heuristics
		dep.Git = s.fallbackGitURL(image)
	}

	// Add package information
	if strings.Contains(dep.Name, "/") {
		dep.Package = []string{dep.Name}
	}

	return dep
}

// fallbackGitURL provides the original heuristic-based Git URL detection
func (s *DockerDependencyScanner) fallbackGitURL(image string) string {
	// Determine registry and construct Git URL if applicable
	if strings.HasPrefix(image, "ghcr.io/") {
		// GitHub Container Registry
		parts := strings.Split(strings.TrimPrefix(image, "ghcr.io/"), "/")
		if len(parts) >= 2 {
			return fmt.Sprintf("https://github.com/%s/%s", parts[0], parts[1])
		}
	} else if strings.HasPrefix(image, "docker.io/") || !strings.Contains(image, "/") || strings.HasPrefix(image, "library/") {
		// Docker Hub
		imageName := image
		imageName = strings.TrimPrefix(imageName, "docker.io/")
		imageName = strings.TrimPrefix(imageName, "library/")

		if !strings.Contains(imageName, "/") {
			// Official image
			return fmt.Sprintf("https://hub.docker.com/_/%s", imageName)
		} else {
			// User/org image
			parts := strings.SplitN(imageName, "/", 2)
			return fmt.Sprintf("https://hub.docker.com/r/%s/%s", parts[0], parts[1])
		}
	} else if strings.HasPrefix(image, "gcr.io/") || strings.HasPrefix(image, "k8s.gcr.io/") ||
		strings.HasPrefix(image, "us.gcr.io/") || strings.HasPrefix(image, "eu.gcr.io/") ||
		strings.HasPrefix(image, "asia.gcr.io/") {
		// Google Container Registry
		return "" // GCR doesn't have a direct web URL for images
	} else if strings.HasPrefix(image, "quay.io/") {
		// Quay.io registry
		parts := strings.Split(strings.TrimPrefix(image, "quay.io/"), "/")
		if len(parts) >= 2 {
			return fmt.Sprintf("https://quay.io/repository/%s/%s", parts[0], strings.Join(parts[1:], "/"))
		}
	} else if strings.HasPrefix(image, "mcr.microsoft.com/") {
		// Microsoft Container Registry
		return "" // MCR doesn't have a direct web URL for images
	} else if strings.Count(image, "/") == 1 {
		// Assume Docker Hub for single slash images (e.g., ubuntu/nginx)
		parts := strings.Split(image, "/")
		return fmt.Sprintf("https://hub.docker.com/r/%s/%s", parts[0], parts[1])
	}

	return ""
}

// substituteVariables replaces ${VAR} or $VAR with their values
func (s *DockerDependencyScanner) substituteVariables(text string, variables map[string]string) string {
	// Replace ${VAR} format
	for name, value := range variables {
		text = strings.ReplaceAll(text, "${"+name+"}", value)
		text = strings.ReplaceAll(text, "$"+name, value)
	}

	// Handle default values ${VAR:-default}
	defaultPattern := regexp.MustCompile(`\$\{([^:}]+):-([^}]+)\}`)
	text = defaultPattern.ReplaceAllStringFunc(text, func(match string) string {
		matches := defaultPattern.FindStringSubmatch(match)
		if len(matches) == 3 {
			varName := matches[1]
			defaultValue := matches[2]
			if value, ok := variables[varName]; ok && value != "" {
				return value
			}
			return defaultValue
		}
		return match
	})

	return text
}

func init() {
	// Auto-register the scanner
	NewDockerDependencyScanner()
}
