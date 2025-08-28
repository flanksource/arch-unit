package pyright

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

// Pyright implements the Linter interface for pyright TypeScript/Python type checker
type Pyright struct {
	linters.RunOptions
}

// NewPyright creates a new pyright linter
func NewPyright(workDir string) *Pyright {
	return &Pyright{
		RunOptions: linters.RunOptions{
			WorkDir: workDir,
		},
	}
}

// SetOptions sets the run options for the linter
func (p *Pyright) SetOptions(opts linters.RunOptions) {
	p.RunOptions = opts
}

// Name returns the linter name
func (p *Pyright) Name() string {
	return "pyright"
}

// DefaultIncludes returns default file patterns this linter should process
func (p *Pyright) DefaultIncludes() []string {
	return []string{
		"**/*.py",
		"**/*.ts",
		"**/*.tsx",
		"**/*.js",
		"**/*.jsx",
	}
}

// DefaultExcludes returns patterns this linter should ignore by default
// Note: Common patterns like .git/**, node_modules/**, examples/**, hack/** are now
// handled by the all_language_excludes macro. This only returns Pyright-specific excludes.
func (p *Pyright) DefaultExcludes() []string {
	return []string{
		"*.d.ts",        // TypeScript declaration files (generated)
		"*.egg-info/**", // Python package metadata
		"venv/**",       // Virtual environments (legacy pattern)
		"env/**",        // Virtual environments (legacy pattern)
	}
}

// GetSupportedLanguages returns the languages this linter can process
func (p *Pyright) GetSupportedLanguages() []string {
	return []string{"python", "typescript"}
}

// GetEffectiveExcludes returns the complete list of exclusion patterns
// using the all_language_excludes macro for the given language and config
func (p *Pyright) GetEffectiveExcludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default excludes if no config
		return p.DefaultExcludes()
	}

	// Use the all_language_excludes macro
	return config.GetAllLanguageExcludes(language, p.DefaultExcludes())
}

// GetEffectiveIncludes returns the complete list of inclusion patterns
// for the given language and config
func (p *Pyright) GetEffectiveIncludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default includes if no config
		return p.DefaultIncludes()
	}

	// Use the combined includes system
	return config.GetAllLanguageIncludes(language, p.DefaultIncludes())
}

// SupportsJSON returns true if linter supports JSON output
func (p *Pyright) SupportsJSON() bool {
	return true
}

// JSONArgs returns additional args needed for JSON output
func (p *Pyright) JSONArgs() []string {
	return []string{"--outputjson"}
}

// SupportsFix returns true if linter supports auto-fixing violations
func (p *Pyright) SupportsFix() bool {
	return false // Pyright doesn't support auto-fixing
}

// FixArgs returns additional args needed for fix mode
func (p *Pyright) FixArgs() []string {
	return []string{} // No fix args since not supported
}

// ValidateConfig validates linter-specific configuration
func (p *Pyright) ValidateConfig(config *models.LinterConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	return nil
}

// Run executes pyright and returns violations
func (p *Pyright) Run(ctx commonsContext.Context, task *clicky.Task) ([]models.Violation, error) {
	var args []string

	// Add configured args
	if p.Config != nil {
		args = append(args, p.Config.Args...)
	}

	// Add JSON format if requested and not already present
	if p.ForceJSON && !p.hasJSONArg(args) {
		args = append(args, "--outputjson")
	}

	// Add extra args
	args = append(args, p.ExtraArgs...)

	// Add files or default to current directory
	if len(p.Files) > 0 {
		args = append(args, p.Files...)
	} else if !p.hasPathArg(args) {
		args = append(args, ".")
	}

	// Execute command
	cmd := exec.CommandContext(ctx, "pyright", args...)
	cmd.Dir = p.WorkDir

	logger.Infof("Executing: pyright %s", strings.Join(args, " "))

	output, err := cmd.CombinedOutput()

	// Handle pyright exit codes
	// Pyright exits with 1 when there are errors/warnings
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Exit code 1 with output means violations found - this is expected
			if len(output) > 0 {
				logger.Debugf("pyright exit code 1 with output - treating as success with violations")
				err = nil
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("pyright execution failed: %w\nOutput:\n%s", err, string(output))
	}

	// Parse JSON output if we have any
	if len(output) == 0 {
		return []models.Violation{}, nil
	}

	return p.parseViolations(output)
}

// hasJSONArg checks if args already contain JSON output flag
func (p *Pyright) hasJSONArg(args []string) bool {
	for _, arg := range args {
		if arg == "--outputjson" {
			return true
		}
	}
	return false
}

// hasPathArg checks if the args already contain a path argument
func (p *Pyright) hasPathArg(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// parseViolations parses pyright JSON output into violations
func (p *Pyright) parseViolations(output []byte) ([]models.Violation, error) {
	var result PyrightOutput
	if err := json.Unmarshal(output, &result); err != nil {
		// If JSON parsing fails, log the output for debugging
		logger.Debugf("Failed to parse pyright JSON output: %v\nOutput: %s", err, string(output))
		return nil, fmt.Errorf("failed to parse pyright JSON output: %w", err)
	}

	var violations []models.Violation
	for _, diagnostic := range result.GeneralDiagnostics {
		violation := diagnostic.ToViolation(p.WorkDir)
		violations = append(violations, violation)
	}

	return violations, nil
}

// PyrightOutput represents the JSON structure from pyright
type PyrightOutput struct {
	Version            string              `json:"version"`
	Time               string              `json:"time"`
	GeneralDiagnostics []PyrightDiagnostic `json:"generalDiagnostics"`
	Summary            struct {
		FilesAnalyzed int     `json:"filesAnalyzed"`
		ErrorCount    int     `json:"errorCount"`
		WarningCount  int     `json:"warningCount"`
		InfoCount     int     `json:"informationCount"`
		TimeInSec     float64 `json:"timeInSec"`
	} `json:"summary"`
}

// PyrightDiagnostic represents a single diagnostic from pyright
type PyrightDiagnostic struct {
	File     string `json:"file"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Rule     string `json:"rule,omitempty"`
	Range    struct {
		Start struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"start"`
		End struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"end"`
	} `json:"range"`
}

// ToViolation converts a PyrightDiagnostic to a generic Violation
func (d *PyrightDiagnostic) ToViolation(workDir string) models.Violation {
	filename := d.File

	// Make filename absolute if it's relative
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(workDir, filename)
	}

	// Build the rule/method name
	calledMethod := d.Severity
	if d.Rule != "" {
		calledMethod = fmt.Sprintf("%s:%s", d.Severity, d.Rule)
	}

	return models.Violation{
		File:          filename,
		Line:          d.Range.Start.Line + 1, // Pyright uses 0-based indexing
		Column:        d.Range.Start.Character + 1,
		CallerPackage: filepath.Dir(filename),
		CallerMethod:  "unknown",
		CalledPackage: "pyright",
		CalledMethod:  calledMethod,
		Message:       d.Message,
		Source:        "pyright",
	}
}
