package analysis

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		complexity := e.calculateCodeBlockComplexity(block.Content, block.Language)

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

// parseMarkdownFile parses a Markdown file and extracts structure
func (e *MarkdownASTExtractor) parseMarkdownFile(filePath string) ([]MarkdownSection, []MarkdownCodeBlock, []MarkdownLink, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, nil, err
	}
	defer file.Close()

	var sections []MarkdownSection
	var codeBlocks []MarkdownCodeBlock
	var links []MarkdownLink

	scanner := bufio.NewScanner(file)
	lineNum := 0
	inCodeBlock := false
	currentSection := ""
	sectionStack := []string{}
	var currentBlock *MarkdownCodeBlock

	// Regular expressions for Markdown elements
	headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	codeBlockStartRegex := regexp.MustCompile("^```(\\w*)")
	codeBlockEndRegex := regexp.MustCompile("^```")
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Handle code blocks
		if !inCodeBlock {
			if matches := codeBlockStartRegex.FindStringSubmatch(line); matches != nil {
				inCodeBlock = true
				language := ""
				if len(matches) > 1 {
					language = matches[1]
				}
				if language == "" {
					language = "text"
				}
				currentBlock = &MarkdownCodeBlock{
					Language:  language,
					StartLine: lineNum,
					InSection: currentSection,
					Content:   "",
				}
			}
		} else {
			if codeBlockEndRegex.MatchString(line) {
				inCodeBlock = false
				if currentBlock != nil {
					currentBlock.EndLine = lineNum
					codeBlocks = append(codeBlocks, *currentBlock)
					currentBlock = nil
				}
			} else if currentBlock != nil {
				if currentBlock.Content != "" {
					currentBlock.Content += "\n"
				}
				currentBlock.Content += line
			}
		}

		// Skip processing other elements if inside code block
		if inCodeBlock {
			continue
		}

		// Handle headers (sections)
		if matches := headerRegex.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			title := strings.TrimSpace(matches[2])

			// Update section stack
			for len(sectionStack) >= level {
				sectionStack = sectionStack[:len(sectionStack)-1]
			}

			parent := ""
			if len(sectionStack) > 0 {
				parent = sectionStack[len(sectionStack)-1]
			}

			section := MarkdownSection{
				Level:     level,
				Title:     title,
				StartLine: lineNum,
				EndLine:   lineNum, // Will be updated when next section or EOF
				Parent:    parent,
			}

			// Update end line of previous section at same or higher level
			for i := len(sections) - 1; i >= 0; i-- {
				if sections[i].EndLine == sections[i].StartLine && sections[i].Level >= level {
					sections[i].EndLine = lineNum - 1
				} else if sections[i].Level < level {
					break
				}
			}

			sections = append(sections, section)
			sectionStack = append(sectionStack, title)
			currentSection = title
		}

		// Handle links
		if matches := linkRegex.FindAllStringSubmatch(line, -1); matches != nil {
			for _, match := range matches {
				link := MarkdownLink{
					Text:      match[1],
					URL:       match[2],
					Line:      lineNum,
					InSection: currentSection,
				}
				links = append(links, link)
			}
		}
	}

	// Update end lines for remaining sections
	totalLines := lineNum
	for i := range sections {
		if sections[i].EndLine == sections[i].StartLine {
			// Find next section at same or higher level
			nextLine := totalLines
			for j := i + 1; j < len(sections); j++ {
				if sections[j].Level <= sections[i].Level {
					nextLine = sections[j].StartLine - 1
					break
				}
			}
			sections[i].EndLine = nextLine
		}
	}

	return sections, codeBlocks, links, scanner.Err()
}

// parseMarkdownContent parses Markdown content from bytes and extracts structure
func (e *MarkdownASTExtractor) parseMarkdownContent(content []byte) ([]MarkdownSection, []MarkdownCodeBlock, []MarkdownLink, error) {
	var sections []MarkdownSection
	var codeBlocks []MarkdownCodeBlock
	var links []MarkdownLink

	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNum := 0
	inCodeBlock := false
	currentSection := ""
	sectionStack := []string{}
	var currentBlock *MarkdownCodeBlock

	// Regular expressions for Markdown elements
	headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	codeBlockStartRegex := regexp.MustCompile("^```([a-zA-Z]*)")
	codeBlockEndRegex := regexp.MustCompile("^```$")
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Handle code blocks
		if matches := codeBlockStartRegex.FindStringSubmatch(line); matches != nil && !inCodeBlock {
			inCodeBlock = true
			language := "text"
			if len(matches) > 1 && matches[1] != "" {
				language = matches[1]
			}
			currentBlock = &MarkdownCodeBlock{
				Language:  language,
				StartLine: lineNum,
				InSection: currentSection,
			}
		} else if codeBlockEndRegex.MatchString(line) && inCodeBlock {
			inCodeBlock = false
			if currentBlock != nil {
				currentBlock.EndLine = lineNum
				codeBlocks = append(codeBlocks, *currentBlock)
				currentBlock = nil
			}
		} else if inCodeBlock && currentBlock != nil {
			currentBlock.Content += line + "\n"
		}

		// Handle headers
		if matches := headerRegex.FindStringSubmatch(line); matches != nil && !inCodeBlock {
			level := len(matches[1])
			title := matches[2]

			// Adjust section stack based on header level
			for len(sectionStack) >= level {
				sectionStack = sectionStack[:len(sectionStack)-1]
			}

			parent := ""
			if len(sectionStack) > 0 {
				parent = sectionStack[len(sectionStack)-1]
			}

			section := MarkdownSection{
				Level:     level,
				Title:     title,
				StartLine: lineNum,
				EndLine:   lineNum, // Will be updated when next section is found
				Parent:    parent,
			}

			// Update end line for previous sections
			for i := len(sections) - 1; i >= 0; i-- {
				if sections[i].EndLine == sections[i].StartLine {
					sections[i].EndLine = lineNum - 1
					break
				}
			}

			sections = append(sections, section)
			sectionStack = append(sectionStack, title)
			currentSection = title
		}

		// Handle links
		if matches := linkRegex.FindAllStringSubmatch(line, -1); matches != nil {
			for _, match := range matches {
				link := MarkdownLink{
					Text:      match[1],
					URL:       match[2],
					Line:      lineNum,
					InSection: currentSection,
				}
				links = append(links, link)
			}
		}
	}

	// Update end line for the last section
	if len(sections) > 0 {
		sections[len(sections)-1].EndLine = lineNum
	}

	return sections, codeBlocks, links, scanner.Err()
}

// countLinesFromContent counts the number of lines in the given content
func (e *MarkdownASTExtractor) countLinesFromContent(content []byte) int {
	return bytes.Count(content, []byte{'\n'}) + 1
}

// analyzeEmbeddedCode analyzes code blocks for supported languages
func (e *MarkdownASTExtractor) analyzeEmbeddedCode(block MarkdownCodeBlock, parentNodeID int64) {
	// Skip analysis for very small code blocks
	if len(strings.TrimSpace(block.Content)) < 10 {
		return
	}

	// Create temporary file with the code content
	var ext string
	switch strings.ToLower(block.Language) {
	case "python", "py":
		ext = ".py"
	case "javascript", "js":
		ext = ".js"
	case "typescript", "ts":
		ext = ".ts"
	case "go", "golang":
		ext = ".go"
	default:
		return // Unsupported language for embedded analysis
	}

	tmpFile, err := os.CreateTemp("", fmt.Sprintf("markdown_code_*%s", ext))
	if err != nil {
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(block.Content); err != nil {
		return
	}

	// Use appropriate extractor based on language
	switch strings.ToLower(block.Language) {
	case "python", "py":
		_ = NewPythonASTExtractor()
		// Note: This would extract to cache, but we want to link it to the parent
		// For now, we skip full extraction of embedded code
	case "go", "golang":
		_ = NewGoASTExtractor()
		// Similar consideration
	}

	// For now, we just track that this code block exists
	// Future enhancement: fully parse embedded code and link to parent document
}

// calculateCodeBlockComplexity estimates complexity of code block
func (e *MarkdownASTExtractor) calculateCodeBlockComplexity(content, language string) int {
	complexity := 1
	lines := strings.Split(content, "\n")

	// Simple heuristic-based complexity calculation
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Common control flow keywords across languages
		controlFlow := []string{"if", "else", "elif", "for", "while", "switch", "case", "catch", "except"}
		for _, keyword := range controlFlow {
			if strings.HasPrefix(trimmed, keyword+" ") || strings.HasPrefix(trimmed, keyword+"(") {
				complexity++
				break
			}
		}

		// Logical operators
		if strings.Contains(trimmed, "&&") || strings.Contains(trimmed, "||") {
			complexity++
		}
	}

	return complexity
}

// extractPackageName extracts package name from file path
func (e *MarkdownASTExtractor) extractPackageName(filePath string) string {
	// Use filename without extension as package name
	base := filepath.Base(filePath)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	// Handle special documentation files
	switch strings.ToLower(name) {
	case "readme":
		// Use parent directory name for README files
		dir := filepath.Dir(filePath)
		if dir != "." && dir != "/" {
			parts := strings.Split(dir, string(filepath.Separator))
			if len(parts) > 0 {
				return parts[len(parts)-1] + "_readme"
			}
		}
		return "readme"
	case "changelog":
		return "changelog"
	case "contributing":
		return "contributing"
	case "license":
		return "license"
	default:
		// Convert to valid package name
		return strings.ReplaceAll(strings.ToLower(name), "-", "_")
	}
}

// countLines counts the number of lines in a file
func (e *MarkdownASTExtractor) countLines(filePath string) int {
	file, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}
	return lineCount
}
