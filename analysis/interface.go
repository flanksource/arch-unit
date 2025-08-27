package analysis

import (
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
)

// Analyzer is the interface for language-specific AST analyzers
// Analyzers should be cache-unaware and focus only on analysis
type Analyzer interface {
	// AnalyzeFile analyzes a single file and returns AST results
	AnalyzeFile(task *clicky.Task, filepath string, content []byte) (*ASTResult, error)
}

// ASTResult contains the complete analysis results for a file
type ASTResult struct {
	// AST nodes found in the file
	Nodes []*models.ASTNode
	
	// Relationships between nodes (calls, inherits, etc.)
	Relationships []*models.ASTRelationship
	
	// External library dependencies
	Libraries []*models.LibraryRelationship
	
	// File metadata
	FilePath    string
	Language    string
	PackageName string
	
	// Analysis statistics
	NodeCount         int
	RelationshipCount int
	LibraryCount      int
}

// NewASTResult creates a new AST result
func NewASTResult(filepath string, language string) *ASTResult {
	return &ASTResult{
		FilePath:                filepath,
		Language:                language,
		Nodes:                   make([]*models.ASTNode, 0),
		Relationships:           make([]*models.ASTRelationship, 0),
		Libraries:               make([]*models.LibraryRelationship, 0),
		Dependencies:            make([]*models.Dependency, 0),
		DependencyRelationships: make([]*models.DependencyRelationship, 0),
	}
}

// AddNode adds an AST node to the result
func (r *ASTResult) AddNode(node *models.ASTNode) {
	r.Nodes = append(r.Nodes, node)
	r.NodeCount++
}

// AddRelationship adds a relationship to the result
func (r *ASTResult) AddRelationship(rel *models.ASTRelationship) {
	r.Relationships = append(r.Relationships, rel)
	r.RelationshipCount++
}

// AddLibrary adds a library dependency to the result
func (r *ASTResult) AddLibrary(lib *models.LibraryRelationship) {
	r.Libraries = append(r.Libraries, lib)
	r.LibraryCount++
}

// Merge combines another result into this one
func (r *ASTResult) Merge(other *ASTResult) {
	if other == nil {
		return
	}
	
	r.Nodes = append(r.Nodes, other.Nodes...)
	r.Relationships = append(r.Relationships, other.Relationships...)
	r.Libraries = append(r.Libraries, other.Libraries...)
	
	r.NodeCount += other.NodeCount
	r.RelationshipCount += other.RelationshipCount
	r.LibraryCount += other.LibraryCount
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
	if task != nil {
		task.Infof(message, args...)
	}
}

// LogDebug logs debug information through the task
func (b *BaseAnalyzer) LogDebug(task *clicky.Task, message string, args ...interface{}) {
	if task != nil {
		// Debug: (message, args...)
	}
}

// LogWarning logs warnings through the task
func (b *BaseAnalyzer) LogWarning(task *clicky.Task, message string, args ...interface{}) {
	if task != nil {
		task.Warnf(message, args...)
	}
}

// LogError logs errors through the task
func (b *BaseAnalyzer) LogError(task *clicky.Task, message string, args ...interface{}) {
	if task != nil {
		task.Errorf(message, args...)
	}
}