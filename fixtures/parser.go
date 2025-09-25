package fixtures

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/api"
	"gopkg.in/yaml.v3"
)

// FixtureTest represents a single test case from the markdown table
type FixtureTest struct {
	Name         string
	CWD          string
	SourceDir    string                 // Directory containing the fixture file
	Query        string
	CLI          string
	CLIArgs      string
	Expected     ExpectedResult
	CEL          string
	Build        string                 // Build command from front-matter
	Exec         string                 // Exec command from front-matter
	Env          map[string]string      // Environment variables from front-matter
	Metadata     map[string]interface{} // Additional metadata from front-matter
	TemplateVars map[string]string      // Template variables (.file, .filename, .dir)
}

func (fixture FixtureTest) String() string {
	if fixture.Exec != "" {
		if fixture.CLIArgs != "" {
			return fmt.Sprintf("%s %s", fixture.Exec, fixture.CLIArgs)
		}
		return fixture.Exec
	}
	if fixture.CLI != "" {
		return fixture.CLI
	}
	if fixture.CLIArgs != "" {
		return fixture.CLIArgs
	}
	if fixture.Query != "" {
		return fmt.Sprintf("query: %s", fixture.Query)
	}
	return "unknown command"
}

func (f FixtureTest) Pretty() api.Text {
	return clicky.Text(f.Name).Append(fmt.Sprintf(" (%s)", f.String()), "text-gray-500")
}

// FrontMatter represents the YAML front-matter in markdown files
type FrontMatter struct {
	Build    string                 `yaml:"build"`
	Exec     string                 `yaml:"exec"`
	BaseExec string                 `yaml:"base_exec"`
	Args     []string               `yaml:"args"`
	Env      map[string]string      `yaml:"env"`
	Files    string                 `yaml:"files"`    // Glob pattern to match files
	Timeout  *time.Duration         `yaml:"timeout,omitempty"`
	Metadata map[string]interface{} `yaml:",inline"`
}

// ExpectedResult contains expected outcomes
type ExpectedResult struct {
	Count      *int
	Output     string
	Format     string
	Error      string
	NodeTypes  []string
	Properties map[string]interface{}
}

// ParseMarkdownFixtures parses markdown files containing test fixtures
func ParseMarkdownFixtures(fixtureFilePath string) ([]FixtureNode, error) {
	file, err := os.Open(fixtureFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open fixture file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Get the directory containing the fixture file
	sourceDir := filepath.Dir(fixtureFilePath)

	// Parse front-matter if present
	frontMatter, content, err := parseFrontMatter(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse front-matter: %w", err)
	}

	// If we have content after front-matter, parse that
	if content != "" {
		nodes, err := parseMarkdownContentWithSourceDir(content, frontMatter, sourceDir)
		if err != nil {
			return nil, err
		}
		// Expand fixtures for file patterns if specified
		nodes, err = expandFixturesForFiles(nodes, frontMatter, sourceDir)
		if err != nil {
			return nil, fmt.Errorf("failed to expand fixtures for files: %w", err)
		}
		// Set SourceDir on all fixture tests
		setSourceDirOnNodes(nodes, sourceDir)
		return nodes, nil
	}

	// No front-matter found, read the entire file
	_, _ = file.Seek(0, 0)
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading fixture file: %w", err)
	}

	fullContent := strings.Join(lines, "\n")
	nodes, err := parseMarkdownContentWithSourceDir(fullContent, nil, sourceDir)
	if err != nil {
		return nil, err
	}
	// No file expansion without frontmatter
	// Set SourceDir on all fixture tests
	setSourceDirOnNodes(nodes, sourceDir)
	return nodes, nil
}

// splitTableRow splits a markdown table row into cells
func splitTableRow(row string) []string {
	// Remove leading and trailing pipes
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")

	// Split by pipe and trim spaces
	parts := strings.Split(row, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	return parts
}

// parseTableRow converts a table row into a FixtureTest
func parseTableRow(headers, values []string) *FixtureNode {
	if len(headers) != len(values) {
		return nil
	}

	fixture := FixtureTest{
		Expected: ExpectedResult{
			Properties: make(map[string]interface{}),
		},
	}

	for i, header := range headers {
		value := values[i]
		header = strings.ToLower(strings.TrimSpace(header))

		switch header {
		case "test name", "name":
			fixture.Name = value
		case "cwd", "working directory", "dir":
			fixture.CWD = value
		case "query":
			fixture.Query = value
		case "cli", "command":
			fixture.CLI = value
		case "cli args", "args", "arguments":
			fixture.CLIArgs = value
		case "expected count", "count":
			if value != "" && value != "-" {
				count, err := strconv.Atoi(value)
				if err == nil {
					fixture.Expected.Count = &count
				}
			}
		case "expected output", "output":
			fixture.Expected.Output = value
		case "expected format", "format":
			fixture.Expected.Format = value
		case "expected error", "error":
			fixture.Expected.Error = value
		case "expected matches", "matches":
			fixture.Expected.Output = value
		case "expected results", "results":
			fixture.Expected.Output = value
		case "expected files", "files":
			fixture.Expected.Output = value
		case "template output":
			fixture.Expected.Output = value
		case "cel validation", "cel", "validation":
			fixture.CEL = value
		default:
			// Store unknown headers as properties
			fixture.Expected.Properties[header] = value
		}
	}

	// Don't return fixtures without names
	if fixture.Name == "" {
		return nil
	}

	return &FixtureNode{
		Type: TestNode,
		Test: &fixture,
	}
}

// parseFrontMatter extracts YAML front-matter from a markdown file
func parseFrontMatter(file *os.File) (*FrontMatter, string, error) {
	scanner := bufio.NewScanner(file)

	// Check for front-matter delimiter
	if !scanner.Scan() {
		return nil, "", nil
	}

	firstLine := strings.TrimSpace(scanner.Text())
	if firstLine != "---" {
		// No front-matter
		_, _ = file.Seek(0, 0)
		return nil, "", nil
	}

	// Collect front-matter lines
	var frontMatterLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			// End of front-matter
			break
		}
		frontMatterLines = append(frontMatterLines, line)
	}

	// Parse YAML front-matter
	frontMatterYAML := strings.Join(frontMatterLines, "\n")
	var frontMatter FrontMatter
	if err := yaml.Unmarshal([]byte(frontMatterYAML), &frontMatter); err != nil {
		return nil, "", fmt.Errorf("failed to parse YAML front-matter: %w", err)
	}

	// Collect remaining content
	var contentLines []string
	for scanner.Scan() {
		contentLines = append(contentLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, "", err
	}

	content := strings.Join(contentLines, "\n")
	return &frontMatter, content, nil
}

// parseMarkdownContentWithSourceDir parses markdown content with source directory context
func parseMarkdownContentWithSourceDir(content string, frontMatter *FrontMatter, sourceDir string) ([]FixtureNode, error) {
	return parseMarkdownContentLegacy(content, frontMatter, sourceDir)
}

// parseMarkdownContentLegacy is the original string-based parser for backward compatibility
func parseMarkdownContentLegacy(content string, frontMatter *FrontMatter, sourceDir string) ([]FixtureNode, error) {
	var fixtures []FixtureNode
	var lines []string

	if content != "" {
		lines = strings.Split(content, "\n")
	} else {
		// Read from stdin or handle as needed
		return fixtures, nil
	}

	var inTable bool
	var headers []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and non-table content
		if line == "" || strings.HasPrefix(line, "#") {
			inTable = false
			headers = nil
			continue
		}

		// Check if this is a table line
		if strings.HasPrefix(line, "|") {
			parts := splitTableRow(line)

			// Check if this is the separator line
			if len(parts) > 0 && strings.Contains(parts[0], "---") {
				inTable = true
				continue
			}

			// If we haven't captured headers yet, this must be the header row
			if !inTable && headers == nil {
				headers = parts
				continue
			}

			// This is a data row
			if inTable && headers != nil {
				fixture := parseTableRow(headers, parts)
				if fixture != nil {
					// Set source directory
					fixture.Test.SourceDir = sourceDir
					
					// Apply front-matter data to fixture
					if frontMatter != nil {
						if fixture.Test.Build == "" {
							fixture.Test.Build = frontMatter.Build
						}
						if fixture.Test.Exec == "" {
							fixture.Test.Exec = frontMatter.Exec
						}
						if fixture.Test.CLI == "" && frontMatter.BaseExec != "" {
							fixture.Test.CLI = frontMatter.BaseExec
						}
						if fixture.Test.Env == nil && frontMatter.Env != nil {
							fixture.Test.Env = frontMatter.Env
						}
						if fixture.Test.Metadata == nil && frontMatter.Metadata != nil {
							fixture.Test.Metadata = frontMatter.Metadata
						}
					}
					fixtures = append(fixtures, *fixture)
				}
			}
		}
	}

	return fixtures, nil
}

// ParseAllFixtures parses all markdown files in a directory
func ParseAllFixtures(dir string) ([]FixtureNode, error) {
	var allFixtures []FixtureNode

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixtures directory: %w", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".md") {
			filepath := dir + "/" + file.Name()
			fixtures, err := ParseMarkdownFixtures(filepath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", file.Name(), err)
			}
			allFixtures = append(allFixtures, fixtures...)
		}
	}

	return allFixtures, nil
}

// GroupFixturesByCategory groups fixtures by their markdown file or section
func GroupFixturesByCategory(fixtures []FixtureTest) map[string][]FixtureTest {
	grouped := make(map[string][]FixtureTest)

	for _, fixture := range fixtures {
		// Use CWD as a simple categorization for now
		category := fixture.CWD
		if category == "" {
			category = "default"
		}
		grouped[category] = append(grouped[category], fixture)
	}

	return grouped
}

// GetFixtureByName finds a specific fixture by name
func GetFixtureByName(fixtures []FixtureTest, name string) *FixtureTest {
	for _, fixture := range fixtures {
		if fixture.Name == name {
			return &fixture
		}
	}
	return nil
}

// ParseMarkdownFixturesWithTree parses markdown files into a hierarchical tree structure
func ParseMarkdownFixturesWithTree(filePath string) (*FixtureNode, error) {
	fileTree := &FixtureNode{
		Name:     filepath.Base(filePath),
		Type:     FileNode,
		Children: make([]*FixtureNode, 0),
	}

	// Get the directory containing the fixture file
	sourceDir := filepath.Dir(filePath)

	// Parse the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Parse front-matter and get content
	frontMatter, content, err := parseFrontMatter(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse front-matter: %w", err)
	}

	// If no content after front-matter, read the entire file
	if content == "" {
		_, _ = file.Seek(0, 0)
		scanner := bufio.NewScanner(file)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		content = strings.Join(lines, "\n")
	}

	// Use the new AST-based parser to build the tree directly
	contentTree, err := parseMarkdownWithGoldmarkTree(content, frontMatter, sourceDir)
	if err != nil {
		// Fall back to legacy parsing if AST parsing fails
		fixtures, parseErr := parseMarkdownContentLegacy(content, frontMatter, sourceDir)
		if parseErr != nil {
			return nil, fmt.Errorf("both AST and legacy parsing failed. AST error: %w, Legacy error: %v", err, parseErr)
		}
		
		// Build tree using legacy method
		legacyErr := parseMarkdownContentWithSections(content, fileTree, fixtures)
		if legacyErr != nil {
			return nil, fmt.Errorf("failed to build tree from legacy parsing: %w", legacyErr)
		}
		return fileTree, nil
	}

	// Move children from content tree to file tree
	for _, child := range contentTree.Children {
		fileTree.AddChild(child)
	}

	return fileTree, nil
}

// parseMarkdownContentWithSections parses markdown content and builds section hierarchy
func parseMarkdownContentWithSections(content string, fileNode *FixtureNode, fixtures []FixtureNode) error {
	lines := strings.Split(content, "\n")

	// Track section hierarchy with stack
	var sectionStack []*FixtureNode
	var currentSection *FixtureNode
	var fixtureIndex int

	// Regex for markdown headers
	headerRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for markdown headers
		if matches := headerRegex.FindStringSubmatch(line); matches != nil {
			level := len(matches[1])
			sectionName := strings.TrimSpace(matches[2])

			// Create section node
			sectionNode := &FixtureNode{
				Name:     sectionName,
				Type:     SectionNode,
				Children: make([]*FixtureNode, 0),
			}

			// Adjust section stack based on level
			adjustSectionStack(&sectionStack, level-1)

			// Add to parent (file or parent section)
			parent := fileNode
			if len(sectionStack) > 0 {
				parent = sectionStack[len(sectionStack)-1]
			}
			parent.AddChild(sectionNode)

			// Update stack
			if len(sectionStack) >= level-1 {
				sectionStack = sectionStack[:level-1]
			}
			sectionStack = append(sectionStack, sectionNode)
			currentSection = sectionNode
			continue
		}

		// Check if this might be a table row (contains pipes)
		if strings.Contains(line, "|") && fixtureIndex < len(fixtures) {
			// This could be where a fixture belongs
			// For now, we'll assign fixtures to the current section
			// In a more sophisticated parser, we'd track table parsing state

			// Skip if this is a header separator (contains ---)
			if strings.Contains(line, "---") {
				continue
			}

			// Check if this looks like a data row by seeing if we have a fixture to assign
			if fixtureIndex < len(fixtures) {
				fixture := fixtures[fixtureIndex]

				// Skip if fixture doesn't have a test
				if fixture.Test == nil {
					fixtureIndex++
					continue
				}

				// Create test node
				testNode := &FixtureNode{
					Name:     fixture.Test.Name,
					Type:     TestNode,
					Test:     fixture.Test,
					Children: make([]*FixtureNode, 0),
				}

				// Add to current section or file if no section
				parent := fileNode
				if currentSection != nil {
					parent = currentSection
				}
				parent.AddChild(testNode)

				fixtureIndex++
			}
		}
	}

	// If there are remaining fixtures not assigned to sections, add them to the file
	for fixtureIndex < len(fixtures) {
		fixture := fixtures[fixtureIndex]

		// Skip if fixture doesn't have a test
		if fixture.Test == nil {
			fixtureIndex++
			continue
		}

		testNode := &FixtureNode{
			Name:     fixture.Test.Name,
			Type:     TestNode,
			Test:     fixture.Test,
			Children: make([]*FixtureNode, 0),
		}
		fileNode.AddChild(testNode)
		fixtureIndex++
	}

	return nil
}

// buildSectionPath constructs a section path from the stack

// adjustSectionStack adjusts the section stack to the correct level
func adjustSectionStack(stack *[]*FixtureNode, targetLevel int) {
	if targetLevel < 0 {
		*stack = []*FixtureNode{}
		return
	}

	if len(*stack) > targetLevel {
		*stack = (*stack)[:targetLevel]
	}
}

// setSourceDirOnNodes recursively sets the SourceDir on all fixture tests in the node tree
func setSourceDirOnNodes(nodes []FixtureNode, sourceDir string) {
	for i := range nodes {
		setSourceDirOnNode(&nodes[i], sourceDir)
	}
}

// setSourceDirOnNode recursively sets SourceDir on a node and its children
func setSourceDirOnNode(node *FixtureNode, sourceDir string) {
	if node.Test != nil {
		node.Test.SourceDir = sourceDir
	}
	for _, child := range node.Children {
		setSourceDirOnNode(child, sourceDir)
	}
}

// expandFixturesForFiles expands a fixture into multiple fixtures based on file glob pattern
func expandFixturesForFiles(fixtures []FixtureNode, frontMatter *FrontMatter, sourceDir string) ([]FixtureNode, error) {
	// If no files pattern specified, return fixtures as-is
	if frontMatter == nil || frontMatter.Files == "" {
		return fixtures, nil
	}

	var expandedFixtures []FixtureNode

	// Find files matching the glob pattern
	// Start search from the source directory (where the fixture file is located)
	searchPath := sourceDir
	pattern := frontMatter.Files

	// If pattern is absolute, use it directly
	if filepath.IsAbs(pattern) {
		searchPath = "/"
	} else {
		// Make pattern relative to search path
		pattern = filepath.Join(searchPath, pattern)
	}

	// Use doublestar to find matching files
	matches, err := doublestar.FilepathGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern '%s': %w", frontMatter.Files, err)
	}

	// If no matches found, return original fixtures
	if len(matches) == 0 {
		return fixtures, nil
	}

	// For each matched file, create a copy of each fixture with template variables
	for _, matchedFile := range matches {
		// Get file info
		absFile, err := filepath.Abs(matchedFile)
		if err != nil {
			continue
		}

		fileInfo, err := os.Stat(absFile)
		if err != nil || fileInfo.IsDir() {
			continue // Skip directories
		}

		// Calculate template variables
		fileDir := filepath.Dir(absFile)
		fileName := filepath.Base(absFile)
		fileNameNoExt := strings.TrimSuffix(fileName, filepath.Ext(fileName))

		// Make paths relative to source directory if possible
		relFile, _ := filepath.Rel(sourceDir, absFile)
		relDir, _ := filepath.Rel(sourceDir, fileDir)
		
		// Create template variables map
		templateVars := map[string]string{
			"file":     relFile,                     // Relative path to file
			"filename": fileNameNoExt,               // Filename without extension
			"dir":      relDir,                      // Directory containing the file
			"absfile":  absFile,                     // Absolute path to file
			"absdir":   fileDir,                     // Absolute directory
			"basename": fileName,                    // Full filename with extension
			"ext":      filepath.Ext(fileName),      // File extension
		}

		// Create a copy of each fixture with the template variables
		for _, fixture := range fixtures {
			// Deep copy the fixture
			expandedFixture := fixture
			if expandedFixture.Test != nil {
				// Create a new test copy
				testCopy := *expandedFixture.Test
				
				// Update the test name to include the file
				if testCopy.Name != "" {
					testCopy.Name = fmt.Sprintf("%s [%s]", testCopy.Name, relFile)
				}
				
				// Set template variables
				testCopy.TemplateVars = templateVars
				
				expandedFixture.Test = &testCopy
			}
			
			expandedFixtures = append(expandedFixtures, expandedFixture)
		}
	}

	// If we expanded fixtures, return the expanded list; otherwise return original
	if len(expandedFixtures) > 0 {
		return expandedFixtures, nil
	}
	
	return fixtures, nil
}
