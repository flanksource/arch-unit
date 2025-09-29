package java

import (
	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/languages"
	"github.com/flanksource/clicky"
)

// javaAnalyzerAdapter adapts the JavaASTExtractor to the languages.ASTAnalyzer interface
type javaAnalyzerAdapter struct {
	extractor *JavaASTExtractor
}

func (a *javaAnalyzerAdapter) AnalyzeFile(task interface{}, filepath string, content []byte) (interface{}, error) {
	// Type assert task to *clicky.Task
	clickyTask, ok := task.(*clicky.Task)
	if !ok {
		// For backward compatibility, create a minimal adapter if not the right type
		return nil, nil
	}

	// Use the generic analyzer to handle the Java extractor
	// This delegates to the existing analysis framework
	genericAnalyzer := languages.GetGenericAnalyzerAdapter()
	return genericAnalyzer.AnalyzeFile(clickyTask, filepath, content)
}

// init registers the Java AST extractor and dependency scanner
func init() {
	// Register Java AST extractor with old registry (for backward compatibility)
	javaExtractor := NewJavaASTExtractor()
	analysis.DefaultExtractorRegistry.Register("java", javaExtractor)

	// Register Java analyzer with unified registry
	javaAnalyzer := &javaAnalyzerAdapter{extractor: javaExtractor}
	languages.SetAnalyzer("java", javaAnalyzer)

	// Register Java dependency scanner
	javaDependencyScanner := NewJavaDependencyScanner()
	analysis.RegisterDependencyScanner(javaDependencyScanner)
}