package fixtures

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FixtureTest represents a single test case from the markdown table
type FixtureTest struct {
	Name     string                 `json:"name,omitempty"`
	CWD      string                 `json:"cwd,omitempty"`
	Query    string                 `json:"query,omitempty"`
	CLI      string                 `json:"cli,omitempty"`
	CLIArgs  string                 `json:"cli_args,omitempty"`
	Expected ExpectedResult         `json:"expected,omitempty"`
	CEL      string                 `json:"cel,omitempty"`
	Build    string                 `json:"build,omitempty"`    // Build command from front-matter
	Exec     string                 `json:"exec,omitempty"`     // Exec command from front-matter
	Env      map[string]string      `json:"env,omitempty"`      // Environment variables from front-matter
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Additional metadata from front-matter
}

// FrontMatter represents the YAML front-matter in markdown files
type FrontMatter struct {
	Build    string                 `yaml:"build"`
	Exec     string                 `yaml:"exec"`
	BaseExec string                 `yaml:"base_exec"`
	Args     []string               `yaml:"args"`
	Env      map[string]string      `yaml:"env"`
	Metadata map[string]interface{} `yaml:",inline"`
}

// ExpectedResult contains expected outcomes
type ExpectedResult struct {
	Count      *int                   `json:"count,omitempty"`
	Output     string                 `json:"output,omitempty"`
	Format     string                 `json:"format,omitempty"`
	Error      string                 `json:"error,omitempty"`
	NodeTypes  []string               `json:"node_types,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// ParseMarkdownFixtures parses markdown files containing test fixtures
func ParseMarkdownFixtures(filepath string) ([]FixtureTest, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open fixture file: %w", err)
	}
	defer file.Close()

	// Parse front-matter if present
	frontMatter, content, err := parseFrontMatter(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse front-matter: %w", err)
	}

	// If we have content after front-matter, parse that
	if content != "" {
		return parseMarkdownContent(content, frontMatter)
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
	return parseMarkdownContent(fullContent, nil)
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
func parseTableRow(headers, values []string) *FixtureTest {
	if len(headers) != len(values) {
		return nil
	}

	fixture := &FixtureTest{
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

	return fixture
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
func parseMarkdownContent(content string, frontMatter *FrontMatter) ([]FixtureTest, error) {
	var fixtures []FixtureTest
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
						if fixture.Build == "" {
							fixture.Build = frontMatter.Build
						}
						if fixture.Exec == "" {
							fixture.Exec = frontMatter.Exec
						}
						if fixture.CLI == "" && frontMatter.BaseExec != "" {
							fixture.CLI = frontMatter.BaseExec
						}
						if fixture.Env == nil && frontMatter.Env != nil {
							fixture.Env = frontMatter.Env
						}
						if fixture.Metadata == nil && frontMatter.Metadata != nil {
							fixture.Metadata = frontMatter.Metadata
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
func ParseAllFixtures(dir string) ([]FixtureTest, error) {
	var allFixtures []FixtureTest

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
