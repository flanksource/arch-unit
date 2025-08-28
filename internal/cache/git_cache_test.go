package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewGitCache(t *testing.T) {
	gc, err := NewGitCache()
	if err != nil {
		t.Fatalf("Failed to create GitCache: %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	expectedBaseDir := filepath.Join(homeDir, ".cache", "arch-unit", "git")
	if gc.baseDir != expectedBaseDir {
		t.Errorf("Expected baseDir %s, got %s", expectedBaseDir, gc.baseDir)
	}

	if gc.ttl != 24*time.Hour {
		t.Errorf("Expected ttl 24h, got %v", gc.ttl)
	}
}

func TestGetRepoName(t *testing.T) {
	gc := &GitCache{}

	tests := []struct {
		name     string
		repoURL  string
		contains string
	}{
		{
			name:     "HTTPS GitHub URL",
			repoURL:  "https://github.com/user/repo.git",
			contains: "repo",
		},
		{
			name:     "SSH GitHub URL",
			repoURL:  "git@github.com:user/repo.git",
			contains: "repo",
		},
		{
			name:     "HTTP URL without .git",
			repoURL:  "http://github.com/user/another-repo",
			contains: "another-repo",
		},
		{
			name:     "GitLab URL",
			repoURL:  "https://gitlab.com/group/subgroup/project.git",
			contains: "project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gc.getRepoName(tt.repoURL)
			if result == "" {
				t.Error("getRepoName returned empty string")
			}
			if len(result) > 50 {
				t.Errorf("Repo name too long: %d characters", len(result))
			}
		})
	}
}

func TestGetCacheDir(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)

	repoURL := "https://github.com/user/test-repo.git"
	cacheDir := gc.getCacheDir(repoURL)

	if !filepath.IsAbs(cacheDir) {
		t.Error("Cache dir should be absolute path")
	}

	if filepath.Dir(cacheDir) != tmpDir {
		t.Errorf("Cache dir parent should be %s, got %s", tmpDir, filepath.Dir(cacheDir))
	}
}

func TestIsCached(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)

	repoURL := "https://github.com/user/test-repo.git"

	if gc.IsCached(repoURL) {
		t.Error("Repository should not be cached initially")
	}

	cacheDir := gc.getCacheDir(repoURL)
	gitDir := filepath.Join(cacheDir, ".git")

	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create test git dir: %v", err)
	}

	if !gc.IsCached(repoURL) {
		t.Error("Repository should be cached after creating .git directory")
	}
}

func TestIsCachedWithTTL(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 100*time.Millisecond)

	repoURL := "https://github.com/user/test-repo.git"
	cacheDir := gc.getCacheDir(repoURL)
	gitDir := filepath.Join(cacheDir, ".git")

	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create test git dir: %v", err)
	}

	if !gc.IsCached(repoURL) {
		t.Error("Repository should be cached immediately after creation")
	}

	time.Sleep(150 * time.Millisecond)

	if gc.IsCached(repoURL) {
		t.Error("Repository should not be cached after TTL expires")
	}
}

func TestCleanRepo(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)

	repoURL := "https://github.com/user/test-repo.git"
	cacheDir := gc.getCacheDir(repoURL)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("Failed to create test cache dir: %v", err)
	}

	testFile := filepath.Join(cacheDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := gc.CleanRepo(repoURL); err != nil {
		t.Errorf("CleanRepo failed: %v", err)
	}

	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("Cache directory should be removed after CleanRepo")
	}
}

func TestCleanStale(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 100*time.Millisecond)

	repo1URL := "https://github.com/user/repo1.git"
	repo2URL := "https://github.com/user/repo2.git"

	cache1Dir := gc.getCacheDir(repo1URL)
	cache2Dir := gc.getCacheDir(repo2URL)

	gitDir1 := filepath.Join(cache1Dir, ".git")
	gitDir2 := filepath.Join(cache2Dir, ".git")

	if err := os.MkdirAll(gitDir1, 0755); err != nil {
		t.Fatalf("Failed to create test git dir 1: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	if err := os.MkdirAll(gitDir2, 0755); err != nil {
		t.Fatalf("Failed to create test git dir 2: %v", err)
	}

	if err := gc.CleanStale(); err != nil {
		t.Errorf("CleanStale failed: %v", err)
	}

	if _, err := os.Stat(cache1Dir); !os.IsNotExist(err) {
		t.Error("Stale cache directory 1 should be removed")
	}

	if _, err := os.Stat(cache2Dir); os.IsNotExist(err) {
		t.Error("Fresh cache directory 2 should not be removed")
	}
}

func TestClean(t *testing.T) {
	tmpDir := t.TempDir()
	cacheBase := filepath.Join(tmpDir, "test-cache")
	gc := NewGitCacheWithOptions(cacheBase, 1*time.Hour)

	repoURL := "https://github.com/user/test-repo.git"
	cacheDir := gc.getCacheDir(repoURL)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("Failed to create test cache dir: %v", err)
	}

	if err := gc.Clean(); err != nil {
		t.Errorf("Clean failed: %v", err)
	}

	if _, err := os.Stat(cacheBase); !os.IsNotExist(err) {
		t.Error("Base cache directory should be removed after Clean")
	}
}

func TestGetCachedPath(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)

	repoURL := "https://github.com/user/test-repo.git"

	_, err := gc.GetCachedPath(repoURL)
	if err == nil {
		t.Error("GetCachedPath should return error for non-cached repo")
	}

	cacheDir := gc.getCacheDir(repoURL)
	gitDir := filepath.Join(cacheDir, ".git")

	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create test git dir: %v", err)
	}

	path, err := gc.GetCachedPath(repoURL)
	if err != nil {
		t.Errorf("GetCachedPath failed for cached repo: %v", err)
	}

	if path != cacheDir {
		t.Errorf("Expected cached path %s, got %s", cacheDir, path)
	}
}
