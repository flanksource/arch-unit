package fixtures

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/gomplate/v3"
	"github.com/samber/lo"
)

// RunnerOptions configures the fixture runner
type RunnerOptions struct {
	Paths      []string // Fixture file paths/patterns
	Format     string   // Output format: tree, table, json, yaml, csv
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
	tree        *FixtureNode // Hierarchical tree structure
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
		tree: &FixtureNode{
			Name: "Fixtures",
			Type: SectionNode,
		},
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
	clicky.WaitForGlobalCompletionSilent()

	fmt.Println(output)

	// Return error if any tests failed
	if results.Summary.Failed > 0 {
		return fmt.Errorf("fixture tests failed")
	}

	return nil
}

// parseFixtureFiles parses all fixture files from the provided paths and builds tree structure
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
			// Parse with tree structure
			fileTree, err := ParseMarkdownFixturesWithTree(filepath)
			if err != nil {
				return fmt.Errorf("failed to parse fixture file '%s': %w", filepath, err)
			}

			// Merge file tree into main tree
			if fileTree != nil {
				r.tree.AddChild(fileTree)
			}

			// Also maintain flat fixture list for backwards compatibility
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

	// Log the loaded fixtures
	fileCount := len(r.tree.Children)
	logger.Infof("Loaded %d total fixtures in %d files", len(allFixtures), fileCount)
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
func (r *Runner) executeFixtures() (*FixtureGroup, error) {
	results := &FixtureGroup{
		Tests:   make([]FixtureNode, 0, len(r.fixtures)),
		Summary: Stats{},
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

	taskToNodeMap := make(map[*clicky.Task]*FixtureNode)
	var tasks []*clicky.Task
	r.tree.Walk(func(node *FixtureNode) {
		if node.Test != nil {
			task := r.createFixtureTask(*node.Test, buildTask)
			tasks = append(tasks, task)
			taskToNodeMap[task] = node
		}
	})

	// Wait for all tasks to complete and collect results
	logger.Debugf("Waiting for %d tasks to complete", len(tasks))
	for _, task := range tasks {
		waitResult := task.WaitFor()
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
		} else {
			logger.Warnf("No tree node found for task: %s", task.Name())
		}

	}

	r.tree.Stats = lo.ToPtr(r.tree.GetStats())

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

// executeBuildCommand runs the build command with context cancellation and gomplate templating
func (r *Runner) executeBuildCommand(ctx flanksourceContext.Context, buildCmd string) error {
	// Prepare template context for build command
	templateData := make(map[string]interface{})
	templateData["PWD"] = r.options.WorkDir
	templateData["WorkDir"] = r.options.WorkDir

	// Template the build command
	templatedCmd, err := renderBuildTemplate(buildCmd, templateData)
	if err != nil {
		ctx.Errorf("Failed to template build command: %v", err)
		return fmt.Errorf("failed to template build command: %w", err)
	}

	ctx.Infof("🔨 Build command: %s", templatedCmd)

	cmd := exec.CommandContext(ctx, "sh", "-c", templatedCmd)
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
	t := r.taskManager.StartWithResult(fixture.String(),
		func(ctx flanksourceContext.Context, task *clicky.Task) (interface{}, error) {
			// Execute the fixture (build task dependency is handled by TaskManager)
			result, err := r.executeFixture(ctx, fixture)
			return result, err
		},
		clicky.WithDependencies(buildTask),
		clicky.WithTaskTimeout(2*time.Minute),
	)

	return t
}

// executeFixture runs a single fixture test
func (r *Runner) executeFixture(ctx flanksourceContext.Context, fixture FixtureTest) (FixtureResult, error) {
	// Get the appropriate fixture type from registry
	fixtureType, err := DefaultRegistry.GetForFixture(fixture)
	if err != nil {
		return FixtureResult{}, fmt.Errorf("fixture type error: %w", err)
	}

	if r.options.WorkDir == "" {
		r.options.WorkDir, _ = os.Getwd()
	}
	ctx.Debugf("Using CWD: %s", r.options.WorkDir)

	// Prepare run options with flanksource context
	opts := RunOptions{
		WorkDir:   r.options.WorkDir,
		Verbose:   ctx.Logger.IsLevelEnabled(logger.Debug),
		NoCache:   false,
		Evaluator: r.evaluator,
		ExtraArgs: map[string]interface{}{
			"flanksource_context": ctx,
		},
	}

	start := time.Now()
	// Run the fixture test
	result := fixtureType.Run(ctx, fixture, opts)
	result.Duration = time.Since(start)

	return result, nil
}

// renderBuildTemplate renders a gomplate template for build commands
func renderBuildTemplate(template string, data map[string]interface{}) (string, error) {
	return gomplate.RunTemplate(data, gomplate.Template{
		Template: template,
	})
}

// taskResultToFixtureResult converts a task result to a FixtureResult
func (r *Runner) taskResultToFixtureResult(t *clicky.Task, waitResult *clicky.WaitResult) FixtureResult {
	// Get the actual fixture result from the task
	taskResult, taskErr := t.GetResult()

	if result, ok := taskResult.(FixtureResult); ok {
		if result.Name == "" {
			result.Name = t.Name()
		}

		if taskErr != nil {
			result.Status = task.StatusERR
			result.Error += taskErr.Error()
		}
		return result
	}

	return FixtureResult{
		Status: task.StatusERR,
		Error:  fmt.Sprintf("Unknown result %t -> %s", taskResult, taskErr),
	}
}
