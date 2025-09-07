package fixtures

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parser AST", func() {
	Context("when parsing markdown with goldmark command blocks", func() {
		DescribeTable("should parse different command block structures",
			func(content string, expectedLen int, validateFunc func(fixtures []FixtureNode)) {
				fixtures, err := parseMarkdownWithGoldmark(content, nil, "/tmp/test")
				Expect(err).NotTo(HaveOccurred())
				Expect(fixtures).To(HaveLen(expectedLen))
				
				if validateFunc != nil {
					validateFunc(fixtures)
				}
			},
			Entry("simple command block", `
### command: test help

`+"```bash"+`
--help
`+"```"+`

`+"```frontmatter"+`
cwd: .
exitCode: 0
`+"```"+`

Validations:
* cel: stdout.contains("Usage")
* cel: exitCode == 0
`, 1, func(fixtures []FixtureNode) {
				f := fixtures[0]
				Expect(f.Test.Name).To(Equal("test help"))
				Expect(f.Test.CLIArgs).To(Equal("--help"))
				Expect(f.Test.CWD).To(Equal("."))
				Expect(f.Test.Expected.Properties["exitCode"]).To(Equal(0))
				Expect(f.Test.CEL).To(ContainSubstring("stdout.contains(\"Usage\")"))
				Expect(f.Test.CEL).To(ContainSubstring("exitCode == 0"))
			}),

			Entry("command block with different validation types", `
### command: validation types test

`+"```bash"+`
ast * --format json
`+"```"+`

`+"```frontmatter"+`
exitCode: 0
`+"```"+`

Validations:
* cel: json.length > 0
* contains: node_type
* regex: .*"file_path".*
* not: contains: error
`, 1, func(fixtures []FixtureNode) {
				f := fixtures[0]
				Expect(f.Test.Name).To(Equal("validation types test"))
				Expect(f.Test.CLIArgs).To(Equal("ast * --format json"))
				
				// Check that different validation types are converted correctly
				cel := f.Test.CEL
				Expect(cel).To(ContainSubstring("json.length > 0"))
				Expect(cel).To(ContainSubstring("stdout.contains(\"node_type\")"))
				Expect(cel).To(ContainSubstring("stdout.matches(\".\\\"file_path\\\".\")"))
				Expect(cel).To(ContainSubstring("!stdout.contains(\"error\")"))
			}),

			Entry("command block with environment variables", `
### command: env test

`+"```bash"+`
ast * --verbose
`+"```"+`

`+"```frontmatter"+`
cwd: ./test
exitCode: 0
env:
  LOG_LEVEL: debug
  OUTPUT: json
`+"```"+`

Validations:
* cel: exitCode == 0
`, 1, func(fixtures []FixtureNode) {
				f := fixtures[0]
				Expect(f.Test.Name).To(Equal("env test"))
				Expect(f.Test.CWD).To(Equal("./test"))
				Expect(f.Test.Env).NotTo(BeNil())
				Expect(f.Test.Env["LOG_LEVEL"]).To(Equal("debug"))
				Expect(f.Test.Env["OUTPUT"]).To(Equal("json"))
			}),

			Entry("multiple command blocks", `
### command: first test

`+"```bash"+`
--help
`+"```"+`

Validations:
* cel: exitCode == 0

### command: second test

`+"```bash"+`
--version
`+"```"+`

Validations:
* contains: arch-unit
`, 2, func(fixtures []FixtureNode) {
				Expect(fixtures[0].Test.Name).To(Equal("first test"))
				Expect(fixtures[0].Test.CLIArgs).To(Equal("--help"))
				Expect(fixtures[0].Test.CEL).To(Equal("exitCode == 0"))
				
				Expect(fixtures[1].Test.Name).To(Equal("second test"))
				Expect(fixtures[1].Test.CLIArgs).To(Equal("--version"))
				Expect(fixtures[1].Test.CEL).To(Equal("stdout.contains(\"arch-unit\")"))
			}),
		)
	})

	Context("when parsing mixed format markdown", func() {
		It("should handle both table and command block formats", func() {
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
			Expect(err).NotTo(HaveOccurred())
			Expect(fixtures).To(HaveLen(2))

			// Check table fixture
			tableFixture := fixtures[0]
			Expect(tableFixture.Test.Name).To(Equal("Table Test"))
			Expect(tableFixture.Test.CLIArgs).To(Equal("--help"))
			Expect(tableFixture.Test.CEL).To(Equal("stdout.contains(\"Usage\")"))

			// Check command block fixture
			commandFixture := fixtures[1]
			Expect(commandFixture.Test.Name).To(Equal("block test"))
			Expect(commandFixture.Test.CLIArgs).To(Equal("ast * --format json"))
			Expect(commandFixture.Test.CEL).To(Equal("stdout.contains(\"json\")"))
		})
	})

	Context("when extracting validations from list", func() {
		DescribeTable("should convert different validation types to CEL",
			func(listContent string, expectedCEL []string) {
				// Parse the list content as markdown and extract validations
				content := "Validations:\n" + listContent
				fixtures, err := parseMarkdownWithGoldmark("### command: test\n```bash\necho\n```\n\n"+content, nil, "/tmp")
				Expect(err).NotTo(HaveOccurred())
				Expect(fixtures).To(HaveLen(1))
				
				// Check that the validations were parsed correctly
				cel := fixtures[0].Test.CEL
				for _, expected := range expectedCEL {
					Expect(cel).To(ContainSubstring(expected))
				}
			},
			Entry("cel validations", 
				"* cel: stdout.contains(\"test\")\n* cel: exitCode == 0",
				[]string{"stdout.contains(\"test\")", "exitCode == 0"}),

			Entry("contains validations", 
				"* contains: expected text\n* contains: another text",
				[]string{"stdout.contains(\"expected text\")", "stdout.contains(\"another text\")"}),

			Entry("regex validations",
				"* regex: .*pattern.*\n* regex: ^start.*end$",
				[]string{"stdout.matches(\".pattern.\")", "stdout.matches(\"^start.*end$\")"}),

			Entry("not validations", 
				"* not: contains: error\n* not: (stdout.contains(\"fail\"))",
				[]string{"!stdout.contains(\"error\")", "!((stdout.contains(\"fail\")))"}),
		)
	})

	Context("when building fixture from command", func() {
		DescribeTable("should build correct fixture structure",
			func(cmd *commandBlockBuilder, expectedTest FixtureTest) {
				fixtureNode := buildFixtureFromCommand(cmd, nil, "/tmp/test")
				Expect(fixtureNode).NotTo(BeNil())
				
				fixture := *fixtureNode.Test
				Expect(fixture.Name).To(Equal(expectedTest.Name))
				Expect(fixture.CLIArgs).To(Equal(expectedTest.CLIArgs))
				Expect(fixture.CWD).To(Equal(expectedTest.CWD))
				Expect(fixture.CEL).To(Equal(expectedTest.CEL))
				
				if expectedTest.Env != nil {
					Expect(fixture.Env).To(Equal(expectedTest.Env))
				}
				
				if expectedExitCode, ok := expectedTest.Expected.Properties["exitCode"]; ok {
					Expect(fixture.Expected.Properties["exitCode"]).To(Equal(expectedExitCode))
				}
			},
			Entry("basic command",
				&commandBlockBuilder{
					name:        "test command",
					bashContent: "--help",
					validations: []string{"exitCode == 0"},
				},
				FixtureTest{
					Name:    "test command",
					CLIArgs: "--help",
					CEL:     "exitCode == 0",
					Expected: ExpectedResult{
						Properties: make(map[string]interface{}),
					},
				}),

			Entry("command with frontmatter",
				&commandBlockBuilder{
					name:        "complex test",
					bashContent: "ast * --format json",
					frontmatter: "cwd: ./test\nexitCode: 0\nenv:\n  DEBUG: true",
					validations: []string{"stdout.contains(\"json\")", "exitCode == 0"},
				},
				FixtureTest{
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
				}),
		)
	})

	Context("when parsing error cases", func() {
		DescribeTable("should handle incomplete command structures gracefully",
			func(content string) {
				fixtures, err := parseMarkdownWithGoldmark(content, nil, "/tmp")
				// Should not error, but should produce no fixtures for incomplete cases
				Expect(err).NotTo(HaveOccurred())
				Expect(fixtures).To(HaveLen(0))
			},
			Entry("command without bash block", `### command: incomplete test
Validations:
* cel: exitCode == 0`),

			Entry("command without name", `### command:

`+"```bash"+`
--help
`+"```"),
		)
	})

	Context("when falling back to legacy parser", func() {
		It("should handle legacy table parsing", func() {
			// Test that legacy table parsing still works
			content := `
| Test Name | CLI Args | CEL Validation |
|-----------|----------|----------------|
| Legacy Test | --help | stdout.contains("Usage") |
`

			fixtures, err := parseMarkdownContentWithSourceDir(content, nil, "/tmp")
			Expect(err).NotTo(HaveOccurred())
			Expect(fixtures).To(HaveLen(1))

			fixture := fixtures[0]
			Expect(fixture.Test.Name).To(Equal("Legacy Test"))
			Expect(fixture.Test.CLIArgs).To(Equal("--help"))
			Expect(fixture.Test.CEL).To(Equal("stdout.contains(\"Usage\")"))
			Expect(fixture.Test.SourceDir).To(Equal("/tmp"))
		})
	})
})