package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/fixtures"
	_ "github.com/flanksource/arch-unit/fixtures/types" // Register fixture types
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

func runASTFixtures(cmd *cobra.Command, args []string) error {
	// Get working directory
	workingDir, err := GetWorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create runner with options
	runner, err := fixtures.NewRunner(fixtures.RunnerOptions{
		Paths:      args,
		Format:     clicky.Flags.Format,
		Filter:     "", // No filter by default
		NoColor:    clicky.Flags.FormatOptions.NoColor,
		WorkDir:    workingDir,
		MaxWorkers: clicky.Flags.MaxConcurrent,
		Logger:     logger.StandardLogger(),
	})
	if err != nil {
		return fmt.Errorf("failed to create fixture runner: %w", err)
	}

	// Run the fixtures
	return runner.Run()
}
