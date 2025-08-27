package handlers

import (
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/languages"
)

// Helper function for strictness-based values
func getValueByStrictness(strictness string, strict, normal, relaxed interface{}) interface{} {
	switch strictness {
	case "strict":
		return strict
	case "relaxed":
		return relaxed
	default:
		return normal
	}
}

// GoHandler implements LanguageHandler for Go
type GoHandler struct{}

// ensure GoHandler implements LanguageHandler
var _ languages.LanguageHandler = (*GoHandler)(nil)

// Name returns the language identifier
func (h *GoHandler) Name() string {
	return "go"
}

// GetDefaultIncludes returns default file patterns
func (h *GoHandler) GetDefaultIncludes() []string {
	return []string{"**/*.go"}
}

// GetDefaultExcludes returns patterns to exclude
func (h *GoHandler) GetDefaultExcludes() []string {
	return []string{
		"vendor/**",
		"**/*_test.go",
		"**/testdata/**",
		"**/*.pb.go",
		"**/*.gen.go",
		"**/mock_*.go",
	}
}

// GetFilePattern returns the file pattern
func (h *GoHandler) GetFilePattern() string {
	return "**/*.go"
}

// GetBestPractices returns Go-specific best practices
func (h *GoHandler) GetBestPractices(strictness string) map[string]interface{} {
	practices := make(map[string]interface{})
	
	practices["max_file_length"] = getValueByStrictness(strictness, 300, 400, 500)
	practices["max_function_length"] = getValueByStrictness(strictness, 40, 50, 70)
	practices["max_cyclomatic_complexity"] = getValueByStrictness(strictness, 5, 10, 15)
	practices["max_function_parameters"] = getValueByStrictness(strictness, 3, 5, 7)
	practices["max_struct_fields"] = getValueByStrictness(strictness, 10, 15, 20)
	practices["max_interface_methods"] = getValueByStrictness(strictness, 5, 10, 15)
	practices["max_package_files"] = getValueByStrictness(strictness, 10, 20, 30)
	practices["min_test_coverage"] = getValueByStrictness(strictness, 80, 70, 60)
	
	return practices
}

// GetStyleGuideOptions returns available style guides
func (h *GoHandler) GetStyleGuideOptions() []languages.StyleGuideOption {
	return []languages.StyleGuideOption{
		{
			ID:          "google-go",
			DisplayName: "Google Go Style Guide",
			Description: "Google's official Go style guide with clear conventions",
		},
		{
			ID:          "uber-go",
			DisplayName: "Uber Go Style Guide",
			Description: "Uber's comprehensive Go style guide with detailed examples",
		},
		{
			ID:          "effective-go",
			DisplayName: "Effective Go",
			Description: "Official Go documentation on writing clear, idiomatic Go code",
		},
	}
}

// IsTestFile determines if a file is a test file
func (h *GoHandler) IsTestFile(filename string) bool {
	return strings.HasSuffix(filename, "_test.go")
}

// GetExtensions returns file extensions
func (h *GoHandler) GetExtensions() []string {
	return []string{".go"}
}

// GetDefaultLinters returns default linters
func (h *GoHandler) GetDefaultLinters() []string {
	return []string{"golangci-lint", "go-vet", "staticcheck"}
}

// GetAnalyzer returns the AST analyzer
func (h *GoHandler) GetAnalyzer() languages.ASTAnalyzer {
	// Return the Go analyzer adapter
	return languages.GetGoAnalyzerAdapter()
}

// GetDependencyScanner returns the dependency scanner for Go
func (h *GoHandler) GetDependencyScanner() analysis.DependencyScanner {
	return analysis.NewGoDependencyScanner()
}

func init() {
	// Register the handler
	languages.DefaultRegistry.RegisterHandler(&GoHandler{})
}