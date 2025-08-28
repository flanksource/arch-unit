package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// GetVersionInfo is implemented in main package
// We'll define a function type to access version info from main
var getVersionInfo func() (version, commit, date string, dirty bool)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Long: `Print the version information including:
- Version number (from git tags)
- Git commit hash
- Build date and time
- Repository status (clean/dirty)`,
	Run: func(cmd *cobra.Command, args []string) {
		if getVersionInfo != nil {
			version, commit, date, isDirty := getVersionInfo()
			status := "clean"
			if isDirty {
				status = "dirty"
			}
			fmt.Printf("arch-unit version %s (commit: %s, built: %s, %s)\n", version, commit, date, status)
		} else {
			fmt.Println("arch-unit version dev (commit: unknown, built: unknown, unknown)")
		}
	},
}

// SetVersionInfo sets the version information function
func SetVersionInfo(fn func() (string, string, string, bool)) {
	getVersionInfo = fn
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
