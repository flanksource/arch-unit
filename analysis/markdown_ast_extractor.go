package analysis

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
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
func (e *MarkdownASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*ASTResult, error) {
	e.filePath = filePath
	e.packageName = e.extractPackageName(filePath)

	result := &ASTResult{
		Nodes:         []*models.ASTNode{},
		Relationships: []*models.ASTRelationship{},
	}

	// Parse Markdown content
	sections, codeBlocks, _, err := e.parseMarkdownContent(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Markdown file: %w", err)
	}

	// Build document structure as nodes
	nodeMap := make(map[string]string) // name -> key

	// Create the document itself as a package node
	docNode := &models.ASTNode{
		FilePath:     filePath,
		PackageName:  e.packageName,
		NodeType:     models.NodeTypePackage,
		StartLine:    1,
		EndLine:      e.countLinesFromContent(content),
		LineCount:    e.countLinesFromContent(content),
		LastModified: time.Now(),
	}

	// Add document node to result
	docKey := docNode.Key()
	nodeMap[e.packageName] = docKey
	result.Nodes = append(result.Nodes, docNode)

	// Build sections as type nodes
	for _, section := range sections {
		sectionNode := &models.ASTNode{
			FilePath:     filePath,
			PackageName:  e.packageName,
			TypeName:     section.Title,
			NodeType:     models.NodeTypeType,
			StartLine:    section.StartLine,
			EndLine:      section.EndLine,
			LineCount:    section.EndLine - section.StartLine + 1,
			LastModified: time.Now(),
		}

		sectionKey := sectionNode.Key()
		fullName := fmt.Sprintf("%s.%s", e.packageName, section.Title)
		nodeMap[fullName] = sectionKey
		result.Nodes = append(result.Nodes, sectionNode)

		// Create relationship to parent section or document
		var fromID, toID int64
		if sectionCacheID, exists := cache.GetASTId(sectionKey); exists {
			fromID = sectionCacheID
		}

		var relationship *models.ASTRelationship
		if section.Parent != "" {
			parentFullName := fmt.Sprintf("%s.%s", e.packageName, section.Parent)
			if parentKey, exists := nodeMap[parentFullName]; exists {
				if parentCacheID, exists := cache.GetASTId(parentKey); exists {
					toID = parentCacheID
				}
				relationship = &models.ASTRelationship{
					FromASTID:        fromID,
					ToASTID:          &toID,
					LineNo:           section.StartLine,
					RelationshipType: models.RelationshipReference,
					Text:             fmt.Sprintf("subsection of %s", section.Parent),
				}
			}
		} else {
			if docCacheID, exists := cache.GetASTId(docKey); exists {
				toID = docCacheID
			}
			relationship = &models.ASTRelationship{
				FromASTID:        fromID,
				ToASTID:          &toID,
				LineNo:           section.StartLine,
				RelationshipType: models.RelationshipReference,
				Text:             "top-level section",
			}
		}

		if relationship != nil {
			result.Relationships = append(result.Relationships, relationship)
		}
	}

	// Build code blocks as method nodes with language as metadata
	for _, block := range codeBlocks {
		// Calculate complexity based on code block content
		complexity := e.calculateCodeBlockComplexity(block.Content)

		blockNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             block.InSection,
			MethodName:           fmt.Sprintf("code_%s_%d", block.Language, block.StartLine),
			NodeType:             models.NodeTypeMethod,
			StartLine:            block.StartLine,
			EndLine:              block.EndLine,
			LineCount:            block.EndLine - block.StartLine + 1,
			CyclomaticComplexity: complexity,
			LastModified:         time.Now(),
		}

		blockKey := blockNode.Key()
		result.Nodes = append(result.Nodes, blockNode)

		// If code block is in a section, create relationship
		if block.InSection != "" {
			sectionFullName := fmt.Sprintf("%s.%s", e.packageName, block.InSection)
			if sectionKey, exists := nodeMap[sectionFullName]; exists {
				var fromID, toID int64
				if blockCacheID, exists := cache.GetASTId(blockKey); exists {
					fromID = blockCacheID
				}
				if sectionCacheID, exists := cache.GetASTId(sectionKey); exists {
					toID = sectionCacheID
				}

				relationship := &models.ASTRelationship{
					FromASTID:        fromID,
					ToASTID:          &toID,
					LineNo:           block.StartLine,
					RelationshipType: models.RelationshipReference,
					Text:             fmt.Sprintf("%s code block", block.Language),
				}
				result.Relationships = append(result.Relationships, relationship)
			}
		}
	}

	return result, nil
}

// extractPackageName extracts package name from the file path
func (e *MarkdownASTExtractor) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)
	return filepath.Base(dir)
}

// parseMarkdownContent parses Markdown content
func (e *MarkdownASTExtractor) parseMarkdownContent(content []byte) ([]MarkdownSection, []MarkdownCodeBlock, []MarkdownLink, error) {
	// This is a stub - the full implementation was removed
	return []MarkdownSection{}, []MarkdownCodeBlock{}, []MarkdownLink{}, nil
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
