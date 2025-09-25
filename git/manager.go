package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultGitRepositoryManager implements GitRepositoryManager
type DefaultGitRepositoryManager struct {
	cacheDir        string
	repositories    map[string]*repositoryEntry
	versionCache    map[string]*CacheEntry // "repo:alias" -> resolved_version
	mutex           sync.RWMutex
	cloneManager    CloneManager
	versionResolver VersionResolver
}

type repositoryEntry struct {
	repository GitRepository
	lastAccess time.Time
	mutex      sync.RWMutex
}

// NewGitRepositoryManager creates a new git repository manager
func NewGitRepositoryManager(cacheDir string) GitRepositoryManager {
	if cacheDir == "" {
		cacheDir = ".cache/arch-unit/repositories"
	}

	manager := &DefaultGitRepositoryManager{
		cacheDir:     cacheDir,
		repositories: make(map[string]*repositoryEntry),
		versionCache: make(map[string]*CacheEntry),
	}

	manager.cloneManager = NewCloneManager()
	manager.versionResolver = NewVersionResolver(manager)

	return manager
}

// GetRepository returns a GitRepository instance for the given URL
func (gm *DefaultGitRepositoryManager) GetRepository(gitURL string) (GitRepository, error) {
	gm.mutex.RLock()
	if entry, exists := gm.repositories[gitURL]; exists {
		entry.mutex.Lock()
		entry.lastAccess = time.Now()
		entry.mutex.Unlock()
		gm.mutex.RUnlock()
		return entry.repository, nil
	}
	gm.mutex.RUnlock()

	// Create new repository
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	// Double-check after acquiring write lock
	if entry, exists := gm.repositories[gitURL]; exists {
		entry.mutex.Lock()
		entry.lastAccess = time.Now()
		entry.mutex.Unlock()
		return entry.repository, nil
	}

	repo, err := gm.createRepository(gitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository for %s: %w", gitURL, err)
	}

	gm.repositories[gitURL] = &repositoryEntry{
		repository: repo,
		lastAccess: time.Now(),
	}

	return repo, nil
}

// GetWorktreePath returns the filesystem path to a specific version's clone
func (gm *DefaultGitRepositoryManager) GetWorktreePath(gitURL, version string, depth int) (string, error) {
	repo, err := gm.GetRepository(gitURL)
	if err != nil {
		return "", err
	}

	return repo.GetWorktree(version, depth)
}

// ResolveVersionAlias resolves version aliases across repositories
func (gm *DefaultGitRepositoryManager) ResolveVersionAlias(gitURL, alias string) (string, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s", gitURL, alias)
	gm.mutex.RLock()
	if entry, exists := gm.versionCache[cacheKey]; exists {
		// Check if cache entry is still valid (1 hour TTL)
		if time.Since(entry.Timestamp) < time.Hour {
			entry.AccessedAt = time.Now()
			gm.mutex.RUnlock()
			if entry.Error != nil {
				return "", entry.Error.(error)
			}
			return entry.Value.(string), nil
		}
	}
	gm.mutex.RUnlock()

	// Resolve version
	resolved, err := gm.versionResolver.ResolveVersion(context.Background(), gitURL, alias)

	// Cache result
	gm.mutex.Lock()
	gm.versionCache[cacheKey] = &CacheEntry{
		Value:      resolved,
		Timestamp:  time.Now(),
		AccessedAt: time.Now(),
		Error:      err,
	}
	gm.mutex.Unlock()

	return resolved, err
}

// GetCacheDir returns the cache directory being used
func (gm *DefaultGitRepositoryManager) GetCacheDir() string {
	return gm.cacheDir
}

// CleanupUnused removes unused repositories and worktrees older than maxAge
func (gm *DefaultGitRepositoryManager) CleanupUnused(maxAge time.Duration) error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	var toRemove []string
	cutoff := time.Now().Add(-maxAge)

	for url, entry := range gm.repositories {
		entry.mutex.RLock()
		if entry.lastAccess.Before(cutoff) {
			toRemove = append(toRemove, url)
		}
		entry.mutex.RUnlock()
	}

	// Remove unused repositories
	for _, url := range toRemove {
		if entry, exists := gm.repositories[url]; exists {
			// Cleanup clones
			clones, err := entry.repository.ListWorktrees()
			if err == nil {
				for _, clone := range clones {
					_ = entry.repository.CleanupWorktree(clone.Version)
				}
			}

			// Remove repository directory
			repoPath := entry.repository.GetRepoPath()
			_ = os.RemoveAll(repoPath)

			delete(gm.repositories, url)
		}
	}

	// Clean version cache
	for key, entry := range gm.versionCache {
		if entry.AccessedAt.Before(cutoff) {
			delete(gm.versionCache, key)
		}
	}

	return nil
}

// SetCacheDir sets the base cache directory
func (gm *DefaultGitRepositoryManager) SetCacheDir(dir string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	gm.cacheDir = dir
}

// ListRepositories returns all managed repositories
func (gm *DefaultGitRepositoryManager) ListRepositories() []string {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	var repos []string
	for url := range gm.repositories {
		repos = append(repos, url)
	}
	return repos
}

// Close cleans up all resources
func (gm *DefaultGitRepositoryManager) Close() error {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	// Clean up all clones for each repository
	for _, entry := range gm.repositories {
		if entry.repository != nil {
			// Get clones for this repository
			repoPath := gm.cacheDir // This is simplified; actual path depends on implementation
			clones, err := gm.cloneManager.ListClones(context.Background(), repoPath)
			if err == nil {
				// Remove all clones
				for _, clone := range clones {
					_ = gm.cloneManager.RemoveClone(context.Background(), clone.Path)
				}
			}
		}
	}

	// Clear the repository and cache maps
	gm.repositories = make(map[string]*repositoryEntry)
	gm.versionCache = make(map[string]*CacheEntry)

	// Optionally clean up the entire cache directory if it's temporary
	// This is commented out as it might be too aggressive
	// os.RemoveAll(gm.cacheDir)

	return nil
}

// createRepository creates a new GitRepository instance
func (gm *DefaultGitRepositoryManager) createRepository(gitURL string) (GitRepository, error) {
	repoPath, err := gm.getRepositoryPath(gitURL)
	if err != nil {
		return nil, err
	}

	return NewDefaultGitRepository(gitURL, repoPath, gm.cloneManager)
}

// getRepositoryPath generates the local path for a repository
func (gm *DefaultGitRepositoryManager) getRepositoryPath(gitURL string) (string, error) {
	// Extract org/repo from URL
	parts := strings.Split(strings.TrimPrefix(gitURL, "https://github.com/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid GitHub repository URL: %s", gitURL)
	}

	org := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	return filepath.Join(gm.cacheDir, "github.com", org, repo), nil
}
