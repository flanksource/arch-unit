package dependencies

import (
	"fmt"
	"strings"
	"sync"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/arch-unit/models"
)

// DepthManager orchestrates depth-based dependency scanning
type DepthManager struct {
	scanner    *Scanner
	gitManager git.GitRepositoryManager
	MaxDepth   int // Exported for access
	visited    map[string]*git.VisitedDep
	conflicts  []git.VersionConflict
	mutex      sync.RWMutex
}

// NewDepthManager creates a new depth manager
func NewDepthManager(scanner *Scanner, gitManager git.GitRepositoryManager, maxDepth int) *DepthManager {
	return &DepthManager{
		scanner:    scanner,
		gitManager: gitManager,
		MaxDepth:   maxDepth,
		visited:    make(map[string]*git.VisitedDep),
		conflicts:  []git.VersionConflict{},
	}
}

// ScanWithDepth performs breadth-first dependency scanning to specified depth
func (dm *DepthManager) ScanWithDepth(ctx *analysis.ScanContext, rootPath string) (*git.DependencyTree, error) {
	// Initialize the dependency tree
	tree := &git.DependencyTree{
		Dependencies: make(map[string]*git.VisitedDep),
		MaxDepth:     dm.MaxDepth,
	}

	// Queue for breadth-first traversal
	queue := []git.ScanJob{{
		Path:    rootPath,
		Depth:   0,
		IsLocal: true,
		Parent:  "root",
	}}

	// Process queue
	for len(queue) > 0 {
		job := queue[0]
		queue = queue[1:]

		if job.Depth > dm.MaxDepth {
			continue
		}

		// Scan current level using existing scanner infrastructure
		deps, err := dm.scanCurrentLevel(ctx, job)
		if err != nil {
			// Log error but continue processing
			if ctx.Task != nil {
				ctx.Task.Warnf("Failed to scan %s at depth %d: %v", dm.getJobIdentifier(job), job.Depth, err)
			}
			continue
		}

		// Process discovered dependencies
		for _, dep := range deps {
			// Track this dependency
			dm.trackDependency(dep, job.Depth, job.Parent)

			// Add to root if this is depth 0
			if job.Depth == 0 {
				tree.Root = append(tree.Root, dm.depToRef(dep))
			}

			// Create next level jobs if we should scan deeper
			if dm.shouldScanDeeper(dep, job.Depth) {
				nextJobs := dm.createNextLevelJobs(dep, job.Depth+1)
				queue = append(queue, nextJobs...)
			}
		}
	}

	// Detect and resolve conflicts
	dm.detectVersionConflicts()

	// Build final dependency tree
	tree.Dependencies = dm.visited
	tree.Conflicts = dm.conflicts

	return tree, nil
}

// scanCurrentLevel scans dependencies at the current level
func (dm *DepthManager) scanCurrentLevel(ctx *analysis.ScanContext, job git.ScanJob) ([]*models.Dependency, error) {
	if job.IsLocal {
		// Use existing Scanner.ScanDirectory for local paths - no changes needed!
		return dm.scanner.ScanDirectory(ctx.Task, job.Path)
	}

	// Git-based scanning - checkout and scan
	worktreePath, err := dm.checkoutDependency(job.GitURL, job.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to checkout %s@%s: %w", job.GitURL, job.Version, err)
	}

	// Create new scan context for the worktree
	worktreeCtx := analysis.NewScanContext(ctx.Task, worktreePath)

	// Use existing Scanner.ScanDirectory on checked out code - no changes needed!
	return dm.scanner.ScanDirectory(worktreeCtx.Task, worktreePath)
}

// checkoutDependency ensures a git dependency is checked out to a worktree
func (dm *DepthManager) checkoutDependency(gitURL, version string) (string, error) {
	// Resolve version aliases (HEAD, GA, latest)
	resolvedVersion, err := dm.gitManager.ResolveVersionAlias(gitURL, version)
	if err != nil {
		// Fallback to original version if resolution fails
		resolvedVersion = version
	}

	// Get or create worktree for this specific version
	worktreePath, err := dm.gitManager.GetWorktreePath(gitURL, resolvedVersion)
	if err != nil {
		return "", fmt.Errorf("failed to get worktree for %s@%s: %w", gitURL, resolvedVersion, err)
	}

	return worktreePath, nil
}

// shouldScanDeeper determines if we should scan deeper for a dependency
func (dm *DepthManager) shouldScanDeeper(dep *models.Dependency, currentDepth int) bool {
	// Don't scan deeper if we've reached max depth
	if currentDepth >= dm.MaxDepth {
		return false
	}

	// Only scan dependencies that have git URLs
	if dep.Git == "" {
		return false
	}

	// Apply any filters from the scanner
	if len(dm.scanner.GitFilters) > 0 {
		return dm.matchesGitFilters(dep.Git)
	}

	return true
}

// createNextLevelJobs creates scanning jobs for the next depth level
func (dm *DepthManager) createNextLevelJobs(dep *models.Dependency, nextDepth int) []git.ScanJob {
	if dep.Git == "" {
		return nil
	}

	version := dep.Version
	if version == "" {
		version = "latest" // Default to latest if no version specified
	}

	return []git.ScanJob{{
		GitURL:  dep.Git,
		Version: version,
		Depth:   nextDepth,
		IsLocal: false,
		Parent:  dep.Name,
	}}
}

// trackDependency tracks a dependency in our visited map
func (dm *DepthManager) trackDependency(dep *models.Dependency, depth int, parent string) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	depKey := dm.getDependencyKey(dep)

	if visited, exists := dm.visited[depKey]; exists {
		// Dependency already seen - track additional occurrence
		visited.SeenAt = append(visited.SeenAt, depth)
		visited.Versions = append(visited.Versions, git.VersionInstance{
			Version: dep.Version,
			Depth:   depth,
			Source:  parent,
		})
	} else {
		// First time seeing this dependency
		dm.visited[depKey] = &git.VisitedDep{
			Dependency: dm.depToRef(dep),
			FirstSeen:  depth,
			SeenAt:     []int{depth},
			Versions: []git.VersionInstance{{
				Version: dep.Version,
				Depth:   depth,
				Source:  parent,
			}},
		}
	}
}

// detectVersionConflicts identifies dependencies with multiple versions
func (dm *DepthManager) detectVersionConflicts() {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	for depName, visited := range dm.visited {
		if len(visited.Versions) > 1 {
			// Check if all versions are the same
			versions := make(map[string]bool)
			for _, v := range visited.Versions {
				if v.Version != "" {
					versions[v.Version] = true
				}
			}

			if len(versions) > 1 {
				// We have a version conflict
				var versionInfos []git.VersionInfo
				for version := range versions {
					versionInfos = append(versionInfos, git.VersionInfo{
						Version: version,
					})
				}

				conflict := git.VersionConflict{
					DependencyName:     depName,
					Versions:           versionInfos,
					ResolutionStrategy: "latest", // Default strategy
				}

				dm.conflicts = append(dm.conflicts, conflict)
			}
		}
	}
}

// Helper methods

func (dm *DepthManager) getJobIdentifier(job git.ScanJob) string {
	if job.IsLocal {
		return job.Path
	}
	return fmt.Sprintf("%s@%s", job.GitURL, job.Version)
}

func (dm *DepthManager) getDependencyKey(dep *models.Dependency) string {
	// Use name and type as the key for tracking
	return fmt.Sprintf("%s:%s", dep.Type, dep.Name)
}

func (dm *DepthManager) depToRef(dep *models.Dependency) *git.DependencyRef {
	return &git.DependencyRef{
		Name:    dep.Name,
		Version: dep.Version,
		Type:    string(dep.Type),
		Git:     dep.Git,
	}
}

func (dm *DepthManager) matchesGitFilters(gitURL string) bool {
	for _, filter := range dm.scanner.GitFilters {
		if strings.Contains(gitURL, filter) {
			return true
		}
	}
	return false
}

// GetVisitedDependencies returns all visited dependencies
func (dm *DepthManager) GetVisitedDependencies() map[string]*git.VisitedDep {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	// Return a copy to avoid concurrent access issues
	result := make(map[string]*git.VisitedDep)
	for k, v := range dm.visited {
		result[k] = v
	}
	return result
}

// GetVersionConflicts returns all detected version conflicts
func (dm *DepthManager) GetVersionConflicts() []git.VersionConflict {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	// Return a copy
	result := make([]git.VersionConflict, len(dm.conflicts))
	copy(result, dm.conflicts)
	return result
}

// Reset clears all tracking state for a new scan
func (dm *DepthManager) Reset() {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	dm.visited = make(map[string]*git.VisitedDep)
	dm.conflicts = []git.VersionConflict{}
}
