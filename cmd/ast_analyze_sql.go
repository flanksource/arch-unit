package cmd

import (
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/sql"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/spf13/cobra"
)

var (
	sqlConnectionString string
	sqlDialect          string
	sqlOutput           string
	sqlShowTree         bool
	sqlAnalysisProgress bool
)

var astAnalyzeSQLCmd = &cobra.Command{
	Use:   "sql",
	Short: "Analyze SQL database schema using dbtpl",
	Long: `Analyze SQL database schema and extract comprehensive AST information using dbtpl.

This command connects to a SQL database and introspects the schema to extract
tables, views, columns, indexes, foreign keys, enums, stored procedures, and functions as AST nodes.

Supported databases: PostgreSQL, MySQL, SQLite, Oracle, SQL Server

Examples:
  # Analyze PostgreSQL database
  arch-unit ast analyze sql --connection "postgres://user:pass@localhost/mydb"

  # Analyze MySQL database
  arch-unit ast analyze sql --connection "mysql://user:pass@localhost/mydb"

  # Analyze SQLite database
  arch-unit ast analyze sql --connection "sqlite3://./mydb.sqlite"

  # Analyze Oracle database
  arch-unit ast analyze sql --connection "oracle://user:pass@localhost:1521/mydb"

  # Save output to file
  arch-unit ast analyze sql --connection "postgres://localhost/mydb" --output schema.json`,
	RunE: runASTAnalyzeSQL,
}

func init() {
	astAnalyzeCmd.AddCommand(astAnalyzeSQLCmd)
	astAnalyzeSQLCmd.Flags().StringVar(&sqlConnectionString, "connection", "", "Database connection string (required)")
	astAnalyzeSQLCmd.Flags().StringVar(&sqlDialect, "dialect", "", "SQL dialect (optional, auto-detected from connection)")
	astAnalyzeSQLCmd.Flags().StringVar(&sqlOutput, "output", "", "Output file path (optional)")
	astAnalyzeSQLCmd.Flags().BoolVar(&sqlShowTree, "show-tree", false, "Display database schema in tree format")
	astAnalyzeSQLCmd.Flags().BoolVar(&sqlAnalysisProgress, "analysis-progress", true, "Show analysis progress (default: true)")

	// Mark connection as required
	_ = astAnalyzeSQLCmd.MarkFlagRequired("connection")
}

func runASTAnalyzeSQL(cmd *cobra.Command, args []string) error {
	if sqlConnectionString == "" {
		return fmt.Errorf("database connection string is required")
	}

	// Create root task that wraps all SQL analysis logic
	clicky.StartTask("SQL Schema Analysis", func(ctx flanksourceContext.Context, t *clicky.Task) (interface{}, error) {
		// Initialize AST cache
		astCache := cache.MustGetASTCache()

		// Create SQL AST extractor
		extractor := sql.NewSQLASTExtractor()

		t.Infof("Connecting to database: %s", maskConnectionString(sqlConnectionString))

		// Extract AST from database connection
		result, err := extractor.ExtractFromConnection(sqlConnectionString)
		if err != nil {
			t.Errorf("Failed to extract schema: %v", err)
			return nil, err
		}

		if result == nil {
			t.Warnf("No schema data extracted")
			return nil, nil
		}

		t.Infof("Extracted %d nodes from database schema", len(result.Nodes))

		// Store results in cache if not using no-cache flag
		if !astNoCache {
			t.Infof("Storing schema data in cache")
			virtualPathMgr := analysis.NewVirtualPathManager()
			virtualPath := virtualPathMgr.CreateVirtualPath(analysis.AnalysisSource{
				Type:             "sql_connection",
				ConnectionString: sqlConnectionString,
			})

			// Store nodes in cache
			nodeMap := make(map[string]int64)
			for _, node := range result.Nodes {
				if node == nil {
					continue
				}

				nodeID, err := astCache.StoreASTNode(node)
				if err != nil {
					t.Warnf("Failed to store AST node: %v", err)
					continue
				}
				nodeMap[node.Key()] = nodeID
			}

			// Update file metadata for virtual path
			if err := astCache.UpdateFileMetadata(virtualPath); err != nil {
				t.Warnf("Failed to update cache metadata: %v", err)
			}

			t.Infof("Stored %d nodes in cache", len(nodeMap))
		}

		// Output results if requested
		if sqlOutput != "" {
			t.Infof("Writing results to %s", sqlOutput)
			if err := writeASTResultToFile(result, sqlOutput); err != nil {
				t.Errorf("Failed to write output: %v", err)
				return nil, err
			}
			t.Infof("Results written to %s", sqlOutput)
		}

		// Summary
		tableCount := 0
		viewCount := 0
		procCount := 0
		columnCount := 0
		indexCount := 0
		foreignKeyCount := 0
		enumCount := 0

		for _, node := range result.Nodes {
			switch node.NodeType {
			case models.NodeTypeTypeTable:
				tableCount++
			case models.NodeTypeTypeView:
				viewCount++
			case models.NodeTypeType:
				if node.Summary != nil && *node.Summary == "Database enum" {
					enumCount++
				}
			case models.NodeTypeMethodStoredProc, models.NodeTypeMethodFunction:
				procCount++
			case models.NodeTypeMethod:
				if node.Summary != nil && *node.Summary == "Database index" {
					indexCount++
				} else if node.Summary != nil && strings.Contains(*node.Summary, "Foreign key") {
					foreignKeyCount++
				}
			case models.NodeTypeFieldColumn:
				columnCount++
			}
		}

		t.Infof("Schema summary: %d tables, %d views, %d enums, %d procedures/functions, %d columns, %d indexes, %d foreign keys",
			tableCount, viewCount, enumCount, procCount, columnCount, indexCount, foreignKeyCount)

		// Display tree output if requested
		if sqlShowTree {
			t.Infof("Displaying database schema tree")
			if err := displaySQLSchemaTree(result.Nodes, sqlConnectionString, result.FilePath); err != nil {
				t.Warnf("Failed to display schema tree: %v", err)
			}
		}

		return result, nil
	})

	// Wait for all clicky tasks to complete
	var exitCode int
	if sqlAnalysisProgress {
		exitCode = clicky.WaitForGlobalCompletion()
	} else {
		exitCode = clicky.WaitForGlobalCompletionSilent()
	}
	if exitCode != 0 {
		return fmt.Errorf("SQL analysis failed with exit code %d", exitCode)
	}

	return nil
}

// maskConnectionString masks sensitive information in connection strings for logging
func maskConnectionString(connStr string) string {
	// Simple masking - in production you'd want more sophisticated masking
	if len(connStr) > 20 {
		return connStr[:20] + "..."
	}
	return connStr
}

// displaySQLSchemaTree displays the database schema in tree format
func displaySQLSchemaTree(nodes []*models.ASTNode, connectionString, virtualPath string) error {
	if len(nodes) == 0 {
		fmt.Println("No schema nodes to display")
		return nil
	}

	// Get working directory for relative paths
	workingDir, err := GetWorkingDir()
	if err != nil {
		// Fallback to current directory if we can't get working directory
		workingDir = "."
	}

	// Use global persistent flags for display configuration
	config := GetDisplayConfigFromFlags()

	// Build enhanced hierarchical AST tree structure for SQL schema display
	tree := models.BuildHierarchicalASTTree(nodes, config, workingDir)

	// Use clicky.Format to handle coloring properly
	output, err := clicky.Format(tree, clicky.FormatOptions{
		Format:  "tree",
		NoColor: clicky.Flags.FormatOptions.NoColor,
	})
	if err != nil {
		return fmt.Errorf("failed to format schema tree: %w", err)
	}

	fmt.Printf("\nDatabase Schema Tree (%s):\n", maskConnectionString(connectionString))
	fmt.Printf("Found %d schema nodes:\n\n", len(nodes))
	fmt.Print(output)

	return nil
}
