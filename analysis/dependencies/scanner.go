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
	Registry        *analysis.DependencyRegistry
	NameFilters     []string
	GitFilters      []string
	ShowIndirect    bool // Whether to include indirect dependencies
	MaxDepth        int  // Maximum depth to traverse (-1 for unlimited)
	
	// Optional depth manager for advanced scanning
	depthManager    *DepthManager
	gitManager      git.GitRepositoryManager
}

// NewScanner creates a new dependency scanner
func NewScanner() *Scanner {
	return &Scanner{
		Registry:     analysis.DefaultDependencyRegistry,
		ShowIndirect: true,  // Default to showing all dependencies
		MaxDepth:     -1,    // Default to unlimited depth
	}
}

// NewScannerWithRegistry creates a new dependency scanner with a custom registry
func NewScannerWithRegistry(registry *analysis.DependencyRegistry) *Scanner {
	return &Scanner{
		Registry:     registry,
		ShowIndirect: true,  // Default to showing all dependencies
		MaxDepth:     -1,    // Default to unlimited depth
	}
}

// ScanPath scans a path (local directory or git URL) for dependencies with optional depth traversal
func (s *Scanner) ScanPath(task *clicky.Task, pathOrURL string, maxDepth int) (*models.ScanResult, error) {
	if maxDepth < 0 {
		maxDepth = 0 // Default to no depth traversal
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
		clicky.UpdateGlobalPhaseProgress(fmt.Sprintf("Scanning local dependencies in %s", pathOrURL))
		scanType = "local"
		deps, err = s.scanDirectory(task, pathOrURL)
		if err != nil {
			return nil, err
		}
		
		// Phase 2: Apply depth traversal if requested for local scan
		if maxDepth > 0 {
			clicky.UpdateGlobalPhaseProgress(fmt.Sprintf("Processing git repositories at depth %d", maxDepth))
			deps, conflicts, err = s.scanWithDepthTraversal(task, pathOrURL, maxDepth)
			if err != nil {
				return nil, err
			}
			scanType = "mixed"
			repoCount = len(conflicts) // Approximation based on conflicts found
		}
	} else {
		// Phase 1: Git repository scanning
		if maxDepth == 0 {
			maxDepth = 1 // For git URLs, default to at least depth 1
		}
		clicky.UpdateGlobalPhaseProgress(fmt.Sprintf("Scanning git repository %s@%s", gitURL, version))
		scanType = "git"
		deps, conflicts, err = s.scanGitRepositoryWithDepth(task, gitURL, version, maxDepth)
		if err != nil {
			return nil, err
		}
		repoCount = 1 + len(conflicts) // Original repo + any repos found in conflicts
	}
	
	// Phase 3: Analyzing results
	if len(conflicts) > 0 {
		clicky.UpdateGlobalPhaseProgress("Detecting version conflicts")
	}
	
	// Build metadata
	metadata := models.ScanMetadata{
		ScanType:          scanType,
		MaxDepth:          maxDepth,
		RepositoriesFound: repoCount,
		TotalDependencies: len(deps),
		ConflictsFound:    len(conflicts),
	}
	
	if s.gitManager != nil {
		// Get git cache dir if available - need to access through interface method or add to interface
		metadata.GitCacheDir = ".cache/arch-unit/repositories" // Default cache dir
	}
	
	// Final phase update
	if len(conflicts) > 0 {
		clicky.UpdateGlobalPhaseProgress(fmt.Sprintf("Found %d dependencies with %d conflicts", len(deps), len(conflicts)))
	} else {
		clicky.UpdateGlobalPhaseProgress(fmt.Sprintf("Found %d dependencies", len(deps)))
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
func (s *Scanner) scanWithScanner(task *clicky.Task, dir string, scanner analysis.DependencyScanner) ([]*models.Dependency, error) {
	var allDeps []*models.Dependency
	
	// Create scan context with the root directory
	ctx := analysis.NewScanContext(task, dir)
	
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
			if task != nil {
				task.Warnf("Error reading %s: %v", path, err)
			}
			return nil
		}
		
		// Scan the file
		deps, err := scanner.ScanFile(ctx, path, content)
		if err != nil {
			if task != nil {
				task.Warnf("Error scanning %s: %v", path, err)
			}
			return nil
		}
		
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

// applyFilters applies the configured filters to the dependencies
func (s *Scanner) applyFilters(deps []*models.Dependency) []*models.Dependency {
	var filtered []*models.Dependency
	
	for _, dep := range deps {
		// Filter out indirect dependencies if not showing them
		if !s.ShowIndirect && dep.Indirect {
			continue
		}
		
		// Apply depth filter
		if s.MaxDepth >= 0 && dep.Depth > s.MaxDepth {
			continue
		}
		
		// Apply filters - a dependency passes if it matches name OR git filters
		// Name filters check both Name and Git fields for better usability
		if len(s.NameFilters) > 0 || len(s.GitFilters) > 0 {
			matched := false
			
			// Check name filters against both Name and Git fields
			if len(s.NameFilters) > 0 {
				if collections.MatchItems(dep.Name, s.NameFilters...) {
					matched = true
				} else if dep.Git != "" && collections.MatchItems(dep.Git, s.NameFilters...) {
					matched = true
				}
			}
			
			// Check git filters (only against Git field)
			if !matched && len(s.GitFilters) > 0 && dep.Git != "" {
				if collections.MatchItems(dep.Git, s.GitFilters...) {
					matched = true
				}
			}
			
			// If no filters matched, skip this dependency
			if !matched && (len(s.NameFilters) > 0 || len(s.GitFilters) > 0) {
				continue
			}
		}
		
		// Recursively filter children
		if len(dep.Children) > 0 {
			dep.Children = s.applyFiltersToChildren(dep.Children)
		}
		
		filtered = append(filtered, dep)
	}
	
	return filtered
}

// applyFiltersToChildren recursively applies filters to child dependencies
func (s *Scanner) applyFiltersToChildren(children []models.Dependency) []models.Dependency {
	var filtered []models.Dependency
	
	for _, child := range children {
		// Create a pointer to apply filters
		childCopy := child
		childDeps := []*models.Dependency{&childCopy}
		filteredDeps := s.applyFilters(childDeps)
		
		if len(filteredDeps) > 0 {
			filtered = append(filtered, *filteredDeps[0])
		}
	}
	
	return filtered
}



// ParseFilters parses filter strings into separate filter slices
// Filters can be:
// - Git filters: contain "/" or start with "github.com", "gitlab.com", etc.
// - Language filters: match known language names (go, python, javascript, etc.)
// - Name filters: everything else
// Filters support wildcards (*) and negation (!)
func ParseFilters(filters []string) (nameFilters, gitFilters []string) {
	knownLanguages := map[string]bool{
		"go": true, "golang": true,
		"python": true, "py": true,
		"javascript": true, "js": true, "typescript": true, "ts": true,
		"java": true,
		"rust": true,
		"ruby": true,
		"php": true,
		"c": true, "cpp": true, "c++": true,
		"helm": true,
		"docker": true,
	}
	
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		
		// Check for legacy prefixes and strip them
		if strings.HasPrefix(filter, "-name ") {
			filter = strings.TrimPrefix(filter, "-name ")
		} else if strings.HasPrefix(filter, "-git ") {
			filter = strings.TrimPrefix(filter, "-git ")
			gitFilters = append(gitFilters, filter)
			continue
		} else if strings.HasPrefix(filter, "-language ") || strings.HasPrefix(filter, "-lang ") {
			// Skip language filters as we're removing language field
			continue
		}
		
		// Auto-detect filter type
		lowerFilter := strings.ToLower(strings.TrimPrefix(filter, "!"))
		
		// Check if it's a git filter (contains "/" or known git domains)
		if strings.Contains(filter, "/") || 
		   strings.Contains(lowerFilter, "github.com") ||
		   strings.Contains(lowerFilter, "gitlab.com") ||
		   strings.Contains(lowerFilter, "bitbucket.org") {
			gitFilters = append(gitFilters, filter)
		} else if knownLanguages[lowerFilter] {
			// Skip language filters as we're removing language field
			continue
		} else {
			// Default to name filter
			nameFilters = append(nameFilters, filter)
		}
	}
	return
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
	
	// Use UnifiedScanner for two-phase scanning
	unifiedScanner := NewUnifiedScanner(s, s.gitManager, maxDepth)
	
	// Create scan context
	ctx := analysis.NewScanContext(task, dir)
	
	// Perform two-phase scan
	return unifiedScanner.ScanWithTwoPhases(ctx, dir)
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
	if task != nil {
		task.Infof("Scanning dependencies in %s", dir)
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
		
		if task != nil {
			task.Infof("Scanning %s dependencies", scanner.Language())
		}
		
		deps, err := s.scanWithScanner(task, dir, scanner)
		if err != nil {
			if task != nil {
				task.Warnf("Error scanning %s: %v", scanner.Language(), err)
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
	
	// Apply filters
	allDeps = s.applyFilters(allDeps)
	
	if task != nil {
		task.Infof("Found %d unique dependencies", len(allDeps))
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
				
				// Create a scan job for this specific file
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
		t.Debugf("Processing job %s at depth %d (max depth: %d)", taskName, job.Depth, ctx.MaxDepth)
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

		// Store ALL discovered dependencies unfiltered for traversal
		s.mutex.Lock()
		s.discoveredDeps[job.Parent] = append(s.discoveredDeps[job.Parent], deps...)
		s.mutex.Unlock()

		// Process ALL dependencies for deeper levels (unfiltered)
		for _, dep := range deps {
			// Track this dependency
			s.trackDependency(dep, job.Depth, job.Parent)

			// For deeper levels, check if we should scan deeper
			// Check filter here - only traverse dependencies that match filter
			if job.Depth < ctx.MaxDepth && dep.Git != "" && dep.Matches(ctx.Filter) {
				s.processDeepDependency(ctx, dep, job.Depth+1)
			}
		}

		// Return filtered deps for task result
		filteredDeps := ctx.FilterDeps(deps)

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
	if strings.HasPrefix(version, "local:") {
		worktreePath := strings.TrimPrefix(version, "local:")
		// No need to checkout, just use the local path
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
		return
	}

	// Create a checkout task for non-local dependencies
	checkoutName := fmt.Sprintf("Checkout %s@%s", dep.Git, version)
	s.discoveryGroup.Add(checkoutName, func(taskCtx commonsCtx.Context, t *clicky.Task) ([]*models.Dependency, error) {
		// Perform the checkout
		worktreePath, err := s.checkoutDependency(dep.Git, version)
		if err != nil {
			t.Errorf("Failed to checkout: %v", err)
			return nil, err
		}
		t.Success()

		// Discover scannable files in the repository
		discoveredFiles, err := s.discoverScanFiles(worktreePath)
		if err != nil {
			return nil, fmt.Errorf("failed to discover files: %w", err)
		}

		// Queue individual scan jobs for discovered files
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

		return []*models.Dependency{}, nil
	})
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