package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fatih/color"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
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
	defer violationCache.Close()

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
	defer violationCache.Close()

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
	// Group violations by file
	fileMap := make(map[string][]models.Violation)
	for _, v := range violations {
		fileMap[v.File] = append(fileMap[v.File], v)
	}

	// Sort files for consistent output
	var files []string
	for file := range fileMap {
		files = append(files, file)
	}
	sort.Strings(files)

	fmt.Printf("📋 %s\n", color.New(color.Bold).Sprint("Cached Violations"))
	fmt.Println(strings.Repeat("─", 80))

	totalCount := 0
	for i, file := range files {
		violations := fileMap[file]
		totalCount += len(violations)

		// Get relative path for display
		relPath := file
		if cwd, err := GetWorkingDir(); err == nil {
			if rel, err := filepath.Rel(cwd, file); err == nil && !strings.HasPrefix(rel, "../") {
				relPath = rel
			}
		}

		// File header with violation count
		isLast := i == len(files)-1
		if isLast {
			fmt.Printf("└── %s (%d violations)\n",
				color.New(color.FgCyan, color.Bold).Sprint(relPath),
				len(violations))
		} else {
			fmt.Printf("├── %s (%d violations)\n",
				color.New(color.FgCyan, color.Bold).Sprint(relPath),
				len(violations))
		}

		// Group violations by source
		sourceMap := make(map[string][]models.Violation)
		for _, v := range violations {
			source := v.Source
			if source == "" {
				source = "arch-unit"
			}
			sourceMap[source] = append(sourceMap[source], v)
		}

		// Sort sources for consistent output
		var sources []string
		for source := range sourceMap {
			sources = append(sources, source)
		}
		sort.Strings(sources)

		prefix := "│   "
		if isLast {
			prefix = "    "
		}

		for j, source := range sources {
			sourceViolations := sourceMap[source]
			isLastSource := j == len(sources)-1

			// Source header
			sourceColor := color.FgYellow
			if source == "arch-unit" {
				sourceColor = color.FgMagenta
			}

			if isLastSource {
				fmt.Printf("%s└── %s\n", prefix, color.New(sourceColor).Sprint(source))
			} else {
				fmt.Printf("%s├── %s\n", prefix, color.New(sourceColor).Sprint(source))
			}

			sourcePrefix := prefix + "│   "
			if isLastSource {
				sourcePrefix = prefix + "    "
			}

			// Display violations for this source
			for k, v := range sourceViolations {
				isLastViolation := k == len(sourceViolations)-1

				// Format violation message
				var violationMsg string
				if v.Message != "" {
					violationMsg = v.Message
				} else if v.Rule != nil {
					violationMsg = v.Rule.String()
				} else {
					violationMsg = fmt.Sprintf("%s.%s", v.CalledPackage, v.CalledMethod)
				}

				// Truncate long messages
				if len(violationMsg) > 60 {
					violationMsg = violationMsg[:57] + "..."
				}

				lineInfo := fmt.Sprintf("line %d", v.Line)
				if v.Column > 0 {
					lineInfo = fmt.Sprintf("line %d:%d", v.Line, v.Column)
				}

				// Add timestamp if available
				timeInfo := ""
				if !v.CreatedAt.IsZero() {
					timeInfo = fmt.Sprintf(" [%s]", formatTimeAgo(v.CreatedAt))
				}

				if isLastViolation {
					fmt.Printf("%s└── %s %s%s\n",
						sourcePrefix,
						color.RedString(violationMsg),
						color.New(color.FgHiBlack).Sprint("("+lineInfo+")"),
						color.New(color.FgHiBlack).Sprint(timeInfo))
				} else {
					fmt.Printf("%s├── %s %s%s\n",
						sourcePrefix,
						color.RedString(violationMsg),
						color.New(color.FgHiBlack).Sprint("("+lineInfo+")"),
						color.New(color.FgHiBlack).Sprint(timeInfo))
				}
			}
		}

		if !isLast {
			fmt.Println("│")
		}
	}

	fmt.Println(strings.Repeat("─", 80))

	// Print summary
	fmt.Printf("\n%s Found %d total violation(s) in %d file(s)\n",
		color.YellowString("⚠"),
		totalCount,
		len(files))

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

func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return fmt.Sprintf("%ds ago", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	} else {
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
