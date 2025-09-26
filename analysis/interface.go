package analysis

import (
	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/clicky"
)

// Analyzer is the interface for high-level AST analysis orchestration
// Analyzers handle cache management, DB operations, and coordinate extraction
type Analyzer interface {
	// AnalyzeFile analyzes a single file and returns AST results
	AnalyzeFile(task *clicky.Task, filepath string, content []byte) (*types.ASTResult, error)
}

// Extractor is the interface for language-specific AST extraction
// Extractors should be pure functions that only extract AST data
// They can use ReadOnlyCache to lookup existing node IDs for relationship building
type Extractor interface {
	// ExtractFile extracts AST information from a file using read-only cache for ID lookups
	ExtractFile(cache cache.ReadOnlyCache, filepath string, content []byte) (*types.ASTResult, error)
}

// BaseAnalyzer provides common functionality for analyzers
type BaseAnalyzer struct {
	Language string
}

// NewBaseAnalyzer creates a new base analyzer
func NewBaseAnalyzer(language string) *BaseAnalyzer {
	return &BaseAnalyzer{
		Language: language,
	}
}

// LogProgress logs analysis progress through the task
func (b *BaseAnalyzer) LogProgress(task *clicky.Task, message string, args ...interface{}) {
	task.Infof(message, args...)
}

// LogDebug logs debug information through the task
func (b *BaseAnalyzer) LogDebug(task *clicky.Task, message string, args ...interface{}) {
	task.Debugf(message, args...)
}

// LogWarning logs warnings through the task
func (b *BaseAnalyzer) LogWarning(task *clicky.Task, message string, args ...interface{}) {
	task.Warnf(message, args...)
}

// LogError logs errors through the task
func (b *BaseAnalyzer) LogError(task *clicky.Task, message string, args ...interface{}) {
	task.Errorf(message, args...)

}
