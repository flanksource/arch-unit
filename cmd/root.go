package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile     string
	outputFile  string
	compact     bool
	workingDir  string
	showVersion bool
)

// VersionInfo represents version information with pretty formatting
type VersionInfo struct {
	Program string `json:"program" pretty:"label=Program,style=text-blue-600 font-bold"`
	Version string `json:"version" pretty:"label=Version,color=green"`
	Commit  string `json:"commit" pretty:"label=Commit,style=text-gray-600"`
	Built   string `json:"built" pretty:"label=Built,style=text-gray-600"`
	Status  string `json:"status" pretty:"label=Status,color=green=clean,yellow=dirty"`
}

var rootCmd = &cobra.Command{
	Use:   "arch-unit",
	Short: "Architecture linter for Go and Python projects",
	Long: `arch-unit is a tool that enforces architectural constraints by analyzing
code dependencies and method calls based on rules defined in .ARCHUNIT files.

It supports both Go and Python codebases and uses AST parsing to identify violations.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Run migrations before any command execution
		if err := runMigrations(); err != nil {
			logger.Errorf("Failed to run migrations: %v", err)
			os.Exit(1)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			vInfo := VersionInfo{
				Program: "arch-unit",
			}

			if getVersionInfo != nil {
				version, commit, date, isDirty := getVersionInfo()
				status := "clean"
				if isDirty {
					status = "dirty"
				}
				vInfo.Version = version
				vInfo.Commit = commit
				vInfo.Built = date
				vInfo.Status = status
			} else {
				vInfo.Version = "dev"
				vInfo.Commit = "unknown"
				vInfo.Built = "unknown"
				vInfo.Status = "unknown"
			}

			output, err := clicky.Format(vInfo)
			if err != nil {
				fmt.Println(err.Error())
				// Fallback to simple output
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
			} else {
				fmt.Print(output)
			}
			return
		}
		_ = cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Wait for any background clicky tasks to complete
	// This will display the task tree if there are any tasks
	exitCode := clicky.WaitForGlobalCompletion()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

// runMigrations executes all pending cache migrations
func runMigrations() error {
	migrationManager, err := cache.NewMigrationManager()
	if err != nil {
		return fmt.Errorf("failed to create migration manager: %w", err)
	}
	defer func() { _ = migrationManager.Close() }()

	return migrationManager.RunMigrations()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.arch-unit.yaml)")
	rootCmd.PersistentFlags().StringVar(&workingDir, "cwd", "", "Working directory for analysis (default: current directory)")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "V", false, "Show version information")

	clicky.BindAllFlags(rootCmd.PersistentFlags())
	// Output file flag
	rootCmd.PersistentFlags().StringVarP(&outputFile, "output", "o", "", "Output file (optional, uses stdout if not specified)")
	rootCmd.PersistentFlags().BoolVarP(&compact, "compact", "c", false, "Compact output showing summary only")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".arch-unit")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		logger.Infof("Using config file: %s", viper.ConfigFileUsed())
	}

	// Apply all clicky flags (formatters, task manager, logging)
	clicky.Flags.UseFlags()
}

// GetWorkingDir returns the working directory to use for analysis
// It respects the --cwd flag if provided, otherwise uses the current directory
func GetWorkingDir() (string, error) {
	if workingDir != "" {
		// Expand ~ to home directory if present
		if workingDir == "~" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home directory: %w", err)
			}
			return home, nil
		}

		// Make absolute if relative
		absPath, err := filepath.Abs(workingDir)
		if err != nil {
			return "", fmt.Errorf("failed to resolve working directory: %w", err)
		}

		// Verify the directory exists
		info, err := os.Stat(absPath)
		if err != nil {
			return "", fmt.Errorf("working directory does not exist: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("working directory is not a directory: %s", absPath)
		}

		return absPath, nil
	}

	// Default to current working directory
	return os.Getwd()
}
