package cmd

import (
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/spf13/cobra"
)

var exclusionsCmd = &cobra.Command{
	Use:   "exclusions",
	Short: "Generate exclusion patterns for use in scripts",
	Long: `Generate exclusion patterns for use in test scripts, build tools, etc.

The exclusions command outputs the centralized exclusion patterns that arch-unit
uses internally. This ensures consistency between linter exclusions and test 
exclusions used in Taskfile.yml or other build scripts.

Examples:
  # Generate grep patterns for go list exclusion
  arch-unit exclusions --format=grep

  # Generate patterns for use in shell scripts  
  arch-unit exclusions --format=list

  # Generate specific pattern types
  arch-unit exclusions --type=test    # Include test-specific exclusions
  arch-unit exclusions --type=builtin # Only built-in patterns`,
	RunE: runExclusions,
}

var (
	exclusionFormat string
	exclusionType   string
)

func init() {
	rootCmd.AddCommand(exclusionsCmd)
	exclusionsCmd.Flags().StringVar(&exclusionFormat, "format", "grep", "Output format: grep, list, json")
	exclusionsCmd.Flags().StringVar(&exclusionType, "type", "builtin", "Exclusion type: builtin, test")
}

func runExclusions(cmd *cobra.Command, args []string) error {
	var patterns []string
	
	switch exclusionType {
	case "builtin":
		patterns = models.GetBuiltinExcludePatterns()
	case "test":
		patterns = models.GetTestExcludePatterns()
	default:
		return fmt.Errorf("unknown exclusion type: %s (valid: builtin, test)", exclusionType)
	}
	
	switch exclusionFormat {
	case "grep":
		// Format for use with grep -v in go list commands
		grepPatterns := make([]string, len(patterns))
		for i, pattern := range patterns {
			// Convert glob pattern to regex-like pattern for grep
			grepPattern := strings.ReplaceAll(pattern, "**", ".*")
			grepPattern = strings.ReplaceAll(grepPattern, "*", "[^/]*")
			grepPatterns[i] = fmt.Sprintf("'/%s/'", grepPattern)
		}
		fmt.Print(strings.Join(grepPatterns, " | grep -v "))
		
	case "list":
		// Simple list format
		for _, pattern := range patterns {
			fmt.Println(pattern)
		}
		
	case "json":
		// JSON format for programmatic use
		fmt.Print("[")
		for i, pattern := range patterns {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf("\"%s\"", pattern)
		}
		fmt.Print("]")
		
	default:
		return fmt.Errorf("unknown format: %s (valid: grep, list, json)", exclusionFormat)
	}
	
	return nil
}