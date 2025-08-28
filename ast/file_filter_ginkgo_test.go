package ast_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("File Filtering", func() {
	var (
		astCache *cache.ASTCache
		analyzer *ast.Analyzer
	)

	BeforeEach(func() {
		astCache = cache.MustGetASTCache()
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	// Note: ShouldIncludeFile is not exported, so we test it indirectly through AnalyzeFilesWithFilter

	Describe("AnalyzeFilesWithFilter", func() {
		var tempDir string
		var testFiles []string

		BeforeEach(func() {
			tempDir = GinkgoT().TempDir()

			// Create test files
			testFiles = []string{
				"main.go",
				"main_test.go",
				"service.py",
				"config.json",
				"src/handler.go",
				"src/handler_test.go",
				"test/utils.go",
				"vendor/lib.go",
			}

			for _, file := range testFiles {
				fullPath := filepath.Join(tempDir, file)
				dir := filepath.Dir(fullPath)

				err := os.MkdirAll(dir, 0755)
				Expect(err).NotTo(HaveOccurred())

				// Create file with minimal content
				var content string
				switch filepath.Ext(file) {
				case ".go":
					content = "package main\n\nfunc main() {}\n"
				case ".py":
					content = "def main():\n    pass\n"
				default:
					content = "{}\n"
				}

				err = os.WriteFile(fullPath, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			analyzer = ast.NewAnalyzer(astCache, tempDir)
		})

		DescribeTable("Filter pattern tests",
			func(includePatterns, excludePatterns []string, description string) {
				// Clear cache before each test
				err := astCache.ClearCache()
				Expect(err).NotTo(HaveOccurred())

				err = analyzer.AnalyzeFilesWithFilter(includePatterns, excludePatterns)

				// Should not error (files might not be valid Go/Python but that's ok for this test)
				// We're mainly testing the filtering logic
				Expect(err).NotTo(HaveOccurred(), description)
			},
			Entry("Include only Go files",
				[]string{"*.go"}, []string{}, "Should analyze only Go files"),
			Entry("Exclude test files",
				[]string{}, []string{"*_test.go"}, "Should exclude test files"),
			Entry("Include Go, exclude tests",
				[]string{"*.go"}, []string{"*_test.go"}, "Should include Go files but exclude tests"),
			Entry("Exclude vendor directory",
				[]string{}, []string{"vendor/**"}, "Should exclude vendor directory"),
			Entry("Include src directory only",
				[]string{"src/**/*.go"}, []string{}, "Should only include Go files in src directory"),
		)
	})

	Describe("FileFilteringIntegration", func() {
		var tempDir string
		var structure map[string]string

		BeforeEach(func() {
			tempDir = GinkgoT().TempDir()

			// Create a realistic Go project structure
			structure = map[string]string{
				"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello World")
}`,
				"main_test.go": `package main

import "testing"

func TestMain(t *testing.T) {
	// test code
}`,
				"internal/service/user.go": `package service

type UserService struct {}

func (s *UserService) GetUser(id string) error {
	return nil
}`,
				"internal/service/user_test.go": `package service

import "testing"

func TestUserService_GetUser(t *testing.T) {
	// test code
}`,
				"vendor/external/lib.go": `package external

func ExternalFunc() {}`,
			}

			for filePath, content := range structure {
				fullPath := filepath.Join(tempDir, filePath)
				dir := filepath.Dir(fullPath)

				err := os.MkdirAll(dir, 0755)
				Expect(err).NotTo(HaveOccurred())

				err = os.WriteFile(fullPath, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			analyzer = ast.NewAnalyzer(astCache, tempDir)
		})

		It("should filter internal Go files excluding tests", func() {
			err := astCache.ClearCache()
			Expect(err).NotTo(HaveOccurred())

			err = analyzer.AnalyzeFilesWithFilter(
				[]string{"internal/**/*.go"},
				[]string{"*_test.go"},
			)
			Expect(err).NotTo(HaveOccurred())

			// Query for all nodes to see what was analyzed
			nodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())

			// Should find nodes from internal/service/user.go but not from test files
			foundMainUserService := false
			foundTestCode := false

			for _, node := range nodes {
				if node.FilePath == filepath.Join(tempDir, "internal/service/user.go") {
					foundMainUserService = true
				}
				if node.FilePath == filepath.Join(tempDir, "internal/service/user_test.go") ||
					node.FilePath == filepath.Join(tempDir, "main_test.go") {
					foundTestCode = true
				}
			}

			Expect(foundMainUserService).To(BeTrue(), "Should find code from internal/service/user.go")
			Expect(foundTestCode).To(BeFalse(), "Should not find any test code")
		})
	})

	Describe("QueryWithFiltering", func() {
		var tempDir string
		var structure map[string]string

		BeforeEach(func() {
			tempDir = GinkgoT().TempDir()

			// Create test files with mixed types
			structure = map[string]string{
				"main.go": `package main

import "fmt"

func main() {
	fmt.Println("Hello World")
	// This function has more than 3 lines
	// Adding more lines to test filtering
	var x int = 1
	var y int = 2
	fmt.Printf("Sum: %d", x+y)
}`,
				"README.md": `# Test Project

This is a test project with more than 3 lines of content.
It should be filtered out when using --exclude "*.md".
This markdown file has enough lines to match a lines(*) > 3 query.

## Section
More content here to ensure it has enough lines.
Even more content.
And more.
Final line.`,
				"service.py": `def main():
    print("Hello from Python")
    # This has more than 3 lines
    x = 1
    y = 2
    print(f"Sum: {x+y}")`,
			}

			for filePath, content := range structure {
				fullPath := filepath.Join(tempDir, filePath)
				dir := filepath.Dir(fullPath)

				err := os.MkdirAll(dir, 0755)
				Expect(err).NotTo(HaveOccurred())

				err = os.WriteFile(fullPath, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			analyzer = ast.NewAnalyzer(astCache, tempDir)
		})

		It("should include markdown files in full analysis", func() {
			// First, analyze all files normally to populate cache
			err := analyzer.AnalyzeFiles()
			Expect(err).NotTo(HaveOccurred())

			// Verify all files are in cache (should include README.md)
			allNodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())

			hasMarkdownNodes := false
			for _, node := range allNodes {
				if strings.HasSuffix(node.FilePath, ".md") {
					hasMarkdownNodes = true
					break
				}
			}
			Expect(hasMarkdownNodes).To(BeTrue(), "Should have markdown nodes in full analysis")
		})

		It("should exclude markdown files when filtered", func() {
			// Clear cache and analyze with filtering
			err := astCache.ClearCache()
			Expect(err).NotTo(HaveOccurred())

			err = analyzer.AnalyzeFilesWithFilter([]string{}, []string{"*.md"})
			Expect(err).NotTo(HaveOccurred())

			// Query again - should not have markdown files
			filteredNodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())

			hasMarkdownNodesAfterFilter := false
			for _, node := range filteredNodes {
				if strings.HasSuffix(node.FilePath, ".md") {
					hasMarkdownNodesAfterFilter = true
					break
				}
			}
			Expect(hasMarkdownNodesAfterFilter).To(BeFalse(), "Should not have markdown nodes after filtering")
		})

		It("should not return markdown files in metric queries when excluded", func() {
			// Test metric query with filtering
			err := astCache.ClearCache()
			Expect(err).NotTo(HaveOccurred())

			err = analyzer.AnalyzeFilesWithFilter([]string{}, []string{"*.md"})
			Expect(err).NotTo(HaveOccurred())

			// Execute a metric query that would match markdown files if they were included
			metricNodes, err := analyzer.ExecuteAQLQuery("lines(*) > 3")
			Expect(err).NotTo(HaveOccurred())

			hasMarkdownInMetricQuery := false
			for _, node := range metricNodes {
				if strings.HasSuffix(node.FilePath, ".md") {
					hasMarkdownInMetricQuery = true
					break
				}
			}
			Expect(hasMarkdownInMetricQuery).To(BeFalse(), "Metric query should not return markdown files when excluded")
		})
	})
})
