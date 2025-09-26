package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
)

// Scanner orchestrates dependency scanning across multiple languages
type Scanner struct {
	Registry    *analysis.DependencyRegistry
	gitManager  git.GitRepositoryManager
	GitFilters  []string // Git URL filters for depth scanning
	NameFilters []string // Package name filters for depth scanning

	// Tracking for two-phase scanning
	visited        map[string]*git.VisitedDep
	repoVisited    map[string]bool
	discoveredDeps map[string][]*models.Dependency       // Map of parent -> dependencies
	mutex          sync.RWMutex                          //nolint:unused // Used in concurrent operations
	discoveryGroup task.TypedGroup[[]*models.Dependency] //nolint:unused // Used in task management
}

// NewScanner creates a new dependency scanner
func NewScanner() *Scanner {
	return &Scanner{
		Registry:       analysis.DefaultDependencyRegistry,
		visited:        make(map[string]*git.VisitedDep),
		gitManager:     git.NewGitRepositoryManager(".cache/arch-unit/repositories"),
		repoVisited:    make(map[string]bool),
		discoveredDeps: make(map[string][]*models.Dependency),
	}
}

// NewScannerWithRegistry creates a new dependency scanner with a custom registry
func NewScannerWithRegistry(registry *analysis.DependencyRegistry) *Scanner {
	return &Scanner{
		gitManager:     git.NewGitRepositoryManager(".cache/arch-unit/repositories"),
		Registry:       registry,
		visited:        make(map[string]*git.VisitedDep),
		repoVisited:    make(map[string]bool),
		discoveredDeps: make(map[string][]*models.Dependency),
	}
}

// ScanPath scans a path (local directory or git URL) for dependencies with optional depth traversal
func (s *Scanner) ScanPath(task *clicky.Task, pathOrURL string, maxDepth int) (*models.ScanResult, error) {
	// Create a scan context with the provided configuration
	ctx := models.NewScanContext(task, pathOrURL).WithDepth(maxDepth)
	return s.ScanWithContext(ctx, pathOrURL)
}

// ScanWithContext performs scanning with a configured context using the new walker
func (s *Scanner) ScanWithContext(ctx *models.ScanContext, pathOrURL string) (*models.ScanResult, error) {
	if ctx.MaxDepth < 0 {
		ctx.MaxDepth = 0 // Default to no depth traversal
	}

	// Parse path to determine if it's local or git
	gitURL, version, subdir, isGit := s.parseGitURLWithSubdir(pathOrURL)

	var rootPath string
	var scanType string

	if !isGit {
		// Local directory scanning
		ctx.Debugf("Scanning local dependencies in %s", pathOrURL)
		rootPath = pathOrURL
		scanType = "local"
		if ctx.MaxDepth > 0 {
			scanType = "mixed" // Will include git dependencies
		}
	} else {
		// Git repository scanning
		if ctx.MaxDepth == 0 {
			ctx.MaxDepth = 1 // For git URLs, default to at least depth 1
		}

		if subdir != "" {
			ctx.Infof("Scanning git repository %s@%s in subdirectory '%s'", gitURL, version, subdir)
		} else {
			ctx.Infof("Scanning git repository %s@%s", gitURL, version)
		}
		scanType = "git"

		// Set task logger for git operations
		if ctx.Task != nil {
			git.SetCurrentTaskLogger(ctx.Task)
		}

		// Initial checkout for git scanning
		worktreePath, err := s.gitManager.GetWorktreePath(gitURL, version, 1) // Shallow clone for dependency scanning
		if err != nil {
			return nil, fmt.Errorf("failed to checkout %s@%s: %w", gitURL, version, err)
		}

		// If subdirectory specified, navigate to it
		if subdir != "" {
			rootPath = filepath.Join(worktreePath, subdir)

			// Verify the subdirectory exists
			if _, err := os.Stat(rootPath); os.IsNotExist(err) {
				// Workaround: Check if the worktree was created with nested cache structure
				// Extract repo info to locate the actual worktree
				if strings.HasPrefix(gitURL, "https://github.com/") {
					parts := strings.Split(strings.TrimPrefix(gitURL, "https://github.com/"), "/")
					if len(parts) >= 2 {
						org := parts[0]
						repo := strings.TrimSuffix(parts[1], ".git")

						// Try the nested path that was actually created
						baseRepoPath := filepath.Join(".cache", "arch-unit", "repositories", "github.com", org, repo)
						nestedWorktreePath := filepath.Join(baseRepoPath, ".cache", "arch-unit", "repositories", "github.com", org, repo, "worktrees", version)
						nestedRootPath := filepath.Join(nestedWorktreePath, subdir)

						if _, nestedErr := os.Stat(nestedRootPath); nestedErr == nil {
							ctx.Warnf("Using nested worktree path due to git manager issue: %s", nestedWorktreePath)
							rootPath = nestedRootPath
						} else {
							return nil, fmt.Errorf("subdirectory '%s' does not exist in repository %s@%s", subdir, gitURL, version)
						}
					} else {
						return nil, fmt.Errorf("subdirectory '%s' does not exist in repository %s@%s", subdir, gitURL, version)
					}
				} else {
					return nil, fmt.Errorf("subdirectory '%s' does not exist in repository %s@%s", subdir, gitURL, version)
				}
			}
		} else {
			rootPath = worktreePath
		}

		// Update context with the scan root path
		ctx.ScanRoot = rootPath
	}

	// Create walker and perform the scan
	walker := NewDependencyWalker(s, ctx)
	walkResult := walker.Walk(ctx, rootPath, 0)

	// Build the final result using tree builder
	treeBuilder := NewTreeBuilder(s)
	result := treeBuilder.BuildScanResult(walkResult, scanType, ctx.MaxDepth)

	// Log final results
	if len(result.Conflicts) > 0 {
		ctx.Debugf("Found %d dependencies with %d conflicts", len(result.Dependencies), len(result.Conflicts))
	} else {
		ctx.Debugf("Found %d dependencies", len(result.Dependencies))
	}

	return result, nil
}

// ScanDirectory scans a directory for dependencies (backward compatibility)
func (s *Scanner) ScanDirectory(task *clicky.Task, dir string) ([]*models.Dependency, error) {
	// Use the unified ScanPath with depth 0 for backward compatibility
	result, err := s.ScanPath(task, dir, 0)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []*models.Dependency{}, nil // Return empty slice if result is nil
	}
	if result.Dependencies == nil {
		return []*models.Dependency{}, nil // Ensure we never return nil
	}
	return result.Dependencies, nil
}

// scanWithScanner scans for dependencies using a specific scanner

// matchesScanner checks if a file matches any of the scanner's supported patterns

// getDependencyKey returns a unique key for a dependency
func (s *Scanner) getDependencyKey(dep *models.Dependency) string {
	// Use name + type as the key
	return fmt.Sprintf("%s:%s", dep.Name, dep.Type)
}

// ScanWithDepth performs depth-based dependency scanning using the walker
func (s *Scanner) ScanWithDepth(task *clicky.Task, dir string, maxDepth int) (*git.DependencyTree, error) {
	// Create scan context
	ctx := models.NewScanContext(task, dir).WithDepth(maxDepth)

	// Use the new walker
	walker := NewDependencyWalker(s, ctx)
	walkResult := walker.Walk(ctx, dir, 0)

	// Build dependency tree using tree builder
	treeBuilder := NewTreeBuilder(s)
	return treeBuilder.BuildDependencyTree(walkResult), nil
}

// scanWithTwoPhases performs two-phase dependency scanning

// ScanGitRepository scans a specific git repository at a given version
func (s *Scanner) ScanGitRepository(task *clicky.Task, gitURL, version string, maxDepth int) (*git.DependencyTree, error) {

	// Set task logger for git operations
	if task != nil {
		git.SetCurrentTaskLogger(task)
	}

	// Get clone path for this repository/version
	worktreePath, err := s.gitManager.GetWorktreePath(gitURL, version, 1) // Shallow clone for dependency scanning
	if err != nil {
		return nil, fmt.Errorf("failed to checkout %s@%s: %w", gitURL, version, err)
	}

	// Scan the checked out repository with depth
	return s.ScanWithDepth(task, worktreePath, maxDepth)
}

// GetGitManager returns the git manager instance
func (s *Scanner) GetGitManager() git.GitRepositoryManager {
	return s.gitManager
}

// SetGitManager sets a custom git manager
func (s *Scanner) SetGitManager(gitManager git.GitRepositoryManager) {
	s.gitManager = gitManager
}

// HasGitSupport returns true if git support is configured
func (s *Scanner) HasGitSupport() bool {
	return s.gitManager != nil
}

// parseGitURL parses a path to determine if it's a git URL and extracts components
// Supports go-getter subdirectory syntax: https://github.com/user/repo//subdir@version

// parseGitURLWithSubdir parses git URLs with optional subdirectory support
// Returns: gitURL (without subdir), version, subdirectory, isGit
func (s *Scanner) parseGitURLWithSubdir(pathOrURL string) (gitURL, version, subdir string, isGit bool) {
	// Check for git URL patterns with version
	if strings.Contains(pathOrURL, "@") {
		parts := strings.Split(pathOrURL, "@")
		if len(parts) == 2 {
			candidateURL := parts[0]
			version = parts[1]

			// Check for go-getter subdirectory syntax (double slash after protocol)
			// Look for // that's not part of the protocol (https://, ssh://, etc.)
			protocolEnd := strings.Index(candidateURL, "://")
			if protocolEnd != -1 {
				afterProtocol := candidateURL[protocolEnd+3:] // Skip past ://
				if strings.Contains(afterProtocol, "//") {
					// Found subdirectory separator
					doubleSlashIdx := strings.Index(afterProtocol, "//")
					gitURL = candidateURL[:protocolEnd+3+doubleSlashIdx]
					subdir = afterProtocol[doubleSlashIdx+2:]
				} else {
					gitURL = candidateURL
				}
			} else {
				gitURL = candidateURL
			}

			// Check if it looks like a git URL
			if strings.Contains(gitURL, "github.com") ||
				strings.Contains(gitURL, "gitlab.com") ||
				strings.Contains(gitURL, "bitbucket.org") ||
				strings.HasPrefix(gitURL, "https://") ||
				strings.HasPrefix(gitURL, "git@") ||
				(strings.Contains(gitURL, "/") && !strings.HasPrefix(gitURL, "/") && !strings.HasPrefix(gitURL, "./")) {
				return gitURL, version, subdir, true
			}
		}
	}

	// Check for URLs without version (default to HEAD)
	candidateURL := pathOrURL

	// Check for go-getter subdirectory syntax (double slash after protocol)
	// Look for // that's not part of the protocol (https://, ssh://, etc.)
	protocolEnd := strings.Index(candidateURL, "://")
	if protocolEnd != -1 {
		afterProtocol := candidateURL[protocolEnd+3:] // Skip past ://
		if strings.Contains(afterProtocol, "//") {
			// Found subdirectory separator
			doubleSlashIdx := strings.Index(afterProtocol, "//")
			gitURL = candidateURL[:protocolEnd+3+doubleSlashIdx]
			subdir = afterProtocol[doubleSlashIdx+2:]
		} else {
			gitURL = candidateURL
		}
	} else {
		gitURL = candidateURL
	}

	if strings.HasPrefix(gitURL, "https://") ||
		strings.HasPrefix(gitURL, "git@") ||
		(strings.Contains(gitURL, "github.com") && !strings.HasPrefix(gitURL, "/")) ||
		(strings.Contains(gitURL, "gitlab.com") && !strings.HasPrefix(gitURL, "/")) {
		return gitURL, "HEAD", subdir, true
	}

	return pathOrURL, "", "", false
}

// scanDirectory is the renamed original ScanDirectory method for internal use

// scanDirectoryWithContext scans a directory using the provided context

// scanWithDepthTraversal performs depth traversal on local dependencies

// scanGitRepositoryWithDepth scans a specific git repository with depth

// convertDepTreeToModels converts git.DependencyTree to models types

// convertRefToModel converts a git.DependencyRef and VisitedDep to models.Dependency

// getRefKey returns a key for a dependency reference

// discoverScanFiles discovers all scannable files in a directory
func (s *Scanner) discoverScanFiles(dir string) ([]git.ScanJob, error) {
	var scanJobs []git.ScanJob
	processedGoMod := make(map[string]bool)

	// Get all registered scanners
	languages := s.Registry.List()
	for _, lang := range languages {
		scanner, ok := s.Registry.Get(lang)
		if !ok || scanner == nil {
			continue
		}

		// Check each supported file pattern for this scanner
		supportedFiles := scanner.SupportedFiles()
		for _, pattern := range supportedFiles {
			matches, err := filepath.Glob(filepath.Join(dir, pattern))
			if err != nil {
				continue // Skip patterns that fail to glob
			}

			for _, match := range matches {
				// Get relative path from the directory
				relPath, err := filepath.Rel(dir, match)
				if err != nil {
					continue
				}

				// Special handling for Go files: combine go.mod and go.sum
				if lang == "go" {
					baseName := filepath.Base(relPath)
					dirName := filepath.Dir(match)

					// If this is go.sum, skip it - it will be handled with go.mod
					if baseName == "go.sum" {
						continue
					}

					// If this is go.mod, check if we already processed it
					if baseName == "go.mod" {
						if processedGoMod[dirName] {
							continue
						}
						processedGoMod[dirName] = true

						// Create a combined scan job for go.mod (which will also scan go.sum)
						scanJob := git.ScanJob{
							Path:        dir,
							FilePath:    relPath, // Just use go.mod path, scanner will handle go.sum
							ScannerType: lang,
							IsLocal:     true,
						}
						scanJobs = append(scanJobs, scanJob)
						continue
					}
				}

				// For non-Go files or non-module files, create normal scan job
				if lang != "go" || (!strings.HasSuffix(relPath, "go.mod") && !strings.HasSuffix(relPath, "go.sum")) {
					scanJob := git.ScanJob{
						Path:        dir,
						FilePath:    relPath,
						ScannerType: lang,
						IsLocal:     true,
					}
					scanJobs = append(scanJobs, scanJob)
				}
			}
		}
	}

	return scanJobs, nil
}

// Close cleans up resources
func (s *Scanner) Close() error {
	if s.gitManager != nil {
		return s.gitManager.Close()
	}
	return nil
}

// readFileContent reads file content (helper for walker)
func (s *Scanner) readFileContent(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

// SetupGitSupport configures git support with cache directory (compatibility method)
func (s *Scanner) SetupGitSupport(cacheDir string) {
	if cacheDir != "" {
		s.gitManager = git.NewGitRepositoryManager(cacheDir)
	}
}

// ParseFilters parses filter strings (compatibility function)
func ParseFilters(filters []string) string {
	if len(filters) == 0 {
		return ""
	}
	return strings.Join(filters, ",")
}

// queueSimpleTask queues a simple scanning task without recursive task groups

// processDeepDependency handles deeper dependency scanning

// scanCurrentLevel scans dependencies at the current level

// buildDependencyTree builds the final dependency tree from discovered dependencies

// detectVersionConflicts identifies dependencies with multiple versions

// trackDependency tracks a dependency in our visited map

// checkoutDependency ensures a git dependency is checked out to a worktree

// getJobIdentifier returns a readable identifier for a scan job

// depToRef converts a models.Dependency to a git.DependencyRef

// treeToLists converts a DependencyTree to lists of dependencies and conflicts
