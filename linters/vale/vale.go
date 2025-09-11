package vale

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/linters"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	commonsContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
)

// Vale implements the Linter interface for vale prose linter
type Vale struct {
	linters.RunOptions
}

// NewVale creates a new vale linter
func NewVale(workDir string) *Vale {
	return &Vale{
		RunOptions: linters.RunOptions{
			WorkDir: workDir,
		},
	}
}

// SetOptions sets the run options for the linter
func (v *Vale) SetOptions(opts linters.RunOptions) {
	v.RunOptions = opts
}

// Name returns the linter name
func (v *Vale) Name() string {
	return "vale"
}

// DefaultIncludes returns default file patterns this linter should process
func (v *Vale) DefaultIncludes() []string {
	return []string{
		"**/*.md",
		"**/*.mdx",
		"**/*.rst",
		"**/*.txt",
		"**/*.adoc",
		"**/*.tex",
		"**/*.html",
		"**/*.xml",
	}
}

// DefaultExcludes returns patterns this linter should ignore by default
// Note: Common patterns like .git/**, node_modules/**, examples/**, hack/** are now
// handled by the all_language_excludes macro. This only returns Vale-specific excludes.
func (v *Vale) DefaultExcludes() []string {
	return []string{
		"LICENSE*",          // License files (prose quality not relevant)
		"CHANGELOG*",        // Changelog files (different writing style)
		"package-lock.json", // Lock files (generated content)
		"yarn.lock",         // Lock files (generated content)
		"go.sum",            // Lock files (generated content)
		"Cargo.lock",        // Lock files (generated content)
	}
}

// GetSupportedLanguages returns the languages this linter can process
func (v *Vale) GetSupportedLanguages() []string {
	return []string{"markdown", "text", "rst", "asciidoc", "html", "xml"}
}

// GetEffectiveExcludes returns the complete list of exclusion patterns
// using the all_language_excludes macro for the given language and config
func (v *Vale) GetEffectiveExcludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default excludes if no config
		return v.DefaultExcludes()
	}

	// Use the all_language_excludes macro
	return config.GetAllLanguageExcludes(language, v.DefaultExcludes())
}

// GetEffectiveIncludes returns the complete list of inclusion patterns
// for the given language and config
func (v *Vale) GetEffectiveIncludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default includes if no config
		return v.DefaultIncludes()
	}

	// Use the combined includes system
	return config.GetAllLanguageIncludes(language, v.DefaultIncludes())
}

// SupportsJSON returns true if linter supports JSON output
func (v *Vale) SupportsJSON() bool {
	return true
}

// JSONArgs returns additional args needed for JSON output
func (v *Vale) JSONArgs() []string {
	return []string{"--output=JSON"}
}

// SupportsFix returns true if linter supports auto-fixing violations
func (v *Vale) SupportsFix() bool {
	return false // Vale doesn't support auto-fixing
}

// FixArgs returns additional args needed for fix mode
func (v *Vale) FixArgs() []string {
	return []string{} // No fix args since not supported
}

// ValidateConfig validates linter-specific configuration
func (v *Vale) ValidateConfig(config *models.LinterConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	return nil
}

// Run executes vale and returns violations
func (v *Vale) Run(ctx commonsContext.Context, task *clicky.Task) ([]models.Violation, error) {
	var args []string

	// Generate dynamic Vale configuration that respects all_language_excludes
	var configPath string
	var cleanupNeeded bool

	// Check if a custom config is already specified in args
	hasCustomConfig := false
	if v.Config != nil {
		for _, arg := range v.Config.Args {
			if strings.HasPrefix(arg, "--config") {
				hasCustomConfig = true
				break
			}
		}
	}

	// If no custom config, generate one with all_language_excludes
	if !hasCustomConfig && v.ArchConfig != nil {
		generatedConfig, err := GenerateValeConfig(v.WorkDir, v.ArchConfig, "markdown", v.DefaultExcludes())
		if err != nil {
			logger.Warnf("Failed to generate Vale config with excludes: %v", err)
		} else {
			configPath = generatedConfig
			cleanupNeeded = true
			args = append(args, "--config="+configPath)
			defer func() {
				if cleanupNeeded {
					CleanupValeConfig(v.WorkDir)
				}
			}()
		}
	}

	// Add configured args
	if v.Config != nil {
		args = append(args, v.Config.Args...)
	}

	// Add JSON format if requested and not already present
	if v.ForceJSON && !v.hasOutputArg(args) {
		args = append(args, "--output=JSON")
	}

	// Add extra args
	args = append(args, v.ExtraArgs...)

	// Add files or default to current directory
	if len(v.Files) > 0 {
		args = append(args, v.Files...)
	} else if !v.hasPathArg(args) {
		// Vale works best with explicit paths
		args = append(args, ".")
	}

	// Execute command
	cmd := exec.CommandContext(ctx, "vale", args...)
	cmd.Dir = v.WorkDir

	logger.Infof("Executing: vale %s", strings.Join(args, " "))

	output, err := cmd.CombinedOutput()

	// Handle vale exit codes
	// Vale exits with 1 when there are errors
	// Vale exits with 2 when there are warnings (configurable)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if (exitCode == 1 || exitCode == 2) && len(output) > 0 {
				// Exit code 1 or 2 with output means violations found - this is expected
				logger.Debugf("vale exit code %d with output - treating as success with violations", exitCode)
				err = nil
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("vale execution failed: %w\nOutput:\n%s", err, string(output))
	}

	// Parse JSON output if we have any
	if len(output) == 0 {
		return []models.Violation{}, nil
	}

	return v.parseViolations(output)
}

// hasOutputArg checks if args already contain output format argument
func (v *Vale) hasOutputArg(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--output") {
			return true
		}
	}
	return false
}

// hasPathArg checks if the args already contain a path argument
func (v *Vale) hasPathArg(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// parseViolations parses vale JSON output into violations
func (v *Vale) parseViolations(output []byte) ([]models.Violation, error) {
	// First check if Vale returned an error response
	var errorResponse struct {
		Code string `json:"Code"`
		Text string `json:"Text"`
	}
	if err := json.Unmarshal(output, &errorResponse); err == nil && errorResponse.Code != "" {
		// Vale returned an error - this might be OK (e.g., no violations found)
		if strings.Contains(errorResponse.Text, ".vale.ini not found") || strings.Contains(errorResponse.Text, "no config file found") {
			// Config issue - return empty violations
			logger.Debugf("Vale config issue: %s", errorResponse.Text)
			return []models.Violation{}, nil
		}
		logger.Debugf("Vale error response: %s", errorResponse.Text)
		return []models.Violation{}, nil
	}

	// Vale JSON output is a map of filename to array of violations
	var results map[string][]ValeMessage
	if err := json.Unmarshal(output, &results); err != nil {
		// If JSON parsing fails, log the output for debugging
		logger.Debugf("Failed to parse vale JSON output: %v\nOutput: %s", err, string(output))
		return nil, fmt.Errorf("failed to parse vale JSON output: %w", err)
	}

	var violations []models.Violation
	for filename, messages := range results {
		for _, message := range messages {
			violation := message.ToViolation(v.WorkDir, filename)
			violations = append(violations, violation)
		}
	}

	return violations, nil
}

// ValeMessage represents a single message from vale
type ValeMessage struct {
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
	Severity string `json:"Severity"`
	Message  string `json:"Message"`
	Check    string `json:"Check"`
	Link     string `json:"Link,omitempty"`
	Span     []int  `json:"Span,omitempty"`
	Match    string `json:"Match,omitempty"`
	Action   struct {
		Name   string   `json:"Name,omitempty"`
		Params []string `json:"Params,omitempty"`
	} `json:"Action,omitempty"`
}

// ToViolation converts a ValeMessage to a generic Violation
func (m *ValeMessage) ToViolation(workDir, filename string) models.Violation {
	// Make filename absolute if it's relative
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(workDir, filename)
	}

	// Build the rule/method name
	calledMethod := m.Check
	if calledMethod == "" {
		calledMethod = m.Severity
	}

	// Build message with match context if available
	message := m.Message
	if m.Match != "" {
		message = fmt.Sprintf("%s [%s]", m.Message, m.Match)
	}

	// Use span for more precise column if available
	column := m.Column
	if len(m.Span) > 0 {
		column = m.Span[0]
	}

	return models.NewViolationBuilder().
		WithFile(filename).
		WithLocation(m.Line, column).
		WithCaller(filepath.Dir(filename), "unknown").
		WithCalled("vale", calledMethod).
		WithMessage(message).
		WithSource("vale").
		WithRuleFromLinter("vale", calledMethod).
		Build()
}
