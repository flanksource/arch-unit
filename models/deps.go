package models

import (
	"fmt"
	"time"

	"github.com/flanksource/clicky/api"
	"github.com/flanksource/commons/collections"
)

type DependencyType string

const (
	DependencyTypeInternal  DependencyType = "internal"  // Internal dependencies within the project
	DependencyTypeMaven     DependencyType = "maven"     // Maven dependencies
	DependencyTypeNpm       DependencyType = "npm"       // NPM dependencies
	DependencyTypePip       DependencyType = "pip"       // Python dependencies
	DependencyTypeGo        DependencyType = "go"        // Go dependencies
	DependencyTypeDocker    DependencyType = "docker"    // Docker image dependecies .e.g. FROM
	DependencyTypeHelm      DependencyType = "helm"      // Helm chart dependencies
	DependencyTypeGit       DependencyType = "git"       // Git repository dependencies
	DependencyTypeKustomize DependencyType = "kustomize" // Kustomize dependencies
	DependencyTypeStdlib    DependencyType = "stdlib"    // Standard library dependencies builtin to the language, version refers to he Go version etc..

)

type Dependency struct {
	ID           int64          `json:"id" pretty:"hide"`
	Name         string         `json:"name" pretty:"label=Name,style=text-blue-500,sort=2"`  // Name of the dependency, e.g. "express", "requests", "flask", "gin", "kubernetes"
	Package      []string       `json:"packages,omitempty" pretty:"label=Packages,omitempty"` // optional provided packages, not relevant to all dependency types
	Type         DependencyType `json:"type" pretty:"label=Type,style=text-purple-600"`
	Version      string         `json:"version,omitempty" pretty:"label=Version,style=text-green-600,omitempty"` // Git Version of the library
	Git          string         `json:"git,omitempty" pretty:"label=Git,omitempty"`                              // Git URL of the library including path to Chart.yaml, Dockerfile, package.json, etc.
	Source       string         `json:"source" pretty:"label=Source"`                                            // Source location where dependency was found, e.g. "go.mod:23" or "package.json:15"
	Indirect     bool           `json:"indirect,omitempty" pretty:"label=Indirect,omitempty"`                    // Whether this is an indirect dependency
	Depth        int            `json:"depth,omitempty" pretty:"label=Depth,omitempty"`                          // Depth in the dependency tree (0 = direct, 1+ = transitive)
	Children     []Dependency   `json:"children,omitempty" pretty:"label=Children,type=tree,omitempty"`          // Child dependencies
	ResolvedFrom string         `json:"resolved_from,omitempty" pretty:"label=Resolved From,omitempty"`          // Original version alias (HEAD, GA, latest) that was resolved
	Homepage     string         `json:"homepage,omitempty" pretty:"label=Homepage,omitempty"`                    // Homepage URL of the library
}

// ScanResult contains the result of dependency scanning with metadata
type ScanResult struct {
	Dependencies []*Dependency     `json:"dependencies" pretty:"label=Dependencies,type=tree"`
	Conflicts    []VersionConflict `json:"conflicts,omitempty" pretty:"label=Version Conflicts,omitempty"`
	Metadata     ScanMetadata      `json:"metadata,omitempty" pretty:"label=Scan Metadata,omitempty"`
}

// VersionConflict represents conflicting versions of the same dependency
type VersionConflict struct {
	DependencyName     string        `json:"dependency_name" pretty:"label=Dependency"`
	Versions           []VersionInfo `json:"versions" pretty:"label=Versions"`
	ResolutionStrategy string        `json:"resolution_strategy" pretty:"label=Resolution Strategy"`
}

// VersionInfo contains information about a specific version
type VersionInfo struct {
	Version    string `json:"version" pretty:"label=Version"`
	Depth      int    `json:"depth" pretty:"label=Depth"`
	Source     string `json:"source" pretty:"label=Source"`
	CommitSHA  string `json:"commit_sha,omitempty" pretty:"label=Commit SHA,omitempty"`
	CommitDate string `json:"commit_date,omitempty" pretty:"label=Commit Date,omitempty"`
}

// ScanMetadata contains metadata about the scan operation
type ScanMetadata struct {
	ScanType          string `json:"scan_type" pretty:"label=Scan Type"` // "local", "git", "mixed"
	MaxDepth          int    `json:"max_depth" pretty:"label=Max Depth"`
	GitCacheDir       string `json:"git_cache_dir,omitempty" pretty:"label=Git Cache Dir,omitempty"`
	RepositoriesFound int    `json:"repositories_found" pretty:"label=Repositories Found"`
	TotalDependencies int    `json:"total_dependencies" pretty:"label=Total Dependencies"`
	ConflictsFound    int    `json:"conflicts_found" pretty:"label=Conflicts Found"`
}

func (d Dependency) Pretty() api.Text {
	icon := "📦"
	switch d.Type {
	case DependencyTypeGo:
		icon = "🐹"
	case DependencyTypeNpm:
		icon = "📦"
	case DependencyTypePip:
		icon = "🐍"
	case DependencyTypeDocker:
		icon = "🐳"
	case DependencyTypeHelm:
		icon = "⎈"
	case DependencyTypeGit:
		icon = "🔗"
	case DependencyTypeStdlib:
		icon = "📚"
	}

	content := fmt.Sprintf("%s %s", icon, d.Name)
	if d.Version != "" {
		content = fmt.Sprintf("%s %s@%s", icon, d.Name, d.Version)
		// Show original alias if version was resolved
		if d.ResolvedFrom != "" && d.ResolvedFrom != d.Version {
			content = fmt.Sprintf("%s %s@%s (was %s)", icon, d.Name, d.Version, d.ResolvedFrom)
		}
	}

	// Add indirect marker
	if d.Indirect {
		content = content + " (indirect)"
	}

	style := "text-blue-600"
	if d.Type == DependencyTypeStdlib {
		style = "text-green-600"
	} else if d.Type == DependencyTypeInternal {
		style = "text-purple-600"
	} else if d.Indirect {
		style = "text-gray-500" // Gray out indirect dependencies
	}

	return api.Text{
		Content: content,
		Style:   style,
	}
}

func (d Dependency) Matches(filter string) bool {
	if filter == "" {
		return true
	}

	matches, negated := collections.MatchAny([]string{d.Git, d.Name, string(d.Type)}, filter)
	if negated {
		return false
	}

	return matches
}

// DependencyAlias represents a cached mapping from package to Git repository
type DependencyAlias struct {
	ID          int64  `json:"id"`
	PackageName string `json:"package_name"` // e.g., "express", "docker.io/library/nginx"
	PackageType string `json:"package_type"` // "npm", "docker", "helm", "go", etc.
	GitURL      string `json:"git_url"`      // Final resolved and validated Git URL (empty if none found)
	LastChecked int64  `json:"last_checked"` // Unix timestamp for cache invalidation
	CreatedAt   int64  `json:"created_at"`
}

// IsExpired checks if the alias cache entry is stale (> 7 days)
func (da *DependencyAlias) IsExpired() bool {
	return da.IsExpiredWithTTL(7 * 24 * time.Hour)
}

// IsExpiredWithTTL checks if the alias cache entry is stale based on provided TTL
func (da *DependencyAlias) IsExpiredWithTTL(ttl time.Duration) bool {
	if ttl == 0 {
		return true // TTL of 0 means always expired (no cache)
	}
	age := time.Since(time.Unix(da.LastChecked, 0))
	return age > ttl
}

// NewDependencyAlias creates a new dependency alias with current timestamp
func NewDependencyAlias(packageName, packageType, gitURL string) *DependencyAlias {
	now := time.Now().Unix()
	return &DependencyAlias{
		PackageName: packageName,
		PackageType: packageType,
		GitURL:      gitURL,
		LastChecked: now,
		CreatedAt:   now,
	}
}
