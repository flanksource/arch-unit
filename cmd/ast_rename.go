package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	"github.com/spf13/cobra"
)

var (
	renameDryRun   bool
	renameNoColor  bool
	renameShowDiff bool
	renameBackup   bool
)

// RenameResult represents the result of a rename operation
type RenameResult struct {
	Status            string `json:"status" pretty:"label=Status,style=text-green-600 font-bold"`
	FilesModified     int    `json:"files_modified" pretty:"label=Files Modified,color=blue"`
	ReferencesUpdated int    `json:"references_updated" pretty:"label=References Updated,color=cyan"`
	BackupCreated     string `json:"backup_created,omitempty" pretty:"label=Backup Files,style=text-gray-600"`
}

var astRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename AST nodes and update all their references",
	Long: `Rename AST nodes (functions, types, fields, variables) and automatically update
all references to them throughout the codebase.

This command:
- Finds the specified node by name/pattern
- Identifies all references using AST relationships
- Updates the node definition and all its callers
- Supports dry-run mode to preview changes

NAME FORMATS:
  - Full qualified names: "package:Type.method"
  - Pattern matching: "UserService:Get*" (first match)
  - Simple names: "GetUser" (searches all packages)

EXAMPLES:
  # Rename a specific method
  arch-unit ast rename "UserService:GetUser" "UserService:FetchUser"

  # Rename with dry-run to see changes
  arch-unit ast rename "UserController:HandleGet" "UserController:HandleRequest" --dry-run

  # Rename a type and show diff
  arch-unit ast rename "models:User" "models:UserModel" --show-diff

  # Rename with backup
  arch-unit ast rename "ProcessData" "ProcessUserData" --backup

OPTIONS:
  --dry-run:    Preview changes without modifying files
  --show-diff:  Show detailed diff of changes
  --backup:     Create backup files before modification
  --no-color:   Disable colored output`,
	Args: cobra.ExactArgs(2),
	RunE: runASTRename,
}

func init() {
	astCmd.AddCommand(astRenameCmd)

	astRenameCmd.Flags().BoolVar(&renameDryRun, "dry-run", false, "Preview changes without modifying files")
	astRenameCmd.Flags().BoolVar(&renameNoColor, "no-color", false, "Disable colored output")
	astRenameCmd.Flags().BoolVar(&renameShowDiff, "show-diff", false, "Show detailed diff of changes")
	astRenameCmd.Flags().BoolVar(&renameBackup, "backup", false, "Create backup files (.bak) before modification")
}

func runASTRename(cmd *cobra.Command, args []string) error {
	oldName := args[0]
	newName := args[1]

	if oldName == newName {
		return fmt.Errorf("old name and new name are identical")
	}

	// Initialize AST cache
	astCache := cache.MustGetASTCache()

	workingDir, err := GetWorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create analyzer
	analyzer := ast.NewAnalyzer(astCache, workingDir)

	// Analyze files if needed
	logger.Infof("Analyzing source files...")
	if err := analyzer.AnalyzeFiles(); err != nil {
		return fmt.Errorf("failed to analyze files: %w", err)
	}

	// Create renamer
	renamer := ast.NewRenamer(workingDir, renameNoColor)

	// Find the node to rename
	logger.Debugf("Finding node to rename: %s", oldName)
	targetNode, err := renamer.FindNodeToRename(analyzer, oldName)
	if err != nil {
		return fmt.Errorf("failed to find node to rename: %w", err)
	}

	if targetNode == nil {
		return fmt.Errorf("node not found: %s", oldName)
	}

	logger.Infof("Found node to rename: %s (%s)", targetNode.GetFullName(), targetNode.NodeType)

	// Find all references to this node
	logger.Debugf("Finding references...")
	references, err := renamer.FindAllReferences(analyzer, targetNode)
	if err != nil {
		return fmt.Errorf("failed to find references: %w", err)
	}

	logger.Infof("Found %d references to rename", len(references))

	// Plan the rename operation
	renameOp, err := renamer.PlanRename(targetNode, newName, references)
	if err != nil {
		return fmt.Errorf("failed to plan rename operation: %w", err)
	}

	// Show preview if dry-run or show-diff
	if renameDryRun || renameShowDiff {
		preview, err := renamer.GeneratePreview(renameOp, renameShowDiff)
		if err != nil {
			return fmt.Errorf("failed to generate preview: %w", err)
		}
		fmt.Println(preview)
	}

	// Exit if dry-run
	if renameDryRun {
		logger.Infof("Dry-run complete. Use without --dry-run to apply changes.")
		return nil
	}

	// Execute the rename
	logger.Infof("Executing rename operation...")
	result, err := renamer.ExecuteRename(renameOp, renameBackup)
	if err != nil {
		return fmt.Errorf("failed to execute rename: %w", err)
	}

	// Show results using clicky formatting
	renameOutput := RenameResult{
		Status:            "✅ Rename completed successfully!",
		FilesModified:     result.FilesModified,
		ReferencesUpdated: result.ReferencesUpdated,
	}
	if renameBackup && result.FilesModified > 0 {
		renameOutput.BackupCreated = "Created with .bak extension"
	}

	output, err := clicky.Format(renameOutput)
	if err != nil {
		// Fallback to simple output
		fmt.Printf("✅ Rename completed successfully!\n")
		fmt.Printf("   Modified %d files\n", result.FilesModified)
		fmt.Printf("   Updated %d references\n", result.ReferencesUpdated)
		if renameBackup && result.FilesModified > 0 {
			fmt.Printf("   Backup files created with .bak extension\n")
		}
	} else {
		fmt.Print(output)
	}

	return nil
}
