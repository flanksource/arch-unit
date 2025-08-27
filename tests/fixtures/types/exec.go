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

	"github.com/flanksource/arch-unit/tests/fixtures"
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
		Name:     fixture.Name,
		Type:     "exec",
		Metadata: make(map[string]interface{}),
	}

	// Prepare template context
	templateData := make(map[string]interface{})
	
	// Use the working directory from where the command is called as base
	// If fixture.CWD is absolute, use it directly; otherwise join with WorkDir
	workDir := opts.WorkDir
	if fixture.CWD != "" && fixture.CWD != "." {
		if filepath.IsAbs(fixture.CWD) {
			// If CWD is absolute, use it directly
			workDir = fixture.CWD
		} else {
			// If CWD is relative, resolve it from the calling directory
			workDir = filepath.Join(opts.WorkDir, fixture.CWD)
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
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("build command failed: %v\nOutput: %s", err, buildOut.String())
			result.Duration = time.Since(start).String()
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
				result.Status = "FAIL"
				result.Error = fmt.Sprintf("failed to create temp file '%s': %v", name, err)
				result.Duration = time.Since(start).String()
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
		// Use exec from front-matter as base command
		command = fixture.Exec
		if fixture.CLIArgs != "" {
			command = fmt.Sprintf("%s %s", fixture.Exec, fixture.CLIArgs)
		}
	} else if fixture.CLI != "" {
		command = fixture.CLI
	} else if fixture.CLIArgs != "" {
		command = fixture.CLIArgs
	}
	
	if command == "" {
		result.Status = "FAIL"
		result.Error = "no command specified"
		result.Duration = time.Since(start).String()
		return result
	}
	
	// Render command with gomplate
	renderedCommand, err := renderTemplate(command, templateData)
	if err != nil {
		result.Status = "FAIL"
		result.Error = fmt.Sprintf("failed to render command template: %v", err)
		result.Duration = time.Since(start).String()
		return result
	}
	
	// Store the command in the result for JSON output
	result.Command = renderedCommand
	
	// Log the command using appropriate logger
	if hasContext {
		flanksourceCtx.Infof("🚀 Exec command: %s", renderedCommand)
	} else if opts.Verbose {
		logger.Infof("🚀 Exec command: %s", renderedCommand)
	}
	
	// Parse rendered command
	args := strings.Fields(renderedCommand)
	if len(args) == 0 {
		result.Status = "FAIL"
		result.Error = "no command specified"
		result.Duration = time.Since(start).String()
		return result
	}
	
	// Execute the command
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workDir
	
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
	err = cmd.Run()
	exitCode := 0
	if exitError, ok := err.(*exec.ExitError); ok {
		exitCode = exitError.ExitCode()
	} else if err != nil && !strings.Contains(err.Error(), "exit status") {
		result.Status = "FAIL"
		result.Error = fmt.Sprintf("command execution failed: %v", err)
		result.Duration = time.Since(start).String()
		return result
	}
	
	// Store execution results in enhanced fields
	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	result.Output = stdoutStr + stderrStr
	result.ExitCode = exitCode
	result.Stdout = stdoutStr
	result.Stderr = stderrStr
	result.Metadata["stdout"] = stdoutStr
	result.Metadata["stderr"] = stderrStr
	result.Metadata["exitCode"] = exitCode
	
	// Log results using appropriate logger
	if hasContext {
		flanksourceCtx.Debugf("Exit code: %d", exitCode)
		if stdoutStr != "" {
			flanksourceCtx.Debugf("Stdout: %s", stdoutStr)
		}
		if stderrStr != "" {
			flanksourceCtx.Debugf("Stderr: %s", stderrStr)
		}
	} else if opts.Verbose {
		logger.Debugf("Exit code: %d", exitCode)
		if stdoutStr != "" {
			logger.Debugf("Stdout: %s", stdoutStr)
		}
		if stderrStr != "" {
			logger.Debugf("Stderr: %s", stderrStr)
		}
	}
	
	// Check for expected error
	if fixture.Expected.Error != "" {
		if exitCode == 0 {
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("expected error containing '%s' but command succeeded", fixture.Expected.Error)
			result.Duration = time.Since(start).String()
			return result
		}
		if !strings.Contains(result.Output, fixture.Expected.Error) {
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("expected error containing '%s', got: %s", fixture.Expected.Error, result.Output)
			result.Duration = time.Since(start).String()
			return result
		}
		result.Status = "PASS"
		result.Details = fmt.Sprintf("Got expected error with exit code %d", exitCode)
		result.Duration = time.Since(start).String()
		return result
	}
	
	// For non-error cases, check exit code
	if fixture.Expected.Properties != nil {
		if expectedCode, ok := fixture.Expected.Properties["exitCode"].(int); ok {
			if exitCode != expectedCode {
				result.Status = "FAIL"
				result.Error = fmt.Sprintf("expected exit code %d, got %d", expectedCode, exitCode)
				result.Duration = time.Since(start).String()
				return result
			}
		}
	} else if exitCode != 0 {
		// Default: expect success (exit code 0)
		result.Status = "FAIL"
		result.Error = fmt.Sprintf("command failed with exit code %d: %s", exitCode, stderrStr)
		result.Duration = time.Since(start).String()
		return result
	}
	
	// Check expected output
	if fixture.Expected.Output != "" {
		if !strings.Contains(result.Output, fixture.Expected.Output) {
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("output should contain '%s', got: %s", fixture.Expected.Output, result.Output)
			result.Duration = time.Since(start).String()
			return result
		}
	}
	
	// Prepare CEL evaluation context
	if fixture.CEL != "" && fixture.CEL != "true" && opts.Evaluator != nil {
		celContext := map[string]interface{}{
			"stdout":   stdoutStr,
			"stderr":   stderrStr,
			"exitCode": exitCode,
			"output":   result.Output,
		}
		
		// Try to parse JSON output if it looks like JSON
		if strings.HasPrefix(strings.TrimSpace(stdoutStr), "{") || strings.HasPrefix(strings.TrimSpace(stdoutStr), "[") {
			var jsonData interface{}
			if err := json.Unmarshal([]byte(stdoutStr), &jsonData); err == nil {
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
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("CEL evaluation failed: %v", err)
			result.Duration = time.Since(start).String()
			return result
		}
		result.CELResult = valid
		if !valid {
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("CEL validation failed: %s", fixture.CEL)
			result.Duration = time.Since(start).String()
			return result
		}
	}
	
	result.Status = "PASS"
	result.Duration = time.Since(start).String()
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
		"path":      t.Path,
		"content":   t.Content,
		"ext":       t.Extension,
		"detected":  t.Detected,
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
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	tmpFile.Close()
	
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
	fixtures.Register(&ExecFixture{})
}