package fixtures

import (
	"context"
	"fmt"
	"sync"
)

// FixtureType defines the interface for fixture test types
type FixtureType interface {
	// Name returns the type identifier (e.g., "query", "exec", "linter")
	Name() string
	
	// Run executes the fixture test and returns the result
	Run(ctx context.Context, fixture FixtureTest, opts RunOptions) FixtureResult
	
	// ValidateFixture validates that the fixture has required fields for this type
	ValidateFixture(fixture FixtureTest) error
	
	// GetRequiredFields returns a list of required fields for this fixture type
	GetRequiredFields() []string
	
	// GetOptionalFields returns a list of optional fields for this fixture type
	GetOptionalFields() []string
}

// RunOptions provides configuration for fixture execution
type RunOptions struct {
	WorkDir    string
	Verbose    bool
	NoCache    bool
	Evaluator  *CELEvaluator
	ExtraArgs  map[string]interface{}
}

// FixtureResult represents the result of running a fixture
type FixtureResult struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"` // PASS, FAIL, SKIP
	Error      string                 `json:"error,omitempty"`
	Expected   interface{}            `json:"expected,omitempty"`
	Actual     interface{}            `json:"actual,omitempty"`
	CELResult  bool                   `json:"cel_result,omitempty"`
	Duration   string                 `json:"duration,omitempty"`
	Details    string                 `json:"details,omitempty"`
	Output     string                 `json:"output,omitempty"`
	ExitCode   int                    `json:"exit_code,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	
	// Enhanced execution details
	Command    string `json:"command,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
}

// Registry manages fixture types
type Registry struct {
	mu    sync.RWMutex
	types map[string]FixtureType
}

// NewRegistry creates a new fixture type registry
func NewRegistry() *Registry {
	return &Registry{
		types: make(map[string]FixtureType),
	}
}

// Register adds a fixture type to the registry
func (r *Registry) Register(fixtureType FixtureType) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	name := fixtureType.Name()
	if _, exists := r.types[name]; exists {
		return fmt.Errorf("fixture type '%s' already registered", name)
	}
	
	r.types[name] = fixtureType
	return nil
}

// Get retrieves a fixture type by name
func (r *Registry) Get(name string) (FixtureType, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	ft, ok := r.types[name]
	return ft, ok
}

// GetForFixture determines the appropriate fixture type for a given fixture
func (r *Registry) GetForFixture(fixture FixtureTest) (FixtureType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Determine type based on fixture fields
	if fixture.CLI != "" || fixture.CLIArgs != "" {
		if ft, ok := r.types["exec"]; ok {
			return ft, nil
		}
		return nil, fmt.Errorf("exec fixture type not registered")
	}
	
	if fixture.Query != "" {
		if ft, ok := r.types["query"]; ok {
			return ft, nil
		}
		return nil, fmt.Errorf("query fixture type not registered")
	}
	
	// Add more type detection logic as needed
	
	return nil, fmt.Errorf("unable to determine fixture type for test '%s'", fixture.Name)
}

// List returns all registered fixture type names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	names := make([]string, 0, len(r.types))
	for name := range r.types {
		names = append(names, name)
	}
	return names
}

// ValidateFixture validates a fixture against its determined type
func (r *Registry) ValidateFixture(fixture FixtureTest) error {
	ft, err := r.GetForFixture(fixture)
	if err != nil {
		return err
	}
	
	return ft.ValidateFixture(fixture)
}

// DefaultRegistry is the global fixture type registry
var DefaultRegistry = NewRegistry()

// Register adds a fixture type to the default registry
func Register(fixtureType FixtureType) error {
	return DefaultRegistry.Register(fixtureType)
}

// Get retrieves a fixture type from the default registry
func Get(name string) (FixtureType, bool) {
	return DefaultRegistry.Get(name)
}