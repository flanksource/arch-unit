package cache

import (
	"fmt"
	"strings"
)

type GitIntegration struct {
	cache *GitCache
}

func NewGitIntegration() (*GitIntegration, error) {
	cache, err := NewGitCache()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize git cache: %w", err)
	}
	
	return &GitIntegration{
		cache: cache,
	}, nil
}

func NewGitIntegrationWithCache(cache *GitCache) *GitIntegration {
	return &GitIntegration{
		cache: cache,
	}
}

func (gi *GitIntegration) PrepareRepository(path string) (string, error) {
	if gi.isRemoteRepo(path) {
		return gi.cache.CloneOrUpdate(path, "")
	}
	
	return path, nil
}

func (gi *GitIntegration) PrepareRepositoryWithRef(path, ref string) (string, error) {
	if gi.isRemoteRepo(path) {
		return gi.cache.CloneOrUpdate(path, ref)
	}
	
	return path, nil
}

func (gi *GitIntegration) isRemoteRepo(path string) bool {
	return strings.HasPrefix(path, "http://") ||
		strings.HasPrefix(path, "https://") ||
		strings.HasPrefix(path, "git@") ||
		strings.HasPrefix(path, "ssh://") ||
		strings.Contains(path, "github.com") ||
		strings.Contains(path, "gitlab.com") ||
		strings.Contains(path, "bitbucket.org")
}

func (gi *GitIntegration) GetCachedPath(repoURL string) (string, error) {
	if !gi.isRemoteRepo(repoURL) {
		return repoURL, nil
	}
	
	return gi.cache.GetCachedPath(repoURL)
}

func (gi *GitIntegration) IsCached(repoURL string) bool {
	if !gi.isRemoteRepo(repoURL) {
		return true
	}
	
	return gi.cache.IsCached(repoURL)
}

func (gi *GitIntegration) CleanCache() error {
	return gi.cache.Clean()
}

func (gi *GitIntegration) CleanStaleCache() error {
	return gi.cache.CleanStale()
}

func (gi *GitIntegration) CleanRepoCache(repoURL string) error {
	if !gi.isRemoteRepo(repoURL) {
		return nil
	}
	
	return gi.cache.CleanRepo(repoURL)
}