package git

import (
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Commit represents a git commit with relevant metadata
type Commit struct {
	Hash             string
	Message          string
	ShortDescription string
	Author           string
	Date             time.Time
	GitHubReferences []GitHubReference
}

// GitHubReference represents a reference to a GitHub issue or PR
type GitHubReference struct {
	Number int
	Type   string // "issue" or "pull"
	URL    string
}

// VersionInfo contains commit information for a specific version/tag
type VersionInfo struct {
	Version    string
	CommitSHA  string
	CommitDate time.Time
}

// ManagedRepository represents a git repository with multiple worktrees
type ManagedRepository struct {
	gitRepo   *git.Repository
	worktrees map[string]string // version -> worktree_path
	lastFetch time.Time
}

// ScanJob represents a scanning task for a specific file at a specific depth
type ScanJob struct {
	Path        string // Directory path containing the file
	FilePath    string // Specific file to scan (e.g., "go.mod", "Chart.yaml")
	GitURL      string
	Version     string
	Depth       int
	IsLocal     bool
	Parent      string
	ScannerType string // Type of scanner needed ("go", "helm", "npm", etc.)
}

// VisitedDep tracks a dependency seen during depth traversal
type VisitedDep struct {
	Dependency *DependencyRef
	FirstSeen  int   // At which depth first encountered
	SeenAt     []int // All depths where this dep was found
	Versions   []VersionInstance
}

// VersionInstance tracks a specific version of a dependency
type VersionInstance struct {
	Version     string
	Depth       int
	Source      string // Which parent dependency led to this
	GitWorktree string // Path to checked out version
}

// VersionConflict represents conflicting versions of the same dependency
type VersionConflict struct {
	DependencyName     string
	Versions           []VersionInfo
	ResolutionStrategy string // "latest", "pinned", "manual"
}

// DependencyRef is a lightweight reference to a dependency for tracking
type DependencyRef struct {
	Name    string
	Version string
	Type    string
	Git     string
}

// DependencyTree represents the full dependency tree with conflicts
type DependencyTree struct {
	Root         []*DependencyRef
	Dependencies map[string]*VisitedDep
	Conflicts    []VersionConflict
	MaxDepth     int
}

// ResolveResult contains the result of version resolution
type ResolveResult struct {
	OriginalAlias   string
	ResolvedVersion string
	ResolvedAt      time.Time
	Error           error
}

// CacheEntry represents a cached git operation result
type CacheEntry struct {
	Value      interface{}
	Timestamp  time.Time
	AccessedAt time.Time
	Error      error
}

// CloneInfo contains information about a managed clone
type CloneInfo struct {
	Path      string
	Version   string
	Depth     int // Clone depth (0 = full, >0 = shallow)
	CreatedAt time.Time
	LastUsed  time.Time
	Hash      plumbing.Hash
}
