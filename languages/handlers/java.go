package handlers

import (
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	javaAnalysis "github.com/flanksource/arch-unit/analysis/java"
	"github.com/flanksource/arch-unit/languages"
)

// JavaHandler implements LanguageHandler for Java
type JavaHandler struct{}

// ensure JavaHandler implements LanguageHandler
var _ languages.LanguageHandler = (*JavaHandler)(nil)

// Name returns the language identifier
func (h *JavaHandler) Name() string {
	return "java"
}

// GetDefaultIncludes returns default file patterns
func (h *JavaHandler) GetDefaultIncludes() []string {
	return []string{"**/*.java"}
}

// GetDefaultExcludes returns patterns to exclude
func (h *JavaHandler) GetDefaultExcludes() []string {
	return []string{
		"**/target/**",
		"**/build/**",
		"**/out/**",
		"**/*Test.java",
		"**/*Tests.java",
		"**/test/**",
		"**/generated/**",
		"**/.gradle/**",
		"**/.m2/**",
	}
}

// GetFilePattern returns the file pattern
func (h *JavaHandler) GetFilePattern() string {
	return "**/*.java"
}

// GetBestPractices returns Java-specific best practices
func (h *JavaHandler) GetBestPractices(strictness string) map[string]interface{} {
	practices := make(map[string]interface{})

	practices["max_file_length"] = getValueByStrictness(strictness, 200, 300, 500)
	practices["max_method_length"] = getValueByStrictness(strictness, 20, 30, 50)
	practices["max_cyclomatic_complexity"] = getValueByStrictness(strictness, 5, 10, 15)
	practices["max_method_parameters"] = getValueByStrictness(strictness, 3, 5, 7)
	practices["max_class_fields"] = getValueByStrictness(strictness, 10, 15, 25)
	practices["max_interface_methods"] = getValueByStrictness(strictness, 5, 10, 15)
	practices["max_package_files"] = getValueByStrictness(strictness, 15, 25, 40)
	practices["min_test_coverage"] = getValueByStrictness(strictness, 80, 70, 60)
	practices["max_inheritance_depth"] = getValueByStrictness(strictness, 4, 6, 8)
	practices["max_constructor_parameters"] = getValueByStrictness(strictness, 3, 5, 8)

	return practices
}

// GetStyleGuideOptions returns available style guides
func (h *JavaHandler) GetStyleGuideOptions() []languages.StyleGuideOption {
	return []languages.StyleGuideOption{
		{
			ID:          "google-java",
			DisplayName: "Google Java Style Guide",
			Description: "Google's comprehensive Java style guide with clear formatting rules",
		},
		{
			ID:          "oracle-java",
			DisplayName: "Oracle Java Code Conventions",
			Description: "Official Oracle Java coding standards and conventions",
		},
		{
			ID:          "sun-java",
			DisplayName: "Sun Java Style Guide",
			Description: "Classic Sun Microsystems Java coding conventions",
		},
		{
			ID:          "checkstyle",
			DisplayName: "Checkstyle Default",
			Description: "Default Checkstyle configuration with common Java best practices",
		},
	}
}

// IsTestFile determines if a file is a test file
func (h *JavaHandler) IsTestFile(filename string) bool {
	lowerName := strings.ToLower(filename)
	return strings.HasSuffix(lowerName, "test.java") ||
		strings.HasSuffix(lowerName, "tests.java") ||
		strings.Contains(lowerName, "/test/") ||
		strings.Contains(lowerName, "\\test\\")
}

// GetExtensions returns file extensions
func (h *JavaHandler) GetExtensions() []string {
	return []string{".java"}
}

// GetDefaultLinters returns default linters
func (h *JavaHandler) GetDefaultLinters() []string {
	return []string{"checkstyle", "spotbugs", "pmd", "arch-unit"}
}

// GetAnalyzer returns the AST analyzer
func (h *JavaHandler) GetAnalyzer() languages.ASTAnalyzer {
	// Return the generic analyzer adapter
	return languages.GetGenericAnalyzerAdapter()
}

// GetDependencyScanner returns the dependency scanner for Java
func (h *JavaHandler) GetDependencyScanner() analysis.DependencyScanner {
	return javaAnalysis.NewJavaDependencyScanner()
}

func init() {
	// Register the handler
	languages.DefaultRegistry.RegisterHandler(&JavaHandler{})
}