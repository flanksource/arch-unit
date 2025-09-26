package java

import (
	"github.com/flanksource/arch-unit/analysis"
)

// init registers the Java AST extractor and dependency scanner
func init() {
	// Register Java AST extractor
	javaExtractor := NewJavaASTExtractor()
	analysis.RegisterExtractor("java", javaExtractor)

	// Register Java dependency scanner
	javaDependencyScanner := NewJavaDependencyScanner()
	analysis.RegisterDependencyScanner(javaDependencyScanner)
}