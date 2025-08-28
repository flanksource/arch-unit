package ast

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	flanksourceContext "github.com/flanksource/commons/context"
)

// Analyzer provides AST analysis functionality
type Analyzer struct {
	cache   *cache.ASTCache
	workDir string
}

// NewAnalyzer creates a new AST analyzer
func NewAnalyzer(cache *cache.ASTCache, workDir string) *Analyzer {
	return &Analyzer{
		cache:   cache,
		workDir: workDir,
	}
}

// fileInfo holds file path and language information
type fileInfo struct {
	path     string
	language string
}

// AnalyzeFiles analyzes all source files in the working directory
func (a *Analyzer) AnalyzeFiles() error {
	startTime := time.Now()

	// Create a context for the entire analysis
	ctx := flanksourceContext.NewContext(context.Background())
	ctx.Infof("🔍 Starting AST analysis in %s", a.workDir)

	// Find all source files
	var sourceFiles []fileInfo
	err := filepath.Walk(a.workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip vendor and .git directories
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.git/") ||
			strings.Contains(path, "/node_modules/") || strings.Contains(path, "/__pycache__/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			// Detect language based on file extension
			var lang string
			switch {
			case strings.HasSuffix(path, ".go"):
				lang = "go"
			case strings.HasSuffix(path, ".py"):
				lang = "python"
			case strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") ||
				strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".cjs"):
				lang = "javascript"
			case strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx"):
				lang = "typescript"
			case strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".markdown") ||
				strings.HasSuffix(path, ".mdx"):
				lang = "markdown"
			default:
				return nil // Skip unsupported files
			}

			sourceFiles = append(sourceFiles, fileInfo{path: path, language: lang})
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to find source files: %w", err)
	}

	if len(sourceFiles) == 0 {
		ctx.Infof("No supported source files found in %s", a.workDir)
		return nil
	}

	// Count files by language
	langCounts := make(map[string]int)
	for _, file := range sourceFiles {
		langCounts[file.language]++
	}

	ctx.Infof("Found %d source files:", len(sourceFiles))
	for lang, count := range langCounts {
		ctx.Infof("  %s: %d files", lang, count)
	}

	// Initialize library resolver
	libResolver := analysis.NewLibraryResolver(a.cache)
	if err := libResolver.StoreLibraryNodes(); err != nil {
		ctx.Warnf("Failed to store library nodes: %v", err)
	}

	// Create extractors for each language
	goExtractor := analysis.NewGoASTExtractor(a.cache)
	pythonExtractor := analysis.NewPythonASTExtractor(a.cache)
	jsExtractor := analysis.NewJavaScriptASTExtractor(a.cache)
	tsExtractor := analysis.NewTypeScriptASTExtractor(a.cache)
	mdExtractor := analysis.NewMarkdownASTExtractor(a.cache)

	ctx.Infof("📊 Analyzing %d source files...", len(sourceFiles))

	// Track statistics
	processedCount := 0
	errorCount := 0
	cachedCount := 0

	// Process files
	for i, file := range sourceFiles {
		if i > 0 && i%10 == 0 {
			ctx.Infof("⏳ Progress: %d/%d files (%.1f%%), %d cached, %d errors",
				i, len(sourceFiles), float64(i)/float64(len(sourceFiles))*100, cachedCount, errorCount)
		}

		// Check if already cached
		relPath, _ := filepath.Rel(a.workDir, file.path)
		needsAnalysis, err := a.cache.NeedsReanalysis(file.path)
		if err == nil && !needsAnalysis {
			cachedCount++
			ctx.Debugf("✓ Using cached AST for %s", relPath)
			continue
		}

		ctx.Debugf("🔨 Extracting AST from %s (%s)", relPath, file.language)
		switch file.language {
		case "go":
			err = goExtractor.ExtractFile(ctx, file.path)
		case "python":
			err = pythonExtractor.ExtractFile(ctx, file.path)
		case "javascript":
			err = jsExtractor.ExtractFile(ctx, file.path)
		case "typescript":
			err = tsExtractor.ExtractFile(ctx, file.path)
		case "markdown":
			err = mdExtractor.ExtractFile(ctx, file.path)
		}

		if err != nil {
			errorCount++
			ctx.Warnf("❌ Failed to extract AST from %s: %v", relPath, err)
			continue
		}
		processedCount++
	}

	elapsed := time.Since(startTime)
	ctx.Infof("✅ AST analysis completed in %.2fs", elapsed.Seconds())
	ctx.Infof("📈 Processed: %d new, %d cached, %d errors (total: %d files)",
		processedCount, cachedCount, errorCount, len(sourceFiles))
	return nil
}

// AnalyzeFilesWithFilter analyzes source files matching include/exclude patterns
func (a *Analyzer) AnalyzeFilesWithFilter(includePatterns, excludePatterns []string) error {
	// Create a context for the entire analysis
	ctx := flanksourceContext.NewContext(context.Background())
	ctx.Infof("Starting AST analysis in %s", a.workDir)

	// Find all source files
	var sourceFiles []fileInfo
	err := filepath.Walk(a.workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip vendor and .git directories
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.git/") ||
			strings.Contains(path, "/node_modules/") || strings.Contains(path, "/__pycache__/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			// Check if file should be filtered
			if !shouldIncludeFile(path, a.workDir, includePatterns, excludePatterns) {
				return nil
			}

			// Detect language based on file extension
			var lang string
			switch {
			case strings.HasSuffix(path, ".go"):
				lang = "go"
			case strings.HasSuffix(path, ".py"):
				lang = "python"
			case strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") ||
				strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".cjs"):
				lang = "javascript"
			case strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx"):
				lang = "typescript"
			case strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".markdown") ||
				strings.HasSuffix(path, ".mdx"):
				lang = "markdown"
			default:
				return nil // Skip unsupported files
			}

			sourceFiles = append(sourceFiles, fileInfo{path: path, language: lang})
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to find source files: %w", err)
	}

	if len(sourceFiles) == 0 {
		ctx.Infof("No supported source files found in %s", a.workDir)
		if len(includePatterns) > 0 {
			ctx.Infof("Include patterns: %v", includePatterns)
		}
		if len(excludePatterns) > 0 {
			ctx.Infof("Exclude patterns: %v", excludePatterns)
		}
		return nil
	}

	// Count files by language
	langCounts := make(map[string]int)
	for _, file := range sourceFiles {
		langCounts[file.language]++
	}

	ctx.Infof("Found %d source files:", len(sourceFiles))
	for lang, count := range langCounts {
		ctx.Infof("  %s: %d files", lang, count)
	}
	if len(includePatterns) > 0 {
		ctx.Infof("Include patterns: %v", includePatterns)
	}
	if len(excludePatterns) > 0 {
		ctx.Infof("Exclude patterns: %v", excludePatterns)
	}

	return a.processSourceFiles(ctx, sourceFiles)
}

// shouldIncludeFile determines if a file should be included based on include/exclude patterns
func shouldIncludeFile(filePath, workDir string, includePatterns, excludePatterns []string) bool {
	// Convert to relative path for pattern matching
	relPath, err := filepath.Rel(workDir, filePath)
	if err != nil {
		relPath = filePath
	}

	// If include patterns are specified, file must match at least one
	if len(includePatterns) > 0 {
		matched := false
		for _, pattern := range includePatterns {
			if match, err := doublestar.Match(pattern, relPath); err == nil && match {
				matched = true
				break
			}
			// Also try matching against the basename
			if match, err := doublestar.Match(pattern, filepath.Base(filePath)); err == nil && match {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// If exclude patterns are specified, file must not match any
	for _, pattern := range excludePatterns {
		if match, err := doublestar.Match(pattern, relPath); err == nil && match {
			return false
		}
		// Also try matching against the basename
		if match, err := doublestar.Match(pattern, filepath.Base(filePath)); err == nil && match {
			return false
		}
	}

	return true
}

// processSourceFiles processes a list of source files
func (a *Analyzer) processSourceFiles(ctx flanksourceContext.Context, sourceFiles []fileInfo) error {
	ctx.Infof("Analyzing %d source files...", len(sourceFiles))

	// Progress tracking
	totalFiles := len(sourceFiles)
	processedFiles := 0
	ctx.Infof("Progress: %d/%d files", processedFiles, totalFiles)

	for _, file := range sourceFiles {
		ctx.Debugf("Processing %s (%s)", file.path, file.language)

		// Check if file needs reanalysis
		needsAnalysis, err := a.cache.NeedsReanalysis(file.path)
		if err != nil {
			ctx.Warnf("Failed to check if %s needs reanalysis: %v", file.path, err)
			needsAnalysis = true // Default to analyzing if unsure
		}

		if !needsAnalysis {
			ctx.Debugf("Skipping %s (cached)", file.path)
			processedFiles++
			continue
		}

		// Extract AST based on file language
		switch file.language {
		case "go":
			goExtractor := analysis.NewGoASTExtractor(a.cache)
			err = goExtractor.ExtractFile(ctx, file.path)
		case "python":
			pythonExtractor := analysis.NewPythonASTExtractor(a.cache)
			err = pythonExtractor.ExtractFile(ctx, file.path)
		case "javascript":
			jsExtractor := analysis.NewJavaScriptASTExtractor(a.cache)
			err = jsExtractor.ExtractFile(ctx, file.path)
		case "typescript":
			tsExtractor := analysis.NewTypeScriptASTExtractor(a.cache)
			err = tsExtractor.ExtractFile(ctx, file.path)
		case "markdown":
			mdExtractor := analysis.NewMarkdownASTExtractor(a.cache)
			err = mdExtractor.ExtractFile(ctx, file.path)
		default:
			ctx.Warnf("Unsupported language for %s: %s", file.path, file.language)
			continue
		}
		processedFiles++

		if processedFiles%10 == 0 || processedFiles == totalFiles {
			ctx.Infof("Progress: %d/%d files", processedFiles, totalFiles)
		}

		if err != nil {
			ctx.Warnf("Failed to extract AST from %s: %v", file.path, err)
			continue
		}
	}

	ctx.Infof("AST analysis completed")
	return nil
}

// RebuildCache rebuilds the entire AST cache
func (a *Analyzer) RebuildCache() error {
	ctx := flanksourceContext.NewContext(context.Background())
	ctx.Infof("Rebuilding AST cache...")

	// Clear existing cache
	if err := a.cache.ClearCache(); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	// Re-analyze all files
	if err := a.AnalyzeFiles(); err != nil {
		return fmt.Errorf("failed to analyze files: %w", err)
	}

	ctx.Infof("AST cache rebuilt successfully")
	return nil
}

// GetCacheStats returns cache statistics
func (a *Analyzer) GetCacheStats() (*CacheStats, error) {
	// Get total nodes count
	query := "SELECT COUNT(*) FROM ast_nodes"
	var totalNodes int
	if err := a.cache.QueryRow(query).Scan(&totalNodes); err != nil {
		return nil, fmt.Errorf("failed to get node count: %w", err)
	}

	// Get cached files count
	query = "SELECT COUNT(DISTINCT file_path) FROM ast_nodes"
	var cachedFiles int
	if err := a.cache.QueryRow(query).Scan(&cachedFiles); err != nil {
		return nil, fmt.Errorf("failed to get file count: %w", err)
	}

	// Get last update time
	query = "SELECT MAX(last_modified) FROM ast_nodes"
	var lastModifiedStr sql.NullString
	if err := a.cache.QueryRow(query).Scan(&lastModifiedStr); err != nil {
		// Ignore error if no rows
		lastModifiedStr.Valid = false
	}

	lastUpdatedStr := "Never"
	if lastModifiedStr.Valid && lastModifiedStr.String != "" {
		// The timestamp is stored in Go's time.String() format, so we need to strip monotonic time
		// and parse it properly. First, let's try to strip the monotonic time part
		timestampStr := lastModifiedStr.String
		if mIdx := strings.Index(timestampStr, " m=+"); mIdx != -1 {
			timestampStr = timestampStr[:mIdx]
		}

		// Try multiple possible timestamp formats from SQLite/Go
		formats := []string{
			"2006-01-02 15:04:05.999999999 -0700 MST", // Go format without monotonic
			"2006-01-02 15:04:05.999999 -0700 MST",    // Go format with less precision
			"2006-01-02 15:04:05 -0700 MST",           // Go format without microseconds
			"2006-01-02 15:04:05",                     // Standard SQL format
			"2006-01-02 15:04",                        // SQL format without seconds
			"2006-01-02T15:04:05Z",                    // ISO format
		}

		for _, format := range formats {
			if lastModified, err := time.Parse(format, timestampStr); err == nil {
				lastUpdatedStr = lastModified.Format("2006-01-02 15:04:05")
				break
			}
		}
	}

	// Count total files in directory (for comparison)
	totalFiles := 0
	filepath.Walk(a.workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Count only supported file types
		if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".py") ||
			strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".ts") ||
			strings.HasSuffix(path, ".md") {
			totalFiles++
		}
		return nil
	})

	return &CacheStats{
		TotalFiles:  totalFiles,
		CachedFiles: cachedFiles,
		TotalNodes:  totalNodes,
		LastUpdated: lastUpdatedStr,
	}, nil
}

// FilterByComplexity filters nodes by complexity threshold
func FilterByComplexity(nodes []*models.ASTNode, threshold int) []*models.ASTNode {
	filtered := make([]*models.ASTNode, 0)
	for _, node := range nodes {
		if node.CyclomaticComplexity >= threshold {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// FilterByNodeType filters nodes by type
func FilterByNodeType(nodes []*models.ASTNode, nodeType string) []*models.ASTNode {
	filtered := make([]*models.ASTNode, 0)
	for _, node := range nodes {
		if string(node.NodeType) == nodeType {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// GetCache returns the underlying AST cache
func (a *Analyzer) GetCache() *cache.ASTCache {
	return a.cache
}

// GetWorkingDirectory returns the working directory
func (a *Analyzer) GetWorkingDirectory() string {
	return a.workDir
}

// QueryPatternWithFilter queries AST nodes matching a pattern and file filters
func (a *Analyzer) QueryPatternWithFilter(pattern string, includePatterns, excludePatterns []string) ([]*models.ASTNode, error) {
	// First get all nodes matching the pattern
	nodes, err := a.QueryPattern(pattern)
	if err != nil {
		return nil, err
	}

	// If no file filtering is needed, return all nodes
	if len(includePatterns) == 0 && len(excludePatterns) == 0 {
		return nodes, nil
	}

	// Filter nodes based on file patterns
	var filteredNodes []*models.ASTNode
	for _, node := range nodes {
		if shouldIncludeFile(node.FilePath, a.workDir, includePatterns, excludePatterns) {
			filteredNodes = append(filteredNodes, node)
		}
	}

	return filteredNodes, nil
}

// GetAllRelationships retrieves all AST relationships
func (a *Analyzer) GetAllRelationships() ([]*models.ASTRelationship, error) {
	query := `
		SELECT id, from_ast_id, to_ast_id, line_no, relationship_type, text
		FROM ast_relationships
		ORDER BY from_ast_id, line_no
	`

	rows, err := a.cache.QueryRaw(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query relationships: %w", err)
	}
	defer rows.Close()

	var relationships []*models.ASTRelationship
	for rows.Next() {
		rel := &models.ASTRelationship{}
		err := rows.Scan(&rel.ID, &rel.FromASTID, &rel.ToASTID,
			&rel.LineNo, &rel.RelationshipType, &rel.Text)
		if err != nil {
			return nil, fmt.Errorf("failed to scan relationship: %w", err)
		}
		relationships = append(relationships, rel)
	}

	return relationships, rows.Err()
}

// GetLibraryRelationships retrieves all library relationships
func (a *Analyzer) GetLibraryRelationships() ([]*models.LibraryRelationship, error) {
	query := `
		SELECT lr.id, lr.ast_id, lr.library_id, lr.line_no, lr.relationship_type, lr.text,
		       ln.id, ln.package, ln.class, ln.method, ln.field, ln.node_type, ln.language, ln.framework
		FROM library_relationships lr
		LEFT JOIN library_nodes ln ON lr.library_id = ln.id
		ORDER BY lr.ast_id, lr.line_no
	`

	rows, err := a.cache.QueryRaw(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query library relationships: %w", err)
	}
	defer rows.Close()

	var relationships []*models.LibraryRelationship
	for rows.Next() {
		rel := &models.LibraryRelationship{}
		libNode := &models.LibraryNode{}

		err := rows.Scan(&rel.ID, &rel.ASTID, &rel.LibraryID, &rel.LineNo,
			&rel.RelationshipType, &rel.Text,
			&libNode.ID, &libNode.Package, &libNode.Class, &libNode.Method,
			&libNode.Field, &libNode.NodeType, &libNode.Language, &libNode.Framework)
		if err != nil {
			return nil, fmt.Errorf("failed to scan library relationship: %w", err)
		}

		rel.LibraryNode = libNode
		relationships = append(relationships, rel)
	}

	return relationships, rows.Err()
}

// GetRelationshipsForNode retrieves all relationships for a specific AST node
func (a *Analyzer) GetRelationshipsForNode(nodeID int64) ([]*models.ASTRelationship, error) {
	query := `
		SELECT id, from_ast_id, to_ast_id, line_no, relationship_type, text
		FROM ast_relationships
		WHERE from_ast_id = ? OR to_ast_id = ?
		ORDER BY line_no
	`

	rows, err := a.cache.QueryRaw(query, nodeID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to query relationships for node %d: %w", nodeID, err)
	}
	defer rows.Close()

	var relationships []*models.ASTRelationship
	for rows.Next() {
		rel := &models.ASTRelationship{}
		err := rows.Scan(&rel.ID, &rel.FromASTID, &rel.ToASTID,
			&rel.LineNo, &rel.RelationshipType, &rel.Text)
		if err != nil {
			return nil, fmt.Errorf("failed to scan relationship: %w", err)
		}
		relationships = append(relationships, rel)
	}

	return relationships, rows.Err()
}
