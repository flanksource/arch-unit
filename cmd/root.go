package cmd

import (
	"fmt"
	"os"

	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile    string
	outputFile string
	compact    bool
	workingDir string
	showVersion bool
	
	// Global options for clicky integration
	taskManagerOpts *clicky.TaskManagerOptions
	formatOpts      *formatters.FormatOptions
)

var rootCmd = &cobra.Command{
	Use:   "arch-unit",
	Short: "Architecture linter for Go and Python projects",
	Long: `arch-unit is a tool that enforces architectural constraints by analyzing
code dependencies and method calls based on rules defined in .ARCHUNIT files.

It supports both Go and Python codebases and uses AST parsing to identify violations.`,
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
		cmd.Help()
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

func init() {
	cobra.OnInitialize(initConfig)
	
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.arch-unit.yaml)")
	
	// Output format flags
	rootCmd.PersistentFlags().StringVarP(&outputFile, "output", "o", "", "Output file (required for html, excel)")
	rootCmd.PersistentFlags().BoolVarP(&json, "json", "j", false, "Format output in json")
	rootCmd.PersistentFlags().BoolVar(&html, "html", false, "Format output in html")
	rootCmd.PersistentFlags().BoolVar(&csv, "csv", false, "Format output in csv")
	rootCmd.PersistentFlags().BoolVar(&excel, "excel", false, "Format output in excel")
	rootCmd.PersistentFlags().BoolVar(&markdown, "markdown", false, "Format output in markdown")
	rootCmd.PersistentFlags().BoolVarP(&compact, "compact", "c", false, "Compact output showing summary only")
	
	logger.BindFlags(rootCmd.PersistentFlags())
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
}