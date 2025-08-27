package eslint

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

// ESLint implements the Linter interface for ESLint JavaScript/TypeScript linter
type ESLint struct {
	linters.RunOptions
}

// NewESLint creates a new ESLint linter
func NewESLint(workDir string) *ESLint {
	return &ESLint{
		RunOptions: linters.RunOptions{
			WorkDir: workDir,
		},
	}
}

// SetOptions sets the run options for the linter
func (e *ESLint) SetOptions(opts linters.RunOptions) {
	e.RunOptions = opts
}

// Name returns the linter name
func (e *ESLint) Name() string {
	return "eslint"
}

// DefaultIncludes returns default file patterns this linter should process
func (e *ESLint) DefaultIncludes() []string {
	return []string{
		"**/*.js",
		"**/*.jsx",
		"**/*.ts",
		"**/*.tsx",
		"**/*.mjs",
		"**/*.cjs",
	}
}

// DefaultExcludes returns patterns this linter should ignore by default
// Note: Common patterns like .git/**, node_modules/**, examples/**, hack/** are now
// handled by the all_language_excludes macro. This only returns ESLint-specific excludes.
func (e *ESLint) DefaultExcludes() []string {
	return []string{
		"bower_components/**", // Bower packages (legacy package manager)
		"jspm_packages/**",    // JSPM packages (legacy package manager)
		"public/**",           // Public assets (usually not source code)
		".cache/**",           // Cache directories
	}
}

// GetSupportedLanguages returns the languages this linter can process
func (e *ESLint) GetSupportedLanguages() []string {
	return []string{"javascript", "typescript"}
}

// GetEffectiveExcludes returns the complete list of exclusion patterns
// using the all_language_excludes macro for the given language and config
func (e *ESLint) GetEffectiveExcludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default excludes if no config
		return e.DefaultExcludes()
	}
	
	// Use the all_language_excludes macro
	return config.GetAllLanguageExcludes(language, e.DefaultExcludes())
}

// GetEffectiveIncludes returns the complete list of inclusion patterns
// for the given language and config
func (e *ESLint) GetEffectiveIncludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default includes if no config
		return e.DefaultIncludes()
	}
	
	// Use the combined includes system
	return config.GetAllLanguageIncludes(language, e.DefaultIncludes())
}

// SupportsJSON returns true if linter supports JSON output
func (e *ESLint) SupportsJSON() bool {
	return true
}

// JSONArgs returns additional args needed for JSON output
func (e *ESLint) JSONArgs() []string {
	return []string{"--format=json"}
}

// SupportsFix returns true if linter supports auto-fixing violations
func (e *ESLint) SupportsFix() bool {
	return true
}

// FixArgs returns additional args needed for fix mode
func (e *ESLint) FixArgs() []string {
	return []string{"--fix"}
}

// ValidateConfig validates linter-specific configuration
func (e *ESLint) ValidateConfig(config *models.LinterConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	return nil
}

// Run executes eslint and returns violations
func (e *ESLint) Run(ctx commonsContext.Context, task *clicky.Task) ([]models.Violation, error) {
	var args []string
	
	// Add configured args
	if e.Config != nil {
		args = append(args, e.Config.Args...)
	}
	
	// Add JSON format if requested and not already present
	if e.ForceJSON && !e.hasFormatArg(args) {
		args = append(args, "--format=json")
	}
	
	// Add extra args
	args = append(args, e.ExtraArgs...)
	
	// Add files or default to current directory
	if len(e.Files) > 0 {
		args = append(args, e.Files...)
	} else if !e.hasPathArg(args) {
		// ESLint needs explicit file patterns, not just "."
		args = append(args, ".")
	}
	
	// Execute command
	cmd := exec.CommandContext(ctx, "eslint", args...)
	cmd.Dir = e.WorkDir
	
	logger.Infof("Executing: eslint %s", strings.Join(args, " "))
	
	output, err := cmd.CombinedOutput()
	
	// Handle ESLint exit codes
	// ESLint exits with 1 when there are linting errors
	// ESLint exits with 2 when there are configuration problems
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// Exit code 1 with output means violations found - this is expected
				if len(output) > 0 {
					logger.Debugf("eslint exit code 1 with output - treating as success with violations")
					err = nil
				}
			} else if exitErr.ExitCode() == 2 {
				// Configuration error
				return nil, fmt.Errorf("eslint configuration error: %s", string(output))
			}
		}
	}
	
	if err != nil {
		return nil, fmt.Errorf("eslint execution failed: %w\nOutput:\n%s", err, string(output))
	}
	
	// Parse JSON output if we have any
	if len(output) == 0 {
		return []models.Violation{}, nil
	}
	
	return e.parseViolations(output)
}

// hasFormatArg checks if the args already contain a format argument
func (e *ESLint) hasFormatArg(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--format") || strings.HasPrefix(arg, "-f") {
			return true
		}
	}
	return false
}

// hasPathArg checks if the args already contain a path argument
func (e *ESLint) hasPathArg(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// parseViolations parses ESLint JSON output into violations
func (e *ESLint) parseViolations(output []byte) ([]models.Violation, error) {
	var results []ESLintResult
	if err := json.Unmarshal(output, &results); err != nil {
		// If JSON parsing fails, log the output for debugging
		logger.Debugf("Failed to parse eslint JSON output: %v\nOutput: %s", err, string(output))
		return nil, fmt.Errorf("failed to parse eslint JSON output: %w", err)
	}
	
	var violations []models.Violation
	for _, result := range results {
		for _, message := range result.Messages {
			violation := message.ToViolation(e.WorkDir, result.FilePath)
			violations = append(violations, violation)
		}
	}
	
	return violations, nil
}

// ESLintResult represents a single file's results from ESLint
type ESLintResult struct {
	FilePath         string          `json:"filePath"`
	Messages         []ESLintMessage `json:"messages"`
	ErrorCount       int             `json:"errorCount"`
	WarningCount     int             `json:"warningCount"`
	FatalErrorCount  int             `json:"fatalErrorCount"`
	FixableErrorCount int            `json:"fixableErrorCount"`
	FixableWarningCount int          `json:"fixableWarningCount"`
}

// ESLintMessage represents a single message from ESLint
type ESLintMessage struct {
	RuleId      string  `json:"ruleId"`
	Severity    int     `json:"severity"`
	Message     string  `json:"message"`
	Line        int     `json:"line"`
	Column      int     `json:"column"`
	NodeType    string  `json:"nodeType"`
	MessageId   string  `json:"messageId,omitempty"`
	EndLine     int     `json:"endLine,omitempty"`
	EndColumn   int     `json:"endColumn,omitempty"`
	Fix         *struct {
		Range []int  `json:"range"`
		Text  string `json:"text"`
	} `json:"fix,omitempty"`
}

// ToViolation converts an ESLintMessage to a generic Violation
func (m *ESLintMessage) ToViolation(workDir, filePath string) models.Violation {
	filename := filePath
	
	// Make filename absolute if it's relative
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(workDir, filename)
	}
	
	// Map severity to string
	severity := "info"
	switch m.Severity {
	case 1:
		severity = "warning"
	case 2:
		severity = "error"
	}
	
	// Build the rule/method name
	calledMethod := m.RuleId
	if calledMethod == "" {
		calledMethod = severity
	}
	
	return models.Violation{
		File:          filename,
		Line:          m.Line,
		Column:        m.Column,
		CallerPackage: filepath.Dir(filename),
		CallerMethod:  "unknown",
		CalledPackage: "eslint",
		CalledMethod:  calledMethod,
		Message:       m.Message,
		Source:        "eslint",
	}
}