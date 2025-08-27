package fixtures

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/flanksource/clicky"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
)

// RunnerOptions configures the fixture runner
type RunnerOptions struct {
	Paths      []string // Fixture file paths/patterns
	Format     string   // Output format: table, tree, json
	Filter     string   // Filter tests by name pattern (glob)
	NoColor    bool     // Disable colored output
	WorkDir    string   // Working directory
	MaxWorkers int      // Maximum number of parallel workers
	Logger     logger.Logger
}

// Runner manages fixture test execution using TaskManager
type Runner struct {
	options     RunnerOptions
	fixtures    []FixtureTest
	evaluator   *CELEvaluator
	taskManager *clicky.TaskManager
	tree        *FixtureTree // Hierarchical tree structure
}

// NewRunner creates a new fixture runner
func NewRunner(opts RunnerOptions) (*Runner, error) {
	// Create CEL evaluator
	evaluator, err := NewCELEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL evaluator: %w", err)
	}

	// Create task manager with specified concurrency (default to 1 if not specified)
	maxWorkers := opts.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	taskManager := clicky.NewTaskManagerWithConcurrency(maxWorkers)
	taskManager.SetNoColor(opts.NoColor)

	return &Runner{
		options:     opts,
		fixtures:    []FixtureTest{},
		evaluator:   evaluator,
		taskManager: taskManager,
		tree:        NewFixtureTree(),
	}, nil
}

// Run executes the fixture tests
func (r *Runner) Run() error {
	// Parse fixture files
	if err := r.parseFixtureFiles(); err != nil {
		return fmt.Errorf("failed to parse fixture files: %w", err)
	}

	// Apply filter if specified
	if r.options.Filter != "" {
		r.filterTests()
	}

	if len(r.fixtures) == 0 {
		return fmt.Errorf("no fixtures found")
	}

	// Execute fixtures using TaskManager
	results, err := r.executeFixtures()
	if err != nil {
		return fmt.Errorf("failed to execute fixtures: %w", err)
	}

	// Display results using clicky.Format() with tree structure
	formatOptions := clicky.FormatOptions{
		Format:  r.options.Format,
		NoColor: r.options.NoColor,
	}
	if r.options.Format == "" {
		formatOptions.Format = "tree" // Default to tree format
	}

	// Debug: print tree structure
	if r.options.Logger != nil && r.options.Logger.IsLevelEnabled(3) {
		r.options.Logger.Debugf("Tree structure: %+v", r.tree)
	}

	output, err := clicky.Format(r.tree, formatOptions)
	if err != nil {
		return fmt.Errorf("failed to format results: %w", err)
	}

	fmt.Println(output)

	// Return error if any tests failed
	if results.Summary.Failed > 0 {
		return fmt.Errorf("fixture tests failed")
	}

	return nil
}

// parseFixtureFiles parses all fixture files from the provided paths
func (r *Runner) parseFixtureFiles() error {
	var allFixtures []FixtureTest

	for _, pattern := range r.options.Paths {
		// Expand glob patterns
		matches, err := doublestar.FilepathGlob(pattern)
		if err != nil {
			return fmt.Errorf("invalid glob pattern '%s': %w", pattern, err)
		}

		if len(matches) == 0 {
			logger.Warnf("No files matched pattern: %s", pattern)
			continue
		}

		for _, filepath := range matches {
			fixtures, err := ParseMarkdownFixtures(filepath)
			if err != nil {
				return fmt.Errorf("failed to parse fixture file '%s': %w", filepath, err)
			}

			logger.Debugf("Parsed %d fixtures from %s", len(fixtures), filepath)
			// Extract FixtureTest from each FixtureNode
			for _, node := range fixtures {
				if node.Test != nil {
					allFixtures = append(allFixtures, *node.Test)
				}
			}
		}
	}

	r.fixtures = allFixtures
	logger.Infof("Loaded %d total fixtures", len(allFixtures))
	return nil
}

// filterTests applies name filtering to loaded tests
func (r *Runner) filterTests() {
	var filtered []FixtureTest

	for _, fixture := range r.fixtures {
		match, err := doublestar.Match(r.options.Filter, fixture.Name)
		if err != nil {
			logger.Warnf("Invalid filter pattern '%s': %v", r.options.Filter, err)
			continue
		}
		if match {
			filtered = append(filtered, fixture)
		}
	}

	logger.Infof("Filtered to %d fixtures matching '%s'", len(filtered), r.options.Filter)
	r.fixtures = filtered
}

// executeFixtures runs all fixtures using TaskManager concurrency
func (r *Runner) executeFixtures() (*FixtureResults, error) {
	results := &FixtureResults{
		Tests:   make([]FixtureResult, 0, len(r.fixtures)),
		Summary: ResultSummary{},
	}

	// Check if any fixtures need build
	buildCmd := r.getBuildCommand()
	var buildTask *clicky.Task

	// Create build task if needed (as dependency for other tasks)
	if buildCmd != "" {
		buildTask = r.taskManager.Start(
			fmt.Sprintf("Build: %s", buildCmd),
			clicky.WithTaskTimeout(5*time.Minute),
			clicky.WithFunc(func(ctx flanksourceContext.Context, task *clicky.Task) error {
				return r.executeBuildCommand(ctx, buildCmd)
			}),
		)
	}

	// Create tasks for each fixture using the tree structure
	testNodes := r.tree.AllTests
	tasks := make([]*clicky.Task, 0, len(testNodes))
	taskToNodeMap := make(map[*clicky.Task]*FixtureNode)

	for _, testNode := range testNodes {
		if testNode.Test != nil {
			task := r.createFixtureTask(*testNode.Test, buildTask)
			tasks = append(tasks, task)
			taskToNodeMap[task] = testNode
		}
	}

	// Wait for all tasks to complete and collect results
	logger.Debugf("Waiting for %d tasks to complete", len(tasks))
	for i, task := range tasks {
		logger.Debugf("Waiting for task %d/%d: %s", i+1, len(tasks), task.Name())
		waitResult := task.WaitFor()
		logger.Debugf("Task %s completed with status: %v", task.Name(), waitResult.Status)
		result := r.taskResultToFixtureResult(task, waitResult)

		// Create a FixtureNode for the result
		resultNode := FixtureNode{
			Name:    result.Name,
			Type:    TestNode,
			Results: &result,
		}
		results.Tests = append(results.Tests, resultNode)

		// Update the corresponding tree node with results
		if testNode, exists := taskToNodeMap[task]; exists {
			testNode.Results = &result
			logger.Debugf("Updated node %s with result: %v (duration: %v)", testNode.Name, result.Status, result.Duration)
		} else {
			logger.Warnf("No tree node found for task: %s", task.Name())
		}

		// Update summary
		results.Summary.Total++
		switch result.Status {
		case "PASS":
			results.Summary.Passed++
		case "FAIL":
			results.Summary.Failed++
		case "SKIP":
			results.Summary.Skipped++
		}
	}

	// Update tree statistics after all tests complete
	r.tree.UpdateStats()

	return results, nil
}

// getBuildCommand extracts build command from first fixture that has one
func (r *Runner) getBuildCommand() string {
	for _, fixture := range r.fixtures {
		if fixture.Build != "" {
			return fixture.Build
		}
	}
	return ""
}

// executeBuildCommand runs the build command with context cancellation
func (r *Runner) executeBuildCommand(ctx flanksourceContext.Context, buildCmd string) error {
	ctx.Infof("🔨 Build command: %s", buildCmd)

	cmd := exec.CommandContext(ctx, "sh", "-c", buildCmd)
	cmd.Dir = r.options.WorkDir

	var buildOut bytes.Buffer
	cmd.Stdout = &buildOut
	cmd.Stderr = &buildOut

	if err := cmd.Run(); err != nil {
		ctx.Errorf("Build failed: %v\nOutput: %s", err, buildOut.String())
		return fmt.Errorf("build command failed: %v\nOutput: %s", err, buildOut.String())
	}

	if buildOut.Len() > 0 {
		ctx.Debugf("Build output: %s", buildOut.String())
	}

	return nil
}

// createFixtureTask creates a task for a single fixture
func (r *Runner) createFixtureTask(fixture FixtureTest, buildTask *clicky.Task) *clicky.Task {
	taskName := r.getFixtureTaskName(fixture)

	// Prepare task options
	opts := []clicky.TaskOption{
		clicky.WithTaskTimeout(2 * time.Minute), // Default 2-minute timeout per test
	}

	// Add build task as dependency if it exists
	if buildTask != nil {
		opts = append(opts, clicky.WithDependencies(buildTask))
	}

	task := r.taskManager.StartWithResult(taskName,
		func(ctx flanksourceContext.Context, task *clicky.Task) (interface{}, error) {
			// Execute the fixture (build task dependency is handled by TaskManager)
			return r.executeFixture(ctx, fixture)
		},
		opts...,
	)

	return task
}

// getFixtureTaskName creates a descriptive task name
func (r *Runner) getFixtureTaskName(fixture FixtureTest) string {
	command := r.getFixtureCommand(fixture)
	return fmt.Sprintf("%s: %s", fixture.Name, command)
}

// getFixtureCommand extracts the command string from a fixture
func (r *Runner) getFixtureCommand(fixture FixtureTest) string {
	if fixture.Exec != "" {
		if fixture.CLIArgs != "" {
			return fmt.Sprintf("%s %s", fixture.Exec, fixture.CLIArgs)
		}
		return fixture.Exec
	}
	if fixture.CLI != "" {
		return fixture.CLI
	}
	if fixture.CLIArgs != "" {
		return fixture.CLIArgs
	}
	if fixture.Query != "" {
		return fmt.Sprintf("query: %s", fixture.Query)
	}
	return "unknown command"
}

// executeFixture runs a single fixture test
func (r *Runner) executeFixture(ctx flanksourceContext.Context, fixture FixtureTest) (interface{}, error) {
	// Get the appropriate fixture type from registry
	fixtureType, err := DefaultRegistry.GetForFixture(fixture)
	if err != nil {
		return nil, fmt.Errorf("fixture type error: %w", err)
	}

	// Prepare run options with flanksource context
	opts := RunOptions{
		WorkDir:   r.options.WorkDir,
		Verbose:   true,
		NoCache:   false,
		Evaluator: r.evaluator,
		ExtraArgs: map[string]interface{}{
			"flanksource_context": ctx,
		},
	}

	// Run the fixture test
	result := fixtureType.Run(ctx, fixture, opts)

	// Check if the fixture failed and return an error
	if result.Status == "FAIL" {
		if result.Error != "" {
			return result, fmt.Errorf("fixture failed: %s", result.Error)
		}
		return result, fmt.Errorf("fixture failed")
	}

	return result, nil
}

// taskResultToFixtureResult converts a task result to a FixtureTestResult
func (r *Runner) taskResultToFixtureResult(task *clicky.Task, waitResult *clicky.WaitResult) FixtureTestResult {
	// Get the actual fixture result from the task
	taskResult, taskErr := task.GetResult()

	// Default result structure
	result := FixtureTestResult{
		Name:     task.Name(),
		Duration: waitResult.Duration.String(),
	}

	if taskErr != nil {
		result.Status = "FAIL"
		result.Error = taskErr.Error()
		return result
	}

	// Try to cast the task result to FixtureResult
	if fixtureResult, ok := taskResult.(FixtureResult); ok {
		// Use the actual fixture result but preserve task-level info
		result = fixtureResult
		if result.Duration == "" {
			result.Duration = waitResult.Duration.String()
		}
		if result.Name == "" {
			result.Name = task.Name()
		}
	} else {
		// Fallback: infer status from task completion
		switch waitResult.Status {
		case clicky.StatusSuccess:
			result.Status = "PASS"
		case clicky.StatusFailed:
			result.Status = "FAIL"
			if waitResult.Error != nil {
				result.Error = waitResult.Error.Error()
			}
		case clicky.StatusCancelled:
			result.Status = "SKIP"
		default:
			result.Status = "SKIP"
		}
	}

	return result
}

// taskResultToFixtureResult converts task result to FixtureTestResult
func (r *Runner) taskResultToFixtureResult(task *clicky.Task, waitResult *clicky.WaitResult) FixtureTestResult {
	// Try to get fixture result from task result
	if result, err := task.GetResult(); err == nil && result != nil {
		if fixtureResult, ok := result.(FixtureResult); ok {
			return FixtureTestResult{
				Name:      fixtureResult.Name,
				Type:      fixtureResult.Type,
				Status:    fixtureResult.Status,
				Error:     fixtureResult.Error,
				Expected:  getIntFromInterface(fixtureResult.Expected),
				Actual:    getIntFromInterface(fixtureResult.Actual),
				CELResult: fixtureResult.CELResult,
				Duration:  fixtureResult.Duration,
				Details:   fixtureResult.Details,
				Command:   fixtureResult.Command,
				CWD:       fixtureResult.CWD,
				Stdout:    fixtureResult.Stdout,
				Stderr:    fixtureResult.Stderr,
				ExitCode:  fixtureResult.ExitCode,
			}
		}
	}

	// Fallback: create result from task info
	status := "FAIL"
	if waitResult.Status == clicky.StatusSuccess {
		status = "PASS"
	} else if waitResult.Status == clicky.StatusCancelled {
		status = "SKIP"
	}

	error := ""
	if waitResult.Error != nil {
		error = waitResult.Error.Error()
	}

	return FixtureTestResult{
		Name:     task.Name(),
		Type:     "task",
		Status:   status,
		Error:    error,
		Duration: waitResult.Duration.String(),
	}
}

// formatResults formats the test results in the specified format
func (r *Runner) formatResults(results *FixtureResults) (string, error) {
	switch r.options.Format {
	case "json":
		return r.formatJSON(results)
	case "tree":
		return r.formatTree(results)
	case "table":
		fallthrough
	default:
		return r.formatTable(results)
	}
}

// formatJSON formats results as JSON
func (r *Runner) formatJSON(results *FixtureResults) (string, error) {
	data, err := jsonMarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to format JSON: %w", err)
	}
	return string(data), nil
}

// formatTree formats results as a tree view (simplified)
func (r *Runner) formatTree(results *FixtureResults) (string, error) {
	output := fmt.Sprintf("📊 Summary: %d total, %d passed, %d failed, %d skipped\n\n",
		results.Summary.Total, results.Summary.Passed, results.Summary.Failed, results.Summary.Skipped)

	for _, test := range results.Tests {
		var symbol string
		switch test.Status {
		case "PASS":
			symbol = "✓"
		case "FAIL":
			symbol = "✗"
		case "SKIP":
			symbol = "⊘"
		default:
			symbol = "?"
		}

		output += fmt.Sprintf("%s %s (%s)\n", symbol, test.Name, test.Duration)
		if test.Error != "" {
			output += fmt.Sprintf("  Error: %s\n", test.Error)
		}
	}

	return output, nil
}

// formatTable formats results as a table (simplified)
func (r *Runner) formatTable(results *FixtureResults) (string, error) {
	output := fmt.Sprintf("Test Results Summary: %d total, %d passed, %d failed, %d skipped\n\n",
		results.Summary.Total, results.Summary.Passed, results.Summary.Failed, results.Summary.Skipped)

	output += "Status | Test Name | Duration | Error\n"
	output += "-------|-----------|----------|------\n"

	for _, test := range results.Tests {
		errorMsg := test.Error
		if len(errorMsg) > 50 {
			errorMsg = errorMsg[:47] + "..."
		}
		output += fmt.Sprintf("%-6s | %-40s | %8s | %s\n",
			test.Status, test.Name, test.Duration, errorMsg)
	}

	return output, nil
}

// Helper functions

// getIntFromInterface safely converts interface{} to int
func getIntFromInterface(val interface{}) int {
	if val == nil {
		return 0
	}
	if intVal, ok := val.(int); ok {
		return intVal
	}
	return 0
}

// jsonMarshalIndent marshals to JSON with indentation
func jsonMarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// renderBuildTemplate renders a gomplate template for build commands
func renderBuildTemplate(template string, data map[string]interface{}) (string, error) {
	// Import gomplate if not already imported
	// This is a simple template rendering for build commands
	// For now, just do basic replacement of {{.PWD}} and {{.WorkDir}}
	result := template
	if pwd, ok := data["PWD"].(string); ok {
		result = strings.ReplaceAll(result, "{{.PWD}}", pwd)
	}
	if workDir, ok := data["WorkDir"].(string); ok {
		result = strings.ReplaceAll(result, "{{.WorkDir}}", workDir)
	}
	return result, nil
}

// taskResultToFixtureResult converts a task result to a FixtureTestResult
func (r *Runner) taskResultToFixtureResult(task *clicky.Task, waitResult *clicky.WaitResult) FixtureTestResult {
	// Get the actual fixture result from the task
	taskResult, taskErr := task.GetResult()

	// Default result structure
	result := FixtureTestResult{
		Name:     task.Name(),
		Duration: waitResult.Duration.String(),
	}

	if taskErr != nil {
		result.Status = "FAIL"
		result.Error = taskErr.Error()
		return result
	}

	// Try to cast the task result to FixtureTestResult
	if fixtureResult, ok := taskResult.(FixtureTestResult); ok {
		// Use the actual fixture result but preserve task-level info
		result = fixtureResult
		if result.Duration == "" {
			result.Duration = waitResult.Duration.String()
		}
		if result.Name == "" {
			result.Name = task.Name()
		}
	} else {
		// Fallback: infer status from task completion
		switch waitResult.Status {
		case clicky.StatusSuccess:
			result.Status = "PASS"
		case clicky.StatusFailed:
			result.Status = "FAIL"
			if waitResult.Error != nil {
				result.Error = waitResult.Error.Error()
			}
		case clicky.StatusCancelled:
			result.Status = "SKIP"
		default:
			result.Status = "SKIP"
		}
	}

	return result
}
