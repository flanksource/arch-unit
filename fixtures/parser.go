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

	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/api"
	"gopkg.in/yaml.v3"
)

// FixtureTest represents a single test case from the markdown table
type FixtureTest struct {
	Name      string
	CWD       string
	SourceDir string // Directory containing the fixture file
	Query     string
	CLI       string
	CLIArgs   string
	Expected  ExpectedResult
	CEL       string
	Build     string                 // Build command from front-matter
	Exec      string                 // Exec command from front-matter
	Env       map[string]string      // Environment variables from front-matter
	Metadata  map[string]interface{} // Additional metadata from front-matter
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
	defer file.Close()

	// Get the directory containing the fixture file
	sourceDir := filepath.Dir(fixtureFilePath)

	// Parse front-matter if present
	frontMatter, content, err := parseFrontMatter(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse front-matter: %w", err)
	}

	// If we have content after front-matter, parse that
	if content != "" {
		nodes, err := parseMarkdownContent(content, frontMatter)
		if err != nil {
			return nil, err
		}
		// Set SourceDir on all fixture tests
		setSourceDirOnNodes(nodes, sourceDir)
		return nodes, nil
	}

	// No front-matter found, read the entire file
	file.Seek(0, 0)
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading fixture file: %w", err)
	}

	fullContent := strings.Join(lines, "\n")
	nodes, err := parseMarkdownContent(fullContent, nil)
	if err != nil {
		return nil, err
	}
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
		file.Seek(0, 0)
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

// parseMarkdownContent parses the markdown content for fixture tables
func parseMarkdownContent(content string, frontMatter *FrontMatter) ([]FixtureNode, error) {
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
	tree := &FixtureNode{
		Name:     filepath.Base(filePath),
		Type:     FileNode,
		Children: make([]*FixtureNode, 0),
	}

	// Create file node
	fileName := filepath.Base(filePath)
	fileNode := tree.AddFileNode(fileName)

	// Parse the markdown content
	fixtures, err := ParseMarkdownFixtures(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse markdown fixtures: %w", err)
	}

	// If no fixtures found, return empty tree
	if len(fixtures) == 0 {
		return tree, nil
	}

	// Parse the file again to extract sections
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for section parsing: %w", err)
	}
	defer file.Close()

	// Parse front-matter and get content
	_, content, err := parseFrontMatter(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse front-matter: %w", err)
	}

	// If no content after front-matter, read the entire file
	if content == "" {
		file.Seek(0, 0)
		scanner := bufio.NewScanner(file)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		content = strings.Join(lines, "\n")
	}

	// Build tree structure with sections
	err = parseMarkdownContentWithSections(content, fileNode, fixtures)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sections: %w", err)
	}

	return tree, nil
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
func buildSectionPath(stack []*FixtureNode, name string) string {
	if len(stack) == 0 {
		return name
	}

	var parts []string
	for _, node := range stack {
		if node.Type != FileNode {
			parts = append(parts, node.Name)
		}
	}
	parts = append(parts, name)
	return strings.Join(parts, " > ")
}

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
