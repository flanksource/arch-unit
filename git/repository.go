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

	// GetWorktree returns the path to a clone for the specified version
	// Creates the clone if it doesn't exist
	// depth: 0 for full clone, >0 for shallow clone with that depth
	GetWorktree(version string, depth int) (string, error)

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

	// ListWorktrees returns all active clones for this repository
	ListWorktrees() ([]CloneInfo, error)

	// CleanupWorktree removes a specific worktree
	CleanupWorktree(version string) error

	// GetRepoPath returns the path to the main git repository
	GetRepoPath() string
}

// GitRepositoryManager interface defines operations across multiple repositories
type GitRepositoryManager interface {
	// GetRepository returns a GitRepository instance for the given URL
	GetRepository(gitURL string) (GitRepository, error)

	// GetWorktreePath returns the filesystem path to a specific version's clone
	// depth: 0 for full clone, >0 for shallow clone with that depth  
	GetWorktreePath(gitURL, version string, depth int) (string, error)

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

// CloneManager interface defines clone lifecycle management
type CloneManager interface {
	// CreateClone creates a new clone for the specified version with given depth
	CreateClone(ctx context.Context, repoPath, version, clonePath string, depth int) error

	// RemoveClone removes an existing clone
	RemoveClone(ctx context.Context, clonePath string) error

	// ListClones lists all clones for a repository
	ListClones(ctx context.Context, repoPath string) ([]CloneInfo, error)

	// CleanupStaleClones removes clones that haven't been used recently
	CleanupStaleClones(ctx context.Context, repoPath string, maxAge time.Duration) error
}
