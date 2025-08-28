package main

import (
	"fmt"
	"log"
	"os"

	"github.com/flanksource/arch-unit/cmd"
	"github.com/google/gops/agent"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	dirty   = "unknown"
)

func main() {
	// Start gops agent for runtime debugging
	if err := agent.Listen(agent.Options{
		ShutdownCleanup: true, // Automatically cleanup on shutdown
	}); err != nil {
		log.Printf("Failed to start gops agent: %v", err)
	}
	defer agent.Close()

	// Set version info function for the cmd package
	cmd.SetVersionInfo(GetVersionInfo)

	if len(os.Args) > 1 && os.Args[1] == "-version" {
		printVersion()
		os.Exit(0)
	}
	cmd.Execute()
}

// printVersion prints the version information
func printVersion() {
	status := "clean"
	if dirty == "true" {
		status = "dirty"
		version += "-dirty"
	}
	fmt.Printf("arch-unit version %s (commit: %s, built: %s, %s)\n", version, commit, date, status)
}

// GetVersionInfo returns version information for use by cmd package
func GetVersionInfo() (string, string, string, bool) {
	isDirty := dirty == "true"
	versionStr := version
	if isDirty {
		versionStr += "-dirty"
	}
	return versionStr, commit, date, isDirty
}
