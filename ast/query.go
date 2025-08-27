package ast

import (
	"encoding/json"
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
		logger.Debugf("  Metric: %q", aqlPattern.Metric)
		logger.Debugf("  IsWildcard: %t", aqlPattern.IsWildcard)
	}
	
	// Build SQL query
	query := "SELECT id, file_path, package_name, type_name, method_name, field_name, node_type, start_line, end_line, cyclomatic_complexity, parameter_count, return_count, line_count, parameters_json, return_values_json FROM ast_nodes WHERE file_path LIKE ?"
	workingDirPattern := a.workDir + "/%"
	args := []interface{}{workingDirPattern}
	
	if aqlPattern.Package != "" && aqlPattern.Package != "*" {
		if strings.Contains(aqlPattern.Package, "*") {
			query += " AND package_name LIKE ?"
			args = append(args, strings.ReplaceAll(aqlPattern.Package, "*", "%"))
		} else {
			query += " AND package_name = ?"
			args = append(args, aqlPattern.Package)
		}
	}
	
	if aqlPattern.Type != "" && aqlPattern.Type != "*" {
		if strings.Contains(aqlPattern.Type, "*") {
			query += " AND type_name LIKE ?"
			args = append(args, strings.ReplaceAll(aqlPattern.Type, "*", "%"))
		} else {
			query += " AND type_name = ?"
			args = append(args, aqlPattern.Type)
		}
	}
	
	if aqlPattern.Method != "" && aqlPattern.Method != "*" {
		if strings.Contains(aqlPattern.Method, "*") {
			query += " AND method_name LIKE ?"
			args = append(args, strings.ReplaceAll(aqlPattern.Method, "*", "%"))
		} else {
			query += " AND method_name = ?"
			args = append(args, aqlPattern.Method)
		}
	}
	
	// Log verbose SQL information
	if logger.IsLevelEnabled(3) { // Debug level
		logger.Debugf("Generated SQL query: %s", query)
		logger.Debugf("Query arguments: %v", args)
	}
	
	rows, err := a.cache.QueryRaw(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query AST nodes: %w", err)
	}
	defer rows.Close()
	
	var nodes []*models.ASTNode
	for rows.Next() {
		node := &models.ASTNode{}
		var parametersJSON, returnValuesJSON []byte
		var parameterCount, returnCount int
		err := rows.Scan(
			&node.ID,
			&node.FilePath,
			&node.PackageName,
			&node.TypeName,
			&node.MethodName,
			&node.FieldName,
			&node.NodeType,
			&node.StartLine,
			&node.EndLine,
			&node.CyclomaticComplexity,
			&parameterCount,
			&returnCount,
			&node.LineCount,
			&parametersJSON,
			&returnValuesJSON,
		)
		if err != nil {
			return nil, err
		}
		
		// Deserialize parameters
		if len(parametersJSON) > 0 {
			if err := json.Unmarshal(parametersJSON, &node.Parameters); err != nil {
				logger.Warnf("Failed to unmarshal parameters for node %d: %v", node.ID, err)
			}
		}
		
		// Deserialize return values
		if len(returnValuesJSON) > 0 {
			if err := json.Unmarshal(returnValuesJSON, &node.ReturnValues); err != nil {
				logger.Warnf("Failed to unmarshal return values for node %d: %v", node.ID, err)
			}
		}
		
		nodes = append(nodes, node)
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
			metricValue = len(node.Parameters)
		case "returns":
			metricValue = len(node.ReturnValues)
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
	defer rows.Close()
	
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