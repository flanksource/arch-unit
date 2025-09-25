package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fatih/color"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var violationsCmd = &cobra.Command{
	Use:   "violations",
	Short: "Manage cached violations",
	Long: `View and manage violations stored in the cache database.
	
This command provides subcommands to list and clear violations from the cache.`,
}

var violationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached violations",
	Long: `List violations stored in the cache database.

Examples:
  # List all violations
  arch-unit violations list

  # List violations from the last 5 minutes
  arch-unit violations list --since "5m"
  
  # List violations from the last 2 hours
  arch-unit violations list --since "2h"
  
  # List violations from the last 7 days
  arch-unit violations list --since "7d"
  
  # List violations for specific files
  arch-unit violations list --path "**/*.go"
  
  # Combine filters
  arch-unit violations list --path "**/*_test.go" --since "1h"`,
	RunE: runViolationsList,
}

var violationsClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear cached violations",
	Long: `Clear violations from the cache database.

Examples:
  # Clear all violations
  arch-unit violations clear
  
  # Clear violations older than 7 days
  arch-unit violations clear --older "7d"
  
  # Clear violations older than 12 hours
  arch-unit violations clear --older "12h"
  
  # Clear violations for specific files
  arch-unit violations clear --path "**/*.go"
  
  # Combine filters
  arch-unit violations clear --path "**/*_test.go" --older "1d"`,
	RunE: runViolationsClear,
}

var (
	violationsSince string
	violationsOlder string
	violationsPath  string
)

func init() {
	rootCmd.AddCommand(violationsCmd)
	violationsCmd.AddCommand(violationsListCmd)
	violationsCmd.AddCommand(violationsClearCmd)

	// List command flags
	violationsListCmd.Flags().StringVar(&violationsSince, "since", "", "Show violations from the last duration (e.g., '5m', '2h', '7d')")
	violationsListCmd.Flags().StringVar(&violationsPath, "path", "", "Filter violations by file path pattern (glob)")

	// Clear command flags
	violationsClearCmd.Flags().StringVar(&violationsOlder, "older", "", "Clear violations older than duration (e.g., '7d', '12h')")
	violationsClearCmd.Flags().StringVar(&violationsPath, "path", "", "Filter violations by file path pattern (glob)")
}

func runViolationsList(cmd *cobra.Command, args []string) error {
	// Open violation cache
	violationCache, err := cache.NewViolationCache()
	if err != nil {
		return fmt.Errorf("failed to open violation cache: %w", err)
	}
	defer func() { _ = violationCache.Close() }()

	// Get all violations
	allViolations, err := violationCache.GetAllViolations()
	if err != nil {
		return fmt.Errorf("failed to get violations: %w", err)
	}

	// Filter violations
	violations := filterViolations(allViolations, violationsSince, violationsPath)

	if len(violations) == 0 {
		logger.Infof("No violations found matching the criteria")
		return nil
	}

	// Display violations
	displayViolationsList(violations)

	return nil
}

func runViolationsClear(cmd *cobra.Command, args []string) error {
	// Open violation cache
	violationCache, err := cache.NewViolationCache()
	if err != nil {
		return fmt.Errorf("failed to open violation cache: %w", err)
	}
	defer func() { _ = violationCache.Close() }()

	// Parse older duration if provided
	var olderThan time.Time
	if violationsOlder != "" {
		duration, err := parseDuration(violationsOlder)
		if err != nil {
			return fmt.Errorf("invalid duration '%s': %w", violationsOlder, err)
		}
		olderThan = time.Now().Add(-duration)
	}

	// Clear violations based on filters
	count, err := violationCache.ClearViolations(olderThan, violationsPath)
	if err != nil {
		return fmt.Errorf("failed to clear violations: %w", err)
	}

	// Display result
	if count == 0 {
		logger.Infof("No violations cleared")
	} else {
		logger.Infof("%s Cleared %d violation(s)",
			color.GreenString("✓"),
			count)

		if violationsOlder != "" {
			logger.Infof("  Older than: %s", violationsOlder)
		}
		if violationsPath != "" {
			logger.Infof("  Path pattern: %s", violationsPath)
		}
	}

	return nil
}

func filterViolations(violations []models.Violation, since string, pathPattern string) []models.Violation {
	var filtered []models.Violation

	// Parse since duration if provided
	var sinceTime time.Time
	if since != "" {
		duration, err := parseDuration(since)
		if err == nil {
			sinceTime = time.Now().Add(-duration)
		}
	}

	for _, v := range violations {
		// Filter by time if since is provided
		if !sinceTime.IsZero() && v.CreatedAt.Before(sinceTime) {
			continue
		}

		// Filter by path pattern if provided
		if pathPattern != "" {
			matched := false

			// Use doublestar for proper glob matching with ** support
			if match, err := doublestar.Match(pathPattern, v.File); err == nil && match {
				matched = true
			}

			// Try matching against basename if full path didn't match
			if !matched {
				if match, err := doublestar.Match(pathPattern, filepath.Base(v.File)); err == nil && match {
					matched = true
				}
			}

			// For relative patterns, try matching against relative path from working directory
			if !matched && !filepath.IsAbs(pathPattern) {
				if cwd, err := GetWorkingDir(); err == nil {
					if relPath, err := filepath.Rel(cwd, v.File); err == nil {
						if match, err := doublestar.Match(pathPattern, relPath); err == nil && match {
							matched = true
						}
					}
				}
			}

			if !matched {
				continue
			}
		}

		filtered = append(filtered, v)
	}

	return filtered
}

func displayViolationsList(violations []models.Violation) {
	// Build violations tree
	tree := models.BuildViolationTree(violations)

	// Format using clicky with tree format
	output, err := clicky.Format(tree, clicky.FormatOptions{Format: "tree"})
	if err != nil {
		logger.Errorf("Failed to format violations tree: %v", err)
		// Fallback to simple display
		fmt.Printf("Found %d violations\n", len(violations))
		for _, v := range violations {
			fmt.Printf("- %s\n", v.String())
		}
		return
	}

	fmt.Println(output)

	// Calculate summary info
	fileMap := make(map[string]bool)
	for _, v := range violations {
		fileMap[v.File] = true
	}
	
	// Print summary
	fmt.Printf("\n%s Found %d total violation(s) in %d file(s)\n",
		color.YellowString("⚠"),
		len(violations),
		len(fileMap))

	if violationsSince != "" {
		fmt.Printf("  Since: %s ago\n", violationsSince)
	}
	if violationsPath != "" {
		fmt.Printf("  Path pattern: %s\n", violationsPath)
	}
}

func parseDuration(s string) (time.Duration, error) {
	// Handle special cases like "7d" for days
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, err
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	// Handle weeks
	if strings.HasSuffix(s, "w") {
		weeks := strings.TrimSuffix(s, "w")
		var w int
		if _, err := fmt.Sscanf(weeks, "%d", &w); err != nil {
			return 0, err
		}
		return time.Duration(w) * 7 * 24 * time.Hour, nil
	}

	// Standard Go duration parsing
	return time.ParseDuration(s)
}

