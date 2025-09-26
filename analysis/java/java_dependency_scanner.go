package java

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/models"
)

// JavaDependencyScanner scans Java dependencies from various build files
type JavaDependencyScanner struct {
	*analysis.BaseDependencyScanner
}

// MavenDependency represents a Maven dependency
type MavenDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

// MavenPOM represents a simplified Maven POM structure
type MavenPOM struct {
	Dependencies []MavenDependency `xml:"dependencies>dependency"`
}

// NewJavaDependencyScanner creates a new Java dependency scanner
func NewJavaDependencyScanner() *JavaDependencyScanner {
	scanner := &JavaDependencyScanner{
		BaseDependencyScanner: analysis.NewBaseDependencyScanner("java", []string{
			"pom.xml",
			"build.gradle",
			"build.gradle.kts",
			"gradle.properties",
			"dependencies.txt", // For simple dependency lists
		}),
	}

	return scanner
}

// ScanFile scans a Java build file and extracts dependencies
func (s *JavaDependencyScanner) ScanFile(ctx *models.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	fileName := strings.ToLower(filepath.Base(filePath))

	switch {
	case fileName == "pom.xml":
		return s.scanMavenPOM(ctx, filePath, content)
	case strings.HasPrefix(fileName, "build.gradle"):
		return s.scanGradleBuild(ctx, filePath, content)
	case fileName == "gradle.properties":
		return s.scanGradleProperties(ctx, filePath, content)
	case fileName == "dependencies.txt":
		return s.scanDependenciesTxt(ctx, filePath, content)
	default:
		return []*models.Dependency{}, nil
	}
}

// scanMavenPOM scans a Maven POM file
func (s *JavaDependencyScanner) scanMavenPOM(ctx *models.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	var pom MavenPOM
	if err := xml.Unmarshal(content, &pom); err != nil {
		// If XML parsing fails, try regex fallback for malformed XML
		return s.scanMavenPOMRegex(ctx, filePath, content)
	}

	var dependencies []*models.Dependency
	for _, dep := range pom.Dependencies {
		if dep.GroupID == "" || dep.ArtifactID == "" {
			continue
		}

		dependency := &models.Dependency{
			Name:    fmt.Sprintf("%s:%s", dep.GroupID, dep.ArtifactID),
			Version: dep.Version,
			Type:    models.DependencyTypeMaven,
			Source:  filePath,
		}

		dependencies = append(dependencies, dependency)
	}

	return dependencies, nil
}

// scanMavenPOMRegex uses regex to parse Maven dependencies as fallback
func (s *JavaDependencyScanner) scanMavenPOMRegex(ctx *models.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	// Regex to match Maven dependencies
	depRegex := regexp.MustCompile(`<dependency>[\s\S]*?<groupId>(.*?)</groupId>[\s\S]*?<artifactId>(.*?)</artifactId>[\s\S]*?(?:<version>(.*?)</version>)?[\s\S]*?(?:<scope>(.*?)</scope>)?[\s\S]*?</dependency>`)

	matches := depRegex.FindAllSubmatch(content, -1)
	var dependencies []*models.Dependency

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		groupID := string(match[1])
		artifactID := string(match[2])
		version := ""

		if len(match) > 3 && match[3] != nil {
			version = string(match[3])
		}

		dependency := &models.Dependency{
			Name:    fmt.Sprintf("%s:%s", groupID, artifactID),
			Version: version,
			Type:    models.DependencyTypeMaven,
			Source:  filePath,
		}

		dependencies = append(dependencies, dependency)
	}

	return dependencies, nil
}

// scanGradleBuild scans a Gradle build file
func (s *JavaDependencyScanner) scanGradleBuild(ctx *models.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	// Regex patterns for Gradle dependencies
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(implementation|compile|testImplementation|testCompile|runtimeOnly|compileOnly)\s+['"](.*?)['"]`),
		regexp.MustCompile(`(implementation|compile|testImplementation|testCompile|runtimeOnly|compileOnly)\s+group:\s*['"](.*?)['"],\s*name:\s*['"](.*?)['"],\s*version:\s*['"](.*?)['"]`),
		regexp.MustCompile(`(implementation|compile|testImplementation|testCompile|runtimeOnly|compileOnly)\s+[(]group:\s*['"](.*?)['"],\s*name:\s*['"](.*?)['"],\s*version:\s*['"](.*?)['"][)]`),
	}

	var dependencies []*models.Dependency
	scanner := bufio.NewScanner(bytes.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		for _, pattern := range patterns {
			matches := pattern.FindStringSubmatch(line)
			if len(matches) >= 3 {
				var name, version string

				if len(matches) == 3 {
					// Simple format: implementation 'group:artifact:version'
					parts := strings.Split(matches[2], ":")
					if len(parts) >= 2 {
						name = fmt.Sprintf("%s:%s", parts[0], parts[1])
						if len(parts) >= 3 {
							version = parts[2]
						}
					}
				} else if len(matches) >= 5 {
					// Extended format: implementation group: 'group', name: 'artifact', version: 'version'
					name = fmt.Sprintf("%s:%s", matches[2], matches[3])
					version = matches[4]
				}

				if name != "" {
					dependency := &models.Dependency{
						Name:    name,
						Version: version,
						Type:    models.DependencyTypeGo, // Using go as generic for gradle since no gradle type exists
						Source:  filePath,
					}
					dependencies = append(dependencies, dependency)
				}
			}
		}
	}

	return dependencies, nil
}

// scanGradleProperties scans gradle.properties for version information
func (s *JavaDependencyScanner) scanGradleProperties(ctx *models.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	// gradle.properties typically contains version variables, not direct dependencies
	// We'll parse it for potential version info but won't create dependencies from it
	return []*models.Dependency{}, nil
}

// scanDependenciesTxt scans a simple text file with dependencies
func (s *JavaDependencyScanner) scanDependenciesTxt(ctx *models.ScanContext, filePath string, content []byte) ([]*models.Dependency, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var dependencies []*models.Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Expected format: group:artifact:version or group:artifact
		parts := strings.Split(line, ":")
		if len(parts) >= 2 {
			name := fmt.Sprintf("%s:%s", parts[0], parts[1])
			version := ""
			if len(parts) >= 3 {
				version = parts[2]
			}

			dependency := &models.Dependency{
				Name:    name,
				Version: version,
				Type:    models.DependencyTypeMaven,
				Source:  filePath,
			}
			dependencies = append(dependencies, dependency)
		}
	}

	return dependencies, nil
}
