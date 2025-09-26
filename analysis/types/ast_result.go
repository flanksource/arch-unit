package types

import (
	"github.com/flanksource/arch-unit/models"
)

// ASTResult contains the complete analysis results for a file
type ASTResult struct {
	// AST nodes found in the file
	Nodes []*models.ASTNode

	// Relationships between nodes (calls, inherits, etc.)
	Relationships []*models.ASTRelationship

	// External library dependencies
	Libraries []*models.LibraryRelationship

	// Dependencies found in the file
	Dependencies []*models.Dependency

	// Relationships between dependencies
	DependencyRelationships []*models.DependencyRelationship

	// Rule violations found in the file (optional)
	Violations []models.Violation

	// File metadata
	FilePath    string
	Language    string
	PackageName string

	// Analysis statistics
	NodeCount         int
	RelationshipCount int
	LibraryCount      int
	ViolationCount    int
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

// AddLibrary adds a library relationship to the result
func (r *ASTResult) AddLibrary(lib *models.LibraryRelationship) {
	r.Libraries = append(r.Libraries, lib)
	r.LibraryCount++
}

// AddDependency adds a dependency to the result
func (r *ASTResult) AddDependency(dep *models.Dependency) {
	r.Dependencies = append(r.Dependencies, dep)
}

// AddDependencyRelationship adds a dependency relationship to the result
func (r *ASTResult) AddDependencyRelationship(rel *models.DependencyRelationship) {
	r.DependencyRelationships = append(r.DependencyRelationships, rel)
}

// AddViolation adds a rule violation to the result
func (r *ASTResult) AddViolation(violation models.Violation) {
	r.Violations = append(r.Violations, violation)
	r.ViolationCount++
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
