package analysis_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AST Extractors", func() {
	It("should run language-specific extractor tests in their respective packages", func() {
		// This test file serves as a placeholder
		// Actual extractor tests have been moved to their respective language packages:
		// - Python tests: analysis/python/python_ast_extractor_test.go
		// - JavaScript/TypeScript tests: analysis/javascript/javascript_ast_extractor_test.go
		// - Go tests: analysis/go/go_ast_extractor_test.go
		// - Markdown tests: analysis/markdown/markdown_ast_extractor_test.go
		//
		// Each package now uses testdata directories instead of inline source code.
		Expect(true).To(BeTrue(), "Language-specific tests moved to respective packages")
	})
})
