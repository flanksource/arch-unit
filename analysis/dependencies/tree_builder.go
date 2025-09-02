package dependencies

import (
	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/arch-unit/models"
)

// TreeBuilder handles building dependency trees from scan results
type TreeBuilder struct {
	scanner *Scanner
}

// NewTreeBuilder creates a new tree builder
func NewTreeBuilder(scanner *Scanner) *TreeBuilder {
	return &TreeBuilder{
		scanner: scanner,
	}
}

// BuildDependencyTree builds a git.DependencyTree from walk results
func (tb *TreeBuilder) BuildDependencyTree(walkResult *WalkResult) *git.DependencyTree {
	tree := &git.DependencyTree{
		Dependencies: make(map[string]*git.VisitedDep),
		MaxDepth:     0,
	}

	// Convert dependencies to visited deps and refs
	for _, dep := range walkResult.Dependencies {
		// Create dependency ref
		ref := &git.DependencyRef{
			Name:    dep.Name,
			Version: dep.Version,
			Type:    string(dep.Type),
			Git:     dep.Git,
		}

		// Create visited dep entry
		depKey := tb.getDependencyKey(dep)
		visited := &git.VisitedDep{
			Dependency: ref,
			FirstSeen:  dep.Depth,
			SeenAt:     []int{dep.Depth},
			Versions: []git.VersionInstance{{
				Version: dep.Version,
				Depth:   dep.Depth,
				Source:  "walker", // Could be enhanced to track actual source
			}},
		}

		tree.Dependencies[depKey] = visited

		// Track max depth
		if dep.Depth > tree.MaxDepth {
			tree.MaxDepth = dep.Depth
		}

		// Add to root if this is a direct dependency (depth 0)
		if dep.Depth == 0 {
			tree.Root = append(tree.Root, ref)
		}
	}

	// Convert conflicts
	for _, conflict := range walkResult.Conflicts {
		gitVersionInfos := make([]git.VersionInfo, len(conflict.Versions))
		for i, versionInfo := range conflict.Versions {
			gitVersionInfos[i] = git.VersionInfo{
				Version: versionInfo.Version,
			}
		}

		gitConflict := git.VersionConflict{
			DependencyName:     conflict.DependencyName,
			Versions:           gitVersionInfos,
			ResolutionStrategy: conflict.ResolutionStrategy,
		}

		tree.Conflicts = append(tree.Conflicts, gitConflict)
	}

	return tree
}

// BuildScanResult builds a models.ScanResult from walk results
func (tb *TreeBuilder) BuildScanResult(walkResult *WalkResult, scanType string, maxDepth int) *models.ScanResult {
	metadata := models.ScanMetadata{
		ScanType:          scanType,
		MaxDepth:          maxDepth,
		TotalDependencies: len(walkResult.Dependencies),
		ConflictsFound:    len(walkResult.Conflicts),
		RepositoriesFound: tb.countRepositories(walkResult.Dependencies),
	}

	return &models.ScanResult{
		Dependencies: walkResult.Dependencies,
		Conflicts:    walkResult.Conflicts,
		Metadata:     metadata,
	}
}

// getDependencyKey creates a unique key for a dependency
func (tb *TreeBuilder) getDependencyKey(dep *models.Dependency) string {
	return tb.scanner.getDependencyKey(dep)
}

// countRepositories counts unique git repositories in dependencies
func (tb *TreeBuilder) countRepositories(deps []*models.Dependency) int {
	repoSet := make(map[string]bool)
	
	for _, dep := range deps {
		if dep.Git != "" {
			repoSet[dep.Git] = true
		}
	}
	
	return len(repoSet)
}