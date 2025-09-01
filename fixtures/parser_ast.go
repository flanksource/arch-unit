package fixtures

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// commandBlockBuilder helps build a command fixture from markdown AST
type commandBlockBuilder struct {
	name         string
	bashContent  string
	frontmatter  string
	validations  []string
	isComplete   bool
}

// parseMarkdownWithGoldmark parses markdown content using goldmark AST parser
func parseMarkdownWithGoldmark(content string, frontMatter *FrontMatter, sourceDir string) ([]FixtureNode, error) {
	var fixtures []FixtureNode
	
	// Create goldmark parser with table extension
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	
	source := []byte(content)
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)
	
	// State for parsing
	var currentCommand *commandBlockBuilder
	var inCommandBlock bool
	
	// Walk the AST
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		
		switch node := n.(type) {
		case *ast.Heading:
			// Check if this is a command heading
			if node.Level == 3 {
				headingText := extractNodeText(node, source)
				if strings.HasPrefix(strings.ToLower(headingText), "command:") {
					// Complete previous command block if exists
					if currentCommand != nil && !currentCommand.isComplete {
						if fixture := buildFixtureFromCommand(currentCommand, frontMatter, sourceDir); fixture != nil {
							fixtures = append(fixtures, *fixture)
						}
					}
					
					// Start new command block
					commandName := strings.TrimSpace(strings.TrimPrefix(headingText, "command:"))
					if commandName == "" {
						commandName = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(headingText), "command:"))
					}
					
					currentCommand = &commandBlockBuilder{
						name:        commandName,
						validations: make([]string, 0),
					}
					inCommandBlock = true
				} else {
					inCommandBlock = false
					// Complete current command if we're leaving command block
					if currentCommand != nil && !currentCommand.isComplete {
						if fixture := buildFixtureFromCommand(currentCommand, frontMatter, sourceDir); fixture != nil {
							fixtures = append(fixtures, *fixture)
						}
						currentCommand = nil
					}
				}
			}
			
		case *ast.FencedCodeBlock:
			if inCommandBlock && currentCommand != nil {
				lang := string(node.Info.Segment.Value(source))
				codeContent := extractCodeBlockContent(node, source)
				
				switch strings.ToLower(lang) {
				case "bash", "shell", "sh":
					currentCommand.bashContent = codeContent
				case "frontmatter", "yaml":
					currentCommand.frontmatter = codeContent
				}
			}
			
		case *ast.List:
			if inCommandBlock && currentCommand != nil {
				// Check if this is a validation list
				listText := extractNodeText(node, source)
				if strings.Contains(strings.ToLower(listText), "validation") || 
				   strings.Contains(listText, "cel:") ||
				   strings.Contains(listText, "regex:") ||
				   strings.Contains(listText, "contains:") {
					
					// Extract validation items
					validations := extractValidationsFromList(node, source)
					currentCommand.validations = append(currentCommand.validations, validations...)
				}
			}
			
		case *extast.Table:
			// Handle existing table format - continue to parse as before
			if !inCommandBlock {
				tableFixtures, err := parseTableFromAST(node, source, frontMatter, sourceDir)
				if err != nil {
					return ast.WalkStop, err
				}
				fixtures = append(fixtures, tableFixtures...)
			}
		}
		
		return ast.WalkContinue, nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("error walking AST: %w", err)
	}
	
	// Complete final command block if exists
	if currentCommand != nil && !currentCommand.isComplete {
		if fixture := buildFixtureFromCommand(currentCommand, frontMatter, sourceDir); fixture != nil {
			fixtures = append(fixtures, *fixture)
		}
	}
	
	return fixtures, nil
}

// extractNodeText extracts plain text content from an AST node
func extractNodeText(node ast.Node, source []byte) string {
	var buf strings.Builder
	
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if text, ok := n.(*ast.Text); ok {
				buf.Write(text.Segment.Value(source))
			}
		}
		return ast.WalkContinue, nil
	})
	
	return buf.String()
}

// extractCodeBlockContent extracts the content from a fenced code block
func extractCodeBlockContent(node *ast.FencedCodeBlock, source []byte) string {
	var buf strings.Builder
	
	for i := 0; i < node.Lines().Len(); i++ {
		line := node.Lines().At(i)
		buf.Write(line.Value(source))
	}
	
	return strings.TrimSpace(buf.String())
}

// extractValidationsFromList extracts validation expressions from a list node
func extractValidationsFromList(listNode *ast.List, source []byte) []string {
	var validations []string
	
	ast.Walk(listNode, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if listItem, ok := n.(*ast.ListItem); ok {
				itemText := extractNodeText(listItem, source)
				itemText = strings.TrimSpace(itemText)
				
				// Skip empty items
				if itemText == "" {
					return ast.WalkSkipChildren, nil
				}
				
				// Process different validation types
				if strings.HasPrefix(itemText, "cel:") {
					validations = append(validations, strings.TrimSpace(strings.TrimPrefix(itemText, "cel:")))
				} else if strings.HasPrefix(itemText, "contains:") {
					containsText := strings.TrimSpace(strings.TrimPrefix(itemText, "contains:"))
					// Remove quotes if present
					containsText = strings.Trim(containsText, `"'`)
					validations = append(validations, fmt.Sprintf(`stdout.contains("%s")`, containsText))
				} else if strings.HasPrefix(itemText, "regex:") {
					regexText := strings.TrimSpace(strings.TrimPrefix(itemText, "regex:"))
					// Remove quotes if present
					regexText = strings.Trim(regexText, `"'`)
					validations = append(validations, fmt.Sprintf(`stdout.matches("%s")`, regexText))
				} else if strings.HasPrefix(itemText, "not:") {
					notText := strings.TrimSpace(strings.TrimPrefix(itemText, "not:"))
					if strings.HasPrefix(notText, "contains:") {
						containsText := strings.TrimSpace(strings.TrimPrefix(notText, "contains:"))
						containsText = strings.Trim(containsText, `"'`)
						validations = append(validations, fmt.Sprintf(`!stdout.contains("%s")`, containsText))
					} else {
						validations = append(validations, fmt.Sprintf("!(%s)", notText))
					}
				} else if strings.Contains(itemText, ":") {
					// Generic validation format - assume it's a CEL expression
					validations = append(validations, itemText)
				}
				
				return ast.WalkSkipChildren, nil
			}
		}
		return ast.WalkContinue, nil
	})
	
	return validations
}

// buildFixtureFromCommand converts a commandBlockBuilder to a FixtureTest
func buildFixtureFromCommand(cmd *commandBlockBuilder, frontMatter *FrontMatter, sourceDir string) *FixtureNode {
	if cmd.name == "" || cmd.bashContent == "" {
		return nil
	}
	
	fixture := FixtureTest{
		Name:      cmd.name,
		CLIArgs:   cmd.bashContent,
		SourceDir: sourceDir,
		Expected: ExpectedResult{
			Properties: make(map[string]interface{}),
		},
	}
	
	// Apply frontmatter from command block if present
	if cmd.frontmatter != "" {
		var cmdFrontMatter struct {
			CWD      string            `yaml:"cwd"`
			ExitCode *int              `yaml:"exitCode"`
			Env      map[string]string `yaml:"env"`
			Timeout  string            `yaml:"timeout"`
		}
		
		if err := yaml.Unmarshal([]byte(cmd.frontmatter), &cmdFrontMatter); err == nil {
			if cmdFrontMatter.CWD != "" {
				fixture.CWD = cmdFrontMatter.CWD
			}
			if cmdFrontMatter.ExitCode != nil {
				fixture.Expected.Properties["exitCode"] = *cmdFrontMatter.ExitCode
			}
			if cmdFrontMatter.Env != nil {
				fixture.Env = cmdFrontMatter.Env
			}
		}
	}
	
	// Apply file-level frontmatter if present
	if frontMatter != nil {
		if fixture.CWD == "" && frontMatter.Env != nil {
			// Don't override command-specific CWD
		}
		if frontMatter.Exec != "" {
			fixture.Exec = frontMatter.Exec
		}
		if frontMatter.Build != "" {
			fixture.Build = frontMatter.Build
		}
		if frontMatter.Env != nil && fixture.Env == nil {
			fixture.Env = frontMatter.Env
		}
	}
	
	// Combine validations into CEL expression
	if len(cmd.validations) > 0 {
		if len(cmd.validations) == 1 {
			fixture.CEL = cmd.validations[0]
		} else {
			fixture.CEL = strings.Join(cmd.validations, " && ")
		}
	}
	
	cmd.isComplete = true
	
	return &FixtureNode{
		Type: TestNode,
		Test: &fixture,
	}
}

// parseTableFromAST parses table-based fixtures from AST (existing functionality)
func parseTableFromAST(tableNode *extast.Table, source []byte, frontMatter *FrontMatter, sourceDir string) ([]FixtureNode, error) {
	var fixtures []FixtureNode
	var headers []string
	
	// Walk through table rows
	for child := tableNode.FirstChild(); child != nil; child = child.NextSibling() {
		if tableHead, ok := child.(*extast.TableHeader); ok {
			// Extract headers
			for headerChild := tableHead.FirstChild(); headerChild != nil; headerChild = headerChild.NextSibling() {
				if cell, ok := headerChild.(*extast.TableCell); ok {
					headerText := extractNodeText(cell, source)
					headers = append(headers, strings.TrimSpace(headerText))
				}
			}
		} else if tableRow, ok := child.(*extast.TableRow); ok {
			// Extract row data
			var values []string
			for cellChild := tableRow.FirstChild(); cellChild != nil; cellChild = cellChild.NextSibling() {
				if cell, ok := cellChild.(*extast.TableCell); ok {
					cellText := extractNodeText(cell, source)
					values = append(values, strings.TrimSpace(cellText))
				}
			}
			
			// Create fixture from row
			if len(headers) > 0 && len(values) == len(headers) {
				if fixtureNode := parseTableRow(headers, values); fixtureNode != nil {
					// Apply frontmatter and source directory
					if fixtureNode.Test != nil {
						applyFrontMatterToFixture(fixtureNode.Test, frontMatter)
						fixtureNode.Test.SourceDir = sourceDir
					}
					fixtures = append(fixtures, *fixtureNode)
				}
			}
		}
	}
	
	return fixtures, nil
}

// applyFrontMatterToFixture applies frontmatter settings to a fixture
func applyFrontMatterToFixture(fixture *FixtureTest, frontMatter *FrontMatter) {
	if frontMatter == nil {
		return
	}
	
	if frontMatter.Build != "" {
		fixture.Build = frontMatter.Build
	}
	if frontMatter.Exec != "" {
		fixture.Exec = frontMatter.Exec
	}
	if frontMatter.Env != nil && fixture.Env == nil {
		fixture.Env = frontMatter.Env
	}
}