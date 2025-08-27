package ast

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/languages"
	"github.com/flanksource/clicky"
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
	Language string
	Tasks    []*TaskInfo
	mu       sync.Mutex
}

// TaskInfo stores task along with its original filename
type TaskInfo struct {
	Task     *clicky.Task
	FileName string
}

// FileResult represents the result of analyzing a file
type FileResult struct {
	Path   string
	Result *analysis.ASTResult
	Error  error
}

// AnalyzeDirectory analyzes all source files in a directory with parallelization
func (c *Coordinator) AnalyzeDirectory(parentTask *clicky.Task, dir string) ([]FileResult, error) {
	// Log initial status
	parentTask.SetStatus("Starting AST Analysis")
	
	// Discovery phase
	parentTask.SetStatus("Discovering files")
	files, err := c.discoverFiles(dir)
	if err != nil {
		parentTask.Errorf("Failed to discover files: %v", err)
		parentTask.Failed()
		return nil, err
	}
	parentTask.Infof("Found %d files", len(files))
	
	// Group files by language
	parentTask.SetStatus("Detecting languages")
	filesByLang := c.groupByLanguage(files)
	for lang, count := range filesByLang {
		parentTask.Infof("%s: %d files", lang, len(count))
	}
	
	// Filter files that need analysis
	parentTask.SetStatus("Checking cache")
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
	
	// Create channels for worker pool
	jobs := make(chan FileJob, len(filesToAnalyze))
	results := make(chan FileResult, len(filesToAnalyze))
	
	// Create language task groups - initialize for all possible languages
	langGroups := make(map[string]*LanguageTaskGroup)
	
	// Start workers
	var wg sync.WaitGroup
	parentTask.Infof("Starting %d workers", c.maxWorkers)
	
	for i := 0; i < c.maxWorkers; i++ {
		wg.Add(1)
		go c.worker(i, jobs, results, &wg)
	}
	
	// Create individual task for each file
	parentTask.SetStatus("Analyzing files")
	for _, file := range filesToAnalyze {
		lang := c.registry.DetectLanguage(file)
		if lang == nil {
			continue // Skip unsupported files
		}
		
		// Use only the filename, not the full path
		fileName := filepath.Base(file)
		// Create a simple task for this file
		fileTask := clicky.StartGlobalTask(fileName)
		
		// Add task to language group - create group if doesn't exist
		if _, exists := langGroups[lang.Name]; !exists {
			langGroups[lang.Name] = &LanguageTaskGroup{
				Language: lang.Name,
				Tasks:    []*TaskInfo{},
			}
		}
		
		group := langGroups[lang.Name]
		group.mu.Lock()
		group.Tasks = append(group.Tasks, &TaskInfo{
			Task:     fileTask,
			FileName: fileName,
		})
		group.mu.Unlock()
		
		jobs <- FileJob{
			Path:     file,
			Language: lang,
			Task:     fileTask,
		}
	}
	close(jobs)
	
	// Wait for completion
	wg.Wait()
	close(results)
	
	// Collect results
	var allResults []FileResult
	var successCount, errorCount int
	var errors []error
	
	for result := range results {
		if result.Error != nil {
			errorCount++
			errors = append(errors, result.Error)
		} else {
			successCount++
			allResults = append(allResults, result)
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
		c.displayLanguageGroupSummary(parentTask, langGroups)
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

// worker processes file analysis jobs
func (c *Coordinator) worker(id int, jobs <-chan FileJob, results chan<- FileResult, wg *sync.WaitGroup) {
	defer wg.Done()
	
	for job := range jobs {
		result := c.analyzeFile(job)
		results <- result
	}
}

// analyzeFile analyzes a single file
func (c *Coordinator) analyzeFile(job FileJob) FileResult {
	task := job.Task
	result := FileResult{Path: job.Path}
	
	// Check cache
	if !c.noCache && !c.shouldAnalyze(job.Path) {
		cached, err := c.getCachedAnalysis(job.Path)
		if err == nil && cached != nil {
			// Debug: ("Using cached analysis")
			// Don't overwrite the task name - keep showing the filename
			// task.SetStatus("Cached")
			task.Success()
			result.Result = cached
			return result
		}
	}
	
	// Check if analyzer is available
	if job.Language.Analyzer == nil {
		task.Warnf("No analyzer for %s (language: %+v)", job.Language.Name, job.Language)
		task.Warning()
		return result
	}
	
	// Read file
	// Don't overwrite the task name - keep showing the filename
	// task.SetStatus("Reading")
	content, err := os.ReadFile(job.Path)
	if err != nil {
		task.Errorf("Failed to read: %v", err)
		task.Failed()
		result.Error = err
		return result
	}
	
	// Analyze
	task.SetStatus("Analyzing")
	task.Debugf("Language: %s", job.Language.Name)
	
	// Call the analyzer through the interface
	analysisResult, err := job.Language.Analyzer.AnalyzeFile(task, job.Path, content)
	if err != nil {
		task.Errorf("Analysis failed: %v", err)
		task.Failed()
		result.Error = err
		return result
	}
	
	// Type assert the result
	astResult, ok := analysisResult.(*analysis.ASTResult)
	if !ok {
		task.Errorf("Invalid analysis result type")
		task.Failed()
		result.Error = fmt.Errorf("invalid analysis result type")
		return result
	}
	
	// Store in cache
	if !c.noCache {
		// Don't overwrite the task name - keep showing the filename
		// task.SetStatus("Caching")
		if err := c.storeResults(job.Path, astResult); err != nil {
			task.Warnf("Failed to cache: %v", err)
		}
	}
	
	task.Infof("✓ %d nodes, %d relationships", len(astResult.Nodes), len(astResult.Relationships))
	task.Success()
	
	result.Result = astResult
	return result
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
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || 
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
func (c *Coordinator) getCachedAnalysis(file string) (*analysis.ASTResult, error) {
	// Query cached nodes for the file
	nodes, err := c.cache.GetASTNodesByFile(file)
	if err != nil {
		return nil, err
	}
	
	// Convert to ASTResult
	result := &analysis.ASTResult{
		FilePath: file,
		Nodes:    nodes,
	}
	
	// TODO: Also retrieve relationships and libraries from cache
	
	return result, nil
}

// storeResults stores analysis results in the cache
func (c *Coordinator) storeResults(file string, result *analysis.ASTResult) error {
	// Use a single transaction for the entire operation to ensure atomicity
	// This prevents concurrent operations from interfering with each other
	return c.cache.StoreFileResults(file, result)
}

// displayLanguageGroupSummary displays a filtered summary of tasks grouped by language
func (c *Coordinator) displayLanguageGroupSummary(parentTask *clicky.Task, langGroups map[string]*LanguageTaskGroup) {
	for lang, group := range langGroups {
		group.mu.Lock()
		taskInfos := group.Tasks
		group.mu.Unlock()
		
		if len(taskInfos) == 0 {
			continue
		}
		
		// Separate successful and failed tasks
		var successfulTasks []*TaskInfo
		var failedTasks []*TaskInfo
		
		for _, taskInfo := range taskInfos {
			status := taskInfo.Task.Status()
			
			switch status {
			case clicky.StatusSuccess:
				successfulTasks = append(successfulTasks, taskInfo)
			case clicky.StatusFailed, clicky.StatusWarning:
				failedTasks = append(failedTasks, taskInfo)
			}
		}
		
		// Log language group summary
		parentTask.Infof("[%s] Analyzed %d files: %d successful, %d failed", 
			lang, len(taskInfos), len(successfulTasks), len(failedTasks))
		
		// Show up to 5 most recent successful tasks
		displayCount := 5
		if len(successfulTasks) < displayCount {
			displayCount = len(successfulTasks)
		}
		
		// Display most recent successful tasks (last 5)
		if displayCount > 0 {
			startIdx := len(successfulTasks) - displayCount
			if startIdx < 0 {
				startIdx = 0
			}
			for i := startIdx; i < len(successfulTasks); i++ {
				taskInfo := successfulTasks[i]
				parentTask.Debugf("  ✓ %s", taskInfo.FileName)
			}
		}
		
		// Display all failed tasks
		for _, taskInfo := range failedTasks {
			parentTask.Warnf("  ✗ %s", taskInfo.FileName)
		}
	}
}