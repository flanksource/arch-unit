package cmd

import (
	jsonenc "encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/api"
	"github.com/flanksource/commons/logger"
)

// DisplayOptions holds configuration for AST node display
type DisplayOptions struct {
	Template       string
	ShowCalls      bool
	ShowLibraries  bool
	ShowComplexity bool
	Threshold      int
	Depth          int
}

// GetDisplayOptionsFromFlags creates DisplayOptions from current flag values
func GetDisplayOptionsFromFlags() DisplayOptions {
	return DisplayOptions{
		Template:       astTemplate,
		ShowCalls:      astShowCalls,
		ShowLibraries:  astShowLibraries,
		ShowComplexity: astShowComplexity,
		Threshold:      astThreshold,
		Depth:          astDepth,
	}
}

// GetDisplayConfigFromFlags converts command-line flags to DisplayConfig
func GetDisplayConfigFromFlags() models.DisplayConfig {
	return models.DisplayConfig{
		ShowDirs:       astShowDirs,
		ShowFiles:      astShowFiles,
		ShowPackages:   astShowPackages,
		ShowTypes:      astShowTypes,
		ShowMethods:    astShowMethods,
		ShowFields:     astShowFields,
		ShowParams:     astShowParams,
		ShowImports:    astShowImports,
		ShowLineNo:     astShowLineNo,
		ShowFileStats:  astShowFileStats,
		ShowComplexity: astShowComplexity,
	}
}

// DisplayNodes displays AST nodes in the requested format
func DisplayNodes(astCache *cache.ASTCache, nodes []*models.ASTNode, pattern string, workingDir string, opts DisplayOptions) error {
	logger.V(4).Infof("DisplayNodes called with %d nodes for pattern '%s'", len(nodes), pattern)

	if len(nodes) == 0 {
		errorMsg := formatNoNodesFoundMessage(astCache, pattern, workingDir)
		logger.Infof("No nodes to display: %s", errorMsg)
		logger.Debugf("No AST nodes found - PrettyRow logging will not be triggered")
		return nil
	}

	// Filter by complexity threshold if specified
	if opts.Threshold > 0 && opts.ShowComplexity {
		var filtered []*models.ASTNode
		for _, node := range nodes {
			if node.CyclomaticComplexity > opts.Threshold {
				filtered = append(filtered, node)
			}
		}
		nodes = filtered

		if len(nodes) == 0 {
			errorMsg := fmt.Sprintf("No nodes found matching pattern: %s with complexity > %d", pattern, opts.Threshold)
			logger.Infof("Complexity filtering resulted in no nodes: %s", errorMsg)
			logger.Debugf("Complexity filtering eliminated all nodes - PrettyRow logging will not be triggered")
			return nil
		}
		logger.V(4).Infof("After complexity filtering: %d nodes remain", len(nodes))
	}

	// Use clicky's format system for all formats
	logger.Debugf("Proceeding to OutputNodes with %d nodes", len(nodes))
	return OutputNodes(astCache, nodes, pattern, workingDir, opts)
}


// OutputNodesTemplate outputs nodes using a template
func OutputNodesTemplate(nodes []*models.ASTNode, workingDir string, templateStr string) error {
	tmpl, err := template.New("ast").Parse(templateStr)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	for _, node := range nodes {
		// Create template data
		data := struct {
			Package    string
			Class      string
			Type       string
			Method     string
			Field      string
			File       string
			Lines      int
			Complexity int
			Params     int
			Returns    int
			NodeType   string
			StartLine  int
			EndLine    int
		}{
			Package:    node.PackageName,
			Class:      node.TypeName,
			Type:       node.TypeName,
			Method:     node.MethodName,
			Field:      node.FieldName,
			File:       MakeRelativePath(node.FilePath, workingDir),
			Lines:      node.LineCount,
			Complexity: node.CyclomaticComplexity,
			Params:     node.ParameterCount,
			Returns:    node.ReturnCount,
			NodeType:   string(node.NodeType),
			StartLine:  node.StartLine,
			EndLine:    node.EndLine,
		}

		if err := tmpl.Execute(os.Stdout, data); err != nil {
			return fmt.Errorf("failed to execute template: %w", err)
		}
		fmt.Println() // Add newline after each node
	}

	return nil
}

// OutputNodes outputs nodes using clicky's format system
func OutputNodes(astCache *cache.ASTCache, nodes []*models.ASTNode, pattern string, workingDir string, opts DisplayOptions) error {
	logger.V(4).Infof("OutputNodes called with %d nodes for format processing", len(nodes))

	// Handle template format specially as it needs custom template processing
	format := clicky.Flags.FormatOptions.ResolveFormat()
	logger.Debugf("Resolved format: %s", format)

	// Check both clicky's resolved format and our astFormat flag for template handling
	if format == "template" || astFormat == "template" {
		if opts.Template == "" {
			return fmt.Errorf("--template flag is required when using --format template")
		}
		return OutputNodesTemplate(nodes, workingDir, opts.Template)
	}

	// Get display configuration from flags
	config := GetDisplayConfigFromFlags()

	// Apply filtering to all formats first
	filteredNodes := models.FilterASTNodes(nodes, config)
	logger.V(4).Infof("After display filtering: %d nodes remain (from %d)", len(filteredNodes), len(nodes))

	// Log node types and whether they implement PrettyRow
	if len(filteredNodes) > 0 {
		logger.V(4).Infof("Node analysis: %s", analyzeNodeTypes(filteredNodes))
	}

	// Use clicky.Format with resolved format options
	formatOptions := clicky.Flags.FormatOptions
	formatOptions.Format = format

	// Default to tree format if pretty is selected (for backward compatibility)
	if format == "pretty" {
		formatOptions.Format = "tree"
	}

	var output string
	var err error

	// Only build hierarchical tree for tree-like formats
	if format == "tree" || format == "pretty" {
		// Build hierarchical tree structure with filesystem awareness
		tree := models.BuildHierarchicalASTTree(filteredNodes, config, workingDir)
		logger.Debugf("Built hierarchical tree, calling clicky.Format with tree structure")
		output, err = clicky.Format(tree, formatOptions)
	} else {
		// Pass filtered slice directly for structured formats (json, yaml, csv, html, etc.)
		logger.Debugf("Calling clicky.Format with %d filtered nodes for format '%s'", len(filteredNodes), format)
		output, err = clicky.Format(filteredNodes, formatOptions)
	}

	if err != nil {
		return fmt.Errorf("failed to format AST nodes: %w", err)
	}

	// Only add header for tree and text-based formats, not for structured formats like JSON
	if format == "tree" || format == "pretty" {
		fmt.Printf("AST Nodes matching pattern: %s\n", pattern)
		fmt.Printf("Found %d nodes:\n\n", len(nodes))
	}

	fmt.Print(output)

	return nil
}

// OutputLocationNodes outputs nodes with focus on file locations
func OutputLocationNodes(nodes []*models.ASTNode, workingDir string, format string) error {
	if len(nodes) == 0 {
		logger.Infof("No nodes found")
		return nil
	}

	switch format {
	case "json":
		return outputLocationJSON(nodes, workingDir)
	default:
		return outputLocationTable(nodes, workingDir)
	}
}

// outputLocationJSON outputs location data as JSON
func outputLocationJSON(nodes []*models.ASTNode, workingDir string) error {
	type LocationData struct {
		File      string `json:"file"`
		Package   string `json:"package,omitempty"`
		Type      string `json:"type,omitempty"`
		Method    string `json:"method,omitempty"`
		Field     string `json:"field,omitempty"`
		NodeType  string `json:"node_type"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}

	var locations []LocationData
	for _, node := range nodes {
		locations = append(locations, LocationData{
			File:      MakeRelativePath(node.FilePath, workingDir),
			Package:   node.PackageName,
			Type:      node.TypeName,
			Method:    node.MethodName,
			Field:     node.FieldName,
			NodeType:  string(node.NodeType),
			StartLine: node.StartLine,
			EndLine:   node.EndLine,
		})
	}

	encoder := jsonenc.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(locations)
}

// outputLocationTable outputs location data as a table
func outputLocationTable(nodes []*models.ASTNode, workingDir string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "Location\tType\tName\tLines\n")
	_, _ = fmt.Fprintf(w, "────────\t────\t────\t─────\n")

	for _, node := range nodes {
		relPath := MakeRelativePath(node.FilePath, workingDir)
		location := fmt.Sprintf("%s:%d", relPath, node.StartLine)

		var name string
		switch {
		case node.MethodName != "":
			name = fmt.Sprintf("%s.%s", node.TypeName, node.MethodName)
		case node.FieldName != "":
			name = fmt.Sprintf("%s.%s", node.TypeName, node.FieldName)
		case node.TypeName != "":
			name = node.TypeName
		default:
			name = node.PackageName
		}

		lines := fmt.Sprintf("%d-%d", node.StartLine, node.EndLine)
		if node.StartLine == node.EndLine {
			lines = fmt.Sprintf("%d", node.StartLine)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			location,
			string(node.NodeType),
			name,
			lines)
	}

	_ = w.Flush()
	return nil
}

// MakeRelativePath converts an absolute path to relative path if it's within workingDir
func MakeRelativePath(filePath, workingDir string) string {
	if strings.HasPrefix(filePath, workingDir+"/") {
		return strings.TrimPrefix(filePath, workingDir+"/")
	}
	return filePath
}

// OutputJSON outputs arbitrary data as JSON
func OutputJSON(data interface{}) error {
	encoder := jsonenc.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// formatNoNodesFoundMessage creates an enhanced error message with analysis source information
func formatNoNodesFoundMessage(astCache *cache.ASTCache, pattern string, workingDir string) string {
	// Query for language statistics from database
	query := `
		SELECT
			CASE
				WHEN language != '' THEN language
				WHEN file_path LIKE 'sql://%' THEN 'sql'
				WHEN file_path LIKE 'openapi://%' THEN 'openapi'
				WHEN file_path LIKE 'virtual://%' THEN 'custom'
				WHEN file_path LIKE '%.go' THEN 'go'
				WHEN file_path LIKE '%.py' THEN 'python'
				WHEN file_path LIKE '%.js' OR file_path LIKE '%.jsx' OR file_path LIKE '%.ts' OR file_path LIKE '%.tsx' THEN 'javascript'
				WHEN file_path LIKE '%.java' THEN 'java'
				WHEN file_path LIKE '%.rs' THEN 'rust'
				ELSE 'unknown'
			END as detected_language,
			COUNT(*) as node_count
		FROM ast_nodes
		WHERE file_path LIKE ? OR file_path LIKE 'sql://%' OR file_path LIKE 'openapi://%' OR file_path LIKE 'virtual://%'
		GROUP BY detected_language
		ORDER BY node_count DESC
	`

	workingDirPattern := workingDir + "/%"
	rows, err := astCache.QueryRaw(query, workingDirPattern)
	if err != nil {
		// Fallback to simple error if we can't get source info
		return fmt.Sprintf("No nodes found matching pattern: '%s'", pattern)
	}
	defer rows.Close()

	var message strings.Builder
	message.WriteString(fmt.Sprintf("No nodes found matching pattern: '%s'\n", pattern))

	var sources []string
	totalNodes := 0
	for rows.Next() {
		var language string
		var count int
		if err := rows.Scan(&language, &count); err != nil {
			continue
		}

		// Create human-readable names
		var sourceName string
		switch language {
		case "sql":
			sourceName = "SQL databases"
		case "openapi":
			sourceName = "OpenAPI specifications"
		case "custom":
			sourceName = "Custom analyzers"
		case "go":
			sourceName = "Go files"
		case "python":
			sourceName = "Python files"
		case "javascript", "typescript":
			sourceName = "JavaScript/TypeScript files"
		case "java":
			sourceName = "Java files"
		case "rust":
			sourceName = "Rust files"
		default:
			sourceName = fmt.Sprintf("%s files", strings.Title(language))
		}

		sources = append(sources, fmt.Sprintf("- %s: %d nodes", sourceName, count))
		totalNodes += count
	}

	if len(sources) == 0 {
		message.WriteString("No analysis sources found. Run analysis first with: arch-unit ast analyze")
	} else {
		// Check if this is a language-specific pattern and provide specific guidance
		aqlPattern, err := models.ParsePattern(pattern)
		if err == nil && aqlPattern.Language != "" {
			// Check if the requested language has any nodes
			hasRequestedLanguage := false
			for _, source := range sources {
				if strings.Contains(strings.ToLower(source), aqlPattern.Language) {
					hasRequestedLanguage = true
					break
				}
			}

			if !hasRequestedLanguage {
				message.WriteString(fmt.Sprintf("No %s nodes found in analysis. ", aqlPattern.Language))
				switch aqlPattern.Language {
				case "sql":
					message.WriteString("To analyze SQL: run 'arch-unit ast analyze-sql' first.\n")
				case "openapi":
					message.WriteString("To analyze OpenAPI: run 'arch-unit ast analyze-openapi' first.\n")
				default:
					message.WriteString(fmt.Sprintf("Ensure %s files are analyzed with 'arch-unit ast analyze'.\n", aqlPattern.Language))
				}
				message.WriteString("\n")
			}
		}

		message.WriteString("Available analysis sources:\n")
		for _, source := range sources {
			message.WriteString(source + "\n")
		}
		message.WriteString(fmt.Sprintf("Total: %d nodes analyzed across %d sources", totalNodes, len(sources)))
	}

	return message.String()
}

// analyzeNodeTypes provides debug information about node types and PrettyRow implementation
func analyzeNodeTypes(nodes []*models.ASTNode) string {
	if len(nodes) == 0 {
		return "no nodes to analyze"
	}

	nodeTypes := make(map[string]int)
	languages := make(map[string]int)
	prettyRowCount := 0

	for _, node := range nodes {
		nodeTypes[node.NodeType]++
		if node.Language != nil {
			languages[*node.Language]++
		}

		// Check if this type implements PrettyRow interface (ASTNode does implement it)
		if _, ok := interface{}(node).(api.PrettyRow); ok {
			prettyRowCount++
		}
	}

	var analysis []string
	analysis = append(analysis, fmt.Sprintf("total=%d", len(nodes)))

	// Add node types summary
	if len(nodeTypes) > 0 {
		var types []string
		for nodeType, count := range nodeTypes {
			types = append(types, fmt.Sprintf("%s(%d)", nodeType, count))
		}
		analysis = append(analysis, fmt.Sprintf("types=[%s]", strings.Join(types, ",")))
	}

	// Add languages summary
	if len(languages) > 0 {
		var langs []string
		for lang, count := range languages {
			langs = append(langs, fmt.Sprintf("%s(%d)", lang, count))
		}
		analysis = append(analysis, fmt.Sprintf("languages=[%s]", strings.Join(langs, ",")))
	}

	// Add PrettyRow info
	analysis = append(analysis, fmt.Sprintf("prettyrow_compatible=%d", prettyRowCount))

	return strings.Join(analysis, ", ")
}
