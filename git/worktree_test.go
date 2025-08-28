package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorktreeNoInfiniteRecursion(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "worktree-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a mock source repository with worktrees directory
	srcRepo := filepath.Join(tempDir, "src-repo")
	srcWorktrees := filepath.Join(srcRepo, "worktrees")
	srcWorktreeVersion := filepath.Join(srcWorktrees, "v1.0.0")

	err = os.MkdirAll(srcWorktreeVersion, 0755)
	if err != nil {
		t.Fatalf("Failed to create source worktree: %v", err)
	}

	// Create some test files in source repo
	testFile := filepath.Join(srcRepo, "README.md")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create test file in existing worktree
	worktreeFile := filepath.Join(srcWorktreeVersion, "worktree-file.txt")
	err = os.WriteFile(worktreeFile, []byte("worktree content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create worktree file: %v", err)
	}

	// Create destination directory
	destRepo := filepath.Join(tempDir, "dest-repo")

	// Copy repository using the worktree manager
	wm := NewWorktreeManager()
	dm := wm.(*DefaultWorktreeManager)

	err = dm.copyRepository(srcRepo, destRepo)
	if err != nil {
		t.Fatalf("copyRepository failed: %v", err)
	}

	// Verify that README.md was copied
	destReadme := filepath.Join(destRepo, "README.md")
	if _, err := os.Stat(destReadme); os.IsNotExist(err) {
		t.Error("README.md was not copied to destination")
	}

	// Verify that worktrees directory was NOT copied (this prevents infinite recursion)
	destWorktrees := filepath.Join(destRepo, "worktrees")
	if _, err := os.Stat(destWorktrees); !os.IsNotExist(err) {
		t.Error("worktrees directory was copied when it should have been skipped")
	}

	// Verify no infinite nesting by checking there are no deep nested directories
	err = filepath.Walk(destRepo, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(destRepo, path)
		depth := len(filepath.SplitList(relPath))

		// If we have more than 10 levels deep, something is wrong
		if depth > 10 {
			t.Errorf("Found suspiciously deep path: %s (depth: %d)", path, depth)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk destination directory: %v", err)
	}
}
