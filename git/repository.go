package git

import (
	"context"
	"time"
)

// GitRepository interface defines operations on a single git repository
type GitRepository interface {
	// Clone clones the repository to local cache
	Clone(ctx context.Context, url string) error
	
	// Fetch updates the repository from remote
	Fetch(ctx context.Context) error
	
	// GetWorktree returns the path to a worktree for the specified version
	// Creates the worktree if it doesn't exist
	GetWorktree(version string) (string, error)
	
	// ResolveVersion resolves version aliases (HEAD, GA, latest) to concrete versions
	ResolveVersion(alias string) (string, error)
	
	// GetCommitsBetween returns commits between two versions
	GetCommitsBetween(from, to string) ([]Commit, error)
	
	// GetVersionInfo returns commit information for a specific version
	GetVersionInfo(version string) (*VersionInfo, error)
	
	// GetTagDate returns the creation date of a specific tag
	GetTagDate(tag string) (time.Time, error)
	
	// FindLastGARelease finds the most recent stable release
	FindLastGARelease() (string, error)
	
	// ListWorktrees returns all active worktrees for this repository
	ListWorktrees() ([]WorktreeInfo, error)
	
	// CleanupWorktree removes a specific worktree
	CleanupWorktree(version string) error
	
	// GetRepoPath returns the path to the main git repository
	GetRepoPath() string
}

// GitRepositoryManager interface defines operations across multiple repositories
type GitRepositoryManager interface {
	// GetRepository returns a GitRepository instance for the given URL
	GetRepository(gitURL string) (GitRepository, error)
	
	// GetWorktreePath returns the filesystem path to a specific version's worktree
	GetWorktreePath(gitURL, version string) (string, error)
	
	// ResolveVersionAlias resolves version aliases across repositories
	ResolveVersionAlias(gitURL, alias string) (string, error)
	
	// CleanupUnused removes unused repositories and worktrees older than maxAge
	CleanupUnused(maxAge time.Duration) error
	
	// GetCacheDir returns the base cache directory
	GetCacheDir() string
	
	// SetCacheDir sets the base cache directory
	SetCacheDir(dir string)
	
	// ListRepositories returns all managed repositories
	ListRepositories() []string
	
	// Close cleans up all resources
	Close() error
}

// VersionResolver interface defines version alias resolution
type VersionResolver interface {
	// ResolveVersion resolves aliases like HEAD, GA, HEAD~1 to actual versions
	ResolveVersion(ctx context.Context, gitURL string, alias string) (string, error)
	
	// IsVersionAlias returns true if the given string is a version alias
	IsVersionAlias(version string) bool
	
	// GetAvailableVersions returns all available versions/tags for a repository
	GetAvailableVersions(ctx context.Context, gitURL string) ([]string, error)
}

// WorktreeManager interface defines worktree lifecycle management
type WorktreeManager interface {
	// CreateWorktree creates a new worktree for the specified version
	CreateWorktree(repoPath, version, worktreePath string) error
	
	// RemoveWorktree removes an existing worktree
	RemoveWorktree(worktreePath string) error
	
	// ListWorktrees lists all worktrees for a repository
	ListWorktrees(repoPath string) ([]WorktreeInfo, error)
	
	// CleanupStaleWorktrees removes worktrees that haven't been used recently
	CleanupStaleWorktrees(repoPath string, maxAge time.Duration) error
}