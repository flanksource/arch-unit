package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewGitIntegration(t *testing.T) {
	gi, err := NewGitIntegration()
	if err != nil {
		t.Fatalf("Failed to create GitIntegration: %v", err)
	}
	
	if gi.cache == nil {
		t.Error("GitIntegration cache should not be nil")
	}
}

func TestIsRemoteRepo(t *testing.T) {
	gi := &GitIntegration{}
	
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"HTTPS URL", "https://github.com/user/repo.git", true},
		{"HTTP URL", "http://github.com/user/repo.git", true},
		{"SSH URL", "git@github.com:user/repo.git", true},
		{"SSH Protocol", "ssh://git@github.com/user/repo.git", true},
		{"GitHub Path", "github.com/user/repo", true},
		{"GitLab Path", "gitlab.com/user/repo", true},
		{"BitBucket Path", "bitbucket.org/user/repo", true},
		{"Local Path", "/home/user/projects/repo", false},
		{"Relative Path", "./repo", false},
		{"Relative Parent", "../repo", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gi.isRemoteRepo(tt.path)
			if result != tt.expected {
				t.Errorf("isRemoteRepo(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestPrepareRepository_LocalPath(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	localPath := "/home/user/local/repo"
	result, err := gi.PrepareRepository(localPath)
	
	if err != nil {
		t.Errorf("PrepareRepository failed for local path: %v", err)
	}
	
	if result != localPath {
		t.Errorf("Expected %s, got %s", localPath, result)
	}
}

func TestPrepareRepositoryWithRef_LocalPath(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	localPath := "/home/user/local/repo"
	result, err := gi.PrepareRepositoryWithRef(localPath, "main")
	
	if err != nil {
		t.Errorf("PrepareRepositoryWithRef failed for local path: %v", err)
	}
	
	if result != localPath {
		t.Errorf("Expected %s, got %s", localPath, result)
	}
}

func TestGetCachedPath_LocalPath(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	localPath := "/home/user/local/repo"
	result, err := gi.GetCachedPath(localPath)
	
	if err != nil {
		t.Errorf("GetCachedPath failed for local path: %v", err)
	}
	
	if result != localPath {
		t.Errorf("Expected %s, got %s", localPath, result)
	}
}

func TestIsCached_LocalPath(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	localPath := "/home/user/local/repo"
	
	if !gi.IsCached(localPath) {
		t.Error("Local paths should always be considered cached")
	}
}

func TestIsCached_RemoteRepo(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	remoteURL := "https://github.com/user/repo.git"
	
	if gi.IsCached(remoteURL) {
		t.Error("Remote repo should not be cached initially")
	}
}

func TestCleanRepoCache_LocalPath(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	localPath := "/home/user/local/repo"
	err := gi.CleanRepoCache(localPath)
	
	if err != nil {
		t.Errorf("CleanRepoCache should not fail for local paths: %v", err)
	}
}

func TestPrepareRepository_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	tests := []string{
		"./local/repo",
		"../parent/repo",
		"relative/path/to/repo",
	}
	
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			result, err := gi.PrepareRepository(path)
			if err != nil {
				t.Errorf("PrepareRepository failed for %s: %v", path, err)
			}
			if result != path {
				t.Errorf("Expected %s, got %s", path, result)
			}
		})
	}
}

func TestGetCachedPath_RemoteRepo_NotCached(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	remoteURL := "https://github.com/user/repo.git"
	_, err := gi.GetCachedPath(remoteURL)
	
	if err == nil {
		t.Error("GetCachedPath should return error for non-cached remote repo")
	}
}

func TestCleanCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "test-cache")
	gc := NewGitCacheWithOptions(cacheDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	err := gi.CleanCache()
	if err != nil {
		t.Errorf("CleanCache failed: %v", err)
	}
}

func TestCleanStaleCache(t *testing.T) {
	tmpDir := t.TempDir()
	gc := NewGitCacheWithOptions(tmpDir, 1*time.Hour)
	gi := NewGitIntegrationWithCache(gc)
	
	err := gi.CleanStaleCache()
	if err != nil {
		t.Errorf("CleanStaleCache failed: %v", err)
	}
}