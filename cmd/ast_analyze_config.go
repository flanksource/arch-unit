package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flanksource/arch-unit/analysis/config"
	"github.com/flanksource/arch-unit/analysis/openapi"
	"github.com/flanksource/arch-unit/analysis/sql"
	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/clicky"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/spf13/cobra"
)

var (
	configFile string
)

var astAnalyzeConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Analyze using configuration file",
	Long: `Analyze multiple sources using a configuration file.

This command uses a .arch-ast.yaml configuration file to analyze multiple
sources including SQL databases, OpenAPI specifications, and files.

Examples:
  # Use default config file (.arch-ast.yaml)
  arch-unit ast analyze config

  # Use specific config file
  arch-unit ast analyze config --config my-config.yaml

Example configuration file (.arch-ast.yaml):
  version: "1.0"
  analyzers:
    - path: "**/*.sql"
      analyzer: "sql"
      options:
        dialect: "postgresql"

    - path: "api/openapi.yaml"
      analyzer: "openapi"
      options:
        version: "3.0"

    - path: "**/*.go"
      analyzer: "go"`,
	RunE: runASTAnalyzeConfig,
}

func init() {
	astAnalyzeCmd.AddCommand(astAnalyzeConfigCmd)
	astAnalyzeConfigCmd.Flags().StringVar(&configFile, "config", "", "Configuration file path (default: .arch-ast.yaml)")
}

func runASTAnalyzeConfig(cmd *cobra.Command, args []string) error {
	// Determine working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create root task that wraps all configuration-based analysis
	clicky.StartTask("Configuration-based Analysis", func(ctx flanksourceContext.Context, t *clicky.Task) (interface{}, error) {
		// Load configuration
		loader := config.NewConfigLoader(workDir)
		cfg, err := loader.LoadConfig(configFile)
		if err != nil {
			t.Errorf("Failed to load configuration: %v", err)
			return nil, err
		}

		t.Infof("Loaded configuration with %d analyzers", len(cfg.Analyzers))

		// Initialize AST cache
		astCache := cache.MustGetASTCache()

		// Process each analyzer configuration
		var allResults []*types.ASTResult
		for i, analyzerConfig := range cfg.Analyzers {
			analyzerTask := fmt.Sprintf("Analyzer %d (%s)", i+1, analyzerConfig.Analyzer)

			subTask := clicky.StartTask(analyzerTask, func(ctx flanksourceContext.Context, st *clicky.Task) (*types.ASTResult, error) {
				st.Infof("Processing %s analyzer for path: %s", analyzerConfig.Analyzer, analyzerConfig.Path)

				switch analyzerConfig.Analyzer {
				case "sql":
					return runSQLAnalyzer(st, analyzerConfig, astCache)
				case "openapi":
					return runOpenAPIAnalyzer(st, analyzerConfig, astCache)
				case "custom":
					st.Warnf("Custom analyzers not yet implemented")
					return nil, nil
				default:
					st.Warnf("Unknown analyzer type: %s", analyzerConfig.Analyzer)
					return nil, nil
				}
			})

			result, err := subTask.GetResult()
			if err != nil {
				t.Warnf("Analyzer %d failed: %v", i+1, err)
				continue
			}

			if result != nil {
				allResults = append(allResults, result)
			}
		}

		// Summary
		totalNodes := 0
		for _, result := range allResults {
			totalNodes += len(result.Nodes)
		}

		t.Infof("Configuration analysis completed: %d analyzers processed, %d total nodes extracted",
			len(cfg.Analyzers), totalNodes)

		return allResults, nil
	})

	// Wait for all clicky tasks to complete
	exitCode := clicky.WaitForGlobalCompletionSilent()
	if exitCode != 0 {
		return fmt.Errorf("configuration analysis failed with exit code %d", exitCode)
	}

	return nil
}

// runSQLAnalyzer processes a SQL analyzer configuration
func runSQLAnalyzer(task *clicky.Task, analyzerConfig config.AnalyzerConfig, astCache *cache.ASTCache) (*types.ASTResult, error) {
	sqlOpts := analyzerConfig.GetSQLOptions()

	if sqlOpts.ConnectionString == "" {
		return nil, fmt.Errorf("SQL analyzer requires connection string in options")
	}

	extractor := sql.NewSQLASTExtractor()

	task.Infof("Analyzing SQL database: %s", maskConnectionString(sqlOpts.ConnectionString))

	result, err := extractor.ExtractFromConnection(sqlOpts.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze SQL database: %w", err)
	}

	if result != nil {
		task.Infof("Extracted %d nodes from SQL database", len(result.Nodes))

		// Store in cache if not disabled
		if !astNoCache {
			storeResultInCache(task, result, astCache)
		}
	}

	return result, nil
}

// runOpenAPIAnalyzer processes an OpenAPI analyzer configuration
func runOpenAPIAnalyzer(task *clicky.Task, analyzerConfig config.AnalyzerConfig, astCache *cache.ASTCache) (*types.ASTResult, error) {
	openAPIopts := analyzerConfig.GetOpenAPIOptions()

	extractor := openapi.NewOpenAPIExtractor()

	var result *types.ASTResult
	var err error

	if openAPIopts.URL != "" {
		// Analyze from URL
		task.Infof("Analyzing OpenAPI spec from URL: %s", openAPIopts.URL)
		result, err = extractor.ExtractFromURL(openAPIopts.URL)
	} else {
		// Analyze from file path
		task.Infof("Analyzing OpenAPI spec from file: %s", analyzerConfig.Path)

		// Read file content
		content, readErr := os.ReadFile(analyzerConfig.Path)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read OpenAPI file: %w", readErr)
		}

		// Extract from file content
		result, err = extractor.ExtractFile(astCache, analyzerConfig.Path, content)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to analyze OpenAPI spec: %w", err)
	}

	if result != nil {
		task.Infof("Extracted %d nodes from OpenAPI specification", len(result.Nodes))

		// Store in cache if not disabled
		if !astNoCache {
			storeResultInCache(task, result, astCache)
		}
	}

	return result, nil
}

// storeResultInCache stores AST result nodes in the cache
func storeResultInCache(task *clicky.Task, result *types.ASTResult, astCache *cache.ASTCache) {
	nodeMap := make(map[string]int64)
	for _, node := range result.Nodes {
		if node == nil {
			continue
		}

		nodeID, err := astCache.StoreASTNode(node)
		if err != nil {
			task.Warnf("Failed to store AST node: %v", err)
			continue
		}
		nodeMap[node.Key()] = nodeID
	}

	// Update file metadata
	if err := astCache.UpdateFileMetadata(result.FilePath); err != nil {
		task.Warnf("Failed to update cache metadata: %v", err)
	}

	task.Debugf("Stored %d nodes in cache", len(nodeMap))
}

// writeASTResultToFile writes an AST result to a JSON file
func writeASTResultToFile(result *types.ASTResult, outputPath string) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Convert result to JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result to JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}
