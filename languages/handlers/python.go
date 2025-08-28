package handlers

import (
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/languages"
)

// PythonHandler implements LanguageHandler for Python
type PythonHandler struct{}

// ensure PythonHandler implements LanguageHandler
var _ languages.LanguageHandler = (*PythonHandler)(nil)

// Name returns the language identifier
func (h *PythonHandler) Name() string {
	return "python"
}

// GetDefaultIncludes returns default file patterns
func (h *PythonHandler) GetDefaultIncludes() []string {
	return []string{"**/*.py"}
}

// GetDefaultExcludes returns patterns to exclude
func (h *PythonHandler) GetDefaultExcludes() []string {
	return []string{
		"venv/**",
		".venv/**",
		"env/**",
		"__pycache__/**",
		"*.pyc",
		"**/test_*.py",
		"**/*_test.py",
		"**/tests/**",
		"**/migrations/**",
		".tox/**",
		"dist/**",
		"build/**",
		"*.egg-info/**",
	}
}

// GetFilePattern returns the file pattern
func (h *PythonHandler) GetFilePattern() string {
	return "**/*.py"
}

// GetBestPractices returns Python-specific best practices
func (h *PythonHandler) GetBestPractices(strictness string) map[string]interface{} {
	practices := make(map[string]interface{})

	practices["max_file_length"] = getValueByStrictness(strictness, 300, 500, 700)
	practices["max_function_length"] = getValueByStrictness(strictness, 30, 50, 75)
	practices["max_cyclomatic_complexity"] = getValueByStrictness(strictness, 5, 10, 15)
	practices["max_function_parameters"] = getValueByStrictness(strictness, 3, 5, 7)
	practices["max_class_methods"] = getValueByStrictness(strictness, 10, 15, 20)
	practices["max_module_members"] = getValueByStrictness(strictness, 12, 20, 30)
	practices["max_line_length"] = getValueByStrictness(strictness, 79, 88, 120)
	practices["min_test_coverage"] = getValueByStrictness(strictness, 80, 70, 60)

	return practices
}

// GetStyleGuideOptions returns available style guides
func (h *PythonHandler) GetStyleGuideOptions() []languages.StyleGuideOption {
	return []languages.StyleGuideOption{
		{
			ID:          "pep8",
			DisplayName: "PEP 8",
			Description: "Official Python style guide for code formatting",
		},
		{
			ID:          "google-python",
			DisplayName: "Google Python Style Guide",
			Description: "Google's Python style guide with detailed conventions",
		},
		{
			ID:          "black",
			DisplayName: "Black Code Style",
			Description: "The uncompromising Python code formatter's style",
		},
	}
}

// IsTestFile determines if a file is a test file
func (h *PythonHandler) IsTestFile(filename string) bool {
	base := strings.TrimSuffix(filename, ".py")
	return strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test") ||
		strings.Contains(filename, "/tests/") ||
		strings.Contains(filename, "/test/")
}

// GetExtensions returns file extensions
func (h *PythonHandler) GetExtensions() []string {
	return []string{".py", ".pyi"}
}

// GetDefaultLinters returns default linters
func (h *PythonHandler) GetDefaultLinters() []string {
	return []string{"ruff", "mypy", "pylint", "flake8"}
}

// GetAnalyzer returns the AST analyzer
func (h *PythonHandler) GetAnalyzer() languages.ASTAnalyzer {
	// This will be set when the analyzer is registered
	return nil
}

// GetDependencyScanner returns the dependency scanner for Python
func (h *PythonHandler) GetDependencyScanner() analysis.DependencyScanner {
	// TODO: Implement Python dependency scanner for requirements.txt, Pipfile, etc.
	return nil
}

func init() {
	// Register the handler
	languages.DefaultRegistry.RegisterHandler(&PythonHandler{})
}
