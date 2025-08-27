package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
	"github.com/flanksource/commons/collections"
	commonsCtx "github.com/flanksource/commons/context"
)

// UnifiedScanner orchestrates two-phase dependency scanning
type UnifiedScanner struct {
	scanner        *Scanner
	gitManager     git.GitRepositoryManager
	maxDepth       int
	visited        map[string]*git.VisitedDep
	repoVisited    map[string]bool
	discoveredDeps map[string][]*models.Dependency // Map of parent -> dependencies
	scanJobs       []git.ScanJob                   // Work queue for scanning phase
	mutex          sync.RWMutex
	discoveryGroup task.TypedGroup[[]*models.Dependency]
	scanningGroup  task.TypedGroup[[]*models.Dependency]
	treeGroup      task.TypedGroup[*git.DependencyTree]
}

// NewUnifiedScanner creates a new unified scanner
func NewUnifiedScanner(scanner *Scanner, gitManager git.GitRepositoryManager, maxDepth int) *UnifiedScanner {
	return &UnifiedScanner{
		scanner:        scanner,
		gitManager:     gitManager,
		maxDepth:       maxDepth,
		visited:        make(map[string]*git.VisitedDep),
		repoVisited:    make(map[string]bool),
		discoveredDeps: make(map[string][]*models.Dependency),
		scanJobs:       []git.ScanJob{},
	}
}

// ScanWithTwoPhases performs two-phase dependency scanning
func (us *UnifiedScanner) ScanWithTwoPhases(ctx *analysis.ScanContext, rootPath string) (*git.DependencyTree, error) {
	// Create task group for discovery phase
	us.discoveryGroup = task.StartGroup[[]*models.Dependency]("Dependency Discovery")

	// Discover and queue initial files
	initialFiles, err := us.scanner.discoverScanFiles(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files in %s: %w", rootPath, err)
	}

	// Queue all initial file scans - these will be processed synchronously
	for _, fileJob := range initialFiles {
		fileJob.Depth = 0
		fileJob.Parent = "root"
		us.queueSimpleTask(ctx, fileJob)
	}

	// Wait for all tasks to complete
	result := us.discoveryGroup.WaitFor()
	if result.Error != nil {
		return nil, fmt.Errorf("discovery phase failed: %w", result.Error)
	}

	tree := us.buildDependencyTree()

	// Detect version conflicts
	us.detectVersionConflicts(tree)

	ctx.Debugf("Completed: Found %d dependencies", len(tree.Dependencies))

	return tree, nil
}

// queueSimpleTask queues a simple scanning task that doesn't create recursive task groups
func (us *UnifiedScanner) queueSimpleTask(ctx *analysis.ScanContext, job git.ScanJob) {
	taskName := us.getJobIdentifier(job)

	us.discoveryGroup.Add(taskName, func(taskCtx commonsCtx.Context, t *clicky.Task) ([]*models.Dependency, error) {
		// Check depth limit
		if job.Depth > us.maxDepth {
			return []*models.Dependency{}, nil
		}

		// Check for recursion prevention for git repositories
		if !job.IsLocal {
			repoKey := fmt.Sprintf("%s@%s", job.GitURL, job.Version)
			us.mutex.Lock()
			if us.repoVisited[repoKey] {
				us.mutex.Unlock()
				t.Warnf("Skipping %s (already processed - recursion prevention)", repoKey)
				return []*models.Dependency{}, nil
			}
			us.repoVisited[repoKey] = true
			us.mutex.Unlock()
		}

		// Scan the current level
		deps, err := us.scanCurrentLevel(ctx, job)
		if err != nil {
			return nil, fmt.Errorf("failed to scan %s: %w", taskName, err)
		}

		// Store discovered dependencies
		us.mutex.Lock()
		us.discoveredDeps[job.Parent] = append(us.discoveredDeps[job.Parent], deps...)
		us.mutex.Unlock()

		// Process discovered dependencies for deeper levels
		for _, dep := range deps {
			// Track this dependency
			us.trackDependency(dep, job.Depth, job.Parent)

			// For deeper levels, discover and queue files synchronously to avoid task group recursion
			if us.shouldScanDeeper(dep, job.Depth) {
				us.processDeepDependency(ctx, dep, job.Depth+1)
			}
		}

		return deps, nil
	})
}

// processDeepDependency handles deeper dependency scanning without recursive task groups
func (us *UnifiedScanner) processDeepDependency(ctx *analysis.ScanContext, dep *models.Dependency, nextDepth int) {
	if dep.Git == "" {
		return
	}

	version := dep.Version
	if version == "" {
		version = "latest"
	}

	// Checkout and discover files synchronously
	worktreePath, err := us.checkoutDependency(dep.Git, version)
	if err != nil {
		ctx.Warnf("Failed to checkout %s@%s: %v", dep.Git, version, err)
		return
	}

	// Discover scannable files in the repository
	discoveredFiles, err := us.scanner.discoverScanFiles(worktreePath)
	if err != nil {
		ctx.Warnf("Failed to discover files in %s: %v", worktreePath, err)
		return
	}

	// Queue individual scan jobs directly
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
		us.queueSimpleTask(ctx, job)
	}
}

// scanCurrentLevel scans dependencies at the current level
func (us *UnifiedScanner) scanCurrentLevel(ctx *analysis.ScanContext, job git.ScanJob) ([]*models.Dependency, error) {
	var filePath string
	var scanner analysis.DependencyScanner

	if job.IsLocal {
		filePath = filepath.Join(job.Path, job.FilePath)
		scannerInterface, ok := us.scanner.Registry.Get(job.ScannerType)
		if !ok {
			return nil, fmt.Errorf("no scanner found for type: %s", job.ScannerType)
		}
		scanner = scannerInterface
	} else {
		// For git repositories, the path is already the worktree path
		filePath = filepath.Join(job.Path, job.FilePath)
		scannerInterface, ok := us.scanner.Registry.Get(job.ScannerType)
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
		return nil, err
	}
	return ctx.Filter(deps), nil
}

// buildDependencyTree builds the final dependency tree from discovered dependencies
func (us *UnifiedScanner) buildDependencyTree() *git.DependencyTree {
	tree := &git.DependencyTree{
		Dependencies: make(map[string]*git.VisitedDep),
		MaxDepth:     us.maxDepth,
	}

	// Copy visited dependencies to tree
	us.mutex.RLock()
	for key, visited := range us.visited {
		tree.Dependencies[key] = visited
	}

	// Build root dependencies
	if rootDeps, exists := us.discoveredDeps["root"]; exists {
		for _, dep := range rootDeps {
			tree.Root = append(tree.Root, us.depToRef(dep))
		}
	}
	us.mutex.RUnlock()

	return tree
}

// detectVersionConflicts identifies dependencies with multiple versions
func (us *UnifiedScanner) detectVersionConflicts(tree *git.DependencyTree) {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	for depName, visited := range us.visited {
		if len(visited.Versions) > 1 {
			// Check if all versions are the same
			versions := make(map[string]bool)
			for _, v := range visited.Versions {
				if v.Version != "" {
					versions[v.Version] = true
				}
			}

			if len(versions) > 1 {
				var versionInfos []git.VersionInfo
				for version := range versions {
					versionInfos = append(versionInfos, git.VersionInfo{
						Version: version,
					})
				}

				conflict := git.VersionConflict{
					DependencyName:     depName,
					Versions:           versionInfos,
					ResolutionStrategy: "latest",
				}

				tree.Conflicts = append(tree.Conflicts, conflict)
			}
		}
	}
}

// Helper methods

func (us *UnifiedScanner) trackDependency(dep *models.Dependency, depth int, parent string) {
	us.mutex.Lock()
	defer us.mutex.Unlock()

	depKey := us.getDependencyKey(dep)

	if visited, exists := us.visited[depKey]; exists {
		visited.SeenAt = append(visited.SeenAt, depth)
		visited.Versions = append(visited.Versions, git.VersionInstance{
			Version: dep.Version,
			Depth:   depth,
			Source:  parent,
		})
	} else {
		us.visited[depKey] = &git.VisitedDep{
			Dependency: us.depToRef(dep),
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

func (us *UnifiedScanner) shouldScanDeeper(dep *models.Dependency, currentDepth int) bool {
	if currentDepth >= us.maxDepth {
		return false
	}

	if dep.Git == "" {
		return false
	}

	// Apply filters - use the same logic as Scanner.applyFilters
	// If filters are configured, the dependency must match them to proceed
	if len(us.scanner.NameFilters) > 0 || len(us.scanner.GitFilters) > 0 {
		matched := false

		// Check name filters against both Name and Git fields
		if len(us.scanner.NameFilters) > 0 {
			if collections.MatchItems(dep.Name, us.scanner.NameFilters...) {
				matched = true
			} else if dep.Git != "" && collections.MatchItems(dep.Git, us.scanner.NameFilters...) {
				matched = true
			}
		}

		// Check git filters (only against Git field)
		if !matched && len(us.scanner.GitFilters) > 0 && dep.Git != "" {
			if collections.MatchItems(dep.Git, us.scanner.GitFilters...) {
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

func (us *UnifiedScanner) checkoutDependency(gitURL, version string) (string, error) {
	resolvedVersion, err := us.gitManager.ResolveVersionAlias(gitURL, version)
	if err != nil {
		resolvedVersion = version
	}

	worktreePath, err := us.gitManager.GetWorktreePath(gitURL, resolvedVersion)
	if err != nil {
		return "", fmt.Errorf("failed to get worktree for %s@%s: %w", gitURL, resolvedVersion, err)
	}

	return worktreePath, nil
}

func (us *UnifiedScanner) getJobIdentifier(job git.ScanJob) string {
	if job.IsLocal {
		if job.FilePath != "" {
			return fmt.Sprintf("%s/%s", job.Path, job.FilePath)
		}
		return job.Path
	}
	return fmt.Sprintf("%s@%s:%s", job.GitURL, job.Version, job.FilePath)
}

func (us *UnifiedScanner) getDependencyKey(dep *models.Dependency) string {
	return fmt.Sprintf("%s:%s", dep.Type, dep.Name)
}

func (us *UnifiedScanner) depToRef(dep *models.Dependency) *git.DependencyRef {
	return &git.DependencyRef{
		Name:    dep.Name,
		Version: dep.Version,
		Type:    string(dep.Type),
		Git:     dep.Git,
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		len(substr) < len(s) && indexOf(s, substr) != -1
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
