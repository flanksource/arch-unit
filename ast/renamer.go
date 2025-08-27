package ast

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

// Renamer handles renaming AST nodes and updating references
type Renamer struct {
	workingDir string
	noColor    bool
}

// NewRenamer creates a new renamer
func NewRenamer(workingDir string, noColor bool) *Renamer {
	return &Renamer{
		workingDir: workingDir,
		noColor:    noColor,
	}
}

// ReferenceLocation represents a location where a node is referenced
type ReferenceLocation struct {
	FilePath   string `json:"file_path"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	Context    string `json:"context"` // Surrounding code for context
}

// RenameOperation represents a planned rename operation
type RenameOperation struct {
	TargetNode  *models.ASTNode       `json:"target_node"`
	OldName     string                `json:"old_name"`
	NewName     string                `json:"new_name"`
	References  []*ReferenceLocation  `json:"references"`
	FilesToModify map[string][]*ReferenceLocation `json:"files_to_modify"`
}

// RenameResult represents the result of a rename operation
type RenameResult struct {
	Success           bool     `json:"success"`
	FilesModified     int      `json:"files_modified"`
	ReferencesUpdated int      `json:"references_updated"`
	ModifiedFiles     []string `json:"modified_files"`
	ErrorMessage      string   `json:"error_message,omitempty"`
}

// FindNodeToRename finds a node to rename by name or pattern
func (r *Renamer) FindNodeToRename(analyzer *Analyzer, nameOrPattern string) (*models.ASTNode, error) {
	// Try to find by exact full name first
	nodes, err := analyzer.QueryPattern(nameOrPattern)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		// Try searching by simple name
		simplePattern := fmt.Sprintf("*:%s", nameOrPattern)
		nodes, err = analyzer.QueryPattern(simplePattern)
		if err != nil {
			return nil, err
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes found matching: %s", nameOrPattern)
	}

	if len(nodes) > 1 {
		return nil, fmt.Errorf("multiple nodes found matching '%s'. Please use a more specific pattern", nameOrPattern)
	}

	return nodes[0], nil
}

// FindAllReferences finds all references to a given AST node
func (r *Renamer) FindAllReferences(analyzer *Analyzer, targetNode *models.ASTNode) ([]*ReferenceLocation, error) {
	// Get all relationships where this node is referenced
	relationships, err := analyzer.GetRelationshipsForNode(targetNode.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	var references []*ReferenceLocation

	// Process each relationship to find the actual reference locations
	for _, rel := range relationships {
		if rel.ToASTID != nil && *rel.ToASTID == targetNode.ID {
			// This is a reference TO our target node
			refLocation, err := r.extractReferenceLocation(rel, targetNode)
			if err != nil {
				// Log warning but continue
				fmt.Fprintf(os.Stderr, "Warning: failed to extract reference at %s:%d: %v\n", 
					rel.FromASTID, rel.LineNo, err)
				continue
			}
			if refLocation != nil {
				references = append(references, refLocation)
			}
		}
	}

	// Also need to update the definition itself
	defLocation, err := r.extractDefinitionLocation(targetNode)
	if err != nil {
		return nil, fmt.Errorf("failed to extract definition location: %w", err)
	}
	if defLocation != nil {
		references = append(references, defLocation)
	}

	return references, nil
}

// extractReferenceLocation extracts the actual location of a reference from a relationship
func (r *Renamer) extractReferenceLocation(rel *models.ASTRelationship, targetNode *models.ASTNode) (*ReferenceLocation, error) {
	// We need to find the file that contains the reference
	// For now, we'll use the relationship text and line number to locate it
	
	// The FromASTID tells us which node contains the reference
	// We need to get that node's file and search around the line number
	
	// This is a simplified implementation - in a real system we'd need more sophisticated parsing
	return &ReferenceLocation{
		FilePath: "unknown", // Would need to resolve from FromASTID
		Line:     rel.LineNo,
		Column:   0, // Would need to parse to find column
		OldText:  extractNameFromNode(targetNode),
		NewText:  "", // Will be filled in during planning
		Context:  rel.Text,
	}, nil
}

// extractDefinitionLocation extracts the location where the node is defined
func (r *Renamer) extractDefinitionLocation(node *models.ASTNode) (*ReferenceLocation, error) {
	oldName := extractNameFromNode(node)
	
	return &ReferenceLocation{
		FilePath: node.FilePath,
		Line:     node.StartLine,
		Column:   0, // Would need to parse the line to find exact column
		OldText:  oldName,
		NewText:  "", // Will be filled during planning
		Context:  fmt.Sprintf("definition of %s", oldName),
	}, nil
}

// extractNameFromNode extracts the renameable name from an AST node
func extractNameFromNode(node *models.ASTNode) string {
	if node.MethodName != "" {
		return node.MethodName
	}
	if node.FieldName != "" {
		return node.FieldName
	}
	if node.TypeName != "" {
		return node.TypeName
	}
	return node.PackageName
}

// PlanRename creates a rename operation plan
func (r *Renamer) PlanRename(targetNode *models.ASTNode, newName string, references []*ReferenceLocation) (*RenameOperation, error) {
	oldName := extractNameFromNode(targetNode)
	
	// Update all references with the new name
	filesToModify := make(map[string][]*ReferenceLocation)
	
	for _, ref := range references {
		ref.NewText = strings.ReplaceAll(ref.OldText, oldName, newName)
		
		if _, exists := filesToModify[ref.FilePath]; !exists {
			filesToModify[ref.FilePath] = make([]*ReferenceLocation, 0)
		}
		filesToModify[ref.FilePath] = append(filesToModify[ref.FilePath], ref)
	}

	return &RenameOperation{
		TargetNode:    targetNode,
		OldName:       oldName,
		NewName:       newName,
		References:    references,
		FilesToModify: filesToModify,
	}, nil
}

// GeneratePreview generates a preview of the rename operation
func (r *Renamer) GeneratePreview(op *RenameOperation, showDiff bool) (string, error) {
	var result strings.Builder
	
	result.WriteString(fmt.Sprintf("🔄 Rename Preview: %s → %s\n", op.OldName, op.NewName))
	result.WriteString("═══════════════════════════════════════════════════════════════\n\n")
	
	result.WriteString(fmt.Sprintf("Target: %s (%s)\n", op.TargetNode.GetFullName(), op.TargetNode.NodeType))
	result.WriteString(fmt.Sprintf("Files to modify: %d\n", len(op.FilesToModify)))
	result.WriteString(fmt.Sprintf("Total references: %d\n\n", len(op.References)))
	
	// Show files and changes
	for filePath, refs := range op.FilesToModify {
		relPath, err := filepath.Rel(r.workingDir, filePath)
		if err != nil {
			relPath = filePath
		}
		
		result.WriteString(fmt.Sprintf("📄 %s (%d changes)\n", relPath, len(refs)))
		
		if showDiff {
			for _, ref := range refs {
				result.WriteString(fmt.Sprintf("  Line %d: %s → %s\n", ref.Line, ref.OldText, ref.NewText))
				if ref.Context != "" {
					result.WriteString(fmt.Sprintf("    Context: %s\n", ref.Context))
				}
			}
		}
		result.WriteString("\n")
	}
	
	return result.String(), nil
}

// ExecuteRename executes the rename operation
func (r *Renamer) ExecuteRename(op *RenameOperation, createBackup bool) (*RenameResult, error) {
	result := &RenameResult{
		Success:       true,
		ModifiedFiles: make([]string, 0),
	}
	
	for filePath, refs := range op.FilesToModify {
		// Create backup if requested
		if createBackup {
			if err := r.createBackup(filePath); err != nil {
				return &RenameResult{
					Success:      false,
					ErrorMessage: fmt.Sprintf("failed to create backup for %s: %v", filePath, err),
				}, err
			}
		}
		
		// Apply changes to the file
		if err := r.applyChangesToFile(filePath, refs); err != nil {
			return &RenameResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("failed to modify %s: %v", filePath, err),
			}, err
		}
		
		result.ModifiedFiles = append(result.ModifiedFiles, filePath)
		result.FilesModified++
		result.ReferencesUpdated += len(refs)
	}
	
	return result, nil
}

// createBackup creates a backup file with .bak extension
func (r *Renamer) createBackup(filePath string) error {
	backupPath := filePath + ".bak"
	
	src, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer src.Close()
	
	dst, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	
	_, err = io.Copy(dst, src)
	return err
}

// applyChangesToFile applies the rename changes to a file
func (r *Renamer) applyChangesToFile(filePath string, refs []*ReferenceLocation) error {
	// Read the original file
	lines, err := r.readFileLines(filePath)
	if err != nil {
		return err
	}
	
	// Sort references by line number (descending) to avoid line number shifting issues
	// For now, do simple text replacement - a real implementation would need more sophisticated parsing
	for _, ref := range refs {
		if ref.Line > 0 && ref.Line <= len(lines) {
			// Simple string replacement on the line
			lineIndex := ref.Line - 1 // Convert to 0-based
			lines[lineIndex] = strings.ReplaceAll(lines[lineIndex], ref.OldText, ref.NewText)
		}
	}
	
	// Write the modified file
	return r.writeFileLines(filePath, lines)
}

// readFileLines reads all lines from a file
func (r *Renamer) readFileLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines, scanner.Err()
}

// writeFileLines writes lines to a file
func (r *Renamer) writeFileLines(filePath string, lines []string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	return nil
}

// findWordBoundaryRename finds and replaces a word with word boundaries
func (r *Renamer) findWordBoundaryRename(text, oldName, newName string) string {
	// Use regex to match word boundaries
	pattern := `\b` + regexp.QuoteMeta(oldName) + `\b`
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(text, newName)
}