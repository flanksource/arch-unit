package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/shutdown"
	"github.com/flanksource/commons/logger"
	"log/slog"
)

// contextKey is used for storing values in context
type contextKey string

const loggerContextKey contextKey = "logger"

// WithLogger adds a logger to the context
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// Thread-local storage for task logger - simple approach for immediate fix
var currentTaskLogger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// SetCurrentTaskLogger sets the current task logger for git operations
func SetCurrentTaskLogger(logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}) {
	currentTaskLogger = logger
}

// getLoggerFromContext extracts logger from context, falls back to current task logger or global logger
func getLoggerFromContext(ctx context.Context) interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
} {
	// First try context
	if ctx != nil {
		if ctxLogger, ok := ctx.Value(loggerContextKey).(*slog.Logger); ok && ctxLogger != nil {
			return &contextLogger{ctxLogger}
		}
	}
	// Then try current task logger
	if currentTaskLogger != nil {
		return currentTaskLogger
	}
	// Fall back to global logger
	return &globalLogger{}
}

// contextLogger wraps slog.Logger to match our interface
type contextLogger struct {
	*slog.Logger
}

func (l *contextLogger) Debugf(format string, args ...interface{}) {
	l.Logger.Debug(fmt.Sprintf(format, args...))
}

func (l *contextLogger) Infof(format string, args ...interface{}) {
	l.Logger.Info(fmt.Sprintf(format, args...))
}

func (l *contextLogger) Warnf(format string, args ...interface{}) {
	l.Logger.Warn(fmt.Sprintf(format, args...))
}

func (l *contextLogger) Errorf(format string, args ...interface{}) {
	l.Logger.Error(fmt.Sprintf(format, args...))
}

// globalLogger wraps the global logger
type globalLogger struct{}

func (l *globalLogger) Debugf(format string, args ...interface{}) {
	logger.V(4).Infof(format, args...)
}

func (l *globalLogger) Infof(format string, args ...interface{}) {
	logger.Infof(format, args...)
}

func (l *globalLogger) Warnf(format string, args ...interface{}) {
	logger.Warnf(format, args...)
}

func (l *globalLogger) Errorf(format string, args ...interface{}) {
	logger.Errorf(format, args...)
}

// DefaultCloneManager implements CloneManager
type DefaultCloneManager struct {
	activeClones map[string]string // clone path -> repo path
	mu           sync.RWMutex
}

// NewCloneManager creates a new clone manager
func NewCloneManager() CloneManager {
	manager := &DefaultCloneManager{
		activeClones: make(map[string]string),
	}

	// Register cleanup hook
	shutdown.AddHookWithPriority("cleanup git clones", shutdown.PriorityWorkers, func() {
		manager.CleanupAll()
	})

	return manager
}

// CreateClone creates a new clone for the specified version with given depth
func (cm *DefaultCloneManager) CreateClone(ctx context.Context, repoPath, version, clonePath string, depth int) error {
	start := time.Now()
	
	// Extract repo name for logging
	repoName := cm.extractRepoName(repoPath)
	
	// Get logger from context (falls back to global logger)
	log := getLoggerFromContext(ctx)
	
	// Log start of operation
	log.Debugf("Creating clone: repo=%s, version=%s, depth=%d", repoName, version, depth)
	// Ensure the repository exists and is up to date
	if err := cm.ensureRepoFetched(repoPath); err != nil {
		return fmt.Errorf("failed to ensure repository is fetched: %w", err)
	}

	// Create temp directory if clonePath is not provided
	if clonePath == "" {
		tempDir, err := os.MkdirTemp("", "arch-unit-clone-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		clonePath = tempDir
	} else {
		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(clonePath), 0755); err != nil {
			return fmt.Errorf("failed to create clone directory: %w", err)
		}
	}

	// Remove existing clone if it exists
	if _, err := os.Stat(clonePath); err == nil {
		if err := os.RemoveAll(clonePath); err != nil {
			return fmt.Errorf("failed to remove existing clone: %w", err)
		}
	}

	// Prepare clone command
	var cmd *exec.Cmd
	if depth <= 0 {
		// Full clone
		log.Debugf("Performing full clone: %s@%s", repoName, version)
		cmd = exec.Command("git", "clone", "--branch", version, repoPath, clonePath)
	} else {
		// Shallow clone with specified depth
		log.Debugf("Performing shallow clone: %s@%s (depth=%d)", repoName, version, depth)
		cmd = exec.Command("git", "clone", "--depth", strconv.Itoa(depth), "--branch", version, repoPath, clonePath)
	}

	// Set up progress output filtering for V(4) and full output for V(9)
	cmd.Stdout = cm.getProgressWriter(ctx, 4)
	cmd.Stderr = cm.getProgressWriter(ctx, 9)
	
	err := cmd.Run()
	if err != nil {
		// Try without --branch if the version might be a tag or commit hash
		log.Debugf("Branch-specific clone failed, trying fallback approach for %s@%s", repoName, version)
		
		// First clone without branch/tag, then checkout
		if depth <= 0 {
			cmd = exec.Command("git", "clone", repoPath, clonePath)
		} else {
			// For shallow clones with commit hash, we need a different approach
			cmd = exec.Command("git", "clone", "--depth", strconv.Itoa(depth), repoPath, clonePath)
		}

		cmd.Stdout = cm.getProgressWriter(ctx, 4)
		cmd.Stderr = cm.getProgressWriter(ctx, 9)
		
		err = cmd.Run()
		if err != nil {
			os.RemoveAll(clonePath)
			log.Errorf("Failed to clone %s: %v", repoName, err)
			return fmt.Errorf("failed to clone repository %s: %w", repoName, err)
		}

		// Now checkout the specific version
		log.Debugf("Checking out version %s in %s", version, repoName)
		checkoutCmd := exec.Command("git", "checkout", version)
		checkoutCmd.Dir = clonePath
		checkoutCmd.Stdout = cm.getProgressWriter(ctx, 9)
		checkoutCmd.Stderr = cm.getProgressWriter(ctx, 9)
		
		err = checkoutCmd.Run()
		if err != nil {
			os.RemoveAll(clonePath)
			log.Errorf("Failed to checkout version %s in %s: %v", version, repoName, err)
			return fmt.Errorf("failed to checkout version %s: %w", version, err)
		}
	}

	// Track the clone for cleanup
	cm.mu.Lock()
	cm.activeClones[clonePath] = repoPath
	cm.mu.Unlock()

	duration := time.Since(start)
	log.Debugf("Clone completed in %v: %s@%s -> %s (depth: %d)", 
		duration, repoName, cm.formatVersion(version), clonePath, depth)

	return nil
}

// RemoveClone removes an existing clone
func (cm *DefaultCloneManager) RemoveClone(ctx context.Context, clonePath string) error {
	log := getLoggerFromContext(ctx)
	
	cm.mu.Lock()
	repoPath, exists := cm.activeClones[clonePath]
	if exists {
		delete(cm.activeClones, clonePath)
	}
	cm.mu.Unlock()

	if exists {
		repoName := cm.extractRepoName(repoPath)
		log.Debugf("Removing clone: %s -> %s", repoName, clonePath)
	}

	// Remove the directory
	if err := os.RemoveAll(clonePath); err != nil {
		log.Errorf("Failed to remove clone directory %s: %v", clonePath, err)
		return fmt.Errorf("failed to remove clone directory %s: %w", clonePath, err)
	}

	if exists {
		log.Debugf("Clone removed: %s", clonePath)
	}
	return nil
}

// ListClones lists all clones for a repository
func (cm *DefaultCloneManager) ListClones(ctx context.Context, repoPath string) ([]CloneInfo, error) {
	// This is a simplified implementation that looks at the active clones
	// In a more complete implementation, we could scan the filesystem
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var clones []CloneInfo
	for clonePath, sourceRepo := range cm.activeClones {
		if sourceRepo == repoPath {
			// Try to get info about the clone
			if info, err := os.Stat(clonePath); err == nil {
				// Try to determine version and depth from the clone path or git info
				version := "unknown"
				depth := 0

				// Parse version and depth from path if following our naming convention
				baseName := filepath.Base(clonePath)
				if strings.Contains(baseName, "-depth") {
					parts := strings.Split(baseName, "-depth")
					if len(parts) == 2 {
						version = parts[0]
						if d, err := strconv.Atoi(parts[1]); err == nil {
							depth = d
						}
					}
				}

				clones = append(clones, CloneInfo{
					Path:      clonePath,
					Version:   version,
					Depth:     depth,
					CreatedAt: info.ModTime(),
					LastUsed:  info.ModTime(),
				})
			}
		}
	}

	return clones, nil
}

// CleanupStaleClones removes clones that haven't been used recently
func (cm *DefaultCloneManager) CleanupStaleClones(ctx context.Context, repoPath string, maxAge time.Duration) error {
	log := getLoggerFromContext(ctx)
	repoName := cm.extractRepoName(repoPath)
	log.Debugf("Cleaning up stale clones for %s (maxAge: %v)", repoName, maxAge)
	
	clones, err := cm.ListClones(ctx, repoPath)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)
	cleanedCount := 0

	for _, clone := range clones {
		if clone.LastUsed.Before(cutoff) {
			if err := cm.RemoveClone(ctx, clone.Path); err != nil {
				log.Warnf("Failed to cleanup stale clone %s: %v", clone.Path, err)
			} else {
				cleanedCount++
			}
		}
	}

	if cleanedCount > 0 {
		log.Debugf("Cleaned up %d stale clones for %s", cleanedCount, repoName)
	}

	return nil
}

// CleanupAll removes all tracked clones
func (cm *DefaultCloneManager) CleanupAll() {
	cm.mu.RLock()
	clones := make(map[string]string)
	for path, repo := range cm.activeClones {
		clones[path] = repo
	}
	cm.mu.RUnlock()

	if len(clones) == 0 {
		return
	}

	// Use global logger for cleanup since no context available
	logger.V(4).Infof("Cleaning up %d active clones", len(clones))

	ctx := context.Background()
	for clonePath := range clones {
		if err := cm.RemoveClone(ctx, clonePath); err != nil {
			logger.Warnf("Failed to cleanup clone %s: %v", clonePath, err)
		}
	}

	logger.Infof("Cleaned up %d clones", len(clones))
}

// ensureRepoFetched ensures the repository is cloned and up to date
func (cm *DefaultCloneManager) ensureRepoFetched(repoPath string) error {
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

// Helper methods for logging operations

// extractRepoName extracts a readable repository name from a path or URL
func (cm *DefaultCloneManager) extractRepoName(repoPath string) string {
	// Handle URLs like https://github.com/org/repo
	if strings.Contains(repoPath, "/") {
		parts := strings.Split(strings.TrimSuffix(repoPath, ".git"), "/")
		if len(parts) >= 2 {
			// Take last two parts (org/repo)
			return strings.Join(parts[len(parts)-2:], "/")
		}
	}
	return filepath.Base(repoPath)
}

// formatVersion formats a version string for logging (truncate long hashes)
func (cm *DefaultCloneManager) formatVersion(version string) string {
	if len(version) > 40 && strings.Contains(version, "0123456789abcdef") {
		// Looks like a commit hash, truncate to 8 characters
		return version[:8]
	}
	return version
}

// getProgressWriter returns a writer that filters git progress output based on log level
func (cm *DefaultCloneManager) getProgressWriter(ctx context.Context, logLevel int) *progressWriter {
	return &progressWriter{
		logLevel: logLevel,
		logger:   getLoggerFromContext(ctx),
	}
}

// progressWriter filters git command output for cleaner logging
type progressWriter struct {
	logLevel int
	logger   interface {
		Debugf(format string, args ...interface{})
		Infof(format string, args ...interface{})
		Warnf(format string, args ...interface{})
		Errorf(format string, args ...interface{})
	}
}

func (pw *progressWriter) Write(p []byte) (n int, err error) {
	output := string(p)
	
	// Filter out common git progress messages at level 4
	if pw.logLevel == 4 {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var filteredLines []string
		
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			
			// Filter out progress messages
			if strings.Contains(line, "Compressing objects:") ||
			   strings.Contains(line, "Counting objects:") ||
			   strings.Contains(line, "Receiving objects:") ||
			   strings.Contains(line, "Resolving deltas:") {
				continue
			}
			
			filteredLines = append(filteredLines, line)
		}
		
		if len(filteredLines) > 0 {
			pw.logger.Debugf("Git: %s", strings.Join(filteredLines, " | "))
		}
	} else {
		// Show all output at higher verbosity levels
		pw.logger.Debugf("Git output: %s", strings.TrimSpace(output))
	}
	
	return len(p), nil
}