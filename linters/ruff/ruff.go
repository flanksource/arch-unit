package ruff

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

// Ruff implements the Linter interface for ruff Python linter
type Ruff struct {
	linters.RunOptions
}

// NewRuff creates a new ruff linter
func NewRuff(workDir string) *Ruff {
	return &Ruff{
		RunOptions: linters.RunOptions{
			WorkDir: workDir,
		},
	}
}

// SetOptions sets the run options for the linter
func (r *Ruff) SetOptions(opts linters.RunOptions) {
	r.RunOptions = opts
}

// Name returns the linter name
func (r *Ruff) Name() string {
	return "ruff"
}

// DefaultIncludes returns default file patterns this linter should process
func (r *Ruff) DefaultIncludes() []string {
	return []string{"**/*.py"}
}

// DefaultExcludes returns patterns this linter should ignore by default
// Note: Common patterns like .git/**, __pycache__/**, examples/**, hack/** are now
// handled by the all_language_excludes macro. This only returns Ruff-specific excludes.
func (r *Ruff) DefaultExcludes() []string {
	return []string{
		"*.pyc",         // Compiled Python files
		"*.pyo",         // Optimized Python files
		"*.egg-info/**", // Python package metadata
		"venv/**",       // Virtual environments (legacy pattern)
		"env/**",        // Virtual environments (legacy pattern)
	}
}

// GetSupportedLanguages returns the languages this linter can process
func (r *Ruff) GetSupportedLanguages() []string {
	return []string{"python"}
}

// GetEffectiveExcludes returns the complete list of exclusion patterns
// using the all_language_excludes macro for the given language and config
func (r *Ruff) GetEffectiveExcludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default excludes if no config
		return r.DefaultExcludes()
	}

	// Use the all_language_excludes macro
	return config.GetAllLanguageExcludes(language, r.DefaultExcludes())
}

// GetEffectiveIncludes returns the complete list of inclusion patterns
// for the given language and config
func (r *Ruff) GetEffectiveIncludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default includes if no config
		return r.DefaultIncludes()
	}

	// Use the combined includes system
	return config.GetAllLanguageIncludes(language, r.DefaultIncludes())
}

// SupportsJSON returns true if linter supports JSON output
func (r *Ruff) SupportsJSON() bool {
	return true
}

// JSONArgs returns additional args needed for JSON output
func (r *Ruff) JSONArgs() []string {
	return []string{"--output-format=json"}
}

// SupportsFix returns true if linter supports auto-fixing violations
func (r *Ruff) SupportsFix() bool {
	return true
}

// FixArgs returns additional args needed for fix mode
func (r *Ruff) FixArgs() []string {
	return []string{"--fix"}
}

// ValidateConfig validates linter-specific configuration
func (r *Ruff) ValidateConfig(config *models.LinterConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	return nil
}

// Run executes ruff and returns violations
func (r *Ruff) Run(ctx commonsContext.Context, task *clicky.Task) ([]models.Violation, error) {
	args := []string{"check"}

	// Add configured args
	if r.Config != nil {
		args = append(args, r.Config.Args...)
	}

	// Add JSON format if requested and not already present
	if r.ForceJSON && !r.hasFormatArg(args, "--output-format") {
		args = append(args, "--output-format=json")
	}

	// Add fix flag if requested
	if r.Fix && r.SupportsFix() && !r.hasArg(args, "--fix") {
		fixArgs := r.FixArgs()
		args = append(args, fixArgs...)
	}

	// Add extra args
	args = append(args, r.ExtraArgs...)

	// Add files or default to current directory
	if len(r.Files) > 0 {
		args = append(args, r.Files...)
	} else if !r.hasPathArg(args) {
		args = append(args, ".")
	}

	// Execute command
	cmd := exec.CommandContext(ctx, "ruff", args...)
	cmd.Dir = r.WorkDir

	logger.Infof("Executing: ruff %s", strings.Join(args, " "))

	output, err := cmd.CombinedOutput()

	// Handle ruff exit codes
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Exit code 1 with output means violations found - this is expected
			if len(output) > 0 {
				logger.Debugf("ruff exit code 1 with output - treating as success with violations")
				err = nil
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("ruff execution failed: %w\nOutput:\n%s", err, string(output))
	}

	// Parse JSON output if we have any
	if len(output) == 0 {
		return []models.Violation{}, nil
	}

	return r.parseViolations(output)
}

// hasFormatArg checks if the args already contain a format argument
func (r *Ruff) hasFormatArg(args []string, formatPrefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, formatPrefix) {
			return true
		}
	}
	return false
}

// hasPathArg checks if the args already contain a path argument
func (r *Ruff) hasPathArg(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// hasArg checks if the args contain a specific argument
func (r *Ruff) hasArg(args []string, argToFind string) bool {
	for _, arg := range args {
		if arg == argToFind {
			return true
		}
	}
	return false
}

// parseViolations parses ruff JSON output into violations
func (r *Ruff) parseViolations(output []byte) ([]models.Violation, error) {
	var issues []RuffIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		// If JSON parsing fails, log the output for debugging
		logger.Debugf("Failed to parse ruff JSON output: %v\nOutput: %s", err, string(output))
		return nil, fmt.Errorf("failed to parse ruff JSON output: %w", err)
	}

	var violations []models.Violation
	for _, issue := range issues {
		violation := issue.ToViolation(r.WorkDir)
		logger.Debugf("Ruff violation: %s, fixable: %v, applicability: %s",
			violation.Message, violation.Fixable, violation.FixApplicability)
		violations = append(violations, violation)
	}

	return violations, nil
}

// RuffIssue represents a single issue from ruff
type RuffIssue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Location struct {
		Row    int `json:"row"`
		Column int `json:"column"`
	} `json:"location"`
	Filename string `json:"filename"`
	Fix      *struct {
		Applicability string `json:"applicability"`
		Message       string `json:"message"`
	} `json:"fix"`
}

// ToViolation converts a RuffIssue to a generic Violation
func (issue *RuffIssue) ToViolation(workDir string) models.Violation {
	filename := issue.Filename

	// Make filename absolute if it's relative
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(workDir, filename)
	}

	// Determine if this issue is fixable
	fixable := false
	fixApplicability := ""
	if issue.Fix != nil {
		fixable = true
		fixApplicability = issue.Fix.Applicability
	}

	return models.Violation{
		File:             filename,
		Line:             issue.Location.Row,
		Column:           issue.Location.Column,
		CallerPackage:    filepath.Dir(filename),
		CallerMethod:     "unknown",
		CalledPackage:    "ruff",
		CalledMethod:     issue.Code,
		Message:          issue.Message,
		Source:           "ruff",
		Fixable:          fixable,
		FixApplicability: fixApplicability,
	}
}
