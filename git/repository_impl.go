package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/flanksource/commons/logger"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DefaultGitRepository implements GitRepository
type DefaultGitRepository struct {
	gitURL       string
	repoPath     string
	gitRepo      *git.Repository
	clones       map[string]CloneInfo // version-depth -> CloneInfo
	mutex        sync.RWMutex
	cloneManager CloneManager
	lastFetch    time.Time
}

// NewDefaultGitRepository creates a new GitRepository instance
func NewDefaultGitRepository(gitURL, repoPath string, cloneManager CloneManager) (GitRepository, error) {
	repo := &DefaultGitRepository{
		gitURL:       gitURL,
		repoPath:     repoPath,
		clones:       make(map[string]CloneInfo),
		cloneManager: cloneManager,
	}

	err := repo.ensureCloned()
	if err != nil {
		return nil, err
	}

	return repo, nil
}

// Clone clones the repository to local cache
func (r *DefaultGitRepository) Clone(ctx context.Context, url string) error {
	return r.ensureCloned()
}

// Fetch updates the repository from remote
func (r *DefaultGitRepository) Fetch(ctx context.Context) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.gitRepo == nil {
		return fmt.Errorf("repository not initialized")
	}

	// Don't fetch too frequently (minimum 5 minute interval)
	if time.Since(r.lastFetch) < 5*time.Minute {
		return nil
	}

	err := r.gitRepo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"refs/*:refs/*"},
	})

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch updates: %w", err)
	}

	r.lastFetch = time.Now()
	return nil
}

// GetWorktree returns the path to a clone for the specified version
func (r *DefaultGitRepository) GetWorktree(version string, depth int) (string, error) {
	// Create cache key that includes both version and depth
	cacheKey := fmt.Sprintf("%s-%d", version, depth)
	
	r.mutex.RLock()
	if info, exists := r.clones[cacheKey]; exists {
		// Check if clone still exists on disk
		if _, err := os.Stat(info.Path); err == nil {
			// Check if existing clone has sufficient depth
			if info.Depth == 0 || (depth > 0 && info.Depth >= depth) {
				// Update last used time
				r.mutex.RUnlock()
				r.mutex.Lock()
				info.LastUsed = time.Now()
				r.clones[cacheKey] = info
				r.mutex.Unlock()
				return info.Path, nil
			}
		}
		// Clone doesn't exist or has insufficient depth, remove from cache
		r.mutex.RUnlock()
		r.mutex.Lock()
		delete(r.clones, cacheKey)
		r.mutex.Unlock()
		r.mutex.RLock()
	}
	r.mutex.RUnlock()

	// Check if we have a deeper clone that can satisfy this request
	r.mutex.RLock()
	for key, info := range r.clones {
		if strings.HasPrefix(key, version+"-") && (info.Depth == 0 || (depth > 0 && info.Depth >= depth)) {
			if _, err := os.Stat(info.Path); err == nil {
				// Update last used time
				r.mutex.RUnlock()
				r.mutex.Lock()
				info.LastUsed = time.Now()
				r.clones[key] = info
				r.mutex.Unlock()
				return info.Path, nil
			}
		}
	}
	r.mutex.RUnlock()

	// Ensure we have the latest refs
	if err := r.Fetch(context.Background()); err != nil {
		logger.Warnf("Failed to fetch latest refs: %v", err)
	}

	// Create new clone
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Double-check after acquiring write lock
	if info, exists := r.clones[cacheKey]; exists {
		if _, err := os.Stat(info.Path); err == nil {
			info.LastUsed = time.Now()
			r.clones[cacheKey] = info
			return info.Path, nil
		}
	}

	clonePath := r.getClonePath(version, depth)

	// Create clone directory
	if err := os.MkdirAll(filepath.Dir(clonePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create clone directory: %w", err)
	}

	// Create the clone
	err := r.cloneManager.CreateClone(context.Background(), r.repoPath, version, clonePath, depth)
	if err != nil {
		return "", fmt.Errorf("failed to create clone for version %s: %w", version, err)
	}

	// Resolve version to hash for tracking
	hash, err := r.resolveRef(version)
	if err != nil {
		// Still track the clone even if we can't resolve the hash
		hash = plumbing.ZeroHash
	}

	// Track the new clone
	r.clones[cacheKey] = CloneInfo{
		Path:      clonePath,
		Version:   version,
		Depth:     depth,
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
		Hash:      hash,
	}

	return clonePath, nil
}

// ResolveVersion resolves version aliases to concrete versions
func (r *DefaultGitRepository) ResolveVersion(versionAlias string) (string, error) {
	if !r.isVersionAlias(versionAlias) {
		return versionAlias, nil
	}

	// Ensure repository is up to date
	if err := r.Fetch(context.Background()); err != nil {
		// Continue with stale data if fetch fails
	}

	switch {
	case versionAlias == "HEAD" || versionAlias == "latest":
		return r.getLatestTag()
	case versionAlias == "GA":
		return r.getLatestStableTag()
	case strings.HasPrefix(versionAlias, "HEAD~"):
		return r.resolveHeadOffset(versionAlias)
	case strings.HasPrefix(versionAlias, "GA~"):
		return r.resolveGAOffset(versionAlias)
	default:
		return versionAlias, nil
	}
}

// GetCommitsBetween returns commits between two versions
func (r *DefaultGitRepository) GetCommitsBetween(from, to string) ([]Commit, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if r.gitRepo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	// Resolve version references
	fromHash, err := r.resolveRef(from)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve 'from' version %s: %w", from, err)
	}

	toHash, err := r.resolveRef(to)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve 'to' version %s: %w", to, err)
	}

	return r.getCommitList(fromHash, toHash)
}

// GetVersionInfo returns commit information for a specific version
func (r *DefaultGitRepository) GetVersionInfo(version string) (*VersionInfo, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if r.gitRepo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	hash, err := r.resolveRef(version)
	if err != nil {
		return nil, err
	}

	commit, err := r.gitRepo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit for version %s: %w", version, err)
	}

	return &VersionInfo{
		CommitSHA:  commit.Hash.String(),
		CommitDate: commit.Committer.When,
	}, nil
}

// GetTagDate returns the creation date of a specific tag
func (r *DefaultGitRepository) GetTagDate(tag string) (time.Time, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if r.gitRepo == nil {
		return time.Time{}, fmt.Errorf("repository not initialized")
	}

	// Try to get the tag reference
	tagRef, err := r.gitRepo.Tag(tag)
	if err != nil {
		// Try with v prefix
		if !strings.HasPrefix(tag, "v") {
			tagRef, err = r.gitRepo.Tag("v" + tag)
		}
		if err != nil {
			return time.Time{}, fmt.Errorf("tag %s not found: %w", tag, err)
		}
	}

	// Try to get the tag object (annotated tag)
	tagObj, err := r.gitRepo.TagObject(tagRef.Hash())
	if err == nil {
		return tagObj.Tagger.When, nil
	}

	// Lightweight tag - get the commit time
	commit, err := r.gitRepo.CommitObject(tagRef.Hash())
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get commit for tag %s: %w", tag, err)
	}

	return commit.Committer.When, nil
}

// FindLastGARelease finds the most recent stable release
func (r *DefaultGitRepository) FindLastGARelease() (string, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if r.gitRepo == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	// Get all tags
	tags, err := r.gitRepo.Tags()
	if err != nil {
		return "", fmt.Errorf("failed to get tags: %w", err)
	}

	type TagWithTime struct {
		Name      string
		Timestamp time.Time
	}

	var allTags []TagWithTime
	err = tags.ForEach(func(ref *plumbing.Reference) error {
		tagName := strings.TrimPrefix(ref.Name().String(), "refs/tags/")

		var timestamp time.Time
		// Try to get the tag object (annotated tag)
		tag, err := r.gitRepo.TagObject(ref.Hash())
		if err == nil {
			timestamp = tag.Tagger.When
		} else {
			// Lightweight tag - get the commit time
			commit, err := r.gitRepo.CommitObject(ref.Hash())
			if err == nil {
				timestamp = commit.Committer.When
			} else {
				return nil // Skip tags we can't get timestamp for
			}
		}

		allTags = append(allTags, TagWithTime{
			Name:      tagName,
			Timestamp: timestamp,
		})
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to iterate tags: %w", err)
	}

	if len(allTags) == 0 {
		return "", fmt.Errorf("no tags found in repository")
	}

	// Sort by timestamp (newest first)
	sort.Slice(allTags, func(i, j int) bool {
		return allTags[i].Timestamp.After(allTags[j].Timestamp)
	})

	// Default pre-release patterns
	preReleasePatterns := []string{"beta", "rc", "alpha", "preview", "pre"}

	// Find the most recent GA release
	for _, tag := range allTags {
		if r.isGARelease(tag.Name, preReleasePatterns) {
			return tag.Name, nil
		}
	}

	return "", fmt.Errorf("no GA release tags found (all %d tags are pre-releases)", len(allTags))
}

// ListWorktrees returns all active clones for this repository
func (r *DefaultGitRepository) ListWorktrees() ([]CloneInfo, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var clones []CloneInfo
	for _, info := range r.clones {
		clones = append(clones, info)
	}
	return clones, nil
}

// CleanupWorktree removes a specific clone (keeping method name for compatibility)
func (r *DefaultGitRepository) CleanupWorktree(version string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Remove all clones for this version (different depths)
	var toDelete []string
	for key, info := range r.clones {
		if strings.HasPrefix(key, version+"-") {
			// Remove clone
			err := r.cloneManager.RemoveClone(context.Background(), info.Path)
			if err != nil {
				return fmt.Errorf("failed to remove clone: %w", err)
			}
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		delete(r.clones, key)
	}
	
	return nil
}

// GetRepoPath returns the path to the main git repository
func (r *DefaultGitRepository) GetRepoPath() string {
	return r.repoPath
}

// Helper methods

func (r *DefaultGitRepository) ensureCloned() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Check if repository already exists (as bare repo or regular repo)
	gitDir := filepath.Join(r.repoPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		// Regular repository exists, open it
		repo, err := git.PlainOpen(r.repoPath)
		if err != nil {
			// Repository corrupted, remove and re-clone
			os.RemoveAll(r.repoPath)
		} else {
			r.gitRepo = repo
			return nil
		}
	} else if _, err := os.Stat(filepath.Join(r.repoPath, "HEAD")); err == nil {
		// Bare repository exists
		repo, err := git.PlainOpen(r.repoPath)
		if err != nil {
			// Repository corrupted, remove and re-clone
			os.RemoveAll(r.repoPath)
		} else {
			r.gitRepo = repo
			return nil
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(r.repoPath), 0755); err != nil {
		return fmt.Errorf("failed to create repository directory: %w", err)
	}

	// Clone repository
	repo, err := git.PlainClone(r.repoPath, false, &git.CloneOptions{
		URL:      r.gitURL,
		Progress: nil,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	r.gitRepo = repo
	r.lastFetch = time.Now()
	return nil
}

func (r *DefaultGitRepository) getClonePath(version string, depth int) string {
	return filepath.Join(r.repoPath, "clones", fmt.Sprintf("%s-depth%d", version, depth))
}

func (r *DefaultGitRepository) isVersionAlias(version string) bool {
	return version == "HEAD" || version == "latest" || version == "GA" ||
		strings.HasPrefix(version, "HEAD~") || strings.HasPrefix(version, "GA~")
}

func (r *DefaultGitRepository) getLatestTag() (string, error) {
	// Implementation similar to omi-cli
	// Get all tags and return the most recent one
	tags, err := r.gitRepo.Tags()
	if err != nil {
		return "", err
	}

	var latestTag string
	var latestTime time.Time

	err = tags.ForEach(func(ref *plumbing.Reference) error {
		tagName := strings.TrimPrefix(ref.Name().String(), "refs/tags/")

		// Get tag timestamp
		var timestamp time.Time
		tag, err := r.gitRepo.TagObject(ref.Hash())
		if err == nil {
			timestamp = tag.Tagger.When
		} else {
			commit, err := r.gitRepo.CommitObject(ref.Hash())
			if err == nil {
				timestamp = commit.Committer.When
			} else {
				return nil
			}
		}

		if timestamp.After(latestTime) {
			latestTime = timestamp
			latestTag = tagName
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	if latestTag == "" {
		return "", fmt.Errorf("no tags found")
	}

	return latestTag, nil
}

func (r *DefaultGitRepository) getLatestStableTag() (string, error) {
	// Similar to FindLastGARelease but returns just the tag name
	tag, err := r.FindLastGARelease()
	return tag, err
}

func (r *DefaultGitRepository) resolveHeadOffset(alias string) (string, error) {
	// Implementation for HEAD~1, HEAD~2, etc.
	// This would get all tags, sort them, and return the nth one back
	return "", fmt.Errorf("HEAD~ offset resolution not yet implemented")
}

func (r *DefaultGitRepository) resolveGAOffset(alias string) (string, error) {
	// Implementation for GA~1, GA~2, etc.
	// Similar to HEAD~ but only considers stable releases
	return "", fmt.Errorf("GA~ offset resolution not yet implemented")
}

func (r *DefaultGitRepository) resolveRef(ref string) (plumbing.Hash, error) {
	// Try as a tag first
	tagRef, err := r.gitRepo.Tag(ref)
	if err == nil {
		tagObj, err := r.gitRepo.TagObject(tagRef.Hash())
		if err == nil {
			return tagObj.Target, nil
		} else {
			return tagRef.Hash(), nil
		}
	}

	// Try as a branch
	branchRef, err := r.gitRepo.Reference(plumbing.ReferenceName("refs/heads/"+ref), true)
	if err == nil {
		return branchRef.Hash(), nil
	}

	// Try as a remote branch
	remoteBranchRef, err := r.gitRepo.Reference(plumbing.ReferenceName("refs/remotes/origin/"+ref), true)
	if err == nil {
		return remoteBranchRef.Hash(), nil
	}

	// Try to parse as commit hash
	hash := plumbing.NewHash(ref)
	if !hash.IsZero() {
		_, err := r.gitRepo.CommitObject(hash)
		if err == nil {
			return hash, nil
		}
	}

	return plumbing.ZeroHash, fmt.Errorf("reference %s not found", ref)
}

func (r *DefaultGitRepository) getCommitList(fromHash, toHash plumbing.Hash) ([]Commit, error) {
	var commits []Commit

	// Get commit iterator
	iter, err := r.gitRepo.Log(&git.LogOptions{From: toHash})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Iterate through commits until we reach fromHash
	err = iter.ForEach(func(commit *object.Commit) error {
		if commit.Hash == fromHash {
			return fmt.Errorf("reached from commit") // Use error to break iteration
		}

		// Extract first line for short description
		lines := strings.Split(commit.Message, "\n")
		shortDesc := commit.Message
		if len(lines) > 0 {
			shortDesc = strings.TrimSpace(lines[0])
		}

		commits = append(commits, Commit{
			Hash:             commit.Hash.String(),
			Message:          commit.Message,
			ShortDescription: shortDesc,
			Author:           commit.Author.Name,
			Date:             commit.Author.When,
			GitHubReferences: []GitHubReference{}, // TODO: Parse GitHub references
		})

		return nil
	})

	// We expect to hit the "reached from commit" error
	if err != nil && err.Error() != "reached from commit" {
		return nil, err
	}

	return commits, nil
}

func (r *DefaultGitRepository) isGARelease(tag string, preReleasePatterns []string) bool {
	tagLower := strings.ToLower(tag)

	for _, pattern := range preReleasePatterns {
		patternLower := strings.ToLower(pattern)
		if strings.Contains(tagLower, patternLower) {
			return false
		}
	}

	return true
}
