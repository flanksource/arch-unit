package javascript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("JavaScript AST Extractor", func() {
	var (
		extractor *JavaScriptASTExtractor
		astCache  *cache.ASTCache
	)

	BeforeEach(func() {
		extractor = NewJavaScriptASTExtractor()
		astCache = cache.MustGetASTCache()
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	Context("when node is available", func() {
		BeforeEach(func() {
			// Skip if node is not installed
			if _, err := os.Stat("/usr/bin/node"); os.IsNotExist(err) {
				if _, err := os.Stat("/usr/local/bin/node"); os.IsNotExist(err) {
					Skip("Node.js not installed")
				}
			}
		})

		Context("when extracting from a JavaScript file", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join("testdata", "user_service.js")
			})

			It("should successfully extract JavaScript nodes or skip gracefully", func() {
				content, err := os.ReadFile(testFile)
				Expect(err).NotTo(HaveOccurred())

				result, err := extractor.ExtractFile(astCache, testFile, content)
				if err != nil {
					// If acorn is not installed globally, skip
					if strings.Contains(err.Error(), "acorn") {
						Skip("JavaScript extraction failed (likely missing acorn): " + err.Error())
					}
					Fail("Unexpected error: " + err.Error())
				}

				Expect(result).NotTo(BeNil())
				Expect(result.Nodes).NotTo(BeEmpty(), "Should extract JavaScript nodes")

				// Print JSON ASTResult
				jsonBytes, err := json.MarshalIndent(result, "", "  ")
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("JavaScript ASTResult JSON:\n%s\n", string(jsonBytes))
			})
		})
	})

	Describe("TypeScript AST Extractor", func() {
		var extractor *TypeScriptASTExtractor

		BeforeEach(func() {
			extractor = NewTypeScriptASTExtractor()
		})

		Context("when node and typescript are available", func() {
			BeforeEach(func() {
				// Skip if node or typescript is not installed
				if _, err := os.Stat("/usr/bin/node"); os.IsNotExist(err) {
					if _, err := os.Stat("/usr/local/bin/node"); os.IsNotExist(err) {
						Skip("Node.js not installed")
					}
				}
			})

			Context("when extracting from a TypeScript file", func() {
				var testFile string

				BeforeEach(func() {
					testFile = filepath.Join("testdata", "user_repository.ts")
				})

				It("should successfully extract TypeScript nodes or skip gracefully", func() {
					content, err := os.ReadFile(testFile)
					Expect(err).NotTo(HaveOccurred())

					result, err := extractor.ExtractFile(astCache, testFile, content)
					if err != nil {
						// If typescript is not installed globally, skip
						if strings.Contains(err.Error(), "typescript") {
							Skip("TypeScript extraction failed (likely missing typescript): " + err.Error())
						}
						Fail("Unexpected error: " + err.Error())
					}

					Expect(result).NotTo(BeNil())
					Expect(result.Nodes).NotTo(BeEmpty(), "Should extract TypeScript nodes")

					// Print JSON ASTResult
					jsonBytes, err := json.MarshalIndent(result, "", "  ")
					Expect(err).NotTo(HaveOccurred())
					fmt.Printf("TypeScript ASTResult JSON:\n%s\n", string(jsonBytes))
				})
			})
		})
	})
})
