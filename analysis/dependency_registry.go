package analysis

import (
	"path/filepath"
	"strings"
	"sync"
)

// DependencyRegistry manages dependency scanners for different languages
type DependencyRegistry struct {
	mu       sync.RWMutex
	scanners map[string]DependencyScanner
}

// NewDependencyRegistry creates a new dependency scanner registry
func NewDependencyRegistry() *DependencyRegistry {
	return &DependencyRegistry{
		scanners: make(map[string]DependencyScanner),
	}
}

// Register adds a dependency scanner to the registry
func (r *DependencyRegistry) Register(scanner DependencyScanner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scanners[scanner.Language()] = scanner
}

// Get retrieves a dependency scanner by language
func (r *DependencyRegistry) Get(language string) (DependencyScanner, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	scanner, ok := r.scanners[language]
	return scanner, ok
}

// List returns all registered scanner languages
func (r *DependencyRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	languages := make([]string, 0, len(r.scanners))
	for language := range r.scanners {
		languages = append(languages, language)
	}
	return languages
}

// Has checks if a scanner is registered for a language
func (r *DependencyRegistry) Has(language string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.scanners[language]
	return ok
}

// GetScannerForFile finds the appropriate scanner for a file based on its name
func (r *DependencyRegistry) GetScannerForFile(filePath string) (DependencyScanner, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filename := filepath.Base(filePath)

	for _, scanner := range r.scanners {
		for _, pattern := range scanner.SupportedFiles() {
			if matched, _ := filepath.Match(pattern, filename); matched {
				return scanner, true
			}
		}
	}

	return nil, false
}

// GetAllSupportedFiles returns all supported file patterns from all scanners
func (r *DependencyRegistry) GetAllSupportedFiles() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var patterns []string
	seen := make(map[string]bool)

	for _, scanner := range r.scanners {
		for _, pattern := range scanner.SupportedFiles() {
			if !seen[pattern] {
				patterns = append(patterns, pattern)
				seen[pattern] = true
			}
		}
	}

	return patterns
}

// GetScannersForLanguage returns all scanners that support a given language
func (r *DependencyRegistry) GetScannersForLanguage(language string) []DependencyScanner {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var scanners []DependencyScanner
	normalizedLang := strings.ToLower(language)

	for _, scanner := range r.scanners {
		if strings.ToLower(scanner.Language()) == normalizedLang {
			scanners = append(scanners, scanner)
		}
	}

	return scanners
}

// Count returns the number of registered scanners
func (r *DependencyRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.scanners)
}

var (
	defaultRegistryInstance *DependencyRegistry
	defaultRegistryOnce     sync.Once
	defaultRegistryMutex    sync.RWMutex
)

// GetDefaultRegistry returns the global dependency registry singleton
func GetDefaultRegistry() *DependencyRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistryInstance = NewDependencyRegistry()
		registerDefaultScanners(defaultRegistryInstance)
	})
	return defaultRegistryInstance
}

// registerDefaultScanners registers all default scanners with the registry
func registerDefaultScanners(registry *DependencyRegistry) {
	// Auto-register default scanners here when available
	// This will be populated by the individual scanner packages via init() functions
}

// ResetDefaultRegistry resets the singleton (for testing)
func ResetDefaultRegistry() {
	defaultRegistryMutex.Lock()
	defer defaultRegistryMutex.Unlock()
	defaultRegistryInstance = nil
	defaultRegistryOnce = sync.Once{}
}

// Global registry instance (deprecated - use GetDefaultRegistry() instead)
var DefaultDependencyRegistry = GetDefaultRegistry()
