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
	commonsCtx "github.com/flanksource/commons/context"
)

// Scanner orchestrates dependency scanning across multiple languages
type Scanner struct {
	Registry     *analysis.DependencyRegistry
	gitManager   git.GitRepositoryManager
	GitFilters   []string // Git URL filters for depth scanning
	NameFilters  []string // Package name filters for depth scanning

	// Tracking for two-phase scanning
	visited        map[string]*git.VisitedDep
	repoVisited    map[string]bool
	discoveredDeps map[string][]*models.Dependency // Map of parent -> dependencies
	mutex          sync.RWMutex
	discoveryGroup task.TypedGroup[[]*models.Dependency]
}

// NewScanner creates a new dependency scanner
func NewScanner() *Scanner {
	return &Scanner{
		Registry:       analysis.DefaultDependencyRegistry,
		visited:        make(map[string]*git.VisitedDep),
		repoVisited:    make(map[string]bool),
		discoveredDeps: make(map[string][]*models.Dependency),
	}
}

// NewScannerWithRegistry creates a new dependency scanner with a custom registry
func NewScannerWithRegistry(registry *analysis.DependencyRegistry) *Scanner {
	return &Scanner{
		Registry:       registry,
		visited:        make(map[string]*git.VisitedDep),
		repoVisited:    make(map[string]bool),
		discoveredDeps: make(map[string][]*models.Dependency),
	}
}

// ScanPath scans a path (local directory or git URL) for dependencies with optional depth traversal
func (s *Scanner) ScanPath(task *clicky.Task, pathOrURL string, maxDepth int) (*models.ScanResult, error) {
	// Create a scan context with the provided configuration
	ctx := analysis.NewScanContext(task, pathOrURL).WithDepth(maxDepth)
	return s.ScanWithContext(ctx, pathOrURL)
}

// ScanWithContext performs scanning with a configured context
func (s *Scanner) ScanWithContext(ctx *analysis.ScanContext, pathOrURL string) (*models.ScanResult, error) {
	if ctx.MaxDepth < 0 {
		ctx.MaxDepth = 0 // Default to no depth traversal
	}

	// Parse path to determine if it's local or git
	gitURL, version, isGit := s.parseGitURL(pathOrURL)

	var deps []*models.Dependency
	var conflicts []models.VersionConflict
	var scanType string
	var repoCount int
	var err error

	if !isGit {
		// Phase 1: Local directory scanning
		ctx.Debugf("Scanning local dependencies in %s", pathOrURL)
		scanType = "local"

		if ctx.MaxDepth > 0 {
			// Ensure git support is set up for depth traversal
			if s.gitManager == nil {
				s.SetupGitSupport(".cache/arch-unit/repositories")
			}
			// Use two-phase scanning for depth traversal
			tree, err := s.scanWithTwoPhases(ctx, pathOrURL)
			if err != nil {
				return nil, err
			}
			deps, conflicts = s.treeToLists(tree)
			scanType = "mixed"
			repoCount = len(tree.Dependencies)
		} else {
			// Simple local scan without depth
			deps, err = s.scanDirectoryWithContext(ctx, pathOrURL)
			if err != nil {
				return nil, err
			}
			// Filtering is already applied in scanDirectoryWithContext
		}
	} else {
		// Phase 1: Git repository scanning
		if ctx.MaxDepth == 0 {
			ctx.MaxDepth = 1 // For git URLs, default to at least depth 1
		}
		ctx.Infof("Scanning git repository %s@%s", gitURL, version)
		scanType = "git"

		// Ensure git support is set up
		if s.gitManager == nil {
			s.SetupGitSupport(".cache/arch-unit/repositories")
		}

		// Checkout and scan the repository
		worktreePath, err := s.gitManager.GetWorktreePath(gitURL, version)
		if err != nil {
			return nil, fmt.Errorf("failed to checkout %s@%s: %w", gitURL, version, err)
		}

		// Update context with the worktree path
		ctx.ScanRoot = worktreePath
		tree, err := s.scanWithTwoPhases(ctx, worktreePath)
		if err != nil {
			return nil, err
		}
		deps, conflicts = s.treeToLists(tree)
		repoCount = 1 + len(tree.Dependencies)
	}

	// Build metadata
	metadata := models.ScanMetadata{
		ScanType:          scanType,
		MaxDepth:          ctx.MaxDepth,
		RepositoriesFound: repoCount,
		TotalDependencies: len(deps),
		ConflictsFound:    len(conflicts),
	}

	if s.gitManager != nil {
		// Get git cache dir if available
		metadata.GitCacheDir = ".cache/arch-unit/repositories" // Default cache dir
	}

	// Final phase update
	if len(conflicts) > 0 {
		ctx.Debugf("Found %d dependencies with %d conflicts", len(deps), len(conflicts))
	} else {
		ctx.Debugf("Found %d dependencies", len(deps))
	}

	return &models.ScanResult{
		Dependencies: deps,
		Conflicts:    conflicts,
		Metadata:     metadata,
	}, nil
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
func (s *Scanner) scanWithScanner(ctx *analysis.ScanContext, dir string, scanner analysis.DependencyScanner) ([]*models.Dependency, error) {
	var allDeps []*models.Dependency

	// Walk the directory tree
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories
		if info.IsDir() {
			// Skip common directories that shouldn't be scanned
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" ||
				name == ".venv" || name == "venv" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if this file matches the scanner's patterns
		if !s.matchesScanner(scanner, path) {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			if ctx.Task != nil {
				ctx.Task.Warnf("Error reading %s: %v", path, err)
			}
			return nil
		}

		// Scan the file
		deps, err := scanner.ScanFile(ctx, path, content)
		if err != nil {
			if ctx.Task != nil {
				ctx.Task.Warnf("Error scanning %s: %v", path, err)
			}
			return nil
		}
		deps = ctx.FilterDeps(deps)

		allDeps = append(allDeps, deps...)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	return allDeps, nil
}

// matchesScanner checks if a file matches any of the scanner's supported patterns
func (s *Scanner) matchesScanner(scanner analysis.DependencyScanner, filePath string) bool {
	fileName := filepath.Base(filePath)
	patterns := scanner.SupportedFiles()

	for _, pattern := range patterns {
		matched, _ := filepath.Match(pattern, fileName)
		if matched {
			return true
		}
	}

	return false
}

// getDependencyKey returns a unique key for a dependency
func (s *Scanner) getDependencyKey(dep *models.Dependency) string {
	// Use name + type as the key
	return fmt.Sprintf("%s:%s", dep.Name, dep.Type)
}

// applyFilters applies filtering from a context (deprecated - use ctx.FilterDeps directly)
func (s *Scanner) applyFilters(deps []*models.Dependency) []*models.Dependency {
	// Create a default context just for filtering
	ctx := &analysis.ScanContext{
		ShowIndirect: true,
	}
	return ctx.FilterDeps(deps)
}

// ParseFilters combines multiple filter strings into a single filter expression
// Filters support wildcards (*) and negation (!)
func ParseFilters(filters []string) string {
	if len(filters) == 0 {
		return ""
	}

	// Join all non-empty filters with space
	var cleanFilters []string
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter != "" {
			// Remove legacy prefixes if present
			if strings.HasPrefix(filter, "-name ") {
				filter = strings.TrimPrefix(filter, "-name ")
			} else if strings.HasPrefix(filter, "-git ") {
				filter = strings.TrimPrefix(filter, "-git ")
			} else if strings.HasPrefix(filter, "-language ") || strings.HasPrefix(filter, "-lang ") {
				// Skip language filters as we're removing language field
				continue
			}
			cleanFilters = append(cleanFilters, filter)
		}
	}

	return strings.Join(cleanFilters, " ")
}

// SetupGitSupport initializes git support for depth-based scanning
func (s *Scanner) SetupGitSupport(cacheDir string) {
	if s.gitManager == nil {
		s.gitManager = git.NewGitRepositoryManager(cacheDir)
	}
}

// ScanWithDepth performs depth-based dependency scanning
func (s *Scanner) ScanWithDepth(task *clicky.Task, dir string, maxDepth int) (*git.DependencyTree, error) {
	// Ensure git support is set up
	if s.gitManager == nil {
		s.SetupGitSupport(".cache/arch-unit/repositories")
	}

	// Create scan context
	ctx := analysis.NewScanContext(task, dir).WithDepth(maxDepth)

	// Perform two-phase scan
	return s.scanWithTwoPhases(ctx, dir)
}

// scanWithTwoPhases performs two-phase dependency scanning
func (s *Scanner) scanWithTwoPhases(ctx *analysis.ScanContext, rootPath string) (*git.DependencyTree, error) {
	// Reset tracking state for new scan
	s.mutex.Lock()
	s.visited = make(map[string]*git.VisitedDep)
	s.repoVisited = make(map[string]bool)
	s.discoveredDeps = make(map[string][]*models.Dependency)
	s.mutex.Unlock()

	// Discover initial files first
	initialFiles, err := s.discoverScanFiles(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to discover files in %s: %w", rootPath, err)
	}

	// Only create task group if we have files to scan
	if len(initialFiles) > 0 {
		// Create task group for discovery phase
		s.discoveryGroup = task.StartGroup[[]*models.Dependency]("Dependency Discovery")

		// Queue all initial file scans
		for _, fileJob := range initialFiles {
			fileJob.Depth = 0
			fileJob.Parent = "root"
			s.queueSimpleTask(ctx, fileJob)
		}

		// Wait for all tasks to complete
		result := s.discoveryGroup.WaitFor()
		if result.Error != nil {
			// Log the error but continue with the dependencies that were successfully discovered
			ctx.Warnf("Some dependency scans failed: %v", result.Error)
			// Continue processing with whatever dependencies we did manage to collect
		}
	}

	tree := s.buildDependencyTree()
	s.detectVersionConflicts(tree)

	ctx.Debugf("Completed: Found %d dependencies", len(tree.Dependencies))
	return tree, nil
}

// ScanGitRepository scans a specific git repository at a given version
func (s *Scanner) ScanGitRepository(task *clicky.Task, gitURL, version string, maxDepth int) (*git.DependencyTree, error) {
	// Ensure git support is set up
	if s.gitManager == nil {
		s.SetupGitSupport(".cache/arch-unit/repositories")
	}

	// Get worktree path for this repository/version
	worktreePath, err := s.gitManager.GetWorktreePath(gitURL, version)
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
func (s *Scanner) parseGitURL(pathOrURL string) (gitURL, version string, isGit bool) {
	// Check for git URL patterns
	if strings.Contains(pathOrURL, "@") {
		parts := strings.Split(pathOrURL, "@")
		if len(parts) == 2 {
			candidateURL := parts[0]
			version = parts[1]

			// Check if it looks like a git URL
			if strings.Contains(candidateURL, "github.com") ||
				strings.Contains(candidateURL, "gitlab.com") ||
				strings.Contains(candidateURL, "bitbucket.org") ||
				strings.HasPrefix(candidateURL, "https://") ||
				strings.HasPrefix(candidateURL, "git@") ||
				(strings.Contains(candidateURL, "/") && !strings.HasPrefix(candidateURL, "/") && !strings.HasPrefix(candidateURL, "./")) {
				return candidateURL, version, true
			}
		}
	}

	// Check for URLs without version (default to HEAD)
	if strings.HasPrefix(pathOrURL, "https://") ||
		strings.HasPrefix(pathOrURL, "git@") ||
		(strings.Contains(pathOrURL, "github.com") && !strings.HasPrefix(pathOrURL, "/")) ||
		(strings.Contains(pathOrURL, "gitlab.com") && !strings.HasPrefix(pathOrURL, "/")) {
		return pathOrURL, "HEAD", true
	}

	return pathOrURL, "", false
}

// scanDirectory is the renamed original ScanDirectory method for internal use
func (s *Scanner) scanDirectory(task *clicky.Task, dir string) ([]*models.Dependency, error) {
	// Create a default context for backward compatibility
	ctx := analysis.NewScanContext(task, dir).WithIndirect(true)
	return s.scanDirectoryWithContext(ctx, dir)
}

// scanDirectoryWithContext scans a directory using the provided context
func (s *Scanner) scanDirectoryWithContext(ctx *analysis.ScanContext, dir string) ([]*models.Dependency, error) {
	if ctx.Task != nil {
		ctx.Task.Infof("Scanning dependencies in %s", dir)
	}

	var allDeps []*models.Dependency
	depMap := make(map[string]*models.Dependency)

	// Get all registered scanners
	languages := s.Registry.List()
	for _, lang := range languages {
		scanner, _ := s.Registry.Get(lang)
		if scanner == nil {
			continue
		}

		if ctx.Task != nil {
			ctx.Task.Infof("Scanning %s dependencies", scanner.Language())
		}

		deps, err := s.scanWithScanner(ctx, dir, scanner)
		if err != nil {
			if ctx.Task != nil {
				ctx.Task.Warnf("Error scanning %s: %v", scanner.Language(), err)
			}
			continue
		}

		// Deduplicate dependencies
		for _, dep := range deps {
			key := s.getDependencyKey(dep)
			if existing, exists := depMap[key]; exists {
				// Merge information if needed - prefer newer non-empty values
				if dep.Version != "" {
					existing.Version = dep.Version
				}
				if dep.Git != "" {
					existing.Git = dep.Git
				}
				if len(dep.Package) > 0 {
					existing.Package = dep.Package
				}
				if dep.Source != "" {
					existing.Source = dep.Source
				}
			} else {
				depMap[key] = dep
			}
		}
	}

	// Convert map to slice
	for _, dep := range depMap {
		allDeps = append(allDeps, dep)
	}

	// Apply filters from the context
	allDeps = ctx.FilterDeps(allDeps)

	if ctx.Task != nil {
		ctx.Task.Infof("Found %d unique dependencies", len(allDeps))
	}
	return allDeps, nil
}

// scanWithDepthTraversal performs depth traversal on local dependencies
func (s *Scanner) scanWithDepthTraversal(task *clicky.Task, dir string, maxDepth int) ([]*models.Dependency, []models.VersionConflict, error) {
	// Setup git support for depth traversal only if not already set up
	if !s.HasGitSupport() {
		s.SetupGitSupport(".cache/arch-unit/repositories")
	}

	// Use existing ScanWithDepth method
	depTree, err := s.ScanWithDepth(task, dir, maxDepth)
	if err != nil {
		return nil, nil, err
	}

	// Convert git types to models types
	return s.convertDepTreeToModels(depTree)
}

// scanGitRepositoryWithDepth scans a specific git repository with depth
func (s *Scanner) scanGitRepositoryWithDepth(task *clicky.Task, gitURL, version string, maxDepth int) ([]*models.Dependency, []models.VersionConflict, error) {
	// Setup git support only if not already set up
	if !s.HasGitSupport() {
		s.SetupGitSupport(".cache/arch-unit/repositories")
	}

	// Use existing ScanGitRepository method
	depTree, err := s.ScanGitRepository(task, gitURL, version, maxDepth)
	if err != nil {
		return nil, nil, err
	}

	// Convert git types to models types
	return s.convertDepTreeToModels(depTree)
}

// convertDepTreeToModels converts git.DependencyTree to models types
func (s *Scanner) convertDepTreeToModels(depTree *git.DependencyTree) ([]*models.Dependency, []models.VersionConflict, error) {
	var deps []*models.Dependency
	var conflicts []models.VersionConflict

	// Convert dependencies
	for _, rootRef := range depTree.Root {
		if visitedDep, exists := depTree.Dependencies[s.getRefKey(rootRef)]; exists {
			dep := s.convertRefToModel(rootRef, visitedDep)
			deps = append(deps, dep)
		}
	}

	// Convert conflicts
	for _, gitConflict := range depTree.Conflicts {
		conflict := models.VersionConflict{
			DependencyName:     gitConflict.DependencyName,
			ResolutionStrategy: gitConflict.ResolutionStrategy,
		}

		for _, gitVersionInfo := range gitConflict.Versions {
			versionInfo := models.VersionInfo{
				Version:    gitVersionInfo.Version,
				CommitSHA:  gitVersionInfo.CommitSHA,
				CommitDate: gitVersionInfo.CommitDate.Format("2006-01-02 15:04:05"),
			}
			conflict.Versions = append(conflict.Versions, versionInfo)
		}

		conflicts = append(conflicts, conflict)
	}

	return deps, conflicts, nil
}

// convertRefToModel converts a git.DependencyRef and VisitedDep to models.Dependency
func (s *Scanner) convertRefToModel(ref *git.DependencyRef, visited *git.VisitedDep) *models.Dependency {
	dep := &models.Dependency{
		Name:    ref.Name,
		Version: ref.Version,
		Git:     ref.Git,
		Type:    models.DependencyType(ref.Type),
		Depth:   visited.FirstSeen,
	}

	// Set resolved from if we have version instances with different original versions
	if len(visited.Versions) > 0 {
		firstVersion := visited.Versions[0]
		if firstVersion.Version != ref.Version {
			dep.ResolvedFrom = firstVersion.Version // The original alias
		}
	}

	return dep
}

// getRefKey returns a key for a dependency reference
func (s *Scanner) getRefKey(ref *git.DependencyRef) string {
	return fmt.Sprintf("%s:%s", ref.Type, ref.Name)
}

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

// queueSimpleTask queues a simple scanning task without recursive task groups
func (s *Scanner) queueSimpleTask(ctx *analysis.ScanContext, job git.ScanJob) {
	taskName := s.getJobIdentifier(job)

	s.discoveryGroup.Add(taskName, func(taskCtx commonsCtx.Context, t *clicky.Task) ([]*models.Dependency, error) {
		// Check depth limit
		if job.Depth > ctx.MaxDepth {
			return []*models.Dependency{}, nil
		}

		// Check for recursion prevention for git repositories
		if !job.IsLocal {
			repoKey := fmt.Sprintf("%s@%s", job.GitURL, job.Version)
			s.mutex.Lock()
			if s.repoVisited[repoKey] {
				s.mutex.Unlock()
				// Skip silently - already processed
				return []*models.Dependency{}, nil
			}
			s.repoVisited[repoKey] = true
			s.mutex.Unlock()
		}

		// Scan the current level
		deps, err := s.scanCurrentLevel(ctx, job)
		if err != nil {
			// Log the error but don't fail the entire scan
			t.Warnf("Failed to scan %s: %v", taskName, err)
			return []*models.Dependency{}, nil
		}

		// Apply filtering immediately after scanning
		deps = ctx.FilterDeps(deps)

		t.SetName(taskName + fmt.Sprintf(" (%d deps, depth: %d)", len(deps), job.Depth))

		// Store discovered dependencies
		s.mutex.Lock()
		s.discoveredDeps[job.Parent] = append(s.discoveredDeps[job.Parent], deps...)
		s.mutex.Unlock()

		// Process discovered dependencies for deeper levels
		for _, dep := range deps {
			// Track this dependency
			s.trackDependency(dep, job.Depth, job.Parent)

			// For deeper levels, check if we should scan deeper
			if job.Depth < ctx.MaxDepth && dep.Git != "" {
				// Log at info level to ensure it shows
				s.processDeepDependency(ctx, dep, job.Depth+1)
			}
		}

		return deps, nil
	})
}

// processDeepDependency handles deeper dependency scanning
func (s *Scanner) processDeepDependency(ctx *analysis.ScanContext, dep *models.Dependency, nextDepth int) {
	if dep.Git == "" {
		return
	}

	version := dep.Version
	if version == "" {
		version = "latest"
	}

	// For local replacements, use the local path directly
	var worktreePath string
	var err error
	if strings.HasPrefix(version, "local:") {
		worktreePath = strings.TrimPrefix(version, "local:")
		// No need to checkout, just use the local path
	} else {
		// Checkout and discover files synchronously
		worktreePath, err = s.checkoutDependency(dep.Git, version)
		if err != nil {
			ctx.Warnf("Failed to checkout %s@%s: %v", dep.Git, version, err)
			return
		}
	}

	// Discover scannable files in the repository
	discoveredFiles, err := s.discoverScanFiles(worktreePath)
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
		s.queueSimpleTask(ctx, job)
	}
}

// scanCurrentLevel scans dependencies at the current level
func (s *Scanner) scanCurrentLevel(ctx *analysis.ScanContext, job git.ScanJob) ([]*models.Dependency, error) {
	var filePath string
	var scanner analysis.DependencyScanner

	if job.IsLocal {
		filePath = filepath.Join(job.Path, job.FilePath)
		scannerInterface, ok := s.Registry.Get(job.ScannerType)
		if !ok {
			return nil, fmt.Errorf("no scanner found for type: %s", job.ScannerType)
		}
		scanner = scannerInterface
	} else {
		// For git repositories, the path is already the worktree path
		filePath = filepath.Join(job.Path, job.FilePath)
		scannerInterface, ok := s.Registry.Get(job.ScannerType)
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
	return deps, nil
}

// buildDependencyTree builds the final dependency tree from discovered dependencies
func (s *Scanner) buildDependencyTree() *git.DependencyTree {
	tree := &git.DependencyTree{
		Dependencies: make(map[string]*git.VisitedDep),
		MaxDepth:     0,
	}

	// Copy visited dependencies to tree
	s.mutex.RLock()
	for key, visited := range s.visited {
		tree.Dependencies[key] = visited
		// Track max depth
		for _, depth := range visited.SeenAt {
			if depth > tree.MaxDepth {
				tree.MaxDepth = depth
			}
		}
	}

	// Build root dependencies - these are dependencies discovered at depth 0
	// We collect all dependencies from discoveredDeps to ensure filtered results are included
	for parent, deps := range s.discoveredDeps {
		// Root dependencies are those with parent "root"
		if parent == "root" {
			for _, dep := range deps {
				tree.Root = append(tree.Root, s.depToRef(dep))
			}
		}
	}
	s.mutex.RUnlock()

	return tree
}

// detectVersionConflicts identifies dependencies with multiple versions
func (s *Scanner) detectVersionConflicts(tree *git.DependencyTree) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for depName, visited := range s.visited {
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

// trackDependency tracks a dependency in our visited map
func (s *Scanner) trackDependency(dep *models.Dependency, depth int, parent string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	depKey := s.getDependencyKey(dep)

	if visited, exists := s.visited[depKey]; exists {
		visited.SeenAt = append(visited.SeenAt, depth)
		visited.Versions = append(visited.Versions, git.VersionInstance{
			Version: dep.Version,
			Depth:   depth,
			Source:  parent,
		})
	} else {
		s.visited[depKey] = &git.VisitedDep{
			Dependency: s.depToRef(dep),
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

// checkoutDependency ensures a git dependency is checked out to a worktree
func (s *Scanner) checkoutDependency(gitURL, version string) (string, error) {
	// Handle local directory replacements
	if strings.HasPrefix(version, "local:") {
		localPath := strings.TrimPrefix(version, "local:")
		// Return the local path directly - no git checkout needed
		return localPath, nil
	}

	resolvedVersion, err := s.gitManager.ResolveVersionAlias(gitURL, version)
	if err != nil {
		resolvedVersion = version
	}

	worktreePath, err := s.gitManager.GetWorktreePath(gitURL, resolvedVersion)
	if err != nil {
		return "", fmt.Errorf("failed to get worktree for %s@%s: %w", gitURL, resolvedVersion, err)
	}

	return worktreePath, nil
}

// getJobIdentifier returns a readable identifier for a scan job
func (s *Scanner) getJobIdentifier(job git.ScanJob) string {
	if job.IsLocal {
		if job.FilePath != "" {
			// Special case for go.mod - indicate it includes go.sum
			if job.ScannerType == "go" && strings.HasSuffix(job.FilePath, "go.mod") {
				return fmt.Sprintf("%s/go.mod", job.Path)
			}
			return fmt.Sprintf("%s/%s", job.Path, job.FilePath)
		}
		return job.Path
	}
	// For remote repositories
	if job.ScannerType == "go" && strings.HasSuffix(job.FilePath, "go.mod") {
		return fmt.Sprintf("%s@%s:go.mod", job.GitURL, job.Version)
	}
	return fmt.Sprintf("%s@%s:%s", job.GitURL, job.Version, job.FilePath)
}

// depToRef converts a models.Dependency to a git.DependencyRef
func (s *Scanner) depToRef(dep *models.Dependency) *git.DependencyRef {
	return &git.DependencyRef{
		Name:    dep.Name,
		Version: dep.Version,
		Type:    string(dep.Type),
		Git:     dep.Git,
	}
}

// treeToLists converts a DependencyTree to lists of dependencies and conflicts
func (s *Scanner) treeToLists(tree *git.DependencyTree) ([]*models.Dependency, []models.VersionConflict) {
	var deps []*models.Dependency
	var conflicts []models.VersionConflict

	// Convert all dependencies from discoveredDeps that made it into the tree
	// We use discoveredDeps because those are the actual filtered results
	// Use a map to deduplicate
	depMap := make(map[string]*models.Dependency)

	s.mutex.RLock()
	for _, depList := range s.discoveredDeps {
		for _, dep := range depList {
			key := s.getDependencyKey(dep)
			if _, exists := depMap[key]; !exists {
				// Create a copy to avoid modifying the original
				depCopy := *dep
				depMap[key] = &depCopy
			}
		}
	}
	s.mutex.RUnlock()

	// Convert map to slice
	for _, dep := range depMap {
		deps = append(deps, dep)
	}

	// Convert conflicts
	for _, gitConflict := range tree.Conflicts {
		conflict := models.VersionConflict{
			DependencyName:     gitConflict.DependencyName,
			ResolutionStrategy: gitConflict.ResolutionStrategy,
		}

		for _, gitVersionInfo := range gitConflict.Versions {
			versionInfo := models.VersionInfo{
				Version:    gitVersionInfo.Version,
				CommitSHA:  gitVersionInfo.CommitSHA,
				CommitDate: gitVersionInfo.CommitDate.Format("2006-01-02 15:04:05"),
			}
			conflict.Versions = append(conflict.Versions, versionInfo)
		}

		conflicts = append(conflicts, conflict)
	}

	return deps, conflicts
}
