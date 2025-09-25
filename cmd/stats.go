package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats [directory]",
	Short: "Display linter execution statistics and intelligent debounce recommendations",
	Long: `Show performance metrics and execution statistics for all configured linters.
	
This command displays:
- Execution frequency and timing
- Success rates and violation patterns  
- Intelligent debounce recommendations
- Performance trends over time

Examples:
  # Show stats for current directory
  arch-unit stats

  # Show stats for specific directory
  arch-unit stats ./src

  # Show detailed stats with recommendations
  arch-unit stats --verbose`,
	RunE: runStats,
}

var (
	statsVerbose bool
)

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().BoolVar(&statsVerbose, "verbose", false, "Show detailed statistics")
}

func runStats(cmd *cobra.Command, args []string) error {
	workDir := "."
	if len(args) > 0 {
		workDir = args[0]
	}

	// Convert to absolute path for display
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	// Create linter stats instance
	linterStats, err := cache.NewLinterStats()
	if err != nil {
		return fmt.Errorf("failed to initialize linter statistics: %w", err)
	}
	defer func() { _ = linterStats.Close() }()

	// Get all linters that have been run
	linters, err := linterStats.GetLinterHistory(workDir)
	if err != nil {
		return fmt.Errorf("failed to get linter history: %w", err)
	}

	if len(linters) == 0 {
		fmt.Printf("No linter execution history found for %s\n", color.CyanString(absWorkDir))
		fmt.Println("Run some linters first with: arch-unit check")
		return nil
	}

	// Display header
	fmt.Printf("📊 Linter Statistics for %s\n", color.CyanString(absWorkDir))
	fmt.Println(strings.Repeat("=", 80))

	for i, linterName := range linters {
		if i > 0 {
			fmt.Println() // Add spacing between linters
		}

		if err := displayLinterStats(linterStats, linterName, workDir, statsVerbose); err != nil {
			fmt.Printf("⚠️  Failed to get stats for %s: %v\n", linterName, err)
			continue
		}
	}

	// Display summary
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("💡 %s Use %s to see detailed statistics\n",
		color.YellowString("Tip:"),
		color.GreenString("arch-unit stats --verbose"))

	return nil
}

func displayLinterStats(linterStats *cache.LinterStats, linterName, workDir string, verbose bool) error {
	stats, err := linterStats.GetStats(linterName, workDir)
	if err != nil {
		return err
	}

	// Get intelligent debounce recommendation
	recommendedDebounce, err := linterStats.GetIntelligentDebounce(linterName, workDir)
	if err != nil {
		recommendedDebounce = 30 * time.Second // fallback
	}

	// Display main stats
	fmt.Printf("🔧 %s\n", color.New(color.FgCyan, color.Bold).Sprint(linterName))

	if stats.RunCount == 0 {
		fmt.Println("   No execution history")
		return nil
	}

	// Basic stats (always shown)
	fmt.Printf("   Last Run: %s (%s ago)\n",
		stats.LastRun.Format("2006-01-02 15:04:05"),
		formatDuration(time.Since(stats.LastRun)))
	fmt.Printf("   Total Runs: %s\n", color.GreenString("%d", stats.RunCount))
	fmt.Printf("   Success Rate: %s\n", formatSuccessRate(stats.SuccessRate))

	// Performance metrics
	if stats.LastDuration > 0 {
		fmt.Printf("   Last Duration: %s\n", formatDuration(stats.LastDuration))
	}
	if stats.AvgDuration > 0 {
		fmt.Printf("   Average Duration: %s\n", formatDuration(stats.AvgDuration))
	}

	// Violation statistics
	if stats.RunCount > 0 {
		avgViolations := float64(stats.ViolationCount) / float64(stats.RunCount)
		fmt.Printf("   Average Violations: %s\n", formatViolationCount(avgViolations))
	}

	// Intelligent debounce
	fmt.Printf("   Recommended Debounce: %s\n",
		color.New(color.FgMagenta, color.Bold).Sprint(formatDuration(recommendedDebounce)))

	// Verbose information
	if verbose {
		fmt.Printf("   Total Violations Found: %s\n", color.RedString("%d", stats.ViolationCount))

		// Performance assessment
		performanceCategory := assessPerformance(stats.AvgDuration)
		fmt.Printf("   Performance Category: %s\n", performanceCategory)

		// Debounce efficiency
		efficiency := assessDebounceEfficiency(stats.AvgDuration, recommendedDebounce)
		fmt.Printf("   Debounce Efficiency: %s\n", efficiency)
	}

	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return color.GreenString("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return color.YellowString("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return color.RedString("%.1fm", d.Minutes())
	} else {
		return color.New(color.FgRed, color.Bold).Sprintf("%.1fh", d.Hours())
	}
}

func formatSuccessRate(rate float64) string {
	percentage := rate * 100
	if percentage >= 95 {
		return color.GreenString("%.1f%%", percentage)
	} else if percentage >= 80 {
		return color.YellowString("%.1f%%", percentage)
	} else {
		return color.RedString("%.1f%%", percentage)
	}
}

func formatViolationCount(count float64) string {
	if count == 0 {
		return color.GreenString("0")
	} else if count < 5 {
		return color.YellowString("%.1f", count)
	} else {
		return color.RedString("%.1f", count)
	}
}

func assessPerformance(avgDuration time.Duration) string {
	if avgDuration < time.Second {
		return color.GreenString("Fast")
	} else if avgDuration < 5*time.Second {
		return color.YellowString("Medium")
	} else if avgDuration < 15*time.Second {
		return color.New(color.FgRed).Sprint("Slow")
	} else {
		return color.New(color.FgRed, color.Bold).Sprint("Very Slow")
	}
}

func assessDebounceEfficiency(avgDuration, debounce time.Duration) string {
	if avgDuration == 0 {
		return color.New(color.FgHiBlack).Sprint("Unknown")
	}

	ratio := float64(debounce) / float64(avgDuration)

	if ratio >= 4 {
		return color.GreenString("Excellent (%.1fx execution time)", ratio)
	} else if ratio >= 2 {
		return color.YellowString("Good (%.1fx execution time)", ratio)
	} else if ratio >= 1 {
		return color.New(color.FgRed).Sprintf("Poor (%.1fx execution time)", ratio)
	} else {
		return color.New(color.FgRed, color.Bold).Sprintf("Very Poor (%.1fx execution time)", ratio)
	}
}
