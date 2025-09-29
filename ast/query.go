package ast

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
)

// ExecuteAQLQuery executes an AQL query and returns matching nodes
func (a *Analyzer) ExecuteAQLQuery(aqlQuery string) ([]*models.ASTNode, error) {
	// Only log if we're in verbose/debug mode
	if logger.IsLevelEnabled(3) { // Debug level
		logger.Debugf("🔍 Executing AQL query: %s", aqlQuery)
	}
	startTime := time.Now()

	// Check if it's a metric query (e.g., "*.lines > 100")
	if strings.Contains(aqlQuery, ">") || strings.Contains(aqlQuery, "<") || strings.Contains(aqlQuery, ">=") || strings.Contains(aqlQuery, "<=") || strings.Contains(aqlQuery, "==") || strings.Contains(aqlQuery, "!=") {
		nodes, err := a.executeMetricQuery(aqlQuery)
		elapsed := time.Since(startTime)
		if err != nil {
			if logger.IsLevelEnabled(3) {
				logger.Debugf("❌ Query failed after %.3fs: %v", elapsed.Seconds(), err)
			}
			return nil, err
		}
		if logger.IsLevelEnabled(3) {
			logger.Debugf("✅ Query completed in %.3fs, found %d nodes", elapsed.Seconds(), len(nodes))
		}
		return nodes, nil
	}

	// Try to parse as a simple pattern
	_, err := models.ParsePattern(aqlQuery)
	if err == nil {
		// It's a simple pattern, query directly
		nodes, err := a.QueryPattern(aqlQuery)
		elapsed := time.Since(startTime)
		if err != nil {
			if logger.IsLevelEnabled(3) {
				logger.Debugf("❌ Query failed after %.3fs: %v", elapsed.Seconds(), err)
			}
			return nil, err
		}
		if logger.IsLevelEnabled(3) {
			logger.Debugf("✅ Query completed in %.3fs, found %d nodes", elapsed.Seconds(), len(nodes))
		}
		return nodes, nil
	}

	return nil, fmt.Errorf("invalid query format: %s", aqlQuery)
}

// QueryPattern queries AST nodes matching a pattern
func (a *Analyzer) QueryPattern(pattern string) ([]*models.ASTNode, error) {
	// Parse the pattern
	aqlPattern, err := models.ParsePattern(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	// Log verbose pattern information
	if logger.IsLevelEnabled(3) { // Debug level
		logger.Debugf("Parsed pattern: %s", aqlPattern.String())
		logger.Debugf("  Package: %q", aqlPattern.Package)
		logger.Debugf("  Type: %q", aqlPattern.Type)
		logger.Debugf("  Method: %q", aqlPattern.Method)
		logger.Debugf("  Field: %q", aqlPattern.Field)
		logger.Debugf("  FilePath: %q", aqlPattern.FilePath)
		logger.Debugf("  Metric: %q", aqlPattern.Metric)
		logger.Debugf("  IsWildcard: %t", aqlPattern.IsWildcard)
	}

	// Build GORM query conditions
	var nodes []*models.ASTNode
	query := a.cache.GetDB().Where("file_path LIKE ?", a.workDir+"/%")

	if aqlPattern.Package != "" && aqlPattern.Package != "*" {
		if strings.Contains(aqlPattern.Package, "*") {
			query = query.Where("package_name LIKE ?", strings.ReplaceAll(aqlPattern.Package, "*", "%"))
		} else {
			query = query.Where("package_name = ?", aqlPattern.Package)
		}
	}

	if aqlPattern.Type != "" && aqlPattern.Type != "*" {
		if strings.Contains(aqlPattern.Type, "*") {
			query = query.Where("type_name LIKE ?", strings.ReplaceAll(aqlPattern.Type, "*", "%"))
		} else {
			query = query.Where("type_name = ?", aqlPattern.Type)
		}
	}

	if aqlPattern.Method != "" && aqlPattern.Method != "*" {
		if strings.Contains(aqlPattern.Method, "*") {
			query = query.Where("method_name LIKE ?", strings.ReplaceAll(aqlPattern.Method, "*", "%"))
		} else {
			query = query.Where("method_name = ?", aqlPattern.Method)
		}
	}

	// Add language filtering if specified
	if aqlPattern.Language != "" && aqlPattern.Language != "*" {
		lang := strings.ToLower(aqlPattern.Language)
		conditions := []string{}
		args := []interface{}{}

		// Direct language match for nodes with explicit language field
		conditions = append(conditions, "language = ?")
		args = append(args, lang)

		// File path-based inference for nodes without explicit language
		switch lang {
		case "sql":
			conditions = append(conditions, "(language = '' OR language IS NULL) AND file_path LIKE ?")
			args = append(args, "sql://%")
		case "openapi":
			conditions = append(conditions, "(language = '' OR language IS NULL) AND file_path LIKE ?")
			args = append(args, "openapi://%")
		default:
			ext := getFileExtensionForLanguage(lang)
			if ext != "" {
				conditions = append(conditions, "(language = '' OR language IS NULL) AND file_path LIKE ?")
				args = append(args, "%."+ext)
			}
		}

		if len(conditions) > 0 {
			query = query.Where(strings.Join(conditions, " OR "), args...)
		}
	}

	// Log verbose query information
	if logger.IsLevelEnabled(3) { // Debug level
		logger.Debugf("Executing GORM query with pattern: %+v", aqlPattern)
	}

	// Execute the query
	err = query.Find(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query AST nodes: %w", err)
	}

	// Filter by file path pattern if specified
	if aqlPattern.FilePath != "" && aqlPattern.FilePath != "*" {
		var filteredNodes []*models.ASTNode
		for _, node := range nodes {
			if aqlPattern.Matches(node) {
				filteredNodes = append(filteredNodes, node)
			}
		}
		nodes = filteredNodes
	}

	// Log verbose results
	if logger.IsLevelEnabled(3) { // Debug level
		logger.Debugf("Found %d matching nodes:", len(nodes))
		for i, node := range nodes {
			if i < 10 { // Limit verbose output to first 10 matches
				fullPattern := a.formatNodeAsPattern(node)
				logger.Debugf("  [%d] %s (line %d-%d, complexity=%d)",
					i+1, fullPattern, node.StartLine, node.EndLine, node.CyclomaticComplexity)
			} else if i == 10 {
				logger.Debugf("  ... and %d more nodes", len(nodes)-10)
				break
			}
		}
	}

	return nodes, nil
}

// executeMetricQuery executes a metric-based query using function syntax
// Supports syntax: metric(pattern) operator value (e.g., lines(*) > 100)
func (a *Analyzer) executeMetricQuery(query string) ([]*models.ASTNode, error) {
	// Find the operator
	var operator string
	var operatorIndex int
	operators := []string{">=", "<=", "!=", "==", ">", "<"} // Check longer operators first

	for _, op := range operators {
		if idx := strings.Index(query, op); idx != -1 {
			operator = op
			operatorIndex = idx
			break
		}
	}

	if operator == "" {
		return nil, fmt.Errorf("no operator found in query: %s", query)
	}

	// Split the query into pattern and value parts
	patternPart := strings.TrimSpace(query[:operatorIndex])
	valuePart := strings.TrimSpace(query[operatorIndex+len(operator):])

	// Parse the value
	value, err := strconv.Atoi(valuePart)
	if err != nil {
		return nil, fmt.Errorf("invalid numeric value: %s", valuePart)
	}

	var pattern, metric string

	// Parse function-style syntax: metric(pattern)
	// Example: lines(*), cyclomatic(Service*), len(*), imports(*)
	matches := regexp.MustCompile(`^(\w+)\((.*?)\)$`).FindStringSubmatch(patternPart)
	if len(matches) != 3 {
		return nil, fmt.Errorf("invalid metric query format: %s. Use function syntax like: lines(*) > 100", patternPart)
	}

	metric = matches[1]
	pattern = matches[2]
	if pattern == "" {
		pattern = "*" // Default to all if empty parentheses
	}

	// Log verbose information
	if logger.IsLevelEnabled(3) { // Debug level
		logger.Debugf("Executing metric query:")
		logger.Debugf("  Pattern: %q", pattern)
		logger.Debugf("  Metric: %q", metric)
		logger.Debugf("  Operator: %q", operator)
		logger.Debugf("  Value: %d", value)
	}

	// Query nodes matching the pattern
	nodes, err := a.QueryPattern(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to query pattern: %w", err)
	}

	// Filter nodes based on the metric condition
	var filteredNodes []*models.ASTNode
	for _, node := range nodes {
		metricValue := 0
		var err error

		switch metric {
		case "lines":
			metricValue = node.LineCount
		case "cyclomatic":
			metricValue = node.CyclomaticComplexity
		case "parameters", "params": // params is an alias for parameters
			// Use ParameterCount if available, otherwise fall back to len(Parameters)
			if node.ParameterCount > 0 {
				metricValue = node.ParameterCount
			} else {
				metricValue = len(node.Parameters)
			}
		case "returns":
			// Use ReturnCount if available, otherwise fall back to len(ReturnValues)
			if node.ReturnCount > 0 {
				metricValue = node.ReturnCount
			} else {
				metricValue = len(node.ReturnValues)
			}
		case "len": // Length of the node's full name
			fullName := a.formatNodeAsPattern(node)
			metricValue = len(fullName)
		case "imports": // Number of import relationships
			metricValue, err = a.cache.CountImports(node.ID)
			if err != nil {
				logger.Warnf("Failed to count imports for node %d: %v", node.ID, err)
				metricValue = 0
			}
		case "calls": // Number of external call relationships
			metricValue, err = a.cache.CountExternalCalls(node.ID)
			if err != nil {
				logger.Warnf("Failed to count calls for node %d: %v", node.ID, err)
				metricValue = 0
			}
		default:
			return nil, fmt.Errorf("unknown metric: %s", metric)
		}

		// Apply the operator
		matches := false
		switch operator {
		case ">":
			matches = metricValue > value
		case "<":
			matches = metricValue < value
		case ">=":
			matches = metricValue >= value
		case "<=":
			matches = metricValue <= value
		case "==":
			matches = metricValue == value
		case "!=":
			matches = metricValue != value
		}

		if matches {
			filteredNodes = append(filteredNodes, node)
			if logger.IsLevelEnabled(3) { // Debug level
				fullPattern := a.formatNodeAsPattern(node)
				logger.Debugf("  Match: %s (%s=%d)", fullPattern, metric, metricValue)
			}
		}
	}

	if logger.IsLevelEnabled(3) { // Debug level
		logger.Debugf("Metric query filtered %d nodes to %d matches", len(nodes), len(filteredNodes))
	}

	return filteredNodes, nil
}

// formatNodeAsPattern formats an AST node as a full pattern string
func (a *Analyzer) formatNodeAsPattern(node *models.ASTNode) string {
	parts := []string{}

	if node.PackageName != "" {
		parts = append(parts, node.PackageName)
	} else {
		parts = append(parts, "*")
	}

	if node.TypeName != "" {
		parts = append(parts, node.TypeName)
	} else if node.NodeType == "type" {
		parts = append(parts, "*")
	}

	if node.MethodName != "" {
		parts = append(parts, node.MethodName)
	} else if node.NodeType == "method" {
		parts = append(parts, "*")
	}

	if node.FieldName != "" {
		parts = append(parts, node.FieldName)
	} else if node.NodeType == "field" {
		parts = append(parts, "*")
	}

	pattern := strings.Join(parts, ":")

	// Add node type annotation for clarity
	pattern += fmt.Sprintf(" [%s]", node.NodeType)

	return pattern
}

// GetOverview returns an overview of AST statistics
func (a *Analyzer) GetOverview() (*Overview, error) {
	// Get node type statistics
	query := "SELECT node_type, COUNT(*) as count FROM ast_nodes WHERE file_path LIKE ? GROUP BY node_type"
	workingDirPattern := a.workDir + "/%"
	rows, err := a.cache.QueryRaw(query, workingDirPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to get AST statistics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats := make(map[string]int)
	total := 0

	for rows.Next() {
		var nodeType string
		var count int
		if err := rows.Scan(&nodeType, &count); err != nil {
			return nil, err
		}
		stats[nodeType] = count
		total += count
	}

	if total == 0 {
		return &Overview{
			Directory: a.workDir,
			Stats:     stats,
			Total:     0,
		}, nil
	}

	return BuildOverview(stats, a.workDir), nil
}

// GetNodeRelationships returns relationships for a specific node
func (a *Analyzer) GetNodeRelationships(nodeID int64, relType string) ([]*models.ASTRelationship, error) {
	return a.cache.GetASTRelationships(nodeID, relType)
}

// GetNodeLibraries returns library dependencies for a specific node
func (a *Analyzer) GetNodeLibraries(nodeID int64, relType string) ([]*models.LibraryRelationship, error) {
	return a.cache.GetLibraryRelationships(nodeID, relType)
}

// QueryComplexNodes returns nodes with complexity above threshold
func (a *Analyzer) QueryComplexNodes(threshold int) ([]*models.ASTNode, error) {
	query := "SELECT * FROM ast_nodes WHERE cyclomatic_complexity >= ? AND file_path LIKE ? ORDER BY cyclomatic_complexity DESC"
	workingDirPattern := a.workDir + "/%"

	return a.cache.QueryASTNodes(query, threshold, workingDirPattern)
}

// QueryByFile returns all nodes in a specific file
func (a *Analyzer) QueryByFile(filePath string) ([]*models.ASTNode, error) {
	query := "SELECT * FROM ast_nodes WHERE file_path = ? ORDER BY start_line"
	return a.cache.QueryASTNodes(query, filePath)
}

// QueryByPackage returns all nodes in a specific package
func (a *Analyzer) QueryByPackage(packageName string) ([]*models.ASTNode, error) {
	query := "SELECT * FROM ast_nodes WHERE package_name = ? AND file_path LIKE ? ORDER BY file_path, start_line"
	workingDirPattern := a.workDir + "/%"
	return a.cache.QueryASTNodes(query, packageName, workingDirPattern)
}

// getFileExtensionForLanguage returns the file extension for a given language
func getFileExtensionForLanguage(language string) string {
	switch strings.ToLower(language) {
	case "go":
		return "go"
	case "python":
		return "py"
	case "javascript":
		return "js"
	case "typescript":
		return "ts"
	case "java":
		return "java"
	case "rust":
		return "rs"
	case "sql":
		return "sql"
	default:
		return ""
	}
}

// AnalysisSource represents a source of analyzed data
type AnalysisSource struct {
	Type     string `json:"type"`      // "files", "sql", "openapi", "custom"
	Name     string `json:"name"`      // Human readable name
	NodeCount int   `json:"node_count"` // Number of nodes from this source
}

// GetAnalysisSources returns a breakdown of all analysis sources
func (a *Analyzer) GetAnalysisSources() ([]AnalysisSource, error) {
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

	workingDirPattern := a.workDir + "/%"
	rows, err := a.cache.QueryRaw(query, workingDirPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to get analysis sources: %w", err)
	}
	defer rows.Close()

	var sources []AnalysisSource
	for rows.Next() {
		var language string
		var count int
		if err := rows.Scan(&language, &count); err != nil {
			return nil, err
		}

		// Create human-readable names
		var sourceName string
		var sourceType string
		switch language {
		case "sql":
			sourceName = "SQL databases"
			sourceType = "sql"
		case "openapi":
			sourceName = "OpenAPI specifications"
			sourceType = "openapi"
		case "custom":
			sourceName = "Custom analyzers"
			sourceType = "custom"
		case "go":
			sourceName = "Go files"
			sourceType = "files"
		case "python":
			sourceName = "Python files"
			sourceType = "files"
		case "javascript", "typescript":
			sourceName = "JavaScript/TypeScript files"
			sourceType = "files"
		case "java":
			sourceName = "Java files"
			sourceType = "files"
		case "rust":
			sourceName = "Rust files"
			sourceType = "files"
		default:
			sourceName = fmt.Sprintf("%s files", strings.Title(language))
			sourceType = "files"
		}

		sources = append(sources, AnalysisSource{
			Type:     sourceType,
			Name:     sourceName,
			NodeCount: count,
		})
	}

	return sources, nil
}

// FormatNoNodesFoundError creates an enhanced error message with analysis source information
func (a *Analyzer) FormatNoNodesFoundError(pattern string) error {
	sources, err := a.GetAnalysisSources()
	if err != nil {
		// Fallback to simple error if we can't get source info
		return fmt.Errorf("no nodes found matching pattern: '%s'", pattern)
	}

	var message strings.Builder
	message.WriteString(fmt.Sprintf("No nodes found matching pattern: '%s'\n", pattern))

	if len(sources) == 0 {
		message.WriteString("No analysis sources found. Run analysis first with: arch-unit ast analyze")
		return fmt.Errorf("%s", message.String())
	}

	message.WriteString("Analysis sources:\n")
	totalNodes := 0
	for _, source := range sources {
		message.WriteString(fmt.Sprintf("- %s: %d nodes\n", source.Name, source.NodeCount))
		totalNodes += source.NodeCount
	}
	message.WriteString(fmt.Sprintf("Total: %d nodes analyzed across %d sources", totalNodes, len(sources)))

	return fmt.Errorf("%s", message.String())
}
