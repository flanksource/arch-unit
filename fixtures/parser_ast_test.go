package fixtures

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarkdownWithGoldmark_CommandBlocks(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectedLen int
		validate    func(t *testing.T, fixtures []FixtureNode)
	}{
		{
			name: "simple command block",
			content: `
### command: test help

` + "```bash" + `
--help
` + "```" + `

` + "```frontmatter" + `
cwd: .
exitCode: 0
` + "```" + `

Validations:
* cel: stdout.contains("Usage")
* cel: exitCode == 0
`,
			expectedLen: 1,
			validate: func(t *testing.T, fixtures []FixtureNode) {
				f := fixtures[0]
				assert.Equal(t, "test help", f.Test.Name)
				assert.Equal(t, "--help", f.Test.CLIArgs)
				assert.Equal(t, ".", f.Test.CWD)
				assert.Equal(t, 0, f.Test.Expected.Properties["exitCode"])
				assert.Contains(t, f.Test.CEL, "stdout.contains(\"Usage\")")
				assert.Contains(t, f.Test.CEL, "exitCode == 0")
			},
		},
		{
			name: "command block with different validation types",
			content: `
### command: validation types test

` + "```bash" + `
ast * --format json
` + "```" + `

` + "```frontmatter" + `
exitCode: 0
` + "```" + `

Validations:
* cel: json.length > 0
* contains: node_type
* regex: .*"file_path".*
* not: contains: error
`,
			expectedLen: 1,
			validate: func(t *testing.T, fixtures []FixtureNode) {
				f := fixtures[0]
				assert.Equal(t, "validation types test", f.Test.Name)
				assert.Equal(t, "ast * --format json", f.Test.CLIArgs)
				
				// Check that different validation types are converted correctly
				cel := f.Test.CEL
				assert.Contains(t, cel, "json.length > 0")
				assert.Contains(t, cel, "stdout.contains(\"node_type\")")
				assert.Contains(t, cel, "stdout.matches(\".*\"file_path\".*\")")
				assert.Contains(t, cel, "!stdout.contains(\"error\")")
			},
		},
		{
			name: "command block with environment variables",
			content: `
### command: env test

` + "```bash" + `
ast * --verbose
` + "```" + `

` + "```frontmatter" + `
cwd: ./test
exitCode: 0
env:
  LOG_LEVEL: debug
  OUTPUT: json
` + "```" + `

Validations:
* cel: exitCode == 0
`,
			expectedLen: 1,
			validate: func(t *testing.T, fixtures []FixtureNode) {
				f := fixtures[0]
				assert.Equal(t, "env test", f.Test.Name)
				assert.Equal(t, "./test", f.Test.CWD)
				assert.NotNil(t, f.Test.Env)
				assert.Equal(t, "debug", f.Test.Env["LOG_LEVEL"])
				assert.Equal(t, "json", f.Test.Env["OUTPUT"])
			},
		},
		{
			name: "multiple command blocks",
			content: `
### command: first test

` + "```bash" + `
--help
` + "```" + `

Validations:
* cel: exitCode == 0

### command: second test

` + "```bash" + `
--version
` + "```" + `

Validations:
* contains: arch-unit
`,
			expectedLen: 2,
			validate: func(t *testing.T, fixtures []FixtureNode) {
				assert.Equal(t, "first test", fixtures[0].Test.Name)
				assert.Equal(t, "--help", fixtures[0].Test.CLIArgs)
				assert.Equal(t, "exitCode == 0", fixtures[0].Test.CEL)
				
				assert.Equal(t, "second test", fixtures[1].Test.Name)
				assert.Equal(t, "--version", fixtures[1].Test.CLIArgs)
				assert.Equal(t, "stdout.contains(\"arch-unit\")", fixtures[1].Test.CEL)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixtures, err := parseMarkdownWithGoldmark(tt.content, nil, "/tmp/test")
			require.NoError(t, err)
			assert.Len(t, fixtures, tt.expectedLen)
			
			if tt.validate != nil {
				tt.validate(t, fixtures)
			}
		})
	}
}

func TestParseMarkdownWithGoldmark_MixedFormat(t *testing.T) {
	content := `
# Mixed Format Test

## Table Format

| Test Name | CLI Args | CEL Validation |
|-----------|----------|----------------|
| Table Test | --help | stdout.contains("Usage") |

## Command Format

### command: block test

` + "```bash" + `
ast * --format json
` + "```" + `

Validations:
* cel: stdout.contains("json")
`

	fixtures, err := parseMarkdownWithGoldmark(content, nil, "/tmp/test")
	require.NoError(t, err)
	assert.Len(t, fixtures, 2)

	// Check table fixture
	tableFixture := fixtures[0]
	assert.Equal(t, "Table Test", tableFixture.Test.Name)
	assert.Equal(t, "--help", tableFixture.Test.CLIArgs)
	assert.Equal(t, "stdout.contains(\"Usage\")", tableFixture.Test.CEL)

	// Check command block fixture
	commandFixture := fixtures[1]
	assert.Equal(t, "block test", commandFixture.Test.Name)
	assert.Equal(t, "ast * --format json", commandFixture.Test.CLIArgs)
	assert.Equal(t, "stdout.contains(\"json\")", commandFixture.Test.CEL)
}

func TestExtractValidationsFromList(t *testing.T) {
	tests := []struct {
		name         string
		listContent  string
		expectedCEL  []string
	}{
		{
			name: "cel validations",
			listContent: "* cel: stdout.contains(\"test\")\n* cel: exitCode == 0",
			expectedCEL: []string{
				"stdout.contains(\"test\")",
				"exitCode == 0",
			},
		},
		{
			name: "contains validations", 
			listContent: "* contains: expected text\n* contains: another text",
			expectedCEL: []string{
				"stdout.contains(\"expected text\")",
				"stdout.contains(\"another text\")",
			},
		},
		{
			name: "regex validations",
			listContent: "* regex: .*pattern.*\n* regex: ^start.*end$",
			expectedCEL: []string{
				"stdout.matches(\".pattern.\")",  // Asterisks are stripped by markdown list parsing
				"stdout.matches(\"^start.*end$\")",
			},
		},
		{
			name: "not validations", 
			listContent: "* not: contains: error\n* not: (stdout.contains(\"fail\"))",
			expectedCEL: []string{
				"!stdout.contains(\"error\")",
				"!((stdout.contains(\"fail\")))", // Extra parentheses preserved
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the list content as markdown and extract validations
			content := "Validations:\n" + tt.listContent
			fixtures, err := parseMarkdownWithGoldmark("### command: test\n```bash\necho\n```\n\n"+content, nil, "/tmp")
			require.NoError(t, err)
			require.Len(t, fixtures, 1)
			
			// Check that the validations were parsed correctly
			cel := fixtures[0].Test.CEL
			for _, expected := range tt.expectedCEL {
				assert.Contains(t, cel, expected)
			}
		})
	}
}

func TestBuildFixtureFromCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *commandBlockBuilder
		expected FixtureTest
	}{
		{
			name: "basic command",
			cmd: &commandBlockBuilder{
				name:        "test command",
				bashContent: "--help",
				validations: []string{"exitCode == 0"},
			},
			expected: FixtureTest{
				Name:    "test command",
				CLIArgs: "--help",
				CEL:     "exitCode == 0",
				Expected: ExpectedResult{
					Properties: make(map[string]interface{}),
				},
			},
		},
		{
			name: "command with frontmatter",
			cmd: &commandBlockBuilder{
				name:        "complex test",
				bashContent: "ast * --format json",
				frontmatter: "cwd: ./test\nexitCode: 0\nenv:\n  DEBUG: true",
				validations: []string{"stdout.contains(\"json\")", "exitCode == 0"},
			},
			expected: FixtureTest{
				Name:    "complex test",
				CLIArgs: "ast * --format json",
				CWD:     "./test",
				CEL:     "stdout.contains(\"json\") && exitCode == 0",
				Env:     map[string]string{"DEBUG": "true"},
				Expected: ExpectedResult{
					Properties: map[string]interface{}{
						"exitCode": 0,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixtureNode := buildFixtureFromCommand(tt.cmd, nil, "/tmp/test")
			require.NotNil(t, fixtureNode)
			
			fixture := *fixtureNode.Test
			assert.Equal(t, tt.expected.Name, fixture.Name)
			assert.Equal(t, tt.expected.CLIArgs, fixture.CLIArgs)
			assert.Equal(t, tt.expected.CWD, fixture.CWD)
			assert.Equal(t, tt.expected.CEL, fixture.CEL)
			
			if tt.expected.Env != nil {
				assert.Equal(t, tt.expected.Env, fixture.Env)
			}
			
			if expectedExitCode, ok := tt.expected.Expected.Properties["exitCode"]; ok {
				assert.Equal(t, expectedExitCode, fixture.Expected.Properties["exitCode"])
			}
		})
	}
}

func TestParseMarkdownWithGoldmark_ErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "command without bash block",
			content: `### command: incomplete test
Validations:
* cel: exitCode == 0`,
		},
		{
			name: "command without name",
			content: `### command:

` + "```bash" + `
--help
` + "```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixtures, err := parseMarkdownWithGoldmark(tt.content, nil, "/tmp")
			// Should not error, but should produce no fixtures for incomplete cases
			require.NoError(t, err)
			assert.Len(t, fixtures, 0)
		})
	}
}

func TestFallbackToLegacyParser(t *testing.T) {
	// Test that legacy table parsing still works
	content := `
| Test Name | CLI Args | CEL Validation |
|-----------|----------|----------------|
| Legacy Test | --help | stdout.contains("Usage") |
`

	fixtures, err := parseMarkdownContentWithSourceDir(content, nil, "/tmp")
	require.NoError(t, err)
	assert.Len(t, fixtures, 1)

	fixture := fixtures[0]
	assert.Equal(t, "Legacy Test", fixture.Test.Name)
	assert.Equal(t, "--help", fixture.Test.CLIArgs)
	assert.Equal(t, "stdout.contains(\"Usage\")", fixture.Test.CEL)
	assert.Equal(t, "/tmp", fixture.Test.SourceDir)
}