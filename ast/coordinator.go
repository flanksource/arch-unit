package ast

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/languages"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
	flanksourceContext "github.com/flanksource/commons/context"
)

// Coordinator manages AST analysis with caching and parallelization
type Coordinator struct {
	cache      *cache.ASTCache
	registry   *languages.Registry
	noCache    bool
	cacheTTL   time.Duration
	maxWorkers int
	workDir    string
}

// CoordinatorOptions configures the coordinator
type CoordinatorOptions struct {
	NoCache    bool
	CacheTTL   time.Duration
	MaxWorkers int
	Languages  []string // Filter to specific languages
}

// NewCoordinator creates a new AST analysis coordinator
func NewCoordinator(cache *cache.ASTCache, workDir string, opts CoordinatorOptions) *Coordinator {
	maxWorkers := opts.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU()
	}

	return &Coordinator{
		cache:      cache,
		registry:   languages.GetRegistry(),
		noCache:    opts.NoCache,
		cacheTTL:   opts.CacheTTL,
		maxWorkers: maxWorkers,
		workDir:    workDir,
	}
}

// FileJob represents a file analysis job
type FileJob struct {
	Path     string
	Language *languages.LanguageConfig
	Task     *clicky.Task
}

// LanguageTaskGroup tracks tasks for a specific language
type LanguageTaskGroup struct {
	task.TypedGroup[FileResult]
	Language string
}

// TaskInfo stores task along with its original filename
type TaskInfo struct {
	Task     *clicky.Task
	FileName string
}

// FileResult represents the result of analyzing a file
type FileResult struct {
	Path   string
	Result *types.ASTResult
	Error  error
}

// AnalyzeDirectory analyzes all source files in a directory with parallelization
func (c *Coordinator) AnalyzeDirectory(parentTask *clicky.Task, dir string) ([]FileResult, error) {
	// Log initial status
	parentTask.SetName("Starting AST Analysis")

	// Discovery phase
	parentTask.SetName("Discovering files")
	files, err := c.discoverFiles(dir)
	if err != nil {
		parentTask.Errorf("Failed to discover files: %v", err)
		parentTask.Failed()
		return nil, err
	}
	parentTask.SetName(fmt.Sprintf("Found %d files", len(files)))

	// Group files by language
	parentTask.SetName("Detecting languages")
	filesByLang := c.groupByLanguage(files)
	for lang, count := range filesByLang {
		parentTask.Infof("%s: %d files", lang, len(count))
	}

	// Filter files that need analysis
	parentTask.SetName("Checking cache")
	filesToAnalyze := c.filterFilesNeedingAnalysis(files)
	cachedCount := len(files) - len(filesToAnalyze)
	parentTask.Infof("%d files need analysis, %d cached", len(filesToAnalyze), cachedCount)

	// If no files need analysis, still return cached results
	if len(filesToAnalyze) == 0 {
		parentTask.Infof("All files are cached and up to date")
		// Return cached results for all files
		var allResults []FileResult
		for _, file := range files {
			cached, err := c.getCachedAnalysis(file)
			if err == nil && cached != nil {
				allResults = append(allResults, FileResult{
					Path:   file,
					Result: cached,
				})
			}
		}
		parentTask.Success()
		return allResults, nil
	}

	// Create language task groups - initialize for all possible languages
	langGroups := make(map[string]*LanguageTaskGroup)

	// Create individual task for each file
	parentTask.SetName("Analyzing files")
	for _, file := range filesToAnalyze {
		lang := c.registry.DetectLanguage(file)
		if lang == nil {
			continue // Skip unsupported files
		}

		// Add task to language group - create group if doesn't exist
		if _, exists := langGroups[lang.Name]; !exists {
			langGroups[lang.Name] = &LanguageTaskGroup{
				Language:   lang.Name,
				TypedGroup: task.StartGroup[FileResult](lang.Name),
			}
		}

		// Use only the filename for display, but we need to pass the full path
		fileName := filepath.Base(file)

		group := langGroups[lang.Name]
		// Create a closure that captures the full file path
		filePath := file
		group.Add(fileName, func(ctx flanksourceContext.Context, task *task.Task) (FileResult, error) {
			return c.analyzeFileWithPath(ctx, task, filePath)
		})

	}

	var allResults []FileResult
	var successCount, errorCount int
	var errors []error
	for _, group := range langGroups {
		if err := group.WaitFor().Error; err != nil {
			return nil, err

		}
		results, _ := group.GetResults()
		for _, result := range results {
			allResults = append(allResults, result)
			if result.Error != nil {
				errorCount++
				errors = append(errors, result.Error)
			} else {
				successCount++
				allResults = append(allResults, result)
			}
		}
	}

	// Also include cached results for files not analyzed
	for _, file := range files {
		needsAnalysis := false
		for _, analyzed := range filesToAnalyze {
			if file == analyzed {
				needsAnalysis = true
				break
			}
		}
		if !needsAnalysis {
			cached, err := c.getCachedAnalysis(file)
			if err == nil && cached != nil {
				allResults = append(allResults, FileResult{
					Path:   file,
					Result: cached,
				})
			}
		}
	}

	// Display filtered task summary by language (only if we have language groups from analyzed files)
	if len(langGroups) > 0 {
		// Add a small delay to ensure all tasks have completed their status updates
		time.Sleep(10 * time.Millisecond)
	}

	// Report final status
	if errorCount > 0 {
		parentTask.Warnf("Completed: %d succeeded, %d failed", successCount, errorCount)
		parentTask.Warning()
	} else {
		parentTask.Infof("Successfully analyzed %d files", successCount)
		parentTask.Success()
	}

	return allResults, nil
}

// analyzeFileWithPath analyzes a single file with the specified path
func (c *Coordinator) analyzeFileWithPath(ctx flanksourceContext.Context, task *task.Task, filePath string) (FileResult, error) {
	result := FileResult{Path: filePath}

	// Check cache
	if !c.noCache && !c.shouldAnalyze(result.Path) {
		cached, err := c.getCachedAnalysis(result.Path)
		if err == nil && cached != nil {
			// Debug: ("Using cached analysis")
			// Don't overwrite the task name - keep showing the filename
			// task.SetStatus("Cached")
			task.Success()
			result.Result = cached
			return result, nil
		}
	}

	// Detect language from file extension
	lang := c.registry.DetectLanguage(result.Path)
	if lang == nil || lang.Analyzer == nil {
		task.Warnf("No analyzer for file %s", result.Path)
		task.Warning()
		return result, nil
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		task.Errorf("Failed to read: %v", err)
		task.Failed()
		result.Error = err
		return result, nil
	}

	analysisResult, err := lang.Analyzer.AnalyzeFile(task, result.Path, content)
	if err != nil {
		_, _ = task.FailedWithError(err)
		return result, nil
	}

	// Type assert the result
	astResult, ok := analysisResult.(*types.ASTResult)
	if !ok {
		task.Errorf("Invalid analysis result type")
		task.Failed()
		result.Error = fmt.Errorf("invalid analysis result type")
		return result, result.Error
	}

	// Check for nil result
	if astResult == nil {
		task.Warnf("No AST data extracted from %s", result.Path)
		return result, nil
	}

	// Store in cache
	if !c.noCache {
		// Don't overwrite the task name - keep showing the filename
		// task.SetStatus("Caching")
		if err := c.storeResults(result.Path, astResult); err != nil {
			task.Warnf("Failed to cache: %v", err)
		}
	}

	task.Infof("✓ %d nodes, %d relationships", len(astResult.Nodes), len(astResult.Relationships))
	task.Success()

	result.Result = astResult
	return result, nil
}

// shouldAnalyze determines if a file needs analysis
func (c *Coordinator) shouldAnalyze(file string) bool {
	if c.noCache {
		return true
	}

	// Check if file needs reanalysis based on modification time
	needsAnalysis, err := c.cache.NeedsReanalysis(file)
	if err != nil {
		return true // Analyze if we can't determine
	}

	if !needsAnalysis {
		// Check TTL if configured
		if c.cacheTTL > 0 {
			// Get file info to check last modified time
			fileInfo, err := os.Stat(file)
			if err != nil {
				return true
			}

			// For now, use modification time as a proxy for cache age
			if time.Since(fileInfo.ModTime()) > c.cacheTTL {
				return true // Consider cache expired
			}
		}

		return false // Cache is valid
	}

	return true
}

// discoverFiles finds all source files in the directory
func (c *Coordinator) discoverFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip common directories
		if info.IsDir() {
			//FIXME support .gitignore
			name := info.Name()
			// Skip hidden directories (starting with .) except root
			if name != "." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			// Skip common directories
			if name == "vendor" || name == "node_modules" ||
				name == "__pycache__" || name == ".venv" || name == "venv" ||
				name == "target" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file has a supported extension
		if c.registry.DetectLanguage(path) != nil {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// groupByLanguage groups files by their detected language
func (c *Coordinator) groupByLanguage(files []string) map[string][]string {
	groups := make(map[string][]string)

	for _, file := range files {
		lang := c.registry.DetectLanguage(file)
		if lang != nil {
			groups[lang.Name] = append(groups[lang.Name], file)
		}
	}

	return groups
}

// filterFilesNeedingAnalysis filters out files that don't need analysis
func (c *Coordinator) filterFilesNeedingAnalysis(files []string) []string {
	if c.noCache {
		return files // All files need analysis
	}

	var needAnalysis []string

	for _, file := range files {
		if c.shouldAnalyze(file) {
			needAnalysis = append(needAnalysis, file)
		}
	}

	return needAnalysis
}

// getCachedAnalysis retrieves cached analysis for a file
func (c *Coordinator) getCachedAnalysis(file string) (*types.ASTResult, error) {
	// Query cached nodes for the file
	nodes, err := c.cache.GetASTNodesByFile(file)
	if err != nil {
		return nil, err
	}

	// Convert to ASTResult
	result := &types.ASTResult{
		FilePath: file,
		Nodes:    nodes,
	}

	// TODO: Also retrieve relationships and libraries from cache

	return result, nil
}

// storeResults stores analysis results in the cache
func (c *Coordinator) storeResults(file string, result *types.ASTResult) error {
	// Use a single transaction for the entire operation to ensure atomicity
	// This prevents concurrent operations from interfering with each other
	return c.cache.StoreFileResults(file, result)
}
