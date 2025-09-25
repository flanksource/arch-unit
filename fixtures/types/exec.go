package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/fixtures"
	"github.com/flanksource/clicky/task"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/gomplate/v3"
)

// ExecFixture implements FixtureType for command execution tests
type ExecFixture struct{}

// ensure ExecFixture implements FixtureType
var _ fixtures.FixtureType = (*ExecFixture)(nil)

// Name returns the type identifier
func (e *ExecFixture) Name() string {
	return "exec"
}

// Run executes the command test with gomplate template support
func (e *ExecFixture) Run(ctx context.Context, fixture fixtures.FixtureTest, opts fixtures.RunOptions) fixtures.FixtureResult {
	start := time.Now()
	result := fixtures.FixtureResult{
		Test:     fixture,
		Name:     fixture.Name,
		Type:     "exec",
		Metadata: make(map[string]interface{}),
	}

	// Prepare template context
	templateData := make(map[string]interface{})

	// Add base template variables
	templateData["PWD"] = opts.WorkDir // Parent directory (where command is called from)
	templateData["WorkDir"] = opts.WorkDir
	templateData["TestName"] = fixture.Name
	templateData["CWD"] = fixture.CWD

	// Determine the base directory for working directory resolution
	// Prefer fixture.SourceDir (directory containing fixture file) over opts.WorkDir
	baseDir := opts.WorkDir
	if fixture.SourceDir != "" {
		baseDir = fixture.SourceDir
	}

	// Use the base directory as default working directory
	// If fixture.CWD is specified, resolve it relative to base directory
	workDir := baseDir
	if fixture.CWD != "" && fixture.CWD != "." {
		if filepath.IsAbs(fixture.CWD) {
			// If CWD is absolute, use it directly
			workDir = fixture.CWD
		} else {
			// If CWD is relative, resolve it from the base directory (fixture file location)
			workDir = filepath.Join(baseDir, fixture.CWD)
		}
	}
	templateData["workDir"] = workDir

	// Get flanksource context if available
	var flanksourceCtx flanksourceContext.Context
	var hasContext bool
	if fCtx, ok := opts.ExtraArgs["flanksource_context"].(flanksourceContext.Context); ok {
		flanksourceCtx = fCtx
		hasContext = true
	}

	// Execute build command if specified (but skip it in task mode since build task handles it)
	if fixture.Build != "" && !hasContext {
		if opts.Verbose {
			logger.Infof("🔨 Build command: %s", fixture.Build)
		}

		buildCmd := exec.CommandContext(ctx, "sh", "-c", fixture.Build)
		buildCmd.Dir = workDir

		var buildOut bytes.Buffer
		buildCmd.Stdout = &buildOut
		buildCmd.Stderr = &buildOut

		if err := buildCmd.Run(); err != nil {
			result.Status = task.StatusFAIL
			result.Error = fmt.Sprintf("build command failed: %v\nOutput: %s", err, buildOut.String())
			result.Duration = time.Since(start)
			return result
		}

		if opts.Verbose && buildOut.Len() > 0 {
			logger.Debugf("Build output: %s", buildOut.String())
		}
	}

	// Create temp files if needed
	tempFiles := make(map[string]*TempFileInfo)
	if tempFileData, ok := opts.ExtraArgs["temp_files"].(map[string]interface{}); ok {
		for name, content := range tempFileData {
			tempFile, err := createTempFile(name, fmt.Sprint(content))
			if err != nil {
				result.Status = task.StatusFAIL
				result.Error = fmt.Sprintf("failed to create temp file '%s': %v", name, err)
				result.Duration = time.Since(start)
				return result
			}
			defer os.Remove(tempFile.Path)

			tempFiles[name] = tempFile
			templateData[name] = tempFile.GetTemplateData()
		}
	}

	// Determine the command to execute
	var command string
	if fixture.Exec != "" {
		// Template the exec command from front-matter
		templatedExec, err := renderTemplate(fixture.Exec, templateData)
		if err != nil {
			result.Status = task.StatusFAIL
			result.Error = err.Error()
			return result
		}
		command = templatedExec

		if fixture.CLIArgs != "" {
			// Template the CLI args
			templatedArgs, err := renderTemplate(fixture.CLIArgs, templateData)
			if err != nil {
				result.Status = task.StatusFAIL
				result.Error = fmt.Sprintf("failed to render CLI args template: %v", err)
				result.Duration = time.Since(start)
				return result
			}
			command = fmt.Sprintf("%s %s", templatedExec, templatedArgs)
		}
	} else if fixture.CLI != "" {
		command = fixture.CLI
	} else if fixture.CLIArgs != "" {
		// Use executable path with CLI args when no exec is specified
		if opts.ExecutablePath != "" {
			// Template CLI args 
			templatedArgs, err := renderTemplate(fixture.CLIArgs, templateData)
			if err != nil {
				result.Status = task.StatusERR
				result.Error = err.Error()
				result.Duration = time.Since(start)
				return result
			}
			command = fmt.Sprintf("%s %s", opts.ExecutablePath, templatedArgs)
		} else {
			// Template CLI args even when no exec is specified (fallback)
			templatedArgs, err := renderTemplate(fixture.CLIArgs, templateData)
			if err != nil {
				result.Status = task.StatusERR
				result.Error = err.Error()
				result.Duration = time.Since(start)
				return result
			}
			command = templatedArgs
		}
	} else if opts.ExecutablePath != "" {
		// Default to using the current executable when no command is specified
		command = opts.ExecutablePath
	}

	if command == "" {
		result.Status = task.StatusERR
		result.Error = "No command specified"
		return result
	}

	// Render command with gomplate
	renderedCommand, err := renderTemplate(command, templateData)
	if err != nil {
		result.Status = task.StatusERR
		result.Error = err.Error()
		return result
	}

	// Store the command and CWD in the result for JSON output
	result.Command = renderedCommand
	result.CWD = workDir

	// Log the command using appropriate logger
	if hasContext {
		flanksourceCtx.Debugf("🚀 Exec command: %s", renderedCommand)
	} else if opts.Verbose {
		logger.Debugf("🚀 Exec command: %s", renderedCommand)
	}

	// Parse rendered command
	args := strings.Fields(renderedCommand)
	if len(args) == 0 {
		result.Status = task.StatusFAIL
		result.Error = "no command specified"
		result.Duration = time.Since(start)
		return result
	}

	// Execute the command
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workDir

	// Log working directory for debugging
	if hasContext {
		flanksourceCtx.Debugf("Command working directory: %s", workDir)
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Add environment variables from fixture
	if envVars, ok := fixture.Expected.Properties["env"].(map[string]interface{}); ok {
		env := os.Environ()
		for key, value := range envVars {
			env = append(env, fmt.Sprintf("%s=%v", key, value))
		}
		cmd.Env = env
	}

	// Run the command
	if hasContext {
		flanksourceCtx.Debugf("Running: %s", renderedCommand)
	}
	result.CWD = workDir
	err = cmd.Run()
	result.Duration = time.Since(start)
	exitCode := 0
	if exitError, ok := err.(*exec.ExitError); ok {
		exitCode = exitError.ExitCode()
	} else if err != nil {
		// Any error that's not an ExitError is a command execution failure
		result.Status = task.StatusFAIL
		result.Error = err.Error()
		return result
	}

	// Store execution results in enhanced fields
	result.ExitCode = exitCode
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	// Check for expected error
	if fixture.Expected.Error != "" {
		if exitCode == 0 {
			result.Status = task.StatusFAIL
			result.Error = fmt.Sprintf("expected error containing '%s' but command succeeded", fixture.Expected.Error)
			return result
		}
		if !strings.Contains(result.Out(), fixture.Expected.Error) {
			result.Status = task.StatusFAIL
			result.Error = fmt.Sprintf("expected error containing '%s', got: %s", fixture.Expected.Error, result.Out())
			return result
		}
		result.Status = "PASS"
		return result
	}

	// For non-error cases, check exit code
	if fixture.Expected.Properties != nil {
		if expectedCode, ok := fixture.Expected.Properties["exitCode"].(int); ok {
			if exitCode != expectedCode {
				result.Status = task.StatusFAIL
				result.Error = fmt.Sprintf("%d != %d", expectedCode, exitCode)
				return result
			}
		}
	} else if exitCode != 0 {
		// Default: expect success (exit code 0)
		result.Status = task.StatusFAIL
		result.Error = fmt.Sprintf("exit code %d", exitCode)
		return result
	}

	// Check expected output
	if fixture.Expected.Output != "" {
		if !strings.Contains(result.Out(), fixture.Expected.Output) {
			result.Status = task.StatusFAIL
			result.Error = fmt.Sprintf("output does not contain'%s'", fixture.Expected.Output)
			return result
		}
	}

	// Prepare CEL evaluation context
	if fixture.CEL != "" && fixture.CEL != "true" && opts.Evaluator != nil {
		celContext := map[string]interface{}{
			"stdout":   result.Stdout,
			"stderr":   result.Stderr,
			"exitCode": exitCode,
			"output":   result.Out(),
		}

		// Try to parse JSON output if it looks like JSON
		if strings.HasPrefix(strings.TrimSpace(result.Stdout), "{") || strings.HasPrefix(strings.TrimSpace(result.Stdout), "[") {
			var jsonData interface{}
			if err := json.Unmarshal([]byte(result.Stdout), &jsonData); err == nil {
				celContext["json"] = jsonData
				result.Metadata["json"] = jsonData
			}
		}

		// Add temp file data to CEL context
		for name, tempFile := range tempFiles {
			celContext[name] = tempFile.GetCELData()
		}

		valid, err := opts.Evaluator.EvaluateResult(fixture.CEL, celContext)
		if err != nil {
			result.Status = task.StatusERR
			result.Error = err.Error()

			return result
		}
		result.CELResult = valid
		if !valid {
			result.Status = task.StatusFAIL
			result.Error = fmt.Sprintf("(%s) != true", fixture.CEL)

			return result
		}
	}

	result.Status = task.StatusPASS

	return result
}

// ValidateFixture validates that the fixture has required fields
func (e *ExecFixture) ValidateFixture(fixture fixtures.FixtureTest) error {
	if fixture.CLI == "" && fixture.CLIArgs == "" {
		return fmt.Errorf("exec fixture requires either 'CLI' or 'CLIArgs' field")
	}
	return nil
}

// GetRequiredFields returns required fields
func (e *ExecFixture) GetRequiredFields() []string {
	return []string{"CLI or CLIArgs"}
}

// GetOptionalFields returns optional fields
func (e *ExecFixture) GetOptionalFields() []string {
	return []string{"CWD", "CEL", "Expected.Output", "Expected.Error", "Expected.exitCode", "env"}
}

// TempFileInfo holds information about a temporary file
type TempFileInfo struct {
	Path      string
	Content   string
	Extension string
	Detected  string // Detected file type (would use libmagic)
}

// GetTemplateData returns data for gomplate templates
func (t *TempFileInfo) GetTemplateData() map[string]interface{} {
	return map[string]interface{}{
		"path":     t.Path,
		"content":  t.Content,
		"ext":      t.Extension,
		"detected": t.Detected,
	}
}

// GetCELData returns data for CEL evaluation
func (t *TempFileInfo) GetCELData() map[string]interface{} {
	data := t.GetTemplateData()

	// Try to parse as JSON if it looks like JSON
	if strings.HasPrefix(strings.TrimSpace(t.Content), "{") || strings.HasPrefix(strings.TrimSpace(t.Content), "[") {
		var jsonData interface{}
		if err := json.Unmarshal([]byte(t.Content), &jsonData); err == nil {
			data["json"] = jsonData
		}
	}

	return data
}

// createTempFile creates a temporary file with the given content
func createTempFile(name, content string) (*TempFileInfo, error) {
	// Determine extension from name
	ext := filepath.Ext(name)
	if ext == "" {
		ext = ".tmp"
	}

	// Create temp file
	tmpFile, err := ioutil.TempFile("", fmt.Sprintf("fixture-%s-*%s", name, ext))
	if err != nil {
		return nil, err
	}

	// Write content
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	_ = tmpFile.Close()

	// Detect file type (simplified - would use libmagic in production)
	detected := "text"
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		detected = "json"
	} else if strings.HasPrefix(content, "<?xml") {
		detected = "xml"
	} else if strings.HasPrefix(content, "---\n") {
		detected = "yaml"
	}

	return &TempFileInfo{
		Path:      tmpFile.Name(),
		Content:   content,
		Extension: ext,
		Detected:  detected,
	}, nil
}

// renderTemplate renders a gomplate template
func renderTemplate(template string, data map[string]interface{}) (string, error) {
	// Use gomplate's RunTemplate function
	tmpl := gomplate.Template{
		Template: template,
	}

	result, err := gomplate.RunTemplate(data, tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return result, nil
}

func init() {
	// Register the exec fixture type
	_ = fixtures.Register(&ExecFixture{})
}
