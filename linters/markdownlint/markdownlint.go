package markdownlint

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

// Markdownlint implements the Linter interface for markdownlint
type Markdownlint struct {
	linters.RunOptions
}

// NewMarkdownlint creates a new markdownlint linter
func NewMarkdownlint(workDir string) *Markdownlint {
	return &Markdownlint{
		RunOptions: linters.RunOptions{
			WorkDir: workDir,
		},
	}
}

// SetOptions sets the run options for the linter
func (m *Markdownlint) SetOptions(opts linters.RunOptions) {
	m.RunOptions = opts
}

// Name returns the linter name
func (m *Markdownlint) Name() string {
	return "markdownlint"
}

// DefaultIncludes returns default file patterns this linter should process
func (m *Markdownlint) DefaultIncludes() []string {
	return []string{
		"**/*.md",
		"**/*.mdx",
		"**/*.markdown",
		"**/*.mdown",
		"**/*.mkd",
		"**/*.mkdn",
	}
}

// DefaultExcludes returns patterns this linter should ignore by default
// Note: Common patterns like .git/**, node_modules/**, examples/**, hack/** are now
// handled by the all_language_excludes macro. This only returns Markdownlint-specific excludes.
func (m *Markdownlint) DefaultExcludes() []string {
	return []string{
		"*.min.md",   // Minified markdown files
		"LICENSE*",   // License files (different writing style)
		"CHANGELOG*", // Changelog files (different writing style)
	}
}

// GetSupportedLanguages returns the languages this linter can process
func (m *Markdownlint) GetSupportedLanguages() []string {
	return []string{"markdown"}
}

// GetEffectiveExcludes returns the complete list of exclusion patterns
// using the all_language_excludes macro for the given language and config
func (m *Markdownlint) GetEffectiveExcludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default excludes if no config
		return m.DefaultExcludes()
	}

	// Use the all_language_excludes macro
	return config.GetAllLanguageExcludes(language, m.DefaultExcludes())
}

// GetEffectiveIncludes returns the complete list of inclusion patterns
// for the given language and config
func (m *Markdownlint) GetEffectiveIncludes(language string, config *models.Config) []string {
	if config == nil {
		// Fallback to default includes if no config
		return m.DefaultIncludes()
	}

	// Use the combined includes system
	return config.GetAllLanguageIncludes(language, m.DefaultIncludes())
}

// SupportsJSON returns true if linter supports JSON output
func (m *Markdownlint) SupportsJSON() bool {
	return true
}

// JSONArgs returns additional args needed for JSON output
func (m *Markdownlint) JSONArgs() []string {
	return []string{"--json"}
}

// SupportsFix returns true if linter supports auto-fixing violations
func (m *Markdownlint) SupportsFix() bool {
	return true
}

// FixArgs returns additional args needed for fix mode
func (m *Markdownlint) FixArgs() []string {
	return []string{"--fix"}
}

// ValidateConfig validates linter-specific configuration
func (m *Markdownlint) ValidateConfig(config *models.LinterConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	return nil
}

// Run executes markdownlint and returns violations
func (m *Markdownlint) Run(ctx commonsContext.Context, task *clicky.Task) ([]models.Violation, error) {
	var args []string

	// Add configured args
	if m.Config != nil {
		args = append(args, m.Config.Args...)
	}

	// Add JSON format if requested and not already present
	if m.ForceJSON && !m.hasJSONArg(args) {
		args = append(args, "--json")
	}

	// Add extra args
	args = append(args, m.ExtraArgs...)

	// Add files or default to markdown files in current directory
	if len(m.Files) > 0 {
		args = append(args, m.Files...)
	} else if !m.hasPathArg(args) {
		// Markdownlint needs explicit file patterns
		args = append(args, "**/*.md")
	}

	// Execute command (markdownlint-cli2 is the modern version)
	cmdName := "markdownlint"
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = m.WorkDir

	logger.Infof("Executing: %s %s", cmdName, strings.Join(args, " "))

	output, err := cmd.CombinedOutput()

	// Handle markdownlint exit codes
	// Markdownlint exits with 1 when there are violations
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Exit code 1 with output means violations found - this is expected
			if len(output) > 0 {
				logger.Debugf("markdownlint exit code 1 with output - treating as success with violations")
				err = nil
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("markdownlint execution failed: %w\nOutput:\n%s", err, string(output))
	}

	// Parse JSON output if we have any
	if len(output) == 0 {
		return []models.Violation{}, nil
	}

	return m.parseViolations(output)
}

// hasJSONArg checks if args already contain JSON output flag
func (m *Markdownlint) hasJSONArg(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "-j" {
			return true
		}
	}
	return false
}

// hasPathArg checks if the args already contain a path argument
func (m *Markdownlint) hasPathArg(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// parseViolations parses markdownlint JSON output into violations
func (m *Markdownlint) parseViolations(output []byte) ([]models.Violation, error) {
	// Markdownlint JSON output is a map of filename to array of violations
	var results map[string][]MarkdownlintIssue
	if err := json.Unmarshal(output, &results); err != nil {
		// Try parsing as markdownlint-cli2 format
		return m.parseMarkdownlintCli2(output)
	}

	var violations []models.Violation
	for filename, issues := range results {
		for _, issue := range issues {
			violation := issue.ToViolation(m.WorkDir, filename)
			violations = append(violations, violation)
		}
	}

	return violations, nil
}

// parseMarkdownlintCli2 parses markdownlint-cli2 JSON output format
func (m *Markdownlint) parseMarkdownlintCli2(output []byte) ([]models.Violation, error) {
	var results []MarkdownlintCli2Result
	if err := json.Unmarshal(output, &results); err != nil {
		// If JSON parsing fails, log the output for debugging
		logger.Debugf("Failed to parse markdownlint JSON output: %v\nOutput: %s", err, string(output))
		return nil, fmt.Errorf("failed to parse markdownlint JSON output: %w", err)
	}

	var violations []models.Violation
	for _, result := range results {
		violation := result.ToViolation(m.WorkDir)
		violations = append(violations, violation)
	}

	return violations, nil
}

// MarkdownlintIssue represents a single issue from markdownlint (cli1 format)
type MarkdownlintIssue struct {
	LineNumber      int      `json:"lineNumber"`
	RuleNames       []string `json:"ruleNames"`
	RuleDescription string   `json:"ruleDescription"`
	RuleInformation string   `json:"ruleInformation,omitempty"`
	ErrorDetail     string   `json:"errorDetail,omitempty"`
	ErrorContext    string   `json:"errorContext,omitempty"`
	ErrorRange      []int    `json:"errorRange,omitempty"`
}

// ToViolation converts a MarkdownlintIssue to a generic Violation
func (i *MarkdownlintIssue) ToViolation(workDir, filename string) models.Violation {
	// Make filename absolute if it's relative
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(workDir, filename)
	}

	// Build rule name
	ruleName := "unknown"
	if len(i.RuleNames) > 0 {
		ruleName = strings.Join(i.RuleNames, "/")
	}

	// Build message
	message := i.RuleDescription
	if i.ErrorDetail != "" {
		message = fmt.Sprintf("%s: %s", i.RuleDescription, i.ErrorDetail)
	}
	if i.ErrorContext != "" {
		message = fmt.Sprintf("%s [%s]", message, i.ErrorContext)
	}

	// Determine column from error range if available
	column := 0
	if len(i.ErrorRange) > 0 {
		column = i.ErrorRange[0]
	}

	return models.Violation{
		File:          filename,
		Line:          i.LineNumber,
		Column:        column,
		CallerPackage: filepath.Dir(filename),
		CallerMethod:  "unknown",
		CalledPackage: "markdownlint",
		CalledMethod:  ruleName,
		Message:       message,
		Source:        "markdownlint",
	}
}

// MarkdownlintCli2Result represents a single result from markdownlint-cli2
type MarkdownlintCli2Result struct {
	FileName        string   `json:"fileName"`
	LineNumber      int      `json:"lineNumber"`
	RuleNames       []string `json:"ruleNames"`
	RuleDescription string   `json:"ruleDescription"`
	RuleInformation string   `json:"ruleInformation,omitempty"`
	ErrorDetail     string   `json:"errorDetail,omitempty"`
	ErrorContext    string   `json:"errorContext,omitempty"`
	ErrorRange      []int    `json:"errorRange,omitempty"`
}

// ToViolation converts a MarkdownlintCli2Result to a generic Violation
func (r *MarkdownlintCli2Result) ToViolation(workDir string) models.Violation {
	issue := &MarkdownlintIssue{
		LineNumber:      r.LineNumber,
		RuleNames:       r.RuleNames,
		RuleDescription: r.RuleDescription,
		RuleInformation: r.RuleInformation,
		ErrorDetail:     r.ErrorDetail,
		ErrorContext:    r.ErrorContext,
		ErrorRange:      r.ErrorRange,
	}
	return issue.ToViolation(workDir, r.FileName)
}
