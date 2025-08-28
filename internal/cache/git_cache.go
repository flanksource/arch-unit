package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GitCache struct {
	baseDir string
	ttl     time.Duration
}

func NewGitCache() (*GitCache, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".cache", "arch-unit", "git")
	return &GitCache{
		baseDir: baseDir,
		ttl:     24 * time.Hour,
	}, nil
}

func NewGitCacheWithOptions(baseDir string, ttl time.Duration) *GitCache {
	return &GitCache{
		baseDir: baseDir,
		ttl:     ttl,
	}
}

func (gc *GitCache) getCacheDir(repoURL string) string {
	repoName := gc.getRepoName(repoURL)
	return filepath.Join(gc.baseDir, repoName)
}

func (gc *GitCache) getRepoName(repoURL string) string {
	repoURL = strings.TrimSuffix(repoURL, ".git")
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	repoURL = strings.TrimPrefix(repoURL, "git@")
	repoURL = strings.ReplaceAll(repoURL, ":", "_")
	repoURL = strings.ReplaceAll(repoURL, "/", "_")

	hash := sha256.Sum256([]byte(repoURL))
	shortHash := hex.EncodeToString(hash[:8])

	parts := strings.Split(repoURL, "_")
	if len(parts) >= 2 {
		repoName := parts[len(parts)-1]
		if len(repoName) > 30 {
			repoName = repoName[:30]
		}
		return fmt.Sprintf("%s_%s", repoName, shortHash)
	}

	return shortHash
}

func (gc *GitCache) IsCached(repoURL string) bool {
	cacheDir := gc.getCacheDir(repoURL)
	gitDir := filepath.Join(cacheDir, ".git")

	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		if gc.ttl <= 0 {
			return true
		}

		if time.Since(info.ModTime()) < gc.ttl {
			return true
		}
	}

	return false
}

func (gc *GitCache) CloneOrUpdate(repoURL string, ref string) (string, error) {
	cacheDir := gc.getCacheDir(repoURL)

	if err := os.MkdirAll(gc.baseDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache base directory: %w", err)
	}

	gitDir := filepath.Join(cacheDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := gc.clone(repoURL, cacheDir); err != nil {
			return "", fmt.Errorf("failed to clone repository: %w", err)
		}
	} else {
		if err := gc.update(cacheDir); err != nil {
			os.RemoveAll(cacheDir)
			if err := gc.clone(repoURL, cacheDir); err != nil {
				return "", fmt.Errorf("failed to re-clone repository after update failure: %w", err)
			}
		}
	}

	if ref != "" && ref != "HEAD" {
		if err := gc.checkout(cacheDir, ref); err != nil {
			return "", fmt.Errorf("failed to checkout ref %s: %w", ref, err)
		}
	}

	return cacheDir, nil
}

func (gc *GitCache) clone(repoURL, targetDir string) error {
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, targetDir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (gc *GitCache) update(repoDir string) error {
	cmd := exec.Command("git", "fetch", "--depth", "1")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch failed: %w\nOutput: %s", err, string(output))
	}

	cmd = exec.Command("git", "reset", "--hard", "origin/HEAD")
	cmd.Dir = repoDir

	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git reset failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (gc *GitCache) checkout(repoDir, ref string) error {
	cmd := exec.Command("git", "checkout", ref)
	cmd.Dir = repoDir

	if _, err := cmd.CombinedOutput(); err != nil {
		cmd = exec.Command("git", "fetch", "origin", ref)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git fetch ref failed: %w\nOutput: %s", err, string(output))
		}

		cmd = exec.Command("git", "checkout", ref)
		cmd.Dir = repoDir

		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git checkout failed: %w\nOutput: %s", err, string(output))
		}
	}

	return nil
}

func (gc *GitCache) GetCachedPath(repoURL string) (string, error) {
	if !gc.IsCached(repoURL) {
		return "", fmt.Errorf("repository %s is not cached", repoURL)
	}

	return gc.getCacheDir(repoURL), nil
}

func (gc *GitCache) Clean() error {
	return os.RemoveAll(gc.baseDir)
}

func (gc *GitCache) CleanRepo(repoURL string) error {
	cacheDir := gc.getCacheDir(repoURL)
	return os.RemoveAll(cacheDir)
}

func (gc *GitCache) CleanStale() error {
	if gc.ttl <= 0 {
		return nil
	}

	entries, err := os.ReadDir(gc.baseDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(gc.baseDir, entry.Name())
		gitDir := filepath.Join(repoPath, ".git")

		if info, err := os.Stat(gitDir); err == nil {
			if time.Since(info.ModTime()) > gc.ttl {
				if err := os.RemoveAll(repoPath); err != nil {
					return fmt.Errorf("failed to remove stale cache %s: %w", repoPath, err)
				}
			}
		}
	}

	return nil
}
