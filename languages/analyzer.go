package languages

import (
	"github.com/flanksource/arch-unit/models"
)

type Language interface {
	Name() string
	Extensions() []string
	AnalyzeFile(filePath string, rules *models.RuleSet) ([]models.Violation, error)
}

type Analyzer interface {
	Language
	AnalyzeFiles(rootDir string, files []string, ruleSets []models.RuleSet) (*models.AnalysisResult, error)
}

// ASTExporter defines the interface for exporting language-specific ASTs to GenericAST
// This interface allows different language analyzers to provide a uniform AST representation
type ASTExporter interface {
	ExportAST(filePath string) (*models.GenericAST, error)
}

// QualityAnalyzer combines traditional analysis with AST export capabilities
type QualityAnalyzer interface {
	Analyzer
	ASTExporter
}

// LanguageAnalyzerRegistry manages different language analyzers
type LanguageAnalyzerRegistry struct {
	analyzers map[string]QualityAnalyzer
}

// NewLanguageAnalyzerRegistry creates a new analyzer registry
func NewLanguageAnalyzerRegistry() *LanguageAnalyzerRegistry {
	return &LanguageAnalyzerRegistry{
		analyzers: make(map[string]QualityAnalyzer),
	}
}

// RegisterAnalyzer registers a language analyzer
func (r *LanguageAnalyzerRegistry) RegisterAnalyzer(language string, analyzer QualityAnalyzer) {
	r.analyzers[language] = analyzer
}

// GetAnalyzer returns an analyzer for a specific language
func (r *LanguageAnalyzerRegistry) GetAnalyzer(language string) (QualityAnalyzer, bool) {
	analyzer, exists := r.analyzers[language]
	return analyzer, exists
}

// GetASTExporter returns an AST exporter for a specific language
func (r *LanguageAnalyzerRegistry) GetASTExporter(language string) (ASTExporter, bool) {
	analyzer, exists := r.analyzers[language]
	return analyzer, exists
}

// SupportedLanguages returns all supported languages
func (r *LanguageAnalyzerRegistry) SupportedLanguages() []string {
	var languages []string
	for lang := range r.analyzers {
		languages = append(languages, lang)
	}
	return languages
}

// DetectLanguage detects the language of a file based on its extension
func DetectLanguage(filePath string) string {
	if len(filePath) < 3 {
		return "unknown"
	}

	// Simple extension-based detection
	switch {
	case filePath[len(filePath)-3:] == ".go":
		return "go"
	case filePath[len(filePath)-3:] == ".py" || (len(filePath) >= 4 && filePath[len(filePath)-4:] == ".pyi"):
		return "python"
	case len(filePath) >= 5 && filePath[len(filePath)-5:] == ".java":
		return "java"
	case len(filePath) >= 4 && filePath[len(filePath)-4:] == ".tsx":
		return "typescript"
	case len(filePath) >= 3 && filePath[len(filePath)-3:] == ".ts":
		return "typescript"
	case len(filePath) >= 4 && filePath[len(filePath)-4:] == ".jsx":
		return "javascript"
	case len(filePath) >= 3 && filePath[len(filePath)-3:] == ".js":
		return "javascript"
	case len(filePath) >= 4 && (filePath[len(filePath)-4:] == ".mjs" || filePath[len(filePath)-4:] == ".cjs"):
		return "javascript"
	case len(filePath) >= 3 && filePath[len(filePath)-3:] == ".rs":
		return "rust"
	case len(filePath) >= 3 && filePath[len(filePath)-3:] == ".rb":
		return "ruby"
	case len(filePath) >= 3 && filePath[len(filePath)-3:] == ".md":
		return "markdown"
	case len(filePath) >= 4 && filePath[len(filePath)-4:] == ".mdx":
		return "markdown"
	case len(filePath) >= 9 && filePath[len(filePath)-9:] == ".markdown":
		return "markdown"
	default:
		return "unknown"
	}
}
