package _go

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("Go AST Extractor", func() {
	var (
		extractor *GoASTExtractor
		astCache  *cache.ASTCache
	)

	BeforeEach(func() {
		extractor = NewGoASTExtractor()
		astCache = cache.MustGetASTCache()
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	Context("when extracting from a Go file", func() {
		var testFile string

		BeforeEach(func() {
			testFile = filepath.Join("testdata", "calculator.go")
		})

		It("should successfully extract AST nodes", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Print JSON ASTResult
			jsonBytes, err := json.MarshalIndent(result, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Printf("Go ASTResult JSON:\n%s\n", string(jsonBytes))
		})

		It("should find expected structs, methods, and functions", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Print JSON ASTResult for detailed analysis
			jsonBytes, err := json.MarshalIndent(result, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Printf("Go Detailed ASTResult JSON:\n%s\n", string(jsonBytes))

			nodes := result.Nodes

			var foundStruct, foundAdd, foundMultiply, foundMain bool
			for _, node := range nodes {
				switch {
				case node.TypeName == "Calculator" && node.NodeType == "type":
					foundStruct = true
				case node.MethodName == "Add" && node.TypeName == "Calculator":
					foundAdd = true
					Expect(node.CyclomaticComplexity).To(BeNumerically(">", 1), "Add method should have complexity > 1")
				case node.MethodName == "Multiply" && node.TypeName == "Calculator":
					foundMultiply = true
					Expect(node.CyclomaticComplexity).To(BeNumerically(">", 2), "Multiply method should have complexity > 2")
				case node.MethodName == "main":
					foundMain = true
				}
			}

			Expect(foundStruct).To(BeTrue(), "Should find Calculator struct")
			Expect(foundAdd).To(BeTrue(), "Should find Add method")
			Expect(foundMultiply).To(BeTrue(), "Should find Multiply method")
			Expect(foundMain).To(BeTrue(), "Should find main function")
		})
	})
})
