package languages

import (
	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/clicky"
)

// goAnalyzerAdapter adapts the analysis.GoAnalyzer to the languages.Analyzer interface
type goAnalyzerAdapter struct {
	analyzer *analysis.GoAnalyzer
}

func (a *goAnalyzerAdapter) AnalyzeFile(task interface{}, filepath string, content []byte) (interface{}, error) {
	// Type assert task to *clicky.Task
	clickyTask, ok := task.(*clicky.Task)
	if !ok {
		// Create a no-op task if not the right type
		return nil, nil
	}
	return a.analyzer.AnalyzeFile(clickyTask, filepath, content)
}

// DefaultRegistry is the global language registry
var DefaultRegistry *Registry

func init() {
	DefaultRegistry = NewRegistry()
	
	// Create Go analyzer
	goAnalyzer := &goAnalyzerAdapter{
		analyzer: analysis.NewGoAnalyzer(),
	}
	
	// Register Go language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "go",
		Extensions: []string{".go"},
		DefaultLinters: []string{
			"golangci-lint",
			"arch-unit",
		},
		Analyzer: goAnalyzer,
	})
	
	// Register Python language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "python",
		Extensions: []string{".py", ".pyw", ".pyi"},
		DefaultLinters: []string{
			"ruff",
			"pyright",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register JavaScript language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "javascript",
		Extensions: []string{".js", ".jsx", ".mjs", ".cjs"},
		DefaultLinters: []string{
			"eslint",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register TypeScript language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "typescript",
		Extensions: []string{".ts", ".tsx", ".mts", ".cts"},
		DefaultLinters: []string{
			"eslint",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register Markdown language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "markdown",
		Extensions: []string{".md", ".markdown", ".mdx"},
		DefaultLinters: []string{
			"markdownlint",
			"vale",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register YAML language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "yaml",
		Extensions: []string{".yaml", ".yml"},
		DefaultLinters: []string{
			"yamllint",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register JSON language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "json",
		Extensions: []string{".json", ".jsonc"},
		DefaultLinters: []string{
			"jsonlint",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register Rust language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "rust",
		Extensions: []string{".rs"},
		DefaultLinters: []string{
			"rustfmt",
			"clippy",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register Java language
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "java",
		Extensions: []string{".java"},
		DefaultLinters: []string{
			"checkstyle",
			"spotbugs",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	// Register C/C++ languages
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "c",
		Extensions: []string{".c", ".h"},
		DefaultLinters: []string{
			"clang-tidy",
			"cppcheck",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
	
	DefaultRegistry.Register(&LanguageConfig{
		Name:       "cpp",
		Extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"},
		DefaultLinters: []string{
			"clang-tidy",
			"cppcheck",
		},
		Analyzer: nil, // Will be set when analyzer is created
	})
}

// GetRegistry returns the default language registry
func GetRegistry() *Registry {
	return DefaultRegistry
}

// SetAnalyzer sets the analyzer for a specific language
func SetAnalyzer(langName string, analyzer ASTAnalyzer) {
	if lang := DefaultRegistry.GetLanguage(langName); lang != nil {
		lang.Analyzer = analyzer
	}
}

// GetGoAnalyzerAdapter returns the Go analyzer adapter
func GetGoAnalyzerAdapter() ASTAnalyzer {
	return &goAnalyzerAdapter{
		analyzer: analysis.NewGoAnalyzer(),
	}
}