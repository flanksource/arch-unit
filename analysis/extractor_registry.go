package analysis

import (
	"path/filepath"
	"strings"
	"sync"
)

var extractorRegistry = NewExtractorRegistry()

// ExtractorRegistry manages AST extractors for different languages
type ExtractorRegistry struct {
	mu         sync.RWMutex
	extractors map[string]Extractor
}

// NewExtractorRegistry creates a new AST extractor registry
func NewExtractorRegistry() *ExtractorRegistry {
	return &ExtractorRegistry{
		extractors: make(map[string]Extractor),
	}
}

// Register adds an AST extractor to the global registry
func RegisterExtractor(language string, extractor Extractor) {
	extractorRegistry.Register(language, extractor)
}

// Register adds an AST extractor to the registry
func (r *ExtractorRegistry) Register(language string, extractor Extractor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extractors[strings.ToLower(language)] = extractor
}

// Get retrieves an AST extractor by language
func (r *ExtractorRegistry) Get(language string) (Extractor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	extractor, ok := r.extractors[strings.ToLower(language)]
	return extractor, ok
}

// List returns all registered extractor languages
func (r *ExtractorRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	languages := make([]string, 0, len(r.extractors))
	for language := range r.extractors {
		languages = append(languages, language)
	}
	return languages
}

// Has checks if an extractor is registered for a language
func (r *ExtractorRegistry) Has(language string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.extractors[strings.ToLower(language)]
	return ok
}

// GetExtractorForFile finds the appropriate extractor for a file based on its extension
func (r *ExtractorRegistry) GetExtractorForFile(filePath string) (Extractor, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ext := strings.ToLower(filepath.Ext(filePath))

	// Map file extensions to languages
	extToLanguage := map[string]string{
		".go":   "go",
		".java": "java",
		".py":   "python",
		".js":   "javascript",
		".ts":   "javascript", // TypeScript uses JavaScript extractor
		".jsx":  "javascript",
		".tsx":  "javascript",
		".md":   "markdown",
	}

	if language, ok := extToLanguage[ext]; ok {
		if extractor, exists := r.extractors[language]; exists {
			return extractor, language, true
		}
	}

	return nil, "", false
}

// Count returns the number of registered extractors
func (r *ExtractorRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.extractors)
}

var (
	defaultExtractorRegistryInstance *ExtractorRegistry
	defaultExtractorRegistryOnce     sync.Once
	defaultExtractorRegistryMutex    sync.RWMutex
)

// GetDefaultExtractorRegistry returns the global extractor registry singleton
func GetDefaultExtractorRegistry() *ExtractorRegistry {
	defaultExtractorRegistryOnce.Do(func() {
		defaultExtractorRegistryInstance = NewExtractorRegistry()
	})
	return defaultExtractorRegistryInstance
}

// ResetDefaultExtractorRegistry resets the singleton (for testing)
func ResetDefaultExtractorRegistry() {
	defaultExtractorRegistryMutex.Lock()
	defer defaultExtractorRegistryMutex.Unlock()
	defaultExtractorRegistryInstance = nil
	defaultExtractorRegistryOnce = sync.Once{}
}

// Global extractor registry instance
var DefaultExtractorRegistry = GetDefaultExtractorRegistry()

// GetExtractorByLanguage is a convenience function to get an extractor by language
func GetExtractorByLanguage(language string) (Extractor, bool) {
	return DefaultExtractorRegistry.Get(language)
}

// GetExtractorByFile is a convenience function to get an extractor by file path
func GetExtractorByFile(filePath string) (Extractor, string, bool) {
	return DefaultExtractorRegistry.GetExtractorForFile(filePath)
}