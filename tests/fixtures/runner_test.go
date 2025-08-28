package fixtures_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/tests/fixtures"
)

var (
	fixtureTests []fixtures.FixtureTest
	evaluator    *fixtures.CELEvaluator
	rootDir      string
)

var _ = BeforeSuite(func() {
	var err error

	// Get the project root directory
	rootDir, err = filepath.Abs("../..")
	Expect(err).NotTo(HaveOccurred())

	// Parse all fixture files
	fixturesDir := filepath.Join(rootDir, "tests", "fixtures")
	fixtureTests, err = fixtures.ParseAllFixtures(fixturesDir)
	Expect(err).NotTo(HaveOccurred())
	Expect(fixtureTests).NotTo(BeEmpty(), "No fixture tests found")

	GinkgoWriter.Printf("Loaded %d fixtures from %s\n", len(fixtureTests), fixturesDir)

	// Create CEL evaluator
	evaluator, err = fixtures.NewCELEvaluator()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Fixture-Based AST Tests", func() {
	Describe("AST Query Tests", func() {
		It("should run query fixtures", func() {
			queryFixtures := filterFixturesByType(fixtureTests, "query")
			Expect(queryFixtures).NotTo(BeEmpty(), "No query fixtures found")

			for _, fixture := range queryFixtures {
				By(fmt.Sprintf("Testing: %s", fixture.Name))
				// Set up working directory
				workDir := filepath.Join(rootDir, fixture.CWD)
				if fixture.CWD == "." || fixture.CWD == "" {
					workDir = rootDir
				}

				// Create AST cache and analyzer
				cacheDir := GinkgoT().TempDir()
				astCache, err := cache.NewASTCacheWithPath(cacheDir)
				Expect(err).NotTo(HaveOccurred())
				defer astCache.Close()

				analyzer := ast.NewAnalyzer(astCache, workDir)

				// Analyze files in the directory
				err = analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())

				// Execute the query
				nodes, err := analyzer.ExecuteAQLQuery(fixture.Query)
				if fixture.Expected.Error != "" {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(fixture.Expected.Error))
					return
				}
				Expect(err).NotTo(HaveOccurred())

				// Check expected count if specified
				if fixture.Expected.Count != nil {
					Expect(nodes).To(HaveLen(*fixture.Expected.Count),
						"Query '%s' returned %d nodes, expected %d",
						fixture.Query, len(nodes), *fixture.Expected.Count)
				}

				// Evaluate CEL expression if provided
				if fixture.CEL != "" && fixture.CEL != "true" {
					valid, err := evaluator.EvaluateNodes(fixture.CEL, nodes)
					Expect(err).NotTo(HaveOccurred(),
						"Failed to evaluate CEL expression: %s", fixture.CEL)
					Expect(valid).To(BeTrue(),
						"CEL validation failed for query '%s': %s",
						fixture.Query, fixture.CEL)
				}
			}
		})
	})

	Describe("AST CLI Tests", func() {
		It("should run CLI fixtures", func() {
			cliFixtures := filterFixturesByType(fixtureTests, "cli")

			for _, fixture := range cliFixtures {
				By(fmt.Sprintf("Testing CLI: %s", fixture.Name))
				// Set up working directory - but not using it directly for exec

				// Build command arguments
				args := []string{"ast"}
				if fixture.CLI != "" {
					args = append(args, fixture.CLI)
				}
				if fixture.CLIArgs != "" {
					args = append(args, strings.Fields(fixture.CLIArgs)...)
				}

				// Execute the CLI command
				cmd := exec.Command("go", append([]string{"run", "main.go"}, args...)...)
				cmd.Dir = rootDir

				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				err := cmd.Run()
				output := stdout.String() + stderr.String()

				// Check for expected error
				if fixture.Expected.Error != "" {
					if err == nil {
						Fail(fmt.Sprintf("Expected error containing '%s' but command succeeded",
							fixture.Expected.Error))
					}
					Expect(output).To(ContainSubstring(fixture.Expected.Error))
					return
				}

				// For non-error cases, command should succeed
				if err != nil {
					Fail(fmt.Sprintf("Command failed: %v\nOutput: %s", err, output))
				}

				// Check expected output
				if fixture.Expected.Output != "" {
					Expect(output).To(ContainSubstring(fixture.Expected.Output),
						"Output should contain expected text")
				}

				// Evaluate CEL expression if provided
				if fixture.CEL != "" && fixture.CEL != "true" {
					valid, err := evaluator.EvaluateOutput(fixture.CEL, output)
					Expect(err).NotTo(HaveOccurred(),
						"Failed to evaluate CEL expression: %s", fixture.CEL)
					Expect(valid).To(BeTrue(),
						"CEL validation failed for CLI command: %s",
						fixture.CEL)
				}
			}
		})
	})

	Describe("Pattern Matching Tests", func() {
		It("should run pattern fixtures", func() {
			patternFixtures := filterFixturesByPattern(fixtureTests)

			for _, fixture := range patternFixtures {
				By(fmt.Sprintf("Testing Pattern: %s", fixture.Name))
				// Set up working directory
				workDir := filepath.Join(rootDir, fixture.CWD)
				if fixture.CWD == "." || fixture.CWD == "" {
					workDir = rootDir
				}

				// Create AST cache and analyzer
				cacheDir := GinkgoT().TempDir()
				astCache, err := cache.NewASTCacheWithPath(cacheDir)
				Expect(err).NotTo(HaveOccurred())
				defer astCache.Close()

				analyzer := ast.NewAnalyzer(astCache, workDir)

				// Analyze files in the directory
				err = analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())

				// Execute pattern query
				pattern := fixture.Query
				if pattern == "" && fixture.CLI != "" {
					pattern = fixture.CLI
				}

				nodes, err := analyzer.QueryPattern(pattern)
				Expect(err).NotTo(HaveOccurred())

				// Check expected count if specified
				if fixture.Expected.Count != nil {
					Expect(nodes).To(HaveLen(*fixture.Expected.Count),
						"Pattern '%s' returned %d nodes, expected %d",
						pattern, len(nodes), *fixture.Expected.Count)
				}

				// Evaluate CEL expression if provided
				if fixture.CEL != "" && fixture.CEL != "true" {
					valid, err := evaluator.EvaluateNodes(fixture.CEL, nodes)
					Expect(err).NotTo(HaveOccurred(),
						"Failed to evaluate CEL expression: %s", fixture.CEL)
					Expect(valid).To(BeTrue(),
						"CEL validation failed for pattern '%s': %s",
						pattern, fixture.CEL)
				}
			}
		})
	})
})

// filterFixturesByType filters fixtures that match a specific type
func filterFixturesByType(fixtureList []fixtures.FixtureTest, testType string) []fixtures.FixtureTest {
	var filtered []fixtures.FixtureTest

	for _, f := range fixtureList {
		switch testType {
		case "query":
			// Queries that contain metric operations like lines(), cyclomatic(), etc.
			if f.Query != "" && (strings.Contains(f.Query, "(") || strings.Contains(f.Query, ")")) {
				filtered = append(filtered, f)
			}
		case "cli":
			if f.CLI != "" || f.CLIArgs != "" {
				filtered = append(filtered, f)
			}
		case "pattern":
			if strings.Contains(f.Query, "*") || strings.Contains(f.CLI, "*") {
				filtered = append(filtered, f)
			}
		}
	}

	return filtered
}

// filterFixturesByPattern filters fixtures that use pattern matching
func filterFixturesByPattern(fixtureList []fixtures.FixtureTest) []fixtures.FixtureTest {
	var filtered []fixtures.FixtureTest

	for _, f := range fixtureList {
		// Check if query or CLI contains wildcards
		if strings.Contains(f.Query, "*") || strings.Contains(f.CLI, "*") {
			filtered = append(filtered, f)
		}
	}

	return filtered
}
