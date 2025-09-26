package python

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("Python AST Extractor", func() {
	var (
		extractor *PythonASTExtractor
		astCache  *cache.ASTCache
	)

	BeforeEach(func() {
		extractor = NewPythonASTExtractor()
		astCache = cache.MustGetASTCache()
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	Context("when extracting from a Python file with classes and methods", func() {
		var testFile string

		BeforeEach(func() {
			testFile = filepath.Join("testdata", "calculator.py")
		})

		It("should successfully extract AST nodes", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result).To(BeAssignableToTypeOf(&types.ASTResult{}))
			Expect(result.Nodes).NotTo(BeEmpty())

			// Print JSON ASTResult
			jsonBytes, err := json.MarshalIndent(result, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Printf("Python ASTResult JSON:\n%s\n", string(jsonBytes))
		})

		It("should find expected classes, methods, and functions", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Print JSON ASTResult for detailed analysis
			jsonBytes, err := json.MarshalIndent(result, "", "  ")
			Expect(err).NotTo(HaveOccurred())
			fmt.Printf("Python Detailed ASTResult JSON:\n%s\n", string(jsonBytes))

			nodes := result.Nodes

			var foundClass, foundInit, foundAdd, foundMultiply, foundMain bool
			for _, node := range nodes {
				switch {
				case node.TypeName == "Calculator" && node.NodeType == "type":
					foundClass = true
				case node.MethodName == "__init__" && node.TypeName == "Calculator":
					foundInit = true
				case node.MethodName == "add" && node.TypeName == "Calculator":
					foundAdd = true
					Expect(node.CyclomaticComplexity).To(BeNumerically(">", 1), "add method should have complexity > 1")
				case node.MethodName == "multiply" && node.TypeName == "Calculator":
					foundMultiply = true
					Expect(node.CyclomaticComplexity).To(BeNumerically(">", 2), "multiply method should have complexity > 2")
				case node.MethodName == "main":
					foundMain = true
				}
			}

			Expect(foundClass).To(BeTrue(), "Should find Calculator class")
			Expect(foundInit).To(BeTrue(), "Should find __init__ method")
			Expect(foundAdd).To(BeTrue(), "Should find add method")
			Expect(foundMultiply).To(BeTrue(), "Should find multiply method")
			Expect(foundMain).To(BeTrue(), "Should find main function")
		})
	})
})
