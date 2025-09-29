package cmd

import (
	"fmt"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/openapi"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/clicky"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/spf13/cobra"
)

var (
	openAPIURL     string
	openAPIVersion string
	openAPIOutput  string
)

var astAnalyzeOpenAPICmd = &cobra.Command{
	Use:   "openapi",
	Short: "Analyze OpenAPI specification",
	Long: `Analyze OpenAPI specification and extract AST information.

This command fetches an OpenAPI specification from a URL or reads from a file
and extracts endpoints, schemas, and parameters as AST nodes.

Examples:
  # Analyze OpenAPI spec from URL
  arch-unit ast analyze openapi --url "https://api.example.com/openapi.json"

  # Analyze OpenAPI spec with specific version
  arch-unit ast analyze openapi --url "https://api.example.com/openapi.yaml" --version "3.1"

  # Save output to file
  arch-unit ast analyze openapi --url "https://api.example.com/openapi.json" --output api_schema.json`,
	RunE: runASTAnalyzeOpenAPI,
}

func init() {
	astAnalyzeCmd.AddCommand(astAnalyzeOpenAPICmd)
	astAnalyzeOpenAPICmd.Flags().StringVar(&openAPIURL, "url", "", "OpenAPI specification URL (required)")
	astAnalyzeOpenAPICmd.Flags().StringVar(&openAPIVersion, "version", "3.0", "OpenAPI version (3.0 or 3.1)")
	astAnalyzeOpenAPICmd.Flags().StringVar(&openAPIOutput, "output", "", "Output file path (optional)")

	// Mark URL as required
	_ = astAnalyzeOpenAPICmd.MarkFlagRequired("url")
}

func runASTAnalyzeOpenAPI(cmd *cobra.Command, args []string) error {
	if openAPIURL == "" {
		return fmt.Errorf("OpenAPI specification URL is required")
	}

	// Validate version
	supportedVersions := []string{"3.0", "3.1"}
	validVersion := false
	for _, v := range supportedVersions {
		if openAPIVersion == v {
			validVersion = true
			break
		}
	}
	if !validVersion {
		return fmt.Errorf("unsupported OpenAPI version: %s (supported: %v)", openAPIVersion, supportedVersions)
	}

	// Create root task that wraps all OpenAPI analysis logic
	clicky.StartTask("OpenAPI Specification Analysis", func(ctx flanksourceContext.Context, t *clicky.Task) (interface{}, error) {
		// Initialize AST cache
		astCache := cache.MustGetASTCache()

		// Create OpenAPI AST extractor
		extractor := openapi.NewOpenAPIExtractor()

		t.Infof("Fetching OpenAPI spec from: %s", openAPIURL)

		// Extract AST from OpenAPI specification
		result, err := extractor.ExtractFromURL(openAPIURL)
		if err != nil {
			t.Errorf("Failed to extract OpenAPI spec: %v", err)
			return nil, err
		}

		if result == nil {
			t.Warnf("No API data extracted")
			return nil, nil
		}

		t.Infof("Extracted %d nodes from OpenAPI specification", len(result.Nodes))

		// Store results in cache if not using no-cache flag
		if !astNoCache {
			t.Infof("Storing API data in cache")
			virtualPathMgr := analysis.NewVirtualPathManager()
			virtualPath := virtualPathMgr.CreateVirtualPath(analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  openAPIURL,
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
		if openAPIOutput != "" {
			t.Infof("Writing results to %s", openAPIOutput)
			if err := writeASTResultToFile(result, openAPIOutput); err != nil {
				t.Errorf("Failed to write output: %v", err)
				return nil, err
			}
			t.Infof("Results written to %s", openAPIOutput)
		}

		// Summary
		endpointCount := 0
		schemaCount := 0
		parameterCount := 0

		for _, node := range result.Nodes {
			switch node.NodeType {
			case "method_http_get", "method_http_post", "method_http_put", "method_http_delete":
				endpointCount++
				parameterCount += len(node.Parameters)
			case "type_http_schema":
				schemaCount++
			}
		}

		t.Infof("API summary: %d endpoints, %d schemas, %d parameters",
			endpointCount, schemaCount, parameterCount)

		return result, nil
	})

	// Wait for all clicky tasks to complete
	exitCode := clicky.WaitForGlobalCompletionSilent()
	if exitCode != 0 {
		return fmt.Errorf("OpenAPI analysis failed with exit code %d", exitCode)
	}

	return nil
}
