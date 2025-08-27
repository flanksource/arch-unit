package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/shutdown"
	"github.com/flanksource/commons/logger"
)

// DefaultWorktreeManager implements WorktreeManager
type DefaultWorktreeManager struct {
	activeWorktrees map[string]string // worktree path -> repo path
	mu              sync.RWMutex
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager() WorktreeManager {
	manager := &DefaultWorktreeManager{
		activeWorktrees: make(map[string]string),
	}
	
	// Register cleanup hook
	shutdown.AddHookWithPriority("cleanup git worktrees", shutdown.PriorityWorkers, func() {
		manager.CleanupAll()
	})
	
	return manager
}

// CreateWorktree creates a new worktree for the specified version
func (wm *DefaultWorktreeManager) CreateWorktree(repoPath, version, worktreePath string) error {
	// Ensure the repository exists and is up to date
	if err := wm.ensureRepoFetched(repoPath); err != nil {
		return fmt.Errorf("failed to ensure repository is fetched: %w", err)
	}

	// Create temp directory if worktreePath is not provided
	if worktreePath == "" {
		tempDir, err := os.MkdirTemp("", "arch-unit-worktree-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		worktreePath = tempDir
	} else {
		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
			return fmt.Errorf("failed to create worktree directory: %w", err)
		}
	}
	
	// Resolve the version to a commit hash
	commitHash, err := wm.resolveVersion(repoPath, version)
	if err != nil {
		os.RemoveAll(worktreePath)
		return fmt.Errorf("failed to resolve version %s: %w", version, err)
	}
	
	// Create worktree using git command
	cmd := exec.Command("git", "worktree", "add", worktreePath, commitHash)
	cmd.Dir = repoPath
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if worktree already exists
		if strings.Contains(string(output), "already exists") {
			// Try to remove the old worktree first
			wm.removeWorktreeFromGit(repoPath, worktreePath)
			
			// Retry creation
			cmd = exec.Command("git", "worktree", "add", worktreePath, commitHash)
			cmd.Dir = repoPath
			output, err = cmd.CombinedOutput()
		}
		
		if err != nil {
			os.RemoveAll(worktreePath)
			return fmt.Errorf("failed to create worktree: %w\nOutput: %s", err, string(output))
		}
	}
	
	// Track the worktree for cleanup
	wm.mu.Lock()
	wm.activeWorktrees[worktreePath] = repoPath
	wm.mu.Unlock()
	
	logger.Debugf("Created worktree at %s for %s@%s", worktreePath, repoPath, version)
	
	return nil
}

// RemoveWorktree removes an existing worktree
func (wm *DefaultWorktreeManager) RemoveWorktree(worktreePath string) error {
	wm.mu.Lock()
	repoPath, exists := wm.activeWorktrees[worktreePath]
	if exists {
		delete(wm.activeWorktrees, worktreePath)
	}
	wm.mu.Unlock()
	
	if !exists {
		// Not tracked, just remove the directory
		return os.RemoveAll(worktreePath)
	}
	
	// Remove from git's worktree list
	if err := wm.removeWorktreeFromGit(repoPath, worktreePath); err != nil {
		logger.Warnf("Failed to remove worktree from git: %v", err)
	}
	
	// Remove the directory
	if err := os.RemoveAll(worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree directory %s: %w", worktreePath, err)
	}
	
	logger.Debugf("Removed worktree at %s", worktreePath)
	return nil
}

// removeWorktreeFromGit removes a worktree from git's tracking
func (wm *DefaultWorktreeManager) removeWorktreeFromGit(repoPath, worktreePath string) error {
	// First try to remove with force
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoPath
	
	if output, err := cmd.CombinedOutput(); err != nil {
		// If that fails, try pruning
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = repoPath
		if _, pruneErr := pruneCmd.CombinedOutput(); pruneErr != nil {
			return fmt.Errorf("failed to remove/prune worktree: %w\nOutput: %s", err, string(output))
		}
	}
	
	return nil
}

// ListWorktrees lists all worktrees for a repository
func (wm *DefaultWorktreeManager) ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}
	
	var worktrees []WorktreeInfo
	lines := strings.Split(string(output), "\n")
	
	var current WorktreeInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" && current.Path != "" {
			// End of worktree info
			worktrees = append(worktrees, current)
			current = WorktreeInfo{}
			continue
		}
		
		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Version = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch ")
			if current.Version == "" {
				current.Version = branch
			}
		}
	}
	
	// Add last worktree if exists
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	
	// Set timestamps based on directory info (approximation)
	for i := range worktrees {
		if info, err := os.Stat(worktrees[i].Path); err == nil {
			worktrees[i].CreatedAt = info.ModTime()
			worktrees[i].LastUsed = info.ModTime()
		}
	}
	
	return worktrees, nil
}

// CleanupStaleWorktrees removes worktrees that haven't been used recently
func (wm *DefaultWorktreeManager) CleanupStaleWorktrees(repoPath string, maxAge time.Duration) error {
	worktrees, err := wm.ListWorktrees(repoPath)
	if err != nil {
		return err
	}
	
	cutoff := time.Now().Add(-maxAge)
	
	for _, worktree := range worktrees {
		// Skip the main worktree (which has the same path as the repo)
		if worktree.Path == repoPath {
			continue
		}
		
		if worktree.LastUsed.Before(cutoff) {
			if err := wm.RemoveWorktree(worktree.Path); err != nil {
				logger.Warnf("Failed to cleanup stale worktree %s: %v", worktree.Path, err)
			}
		}
	}
	
	// Also run git worktree prune to clean up any broken references
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoPath
	if _, err := cmd.CombinedOutput(); err != nil {
		logger.Warnf("Failed to prune worktrees: %v", err)
	}
	
	return nil
}

// CleanupAll removes all tracked worktrees
func (wm *DefaultWorktreeManager) CleanupAll() {
	wm.mu.RLock()
	worktrees := make(map[string]string)
	for path, repo := range wm.activeWorktrees {
		worktrees[path] = repo
	}
	wm.mu.RUnlock()
	
	for worktreePath := range worktrees {
		if err := wm.RemoveWorktree(worktreePath); err != nil {
			logger.Warnf("Failed to cleanup worktree %s: %v", worktreePath, err)
		}
	}
	
	logger.Infof("Cleaned up %d worktrees", len(worktrees))
}

// ensureRepoFetched ensures the repository is cloned and up to date
func (wm *DefaultWorktreeManager) ensureRepoFetched(repoPath string) error {
	// Check if repository exists (bare or regular)
	gitDir := filepath.Join(repoPath, ".git")
	bareHead := filepath.Join(repoPath, "HEAD")
	
	if _, err := os.Stat(gitDir); err != nil {
		if _, err := os.Stat(bareHead); err != nil {
			return fmt.Errorf("repository does not exist at %s", repoPath)
		}
		// It's a bare repository, which is fine
	}
	
	// Fetch latest changes
	cmd := exec.Command("git", "fetch", "--all", "--tags")
	cmd.Dir = repoPath
	
	if output, err := cmd.CombinedOutput(); err != nil {
		// Don't fail if fetch fails (might be offline), just warn
		logger.Warnf("Failed to fetch updates for %s: %v\nOutput: %s", repoPath, err, string(output))
	}
	
	return nil
}

// resolveVersion resolves a version string to a commit hash
func (wm *DefaultWorktreeManager) resolveVersion(repoPath, version string) (string, error) {
	// Try different formats to resolve the version
	candidates := []string{
		version,                     // As-is
		"v" + version,              // With v prefix
		"origin/" + version,        // Remote branch
		"refs/tags/" + version,     // Tag ref
		"refs/tags/v" + version,    // Tag ref with v
		"refs/remotes/origin/" + version, // Remote ref
	}
	
	for _, candidate := range candidates {
		cmd := exec.Command("git", "rev-parse", candidate)
		cmd.Dir = repoPath
		
		if output, err := cmd.Output(); err == nil {
			hash := strings.TrimSpace(string(output))
			if hash != "" {
				return hash, nil
			}
		}
	}
	
	// If nothing worked, try as a partial commit hash
	cmd := exec.Command("git", "rev-parse", version)
	cmd.Dir = repoPath
	if output, err := cmd.Output(); err == nil {
		hash := strings.TrimSpace(string(output))
		if hash != "" {
			return hash, nil
		}
	}
	
	return "", fmt.Errorf("failed to resolve version %s in repository %s", version, repoPath)
}