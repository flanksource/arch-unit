package dependencies

import (
	"fmt"
	"sync"

	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
	commonsCtx "github.com/flanksource/commons/context"
)

// DependencyWalker handles recursive dependency traversal with task-based processing
type DependencyWalker struct {
	scanner   *Scanner
	ctx       *models.ScanContext
	visited   sync.Map // map[string]bool - thread-safe visited tracking
	taskGroup task.TypedGroup[[]*models.Dependency]
	allDeps   sync.Map // map[string]*models.Dependency - all discovered dependencies
}

// WalkResult contains the result of a dependency walk
type WalkResult struct {
	Dependencies []*models.Dependency
	Conflicts    []models.VersionConflict
	TotalScanned int
}

// NewDependencyWalker creates a new dependency walker
func NewDependencyWalker(scanner *Scanner, ctx *models.ScanContext) *DependencyWalker {
	return &DependencyWalker{
		scanner: scanner,
		ctx:     ctx,
	}
}

// Walk starts the recursive dependency walking process
func (w *DependencyWalker) Walk(ctxArg interface{}, path string, depth int) *WalkResult {
	// Create task group for parallel processing
	w.taskGroup = task.StartGroup[[]*models.Dependency]("Dependency Walk")

	// Start the recursive walk
	w.queueScanJob(path, "", depth, true, "root")

	// Wait for all tasks to complete
	result := w.taskGroup.WaitFor()
	if result.Error != nil {
		w.ctx.Errorf("Some dependency scans failed: %v", result.Error)
	}

	// Collect all dependencies from the sync.Map
	var allDeps []*models.Dependency
	w.allDeps.Range(func(key, value interface{}) bool {
		if dep, ok := value.(*models.Dependency); ok {
			allDeps = append(allDeps, dep)
		}
		return true
	})

	// Build conflicts (simplified for now)
	conflicts := w.detectConflicts(allDeps)

	return &WalkResult{
		Dependencies: allDeps,
		Conflicts:    conflicts,
		TotalScanned: len(allDeps),
	}
}

// queueScanJob queues a scanning job for a path or git dependency
func (w *DependencyWalker) queueScanJob(path, gitURL string, depth int, isLocal bool, parent string) {
	// Check if we've already processed this
	var jobKey string
	if isLocal {
		jobKey = fmt.Sprintf("local:%s", path)
	} else {
		jobKey = fmt.Sprintf("git:%s", gitURL)
	}

	if _, exists := w.visited.LoadOrStore(jobKey, true); exists {
		return // Already processing or processed
	}

	// Check depth limit
	if depth > w.ctx.MaxDepth {
		return
	}

	taskName := w.getJobName(path, gitURL, isLocal)

	w.taskGroup.Add(taskName, func(taskCtx commonsCtx.Context, t *clicky.Task) ([]*models.Dependency, error) {
		var deps []*models.Dependency
		var err error

		if isLocal {
			deps, err = w.scanLocalPath(taskCtx, t, path)
		} else {
			// Extract version from gitURL (which includes @version)
			cleanGitURL, version := w.parseGitURLAndVersion(gitURL)
			deps, err = w.scanGitDependency(taskCtx, t, cleanGitURL, version, depth)
		}

		if err != nil {
			return []*models.Dependency{}, err // Continue with empty results
		}

		// Apply filtering
		deps = w.ctx.FilterDeps(deps)

		// Store discovered dependencies and queue deeper scans
		for _, dep := range deps {
			depKey := w.getDependencyKey(dep)
			w.allDeps.Store(depKey, dep)

			// Queue deeper scanning if this dependency has a git URL
			if depth < w.ctx.MaxDepth && dep.Git != "" {
				w.queueScanJob("", dep.Git+"@"+dep.Version, depth+1, false, dep.Name)
			}
		}

		t.Infof("Found %d dependencies", len(deps))
		return deps, nil
	})
}

// scanLocalPath scans a local directory for dependencies
func (w *DependencyWalker) scanLocalPath(ctx commonsCtx.Context, t *clicky.Task, path string) ([]*models.Dependency, error) {
	t.Infof("Scanning local path: %s", path)

	// Discover scannable files
	files, err := w.scanner.discoverScanFiles(path)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files: %w", err)
	}

	var allDeps []*models.Dependency

	// Scan each discovered file
	for _, fileJob := range files {
		deps, err := w.scanFile(fileJob)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file %s: %w", fileJob.FilePath, err)
		}
		allDeps = append(allDeps, deps...)
	}

	return allDeps, nil
}

// scanGitDependency scans a git dependency (with just-in-time resolution and checkout)
func (w *DependencyWalker) scanGitDependency(ctx commonsCtx.Context, t *clicky.Task, gitURL, version string, depth int) ([]*models.Dependency, error) {
	t.Infof("Scanning git dependency: %s@%s", gitURL, version)

	// Parse git URL and version
	gitURL, version = w.parseGitURLAndVersion(gitURL)

	// Step 1: Resolve version (expensive - network operation)
	t.Debugf("Resolving version %s for %s", version, gitURL)
	resolvedVersion, err := w.scanner.gitManager.ResolveVersionAlias(gitURL, version)
	if err != nil {
		t.Errorf("Failed to resolve version %s for %s: %v", version, gitURL, err)
		resolvedVersion = version // fallback to original version
	}

	// Step 2: Checkout to clone (expensive - disk I/O)
	t.Debugf("Checking out %s@%s to clone", gitURL, resolvedVersion)
	// Set task logger for git operations
	git.SetCurrentTaskLogger(t)
	worktreePath, err := w.scanner.gitManager.GetWorktreePath(gitURL, resolvedVersion, 1) // Shallow clone for dependency scanning
	if err != nil {
		return nil, fmt.Errorf("failed to checkout %s@%s: %w", gitURL, resolvedVersion, err)
	}

	// Step 3: Scan the checked out repository
	return w.scanLocalPath(ctx, t, worktreePath)
}

// scanFile scans a single file using the appropriate scanner
func (w *DependencyWalker) scanFile(job git.ScanJob) ([]*models.Dependency, error) {
	scanner, ok := w.scanner.Registry.Get(job.ScannerType)
	if !ok {
		return nil, fmt.Errorf("no scanner found for type: %s", job.ScannerType)
	}

	filePath := job.Path + "/" + job.FilePath
	content, err := w.readFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return scanner.ScanFile(w.ctx, filePath, content)
}

// readFile reads a file (could be made into a task for large files)
func (w *DependencyWalker) readFile(filePath string) ([]byte, error) {
	// TODO: For files > 1MB, consider making this a task
	return w.scanner.readFileContent(filePath)
}

// parseGitURLAndVersion parses a git URL that may include version and subdirectory info
func (w *DependencyWalker) parseGitURLAndVersion(gitURLWithVersion string) (string, string) {
	gitURL, version, _, _ := w.scanner.parseGitURLWithSubdir(gitURLWithVersion)
	if version == "" {
		version = "HEAD"
	}
	return gitURL, version
}

// getJobName creates a readable name for a scan job
func (w *DependencyWalker) getJobName(path, gitURL string, isLocal bool) string {
	if isLocal {
		return fmt.Sprintf("scan:%s", path)
	}
	return fmt.Sprintf("scan:%s", gitURL)
}

// getDependencyKey creates a unique key for a dependency
func (w *DependencyWalker) getDependencyKey(dep *models.Dependency) string {
	return fmt.Sprintf("%s:%s:%s", dep.Type, dep.Name, dep.Version)
}

// detectConflicts identifies version conflicts in the dependencies
func (w *DependencyWalker) detectConflicts(deps []*models.Dependency) []models.VersionConflict {
	// Track versions for each dependency name
	versionMap := make(map[string]map[string]*models.Dependency)

	for _, dep := range deps {
		if versionMap[dep.Name] == nil {
			versionMap[dep.Name] = make(map[string]*models.Dependency)
		}
		versionMap[dep.Name][dep.Version] = dep
	}

	var conflicts []models.VersionConflict

	// Find dependencies with multiple versions
	for depName, versions := range versionMap {
		if len(versions) > 1 {
			var versionInfos []models.VersionInfo
			for version := range versions {
				versionInfos = append(versionInfos, models.VersionInfo{
					Version: version,
				})
			}

			conflicts = append(conflicts, models.VersionConflict{
				DependencyName:     depName,
				Versions:           versionInfos,
				ResolutionStrategy: "latest", // Default strategy
			})
		}
	}

	return conflicts
}
