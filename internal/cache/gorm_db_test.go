package cache

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestWriteAccess(t *testing.T) {
	// Test successful write access case
	t.Run("successful write access", func(t *testing.T) {
		tempDir := t.TempDir()
		defer ResetGormDB()

		// Create database in temp directory
		db, err := NewGormDBWithPath(tempDir)
		require.NoError(t, err)

		// Set the global instance
		ResetGormDB()
		gormInstance = db
		gormOnce = sync.Once{} // Reset to prevent re-initialization

		err = TestWriteAccess()
		assert.NoError(t, err)
	})
}

func TestFormatWriteAccessError(t *testing.T) {
	tests := []struct {
		name        string
		inputErr    error
		wantContains []string
	}{
		{
			name:     "permission denied error",
			inputErr: errors.New("permission denied"),
			wantContains: []string{
				"insufficient file permissions",
				"chmod 755",
				"chmod 644",
			},
		},
		{
			name:     "database locked error",
			inputErr: errors.New("database is locked"),
			wantContains: []string{
				"database is locked",
				"another arch-unit process",
			},
		},
		{
			name:     "disk full error",
			inputErr: errors.New("no space left on device"),
			wantContains: []string{
				"insufficient disk space",
				"free up space",
			},
		},
		{
			name:     "read-only error",
			inputErr: errors.New("attempt to write a readonly database"),
			wantContains: []string{
				"read-only",
				"mount options",
			},
		},
		{
			name:     "generic database error",
			inputErr: errors.New("unknown database error"),
			wantContains: []string{
				"database write access test failed",
				"ensure ~/.cache/arch-unit/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatWriteAccessError(tt.inputErr)

			assert.Error(t, result)
			for _, wantStr := range tt.wantContains {
				assert.Contains(t, strings.ToLower(result.Error()), strings.ToLower(wantStr))
			}

			// Ensure original error is wrapped
			assert.ErrorIs(t, result, tt.inputErr)
		})
	}
}

func TestTestWriteAccessWithNonExistentDB(t *testing.T) {
	// Reset any existing GORM instance
	ResetGormDB()
	defer ResetGormDB()

	// Create a directory that will be removed to simulate missing cache
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "non-existent")

	// Create database in non-existent directory - GORM will create it
	db, err := NewGormDBWithPath(nonExistentDir)
	require.NoError(t, err)

	// Set the global instance
	gormInstance = db
	gormOnce = sync.Once{} // Reset to prevent re-initialization

	err = TestWriteAccess()

	// Should succeed because GORM creates missing directories
	assert.NoError(t, err)

	// Verify the directory was created
	_, err = os.Stat(nonExistentDir)
	assert.NoError(t, err)
}