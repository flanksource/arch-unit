package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// DefaultVersionResolver implements VersionResolver
type DefaultVersionResolver struct {
	gitManager GitRepositoryManager
	cache      map[string]string // "repo:alias" -> resolved_version
	mutex      sync.RWMutex
}

// NewVersionResolver creates a new version resolver
func NewVersionResolver(gitManager GitRepositoryManager) VersionResolver {
	return &DefaultVersionResolver{
		gitManager: gitManager,
		cache:      make(map[string]string),
	}
}

// ResolveVersion resolves aliases like HEAD, GA, HEAD~1 to actual versions
func (vr *DefaultVersionResolver) ResolveVersion(ctx context.Context, gitURL string, alias string) (string, error) {
	// If it's not an alias, return as-is
	if !vr.IsVersionAlias(alias) {
		return alias, nil
	}

	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s", gitURL, alias)
	vr.mutex.RLock()
	if cached, exists := vr.cache[cacheKey]; exists {
		vr.mutex.RUnlock()
		return cached, nil
	}
	vr.mutex.RUnlock()

	// Get repository
	repo, err := vr.gitManager.GetRepository(gitURL)
	if err != nil {
		return "", fmt.Errorf("failed to get repository: %w", err)
	}

	// Resolve the alias
	var resolved string
	switch {
	case alias == "HEAD" || alias == "latest":
		resolved, err = vr.resolveHead(repo)
	case alias == "GA":
		resolved, err = vr.resolveGA(repo)
	case strings.HasPrefix(alias, "HEAD~"):
		resolved, err = vr.resolveHeadOffset(repo, alias)
	case strings.HasPrefix(alias, "GA~"):
		resolved, err = vr.resolveGAOffset(repo, alias)
	default:
		return alias, nil // Not actually an alias
	}

	if err != nil {
		return "", err
	}

	// Cache the result
	vr.mutex.Lock()
	vr.cache[cacheKey] = resolved
	vr.mutex.Unlock()

	return resolved, nil
}

// IsVersionAlias returns true if the given string is a version alias
func (vr *DefaultVersionResolver) IsVersionAlias(version string) bool {
	return version == "HEAD" ||
		version == "latest" ||
		version == "GA" ||
		strings.HasPrefix(version, "HEAD~") ||
		strings.HasPrefix(version, "GA~")
}

// GetAvailableVersions returns all available versions/tags for a repository
func (vr *DefaultVersionResolver) GetAvailableVersions(ctx context.Context, gitURL string) ([]string, error) {
	_, err := vr.gitManager.GetRepository(gitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	// This is a placeholder - we would need to implement tag listing
	// For now, return empty list
	return []string{}, nil
}

// Helper methods

func (vr *DefaultVersionResolver) resolveHead(repo GitRepository) (string, error) {
	// HEAD refers to the latest tag in git (including pre-releases, betas, RCs, etc.)
	// Use the repository's own method to get the latest tag
	return vr.getLatestTag(repo)
}

func (vr *DefaultVersionResolver) resolveGA(repo GitRepository) (string, error) {
	// GA refers to the latest stable (non-prerelease) version
	return repo.FindLastGARelease()
}

func (vr *DefaultVersionResolver) resolveHeadOffset(repo GitRepository, alias string) (string, error) {
	// Parse the offset (e.g., "HEAD~1" -> 1)
	offsetStr := strings.TrimPrefix(alias, "HEAD~")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return "", fmt.Errorf("invalid HEAD~ offset: %s", offsetStr)
	}

	// Get all versions and return the nth one back from HEAD
	versions, err := vr.getAllVersionsSorted(repo)
	if err != nil {
		return "", err
	}

	if offset >= len(versions) {
		return "", fmt.Errorf("HEAD~%d goes beyond available versions (found %d versions)", offset, len(versions))
	}

	return versions[offset], nil
}

func (vr *DefaultVersionResolver) resolveGAOffset(repo GitRepository, alias string) (string, error) {
	// Parse the offset (e.g., "GA~1" -> 1)
	offsetStr := strings.TrimPrefix(alias, "GA~")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return "", fmt.Errorf("invalid GA~ offset: %s", offsetStr)
	}

	// Get all stable versions and return the nth one back from GA
	stableVersions, err := vr.getStableVersionsSorted(repo)
	if err != nil {
		return "", err
	}

	if offset >= len(stableVersions) {
		return "", fmt.Errorf("GA~%d goes beyond available stable versions (found %d versions)", offset, len(stableVersions))
	}

	return stableVersions[offset], nil
}

func (vr *DefaultVersionResolver) getLatestTag(repo GitRepository) (string, error) {
	// This is a placeholder implementation
	// We would need to access the underlying git repository to get tags
	// For now, we'll use the repository's resolve method as fallback
	return repo.ResolveVersion("HEAD")
}

func (vr *DefaultVersionResolver) getAllVersionsSorted(repo GitRepository) ([]string, error) {
	// This is a placeholder - we would need to implement proper tag listing and sorting
	// The real implementation would:
	// 1. Get all tags from the repository
	// 2. Sort them by creation date (newest first)
	// 3. Return the sorted list
	return []string{}, fmt.Errorf("version sorting not yet implemented")
}

func (vr *DefaultVersionResolver) getStableVersionsSorted(repo GitRepository) ([]string, error) {
	// This is a placeholder - we would need to implement stable version filtering
	// The real implementation would:
	// 1. Get all tags from the repository
	// 2. Filter out pre-release tags (containing "alpha", "beta", "rc", etc.)
	// 3. Sort by creation date (newest first)
	// 4. Return the sorted stable versions
	return []string{}, fmt.Errorf("stable version filtering not yet implemented")
}

func (vr *DefaultVersionResolver) isPreRelease(version string, preReleasePatterns []string) bool {
	if preReleasePatterns == nil {
		preReleasePatterns = []string{"alpha", "beta", "rc", "preview", "pre"}
	}

	versionLower := strings.ToLower(version)
	for _, pattern := range preReleasePatterns {
		if strings.Contains(versionLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
