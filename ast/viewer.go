package ast

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
)

// SourceViewer handles viewing and pretty-printing source code for AST nodes
type SourceViewer struct {
	prettyParser *clicky.PrettyParser
	workingDir   string
}

// NewSourceViewer creates a new source viewer
func NewSourceViewer(workingDir string, noColor bool) *SourceViewer {
	parser := clicky.NewPrettyParser()
	parser.NoColor = noColor

	return &SourceViewer{
		prettyParser: parser,
		workingDir:   workingDir,
	}
}

// NodeSourceView represents source code view for an AST node
type NodeSourceView struct {
	Node        *models.ASTNode `json:"node"`
	SourceLines []string        `json:"source_lines"`
	StartLine   int             `json:"start_line"`
	EndLine     int             `json:"end_line"`
	FilePath    string          `json:"file_path"`
}

// ViewNodeSource extracts and formats source code for an AST node
func (sv *SourceViewer) ViewNodeSource(node *models.ASTNode) (*NodeSourceView, error) {
	// Read the source file
	sourceLines, err := sv.readFileLines(node.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read source file %s: %w", node.FilePath, err)
	}

	// Extract relevant lines (with some context)
	startLine := max(1, node.StartLine-2) // Add 2 lines of context
	endLine := min(len(sourceLines), node.EndLine+2)

	if startLine > len(sourceLines) {
		return nil, fmt.Errorf("start line %d exceeds file length %d", startLine, len(sourceLines))
	}

	// Extract the lines (convert to 0-based indexing)
	relevantLines := sourceLines[startLine-1 : endLine]

	// Get relative path for display
	relPath, err := filepath.Rel(sv.workingDir, node.FilePath)
	if err != nil {
		relPath = node.FilePath
	}

	return &NodeSourceView{
		Node:        node,
		SourceLines: relevantLines,
		StartLine:   startLine,
		EndLine:     endLine,
		FilePath:    relPath,
	}, nil
}

// ViewMultipleNodes views source for multiple AST nodes
func (sv *SourceViewer) ViewMultipleNodes(nodes []*models.ASTNode) ([]*NodeSourceView, error) {
	var views []*NodeSourceView

	for _, node := range nodes {
		view, err := sv.ViewNodeSource(node)
		if err != nil {
			// Log error but continue with other nodes
			fmt.Fprintf(os.Stderr, "Warning: failed to view source for node %s: %v\n", node.GetFullName(), err)
			continue
		}
		views = append(views, view)
	}

	return views, nil
}

// FormatSourceView formats a source view for display
func (sv *SourceViewer) FormatSourceView(view *NodeSourceView, format string) (string, error) {
	switch format {
	case "json":
		return sv.prettyParser.Parse(view)
	case "tree":
		return sv.formatAsTree(view)
	case "plain":
		return sv.formatAsPlain(view)
	default:
		return sv.formatAsTree(view) // Default to tree format
	}
}

// FormatMultipleViews formats multiple source views
func (sv *SourceViewer) FormatMultipleViews(views []*NodeSourceView, format string) (string, error) {
	var formatted []string

	for i, view := range views {
		viewFormatted, err := sv.FormatSourceView(view, format)
		if err != nil {
			return "", fmt.Errorf("failed to format view %d: %w", i, err)
		}
		formatted = append(formatted, viewFormatted)
	}

	return strings.Join(formatted, "\n\n"), nil
}

// formatAsTree formats the source view as a tree structure
func (sv *SourceViewer) formatAsTree(view *NodeSourceView) (string, error) {
	var result strings.Builder

	// Header with file and node info
	result.WriteString(fmt.Sprintf("📄 %s:%d-%d\n", view.FilePath, view.Node.StartLine, view.Node.EndLine))
	result.WriteString(fmt.Sprintf("🔍 %s (%s)\n", view.Node.GetFullName(), view.Node.NodeType))

	if view.Node.CyclomaticComplexity > 0 {
		result.WriteString(fmt.Sprintf("📊 Complexity: %d, Params: %d, Lines: %d\n",
			view.Node.CyclomaticComplexity, len(view.Node.Parameters), view.Node.LineCount))
	}

	result.WriteString("─────────────────────────────────────────────────────────────\n")

	// Source lines with line numbers
	for i, line := range view.SourceLines {
		lineNum := view.StartLine + i
		isNodeLine := lineNum >= view.Node.StartLine && lineNum <= view.Node.EndLine

		prefix := "  "
		if isNodeLine {
			prefix = "▶ " // Highlight lines that belong to the node
		}

		result.WriteString(fmt.Sprintf("%s%4d │ %s\n", prefix, lineNum, line))
	}

	return result.String(), nil
}

// formatAsPlain formats the source view as plain text
func (sv *SourceViewer) formatAsPlain(view *NodeSourceView) (string, error) {
	var result strings.Builder

	// Simple header
	result.WriteString(fmt.Sprintf("%s:%d-%d %s\n",
		view.FilePath, view.Node.StartLine, view.Node.EndLine, view.Node.GetFullName()))

	// Source lines with line numbers
	for i, line := range view.SourceLines {
		lineNum := view.StartLine + i
		result.WriteString(fmt.Sprintf("%4d │ %s\n", lineNum, line))
	}

	return result.String(), nil
}

// readFileLines reads all lines from a file
func (sv *SourceViewer) readFileLines(filePath string) ([]string, error) {
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

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// Helper functions
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
