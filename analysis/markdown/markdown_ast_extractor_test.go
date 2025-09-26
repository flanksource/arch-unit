package markdown

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("Markdown AST Extractor", func() {
	var (
		extractor *MarkdownASTExtractor
		astCache  *cache.ASTCache
	)

	BeforeEach(func() {
		extractor = NewMarkdownASTExtractor()
		astCache = cache.MustGetASTCache()
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	Context("when extracting from a Markdown file", func() {
		var testFile string

		BeforeEach(func() {
			testFile = filepath.Join("testdata", "README.md")
		})

		It("should successfully extract AST nodes", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())
		})

		It("should find expected document structure and code blocks", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			nodes := result.Nodes

			var foundPackage, foundInstallation, foundUsage, foundAPI bool
			var codeBlockCount int

			for _, node := range nodes {
				// Check code blocks first (before checking sections)
				if node.NodeType == "method" && strings.HasPrefix(node.MethodName, "code_") {
					codeBlockCount++
				}

				switch {
				case node.NodeType == "package":
					foundPackage = true
				case node.TypeName == "Installation":
					foundInstallation = true
				case node.TypeName == "Usage":
					foundUsage = true
				case node.TypeName == "API Reference":
					foundAPI = true
				}
			}

			Expect(foundPackage).To(BeTrue(), "Should find document package node")
			Expect(foundInstallation).To(BeTrue(), "Should find Installation section")
			Expect(foundUsage).To(BeTrue(), "Should find Usage section")
			Expect(foundAPI).To(BeTrue(), "Should find API Reference section")
			Expect(codeBlockCount).To(Equal(3), "Should find 3 code blocks")
		})
	})
})
