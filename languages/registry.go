package languages

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/flanksource/arch-unit/analysis"
)

// ASTAnalyzer interface for language-specific AST analysis
// This is a simplified interface that references the actual analyzer in the analysis package
type ASTAnalyzer interface {
	AnalyzeFile(task interface{}, filepath string, content []byte) (interface{}, error)
}

// LanguageHandler defines the interface for language-specific operations
type LanguageHandler interface {
	// Name returns the language identifier (e.g., "go", "python")
	Name() string

	// GetDefaultIncludes returns default file patterns this language uses
	GetDefaultIncludes() []string

	// GetDefaultExcludes returns patterns to exclude by default
	GetDefaultExcludes() []string

	// GetFilePattern returns the file pattern for this language
	GetFilePattern() string

	// GetBestPractices returns language-specific best practices based on strictness
	GetBestPractices(strictness string) map[string]interface{}

	// GetStyleGuideOptions returns available style guide options for this language
	GetStyleGuideOptions() []StyleGuideOption

	// IsTestFile determines if a file is a test file for this language
	IsTestFile(filename string) bool

	// GetExtensions returns file extensions for this language
	GetExtensions() []string

	// GetDefaultLinters returns default linters for this language
	GetDefaultLinters() []string

	// GetAnalyzer returns the AST analyzer for this language
	GetAnalyzer() ASTAnalyzer

	// GetDependencyScanner returns the dependency scanner for this language
	GetDependencyScanner() analysis.DependencyScanner
}

// StyleGuideOption represents a style guide choice
type StyleGuideOption struct {
	ID          string
	DisplayName string
	Description string
}

// LanguageConfig represents a programming language configuration
type LanguageConfig struct {
	Name           string
	Extensions     []string
	DefaultLinters []string
	Analyzer       ASTAnalyzer
}

// Registry manages language configurations
type Registry struct {
	mu           sync.RWMutex
	languages    map[string]*LanguageConfig
	extensionMap map[string]*LanguageConfig
	handlers     map[string]LanguageHandler
}

// NewRegistry creates a new language registry
func NewRegistry() *Registry {
	return &Registry{
		languages:    make(map[string]*LanguageConfig),
		extensionMap: make(map[string]*LanguageConfig),
		handlers:     make(map[string]LanguageHandler),
	}
}

// Register adds a language to the registry
func (r *Registry) Register(lang *LanguageConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.languages[lang.Name] = lang

	// Map extensions to language
	for _, ext := range lang.Extensions {
		r.extensionMap[ext] = lang
	}
}

// RegisterHandler adds a language handler to the registry
func (r *Registry) RegisterHandler(handler LanguageHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := handler.Name()
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("language handler '%s' already registered", name)
	}

	r.handlers[name] = handler

	// Also create a LanguageConfig from the handler
	config := &LanguageConfig{
		Name:           name,
		Extensions:     handler.GetExtensions(),
		DefaultLinters: handler.GetDefaultLinters(),
		Analyzer:       handler.GetAnalyzer(),
	}
	r.languages[name] = config

	// Map extensions to language
	for _, ext := range config.Extensions {
		r.extensionMap[ext] = config
	}

	return nil
}

// DetectLanguage determines the language of a file based on its extension
func (r *Registry) DetectLanguage(filePath string) *LanguageConfig {
	ext := strings.ToLower(filepath.Ext(filePath))
	return r.extensionMap[ext]
}

// GetLanguage returns a language by name
func (r *Registry) GetLanguage(name string) *LanguageConfig {
	return r.languages[name]
}

// GetAnalyzer returns the analyzer for a language
func (r *Registry) GetAnalyzer(langName string) ASTAnalyzer {
	if lang := r.languages[langName]; lang != nil {
		return lang.Analyzer
	}
	return nil
}

// GetDefaultLinters returns the default linters for a language
func (r *Registry) GetDefaultLinters(langName string) []string {
	if lang := r.languages[langName]; lang != nil {
		return lang.DefaultLinters
	}
	return nil
}

// ListLanguages returns all registered language names
func (r *Registry) ListLanguages() []string {
	names := make([]string, 0, len(r.languages))
	for name := range r.languages {
		names = append(names, name)
	}
	return names
}

// GetLanguageForFile returns the language configuration for a file
func (r *Registry) GetLanguageForFile(filePath string) *LanguageConfig {
	return r.DetectLanguage(filePath)
}

// HasAnalyzer checks if a language has an analyzer configured
func (r *Registry) HasAnalyzer(langName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lang := r.languages[langName]
	return lang != nil && lang.Analyzer != nil
}

// GetHandler retrieves a language handler by name
func (r *Registry) GetHandler(name string) (LanguageHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, ok := r.handlers[name]
	return handler, ok
}

// GetFilePatternForLanguage returns the file pattern for a language
func (r *Registry) GetFilePatternForLanguage(language string) string {
	if handler, ok := r.GetHandler(language); ok {
		return handler.GetFilePattern()
	}

	// Fallback to legacy behavior
	switch language {
	case "go":
		return "**/*.go"
	case "python":
		return "**/*.py"
	case "javascript":
		return "**/*.{js,jsx}"
	case "typescript":
		return "**/*.{ts,tsx}"
	case "java":
		return "**/*.java"
	case "rust":
		return "**/*.rs"
	case "markdown":
		return "**/*.{md,mdx}"
	default:
		return "**/*"
	}
}

// GetBestPractices returns best practices for a language
func (r *Registry) GetBestPractices(language, strictness string) map[string]interface{} {
	if handler, ok := r.GetHandler(language); ok {
		return handler.GetBestPractices(strictness)
	}

	// Return empty map for unknown languages
	return make(map[string]interface{})
}

// Helper function for strictness-based values
