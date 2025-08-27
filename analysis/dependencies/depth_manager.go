package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/collections"
	commonsCtx "github.com/flanksource/commons/context"
)

// DepthManager orchestrates depth-based dependency scanning
type DepthManager struct {
	scanner     *Scanner
	gitManager  git.GitRepositoryManager
	MaxDepth    int // Exported for access
	visited     map[string]*git.VisitedDep
	conflicts   []git.VersionConflict
	repoVisited map[string]bool // Track visited repo@version to prevent recursion
	mutex       sync.RWMutex
}

// NewDepthManager creates a new depth manager
func NewDepthManager(scanner *Scanner, gitManager git.GitRepositoryManager, maxDepth int) *DepthManager {
	return &DepthManager{
		scanner:     scanner,
		gitManager:  gitManager,
		MaxDepth:    maxDepth,
		visited:     make(map[string]*git.VisitedDep),
		conflicts:   []git.VersionConflict{},
		repoVisited: make(map[string]bool),
	}
}

// ScanWithDepth performs breadth-first dependency scanning to specified depth
func (dm *DepthManager) ScanWithDepth(ctx *analysis.ScanContext, rootPath string) (*git.DependencyTree, error) {
	// Initialize the dependency tree
	tree := &git.DependencyTree{
		Dependencies: make(map[string]*git.VisitedDep),
		MaxDepth:     dm.MaxDepth,
	}

	// Start with file discovery for the root directory
	initialFiles, err := dm.scanner.discoverScanFiles(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files in %s: %w", rootPath, err)
	}

	// Queue individual file scan jobs directly
	for _, fileJob := range initialFiles {
		fileJob.Depth = 0
		fileJob.Parent = "root"
		dm.queueScanJob(ctx, fileJob, tree)
	}

	return tree, nil
}

// queueScanJob queues an individual scan job as a task
func (dm *DepthManager) queueScanJob(ctx *analysis.ScanContext, job git.ScanJob, tree *git.DependencyTree) {
	taskName := dm.getJobIdentifier(job)

	clicky.StartTask(taskName, func(taskCtx commonsCtx.Context, t *clicky.Task) ([]*models.Dependency, error) {
		// Check depth limit
		if job.Depth > dm.MaxDepth {
			return []*models.Dependency{}, nil
		}

		// Scan the specific file
		deps, err := dm.scanCurrentLevel(ctx, job)
		if err != nil {
			return nil, fmt.Errorf("failed to scan %s: %w", taskName, err)
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
				if err := dm.queueNextLevelJobs(ctx, dep, job.Depth+1, tree); err != nil {
					t.Warnf("Failed to queue next level jobs for %s: %v", dep.Name, err)
				}
			}
		}

		return deps, nil
	})
}

// scanCurrentLevel scans dependencies at the current level for a specific file
func (dm *DepthManager) scanCurrentLevel(ctx *analysis.ScanContext, job git.ScanJob) ([]*models.Dependency, error) {
	var filePath string
	var scanner analysis.DependencyScanner

	if job.IsLocal {
		// Local file scanning
		filePath = filepath.Join(job.Path, job.FilePath)

		// Get the appropriate scanner for this file type
		scannerInterface, ok := dm.scanner.Registry.Get(job.ScannerType)
		if !ok {
			return nil, fmt.Errorf("no scanner found for type: %s", job.ScannerType)
		}
		scanner = scannerInterface
	} else {
		// Git repository file scanning
		repoKey := fmt.Sprintf("%s@%s", job.GitURL, job.Version)

		// Check for recursion prevention
		dm.mutex.Lock()
		if dm.repoVisited[repoKey] {
			dm.mutex.Unlock()
			// Already processed this repo@version, skip to prevent recursion
			if ctx.Task != nil {
				ctx.Task.Warnf("Skipping %s (already processed - recursion prevention)", repoKey)
			}
			return []*models.Dependency{}, nil
		}
		dm.repoVisited[repoKey] = true
		dm.mutex.Unlock()

		// Checkout the repository first
		worktreePath, err := dm.checkoutDependency(job.GitURL, job.Version)
		if err != nil {
			return nil, fmt.Errorf("failed to checkout %s@%s: %w", job.GitURL, job.Version, err)
		}

		filePath = filepath.Join(worktreePath, job.FilePath)

		// Get the appropriate scanner for this file type
		scannerInterface, ok := dm.scanner.Registry.Get(job.ScannerType)
		if !ok {
			return nil, fmt.Errorf("no scanner found for type: %s", job.ScannerType)
		}
		scanner = scannerInterface
	}

	// Read the specific file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Scan the specific file
	deps, err := scanner.ScanFile(ctx, filePath, content)
	if err != nil {
		return nil, fmt.Errorf("failed to scan file %s: %w", filePath, err)
	}
	return deps, nil
}

// scanGitRepository handles the actual git repository scanning with cache status
func (dm *DepthManager) scanGitRepository(task *clicky.Task, job git.ScanJob) ([]*models.Dependency, error) {
	repoKey := fmt.Sprintf("%s@%s", job.GitURL, job.Version)

	// Create individual task for this repository scan
	repoTask := clicky.StartTask(fmt.Sprintf("Scanning %s", repoKey), func(ctx commonsCtx.Context, t *clicky.Task) ([]*models.Dependency, error) {
		// Checkout the repository
		t.SetName(fmt.Sprintf("Checking out %s", repoKey))
		worktreePath, err := dm.checkoutDependency(job.GitURL, job.Version)
		if err != nil {
			t.SetName(fmt.Sprintf("Failed to checkout %s", repoKey))
			return nil, fmt.Errorf("failed to checkout %s@%s: %w", job.GitURL, job.Version, err)
		}

		// Check if this was a cache hit or fresh clone
		cacheStatus := dm.determineCacheStatus(worktreePath)
		t.SetName(fmt.Sprintf("Scanning %s (%s)", repoKey, cacheStatus))

		// Create new scan context for the worktree
		worktreeCtx := analysis.NewScanContext(t, worktreePath)

		// Use existing Scanner.ScanDirectory on checked out code
		deps, err := dm.scanner.ScanDirectory(worktreeCtx.Task, worktreePath)

		// Update task name with results
		if err != nil {
			t.SetName(fmt.Sprintf("Failed to scan %s (%s): %v", repoKey, cacheStatus, err))
		} else {
			t.SetName(fmt.Sprintf("Scanned %s (%s) - %d dependencies", repoKey, cacheStatus, len(deps)))
		}

		return deps, err
	})

	// Wait for the task to complete and get the result
	return repoTask.GetResult()
}

// determineCacheStatus checks if a worktree path was cached or freshly created
func (dm *DepthManager) determineCacheStatus(worktreePath string) string {
	// Check if the worktree directory existed before (simple heuristic)
	if stat, err := os.Stat(worktreePath); err == nil {
		// Directory exists - check if it was recently created (within last 10 seconds)
		// This is a simple heuristic; could be improved with more sophisticated tracking
		now := time.Now()
		if now.Sub(stat.ModTime()) < 10*time.Second {
			return "freshly cloned"
		}
		return "cached"
	}
	return "unknown"
}

// checkoutDependency ensures a git dependency is checked out to a worktree
func (dm *DepthManager) checkoutDependency(gitURL, version string) (string, error) {
	// Handle local directory replacements
	if strings.HasPrefix(version, "local:") {
		// Extract the local path
		localPath := strings.TrimPrefix(version, "local:")
		// Return the local path directly - no git checkout needed
		return localPath, nil
	}

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

	// Apply filters - use the same logic as Scanner.applyFilters
	// If filters are configured, the dependency must match them to proceed
	if len(dm.scanner.NameFilters) > 0 || len(dm.scanner.GitFilters) > 0 {
		matched := false

		// Check name filters against both Name and Git fields
		if len(dm.scanner.NameFilters) > 0 {
			if collections.MatchItems(dep.Name, dm.scanner.NameFilters...) {
				matched = true
			} else if dep.Git != "" && collections.MatchItems(dep.Git, dm.scanner.NameFilters...) {
				matched = true
			}
		}

		// Check git filters (only against Git field)
		if !matched && len(dm.scanner.GitFilters) > 0 && dep.Git != "" {
			if collections.MatchItems(dep.Git, dm.scanner.GitFilters...) {
				matched = true
			}
		}

		// If no filters matched, don't scan deeper
		if !matched {
			return false
		}
	}

	return true
}

// queueNextLevelJobs creates and queues scanning jobs for the next depth level
func (dm *DepthManager) queueNextLevelJobs(ctx *analysis.ScanContext, dep *models.Dependency, nextDepth int, tree *git.DependencyTree) error {
	if dep.Git == "" {
		return nil
	}

	version := dep.Version
	if version == "" {
		version = "latest" // Default to latest if no version specified
	}

	// Checkout the repository to discover its files
	worktreePath, err := dm.checkoutDependency(dep.Git, version)
	if err != nil {
		return fmt.Errorf("failed to checkout %s@%s: %w", dep.Git, version, err)
	}

	// Discover scannable files in the repository
	discoveredFiles, err := dm.scanner.discoverScanFiles(worktreePath)
	if err != nil {
		return fmt.Errorf("failed to discover files in %s: %w", worktreePath, err)
	}

	// Queue individual scan jobs for each discovered file
	for _, fileJob := range discoveredFiles {
		job := git.ScanJob{
			Path:        worktreePath,
			FilePath:    fileJob.FilePath,
			GitURL:      dep.Git,
			Version:     version,
			Depth:       nextDepth,
			IsLocal:     false,
			Parent:      dep.Name,
			ScannerType: fileJob.ScannerType,
		}
		dm.queueScanJob(ctx, job, tree)
	}

	return nil
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
	dm.repoVisited = make(map[string]bool)
}
