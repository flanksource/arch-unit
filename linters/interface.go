package linters

import (
	"context"
	"fmt"
	"time"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky/api"
)

// Linter represents a generic linter that can analyze files
type Linter interface {
	// Name returns the linter name (e.g., "golangci-lint", "eslint")
	Name() string

	// Run executes the linter and returns violations
	Run(ctx context.Context, opts RunOptions) ([]models.Violation, error)

	// DefaultIncludes returns default file patterns this linter should process
	DefaultIncludes() []string

	// DefaultExcludes returns patterns this linter should ignore by default
	DefaultExcludes() []string

	// SupportsJSON returns true if linter supports JSON output
	SupportsJSON() bool

	// JSONArgs returns additional args needed for JSON output
	JSONArgs() []string

	// SupportsFix returns true if linter supports auto-fixing violations
	SupportsFix() bool

	// FixArgs returns additional args needed for fix mode
	FixArgs() []string

	// ValidateConfig validates linter-specific configuration
	ValidateConfig(config *models.LinterConfig) error
}

// OptionsMixin provides a way to set options on linters that support it
type OptionsMixin interface {
	SetOptions(opts RunOptions)
}

// LinterWithLanguageSupport extends Linter to provide language-aware file filtering
type LinterWithLanguageSupport interface {
	Linter

	// GetSupportedLanguages returns the languages this linter can process
	GetSupportedLanguages() []string

	// GetEffectiveExcludes returns the complete list of exclusion patterns
	// using the all_language_excludes macro for the given language and config
	GetEffectiveExcludes(language string, config *models.Config) []string

	// GetEffectiveIncludes returns the complete list of inclusion patterns
	// for the given language and config
	GetEffectiveIncludes(language string, config *models.Config) []string
}

// RunOptions provides configuration for linter execution
type RunOptions struct {
	WorkDir    string
	Files      []string
	Config     *models.LinterConfig
	ArchConfig *models.Config // Full arch-unit config for all_language_excludes macro
	ForceJSON  bool
	Fix        bool // Enable auto-fixing mode
	NoCache    bool // Disable caching
	ExtraArgs  []string
}

// Registry manages available linters
type Registry struct {
	linters map[string]Linter
}

// NewRegistry creates a new linter registry
func NewRegistry() *Registry {
	return &Registry{
		linters: make(map[string]Linter),
	}
}

// Register adds a linter to the registry
func (r *Registry) Register(linter Linter) {
	r.linters[linter.Name()] = linter
}

// Get retrieves a linter by name
func (r *Registry) Get(name string) (Linter, bool) {
	l, ok := r.linters[name]
	return l, ok
}

// List returns all registered linter names
func (r *Registry) List() []string {
	var names []string
	for name := range r.linters {
		names = append(names, name)
	}
	return names
}

// Has checks if a linter is registered
func (r *Registry) Has(name string) bool {
	_, ok := r.linters[name]
	return ok
}

// Count returns the number of registered linters
func (r *Registry) Count() int {
	return len(r.linters)
}

// Global registry instance
var DefaultRegistry = NewRegistry()

// LinterResult represents the result of running a linter
type LinterResult struct {
	Linter       string             `json:"linter"`
	Success      bool               `json:"success"`
	Duration     time.Duration      `json:"duration"`
	Violations   []models.Violation `json:"violations"`
	RawOutput    string             `json:"raw_output,omitempty"`
	Error        string             `json:"error,omitempty"`
	Debounced    bool               `json:"debounced,omitempty"`
	DebounceUsed time.Duration      `json:"debounce_used,omitempty"`
}

// GetViolationCount returns the number of violations found
func (lr *LinterResult) GetViolationCount() int {
	return len(lr.Violations)
}

// HasViolations returns true if violations were found
func (lr *LinterResult) HasViolations() bool {
	return len(lr.Violations) > 0
}

// IsSuccessWithViolations returns true if the linter ran successfully but found violations
func (lr *LinterResult) IsSuccessWithViolations() bool {
	return lr.Success && lr.HasViolations()
}

// Pretty returns a formatted text representation of the linter result
func (lr *LinterResult) Pretty() api.Text {
	var status string
	var style string

	if lr.Success {
		if lr.HasViolations() {
			status = "⚠️"
			style = "text-yellow-600"
		} else {
			status = "✅"
			style = "text-green-600"
		}
	} else {
		status = "❌"
		style = "text-red-600"
	}

	text := fmt.Sprintf("%s %s", status, lr.Linter)
	if lr.Debounced {
		text += fmt.Sprintf(" (cached, %v)", lr.DebounceUsed)
	} else {
		text += fmt.Sprintf(" (%d violations, %v)", len(lr.Violations), lr.Duration)
	}

	if lr.Error != "" {
		text += fmt.Sprintf(" - Error: %s", lr.Error)
	}

	return api.Text{Content: text, Style: style}
}
