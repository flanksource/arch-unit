package markdown

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// MarkdownASTExtractor extracts structure and code blocks from Markdown files
type MarkdownASTExtractor struct {
	filePath    string
	packageName string
}

// NewMarkdownASTExtractor creates a new Markdown AST extractor
func NewMarkdownASTExtractor() *MarkdownASTExtractor {
	return &MarkdownASTExtractor{}
}

// MarkdownSection represents a section in a Markdown document
type MarkdownSection struct {
	Level     int
	Title     string
	StartLine int
	EndLine   int
	Parent    string
}

// MarkdownCodeBlock represents a code block in Markdown
type MarkdownCodeBlock struct {
	Language  string
	Content   string
	StartLine int
	EndLine   int
	InSection string
}

// MarkdownLink represents a link in Markdown
type MarkdownLink struct {
	Text      string
	URL       string
	Line      int
	InSection string
}

// ExtractFile extracts structure information from a Markdown file
func (e *MarkdownASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*types.ASTResult, error) {
	e.filePath = filePath
	e.packageName = e.extractPackageName(filePath)

	result := &types.ASTResult{
		Nodes:         []*models.ASTNode{},
		Relationships: []*models.ASTRelationship{},
	}

	// Parse Markdown content using goldmark
	nodes, relationships := e.parseMarkdownContent(string(content), filePath)

	// Add all parsed nodes and relationships to result
	for _, node := range nodes {
		result.Nodes = append(result.Nodes, &node)
	}
	for _, rel := range relationships {
		result.Relationships = append(result.Relationships, &rel)
	}

	return result, nil
}

// extractPackageName extracts package name from the file path
func (e *MarkdownASTExtractor) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)
	return filepath.Base(dir)
}

// parseMarkdownContent parses Markdown content using goldmark
func (e *MarkdownASTExtractor) parseMarkdownContent(content string, sourceFile string) ([]models.ASTNode, []models.ASTRelationship) {
	var nodes []models.ASTNode
	var relationships []models.ASTRelationship

	// Parse markdown using goldmark
	md := goldmark.New()
	source := []byte(content)
	doc := md.Parser().Parse(text.NewReader(source))

	// Create the document node as a package
	docNode := models.ASTNode{
		FilePath:     sourceFile,
		PackageName:  e.packageName,
		NodeType:     models.NodeTypePackage,
		StartLine:    1,
		EndLine:      e.countLinesFromContent([]byte(content)),
		LineCount:    e.countLinesFromContent([]byte(content)),
		LastModified: time.Now(),
	}
	nodes = append(nodes, docNode)

	// Walk the AST tree to extract sections and code blocks
	lines := strings.Split(content, "\n")
	currentSection := ""
	sectionLevel := 0

	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n := node.(type) {
		case *ast.Heading:
			// Extract heading text and level
			headingText := e.getNodeText(n, source)
			level := n.Level
			startLine := e.getLineNumber(n, source, lines)
			endLine := startLine

			// Create section node
			sectionNode := models.ASTNode{
				FilePath:     sourceFile,
				PackageName:  e.packageName,
				TypeName:     headingText,
				NodeType:     models.NodeTypeType,
				StartLine:    startLine,
				EndLine:      endLine,
				LineCount:    1,
				LastModified: time.Now(),
			}
			nodes = append(nodes, sectionNode)

			// Update current section context
			if level <= sectionLevel || currentSection == "" {
				currentSection = headingText
				sectionLevel = level
			}

		case *ast.FencedCodeBlock:
			// Extract code block information
			startLine := e.getLineNumber(n, source, lines)
			endLine := startLine + e.countNodeLines(n, source) - 1
			language := string(n.Language(source))
			if language == "" {
				language = "text"
			}

			// Create code block node as a method
			methodName := fmt.Sprintf("code_%s_%d", language, startLine)
			codeNode := models.ASTNode{
				FilePath:             sourceFile,
				PackageName:          e.packageName,
				TypeName:             currentSection,
				MethodName:           methodName,
				NodeType:             models.NodeTypeMethod,
				StartLine:            startLine,
				EndLine:              endLine,
				LineCount:            endLine - startLine + 1,
				CyclomaticComplexity: e.calculateCodeBlockComplexity(string(n.Text(source))),
				LastModified:         time.Now(),
			}
			nodes = append(nodes, codeNode)
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		// If walking fails, return what we have so far
		return nodes, relationships
	}

	return nodes, relationships
}

// countLinesFromContent counts lines in content
func (e *MarkdownASTExtractor) countLinesFromContent(content []byte) int {
	return strings.Count(string(content), "\n") + 1
}

// calculateCodeBlockComplexity calculates complexity for a code block
func (e *MarkdownASTExtractor) calculateCodeBlockComplexity(content string) int {
	// Simple heuristic for complexity
	return 1
}

// getNodeText extracts text content from an AST node
func (e *MarkdownASTExtractor) getNodeText(node ast.Node, source []byte) string {
	var text strings.Builder
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		if textNode, ok := c.(*ast.Text); ok {
			text.Write(textNode.Text(source))
		}
	}
	return text.String()
}

// getLineNumber calculates line number from node position
func (e *MarkdownASTExtractor) getLineNumber(node ast.Node, source []byte, lines []string) int {
	segment := node.Lines().At(0)
	start := segment.Start

	// Count newlines up to this position
	lineNum := 1
	for i := 0; i < start && i < len(source); i++ {
		if source[i] == '\n' {
			lineNum++
		}
	}
	return lineNum
}

// countNodeLines counts the number of lines in a node
func (e *MarkdownASTExtractor) countNodeLines(node ast.Node, source []byte) int {
	lines := node.Lines()
	if lines.Len() == 0 {
		return 1
	}

	first := lines.At(0)
	last := lines.At(lines.Len() - 1)

	// Count newlines in the range
	count := 1
	for i := first.Start; i < last.Stop && i < len(source); i++ {
		if source[i] == '\n' {
			count++
		}
	}
	return count
}

func init() {
	// Register Markdown AST extractor
	mdExtractor := NewMarkdownASTExtractor()
	analysis.RegisterExtractor("markdown", mdExtractor)
}
