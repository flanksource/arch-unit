package ast

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky/api"
)

// ASTAnalysisReport represents the complete AST analysis with proper clicky tags
type ASTAnalysisReport struct {
	Summary    ASTSummary      `json:"summary" pretty:"struct"`
	Files      []FileAnalysis  `json:"files" pretty:"table,title=Analyzed Files,sort=file"`
	Complexity []MethodComplexity `json:"complexity,omitempty" pretty:"table,title=High Complexity Methods,sort=complexity,dir=desc"`
	Parameters []MethodSignature `json:"parameters,omitempty" pretty:"table,title=Method Signatures"`
}

// ASTSummary contains summary statistics with pretty formatting
type ASTSummary struct {
	TotalFiles         int     `json:"total_files" pretty:"color,blue,label=Total Files"`
	TotalNodes         int     `json:"total_nodes" pretty:"color,green,label=Total Nodes"`
	TotalMethods       int     `json:"total_methods" pretty:"color,cyan,label=Total Methods"`
	TotalTypes         int     `json:"total_types" pretty:"color,magenta,label=Total Types"`
	TotalRelationships int     `json:"total_relationships" pretty:"color,yellow,label=Total Relationships"`
	AverageComplexity  float64 `json:"avg_complexity" pretty:"format=%.2f,label=Average Complexity"`
	MaxComplexity      int     `json:"max_complexity" pretty:"color,red,label=Max Complexity"`
}

// FileAnalysis represents analysis data for a single file
type FileAnalysis struct {
	File          string `json:"file" pretty:"label=File"`
	Package       string `json:"package" pretty:"label=Package"`
	Methods       int    `json:"methods" pretty:"label=Methods"`
	Types         int    `json:"types" pretty:"label=Types"`
	Fields        int    `json:"fields" pretty:"label=Fields"`
	Variables     int    `json:"variables" pretty:"label=Variables"`
	Relationships int    `json:"relationships" pretty:"label=Relations"`
	Libraries     int    `json:"libraries" pretty:"label=Libraries"`
}

// MethodComplexity represents a complex method
type MethodComplexity struct {
	File       string `json:"file" pretty:"label=File"`
	Method     string `json:"method" pretty:"label=Method"`
	Complexity int    `json:"complexity" pretty:"color,red>=10,yellow>=5,green<5,label=Complexity"`
	Lines      int    `json:"lines" pretty:"label=Lines"`
	Params     int    `json:"params" pretty:"label=Params"`
	Returns    int    `json:"returns" pretty:"label=Returns"`
}

// MethodSignature represents a method with its parameters and return values
type MethodSignature struct {
	File       string `json:"file" pretty:"label=File"`
	Method     string `json:"method" pretty:"label=Method"`
	Parameters string `json:"parameters" pretty:"label=Parameters"`
	Returns    string `json:"returns" pretty:"label=Returns"`
}

// ASTQueryResult represents the result of an AST pattern query with proper clicky tags
type ASTQueryResult struct {
	Summary ASTQuerySummary `json:"summary" pretty:"struct"`
	Results []ASTQueryNode  `json:"results" pretty:"table,title=Query Results,sort=file"`
}

// ASTQuerySummary contains query summary information
type ASTQuerySummary struct {
	Query       string `json:"query" pretty:"label=Query Pattern"`
	TotalNodes  int    `json:"total_nodes" pretty:"color,green,label=Total Matches"`
	FileCount   int    `json:"file_count" pretty:"color,blue,label=Files Matched"`
	PackageCount int   `json:"package_count" pretty:"color,cyan,label=Packages"`
}

// ASTQueryNode represents a single AST node result
type ASTQueryNode struct {
	File       string `json:"file" pretty:"label=File"`
	Package    string `json:"package" pretty:"label=Package"`
	Type       string `json:"type,omitempty" pretty:"label=Type"`
	Method     string `json:"method,omitempty" pretty:"label=Method"`
	Field      string `json:"field,omitempty" pretty:"label=Field"`
	NodeType   string `json:"node_type" pretty:"label=Node Type"`
	StartLine  int    `json:"start_line" pretty:"label=Line"`
	EndLine    int    `json:"end_line,omitempty" pretty:"label=End Line"`
	Complexity int    `json:"complexity,omitempty" pretty:"color,red>=10,yellow>=5,green<5,label=Complexity"`
	Lines      int    `json:"lines,omitempty" pretty:"label=Lines"`
	Parameters string `json:"parameters,omitempty" pretty:"label=Parameters"`
	Returns    string `json:"returns,omitempty" pretty:"label=Returns"`
}

// Pretty implements the api.Pretty interface for custom formatting
func (n ASTQueryNode) Pretty() api.Text {
	// Format the file path with appropriate styling
	return api.Text{
		Content: n.File,
		Style:   "text-blue-600",
	}
}

// ASTResultFormatter formats AST analysis results using clicky
type ASTResultFormatter struct {
	Results      []FileResult
	NoColor      bool
	CompactMode  bool
}

// NewASTResultFormatter creates a new formatter
func NewASTResultFormatter(results []FileResult) *ASTResultFormatter {
	return &ASTResultFormatter{
		Results: results,
	}
}

// CreateQueryResult creates an ASTQueryResult from AST nodes for pattern queries
func CreateQueryResult(nodes []*models.ASTNode, query string, workingDir string) ASTQueryResult {
	if len(nodes) == 0 {
		return ASTQueryResult{
			Summary: ASTQuerySummary{
				Query:       query,
				TotalNodes:  0,
				FileCount:   0,
				PackageCount: 0,
			},
			Results: []ASTQueryNode{},
		}
	}
	
	// Build summary statistics
	fileSet := make(map[string]bool)
	packageSet := make(map[string]bool)
	var queryNodes []ASTQueryNode
	
	for _, node := range nodes {
		// Track unique files and packages
		fileSet[node.FilePath] = true
		if node.PackageName != "" {
			packageSet[node.PackageName] = true
		}
		
		// Convert to relative path
		fileName := node.FilePath
		if workingDir != "" {
			if relPath, err := filepath.Rel(workingDir, node.FilePath); err == nil && !strings.HasPrefix(relPath, "..") {
				fileName = relPath
			} else {
				// Fallback to just the filename
				fileName = filepath.Base(node.FilePath)
			}
		} else {
			// No working dir provided, use just the filename
			fileName = filepath.Base(node.FilePath)
		}
		
		// Build parameter and return strings
		var params []string
		for _, p := range node.Parameters {
			params = append(params, fmt.Sprintf("%s %s", p.Name, p.Type))
		}
		
		var returns []string
		for _, r := range node.ReturnValues {
			if r.Name != "" {
				returns = append(returns, fmt.Sprintf("%s %s", r.Name, r.Type))
			} else {
				returns = append(returns, r.Type)
			}
		}
		
		queryNode := ASTQueryNode{
			File:       fileName,
			Package:    node.PackageName,
			Type:       node.TypeName,
			Method:     node.MethodName,
			Field:      node.FieldName,
			NodeType:   node.NodeType,
			StartLine:  node.StartLine,
			EndLine:    node.EndLine,
			Complexity: node.CyclomaticComplexity,
			Lines:      node.LineCount,
			Parameters: strings.Join(params, ", "),
			Returns:    strings.Join(returns, ", "),
		}
		
		queryNodes = append(queryNodes, queryNode)
	}
	
	return ASTQueryResult{
		Summary: ASTQuerySummary{
			Query:        query,
			TotalNodes:   len(nodes),
			FileCount:    len(fileSet),
			PackageCount: len(packageSet),
		},
		Results: queryNodes,
	}
}

// FormatQueryWithClicky formats AST query results using clicky formatters
func FormatQueryWithClicky(nodes []*models.ASTNode, query string, workingDir string, formatter interface{}) (string, error) {
	// Create the query result struct
	result := CreateQueryResult(nodes, query, workingDir)
	
	// Use the formatter interface to format the result directly
	switch fmtr := formatter.(type) {
	case interface{ Format(interface{}) (string, error) }:
		return fmtr.Format(result)
	default:
		return "", fmt.Errorf("unsupported formatter type: %T", formatter)
	}
}

// CreateReport creates an ASTAnalysisReport with proper clicky structures
func (f *ASTResultFormatter) CreateReport() ASTAnalysisReport {
	report := ASTAnalysisReport{
		Summary:    f.buildSummaryStruct(),
		Files:      f.buildFileAnalysisList(),
		Complexity: f.buildComplexityList(),
	}
	
	// Add parameters only if not in compact mode
	if !f.CompactMode {
		report.Parameters = f.buildParametersList()
	}
	
	return report
}

// FormatWithClicky uses clicky's proper workflow to format the report
func (f *ASTResultFormatter) FormatWithClicky(formatter interface{}) (string, error) {
	// Create the report struct
	report := f.CreateReport()
	
	// Use the formatter interface to format the report directly
	switch fmtr := formatter.(type) {
	case interface{ Format(interface{}) (string, error) }:
		return fmtr.Format(report)
	default:
		return "", fmt.Errorf("unsupported formatter type: %T", formatter)
	}
}

// buildSummaryStruct creates summary statistics as a struct
func (f *ASTResultFormatter) buildSummaryStruct() ASTSummary {
	totalFiles := 0
	totalNodes := 0
	totalMethods := 0
	totalTypes := 0
	totalRelationships := 0
	totalComplexity := 0
	maxComplexity := 0
	methodCount := 0
	
	for _, result := range f.Results {
		if result.Result == nil {
			continue
		}
		
		totalFiles++
		totalNodes += len(result.Result.Nodes)
		totalRelationships += len(result.Result.Relationships)
		
		for _, node := range result.Result.Nodes {
			switch node.NodeType {
			case models.NodeTypeMethod:
				totalMethods++
				methodCount++
				totalComplexity += node.CyclomaticComplexity
				if node.CyclomaticComplexity > maxComplexity {
					maxComplexity = node.CyclomaticComplexity
				}
			case models.NodeTypeType:
				totalTypes++
			}
		}
	}
	
	avgComplexity := 0.0
	if methodCount > 0 {
		avgComplexity = float64(totalComplexity) / float64(methodCount)
	}
	
	return ASTSummary{
		TotalFiles:         totalFiles,
		TotalNodes:         totalNodes,
		TotalMethods:       totalMethods,
		TotalTypes:         totalTypes,
		TotalRelationships: totalRelationships,
		AverageComplexity:  avgComplexity,
		MaxComplexity:      maxComplexity,
	}
}

// buildFileAnalysisList creates a list of file analysis data
func (f *ASTResultFormatter) buildFileAnalysisList() []FileAnalysis {
	var files []FileAnalysis
	
	for _, result := range f.Results {
		if result.Result == nil {
			continue
		}
		
		// Count different node types
		methods := 0
		types := 0
		fields := 0
		vars := 0
		
		for _, node := range result.Result.Nodes {
			switch node.NodeType {
			case models.NodeTypeMethod:
				methods++
			case models.NodeTypeType:
				types++
			case models.NodeTypeField:
				fields++
			case models.NodeTypeVariable:
				vars++
			}
		}
		
		// Get file name only
		path := result.Path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			path = path[idx+1:]
		}
		
		files = append(files, FileAnalysis{
			File:          path,
			Package:       result.Result.PackageName,
			Methods:       methods,
			Types:         types,
			Fields:        fields,
			Variables:     vars,
			Relationships: len(result.Result.Relationships),
			Libraries:     len(result.Result.Libraries),
		})
	}
	
	return files
}

// buildComplexityList creates a list of high complexity methods
func (f *ASTResultFormatter) buildComplexityList() []MethodComplexity {
	var methods []MethodComplexity
	
	for _, result := range f.Results {
		if result.Result == nil {
			continue
		}
		
		for _, node := range result.Result.Nodes {
			if node.NodeType == models.NodeTypeMethod && node.CyclomaticComplexity > 5 {
				// Get file name only
				path := result.Path
				if idx := strings.LastIndex(path, "/"); idx >= 0 {
					path = path[idx+1:]
				}
				
				fullName := node.MethodName
				if node.TypeName != "" {
					fullName = fmt.Sprintf("%s.%s", node.TypeName, node.MethodName)
				}
				
				methods = append(methods, MethodComplexity{
					File:       path,
					Method:     fullName,
					Complexity: node.CyclomaticComplexity,
					Lines:      node.LineCount,
					Params:     len(node.Parameters),
					Returns:    len(node.ReturnValues),
				})
			}
		}
	}
	
	// Sort by complexity descending
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Complexity > methods[j].Complexity
	})
	
	return methods
}

// buildParametersList creates a list of method signatures with parameters
func (f *ASTResultFormatter) buildParametersList() []MethodSignature {
	var signatures []MethodSignature
	
	for _, result := range f.Results {
		if result.Result == nil {
			continue
		}
		
		for _, node := range result.Result.Nodes {
			if node.NodeType == models.NodeTypeMethod && len(node.Parameters) > 0 {
				// Get file name only
				path := result.Path
				if idx := strings.LastIndex(path, "/"); idx >= 0 {
					path = path[idx+1:]
				}
				
				fullName := node.MethodName
				if node.TypeName != "" {
					fullName = fmt.Sprintf("%s.%s", node.TypeName, node.MethodName)
				}
				
				// Build parameter list
				var params []string
				for _, p := range node.Parameters {
					params = append(params, fmt.Sprintf("%s %s", p.Name, p.Type))
				}
				
				// Build return list
				var returns []string
				for _, r := range node.ReturnValues {
					if r.Name != "" {
						returns = append(returns, fmt.Sprintf("%s %s", r.Name, r.Type))
					} else {
						returns = append(returns, r.Type)
					}
				}
				
				signatures = append(signatures, MethodSignature{
					File:       path,
					Method:     fullName,
					Parameters: strings.Join(params, ", "),
					Returns:    strings.Join(returns, ", "),
				})
			}
		}
	}
	
	return signatures
}

// GetSummaryData returns summary data for backwards compatibility (used by JSON output)
func (f *ASTResultFormatter) GetSummaryData() map[string]interface{} {
	summary := f.buildSummaryStruct()
	return map[string]interface{}{
		"total_files":         summary.TotalFiles,
		"total_nodes":         summary.TotalNodes,
		"total_methods":       summary.TotalMethods,
		"total_types":         summary.TotalTypes,
		"total_relationships": summary.TotalRelationships,
		"avg_complexity":      summary.AverageComplexity,
		"max_complexity":      summary.MaxComplexity,
	}
}

// CreateQueryResult creates an ASTQueryResult from AST nodes for pattern queries
func CreateQueryResult(nodes []*models.ASTNode, query string) ASTQueryResult {
	if len(nodes) == 0 {
		return ASTQueryResult{
			Summary: ASTQuerySummary{
				Query:       query,
				TotalNodes:  0,
				FileCount:   0,
				PackageCount: 0,
			},
			Results: []ASTQueryNode{},
		}
	}
	
	// Build summary statistics
	fileSet := make(map[string]bool)
	packageSet := make(map[string]bool)
	var queryNodes []ASTQueryNode
	
	for _, node := range nodes {
		// Track unique files and packages
		fileSet[node.FilePath] = true
		if node.PackageName != "" {
			packageSet[node.PackageName] = true
		}
		
		// Get file name only
		fileName := node.FilePath
		if idx := strings.LastIndex(fileName, "/"); idx >= 0 {
			fileName = fileName[idx+1:]
		}
		
		// Build parameter and return strings
		var params []string
		for _, p := range node.Parameters {
			params = append(params, fmt.Sprintf("%s %s", p.Name, p.Type))
		}
		
		var returns []string
		for _, r := range node.ReturnValues {
			if r.Name != "" {
				returns = append(returns, fmt.Sprintf("%s %s", r.Name, r.Type))
			} else {
				returns = append(returns, r.Type)
			}
		}
		
		queryNode := ASTQueryNode{
			File:       fileName,
			Package:    node.PackageName,
			Type:       node.TypeName,
			Method:     node.MethodName,
			Field:      node.FieldName,
			NodeType:   node.NodeType,
			StartLine:  node.StartLine,
			EndLine:    node.EndLine,
			Complexity: node.CyclomaticComplexity,
			Lines:      node.LineCount,
			Parameters: strings.Join(params, ", "),
			Returns:    strings.Join(returns, ", "),
		}
		
		queryNodes = append(queryNodes, queryNode)
	}
	
	return ASTQueryResult{
		Summary: ASTQuerySummary{
			Query:        query,
			TotalNodes:   len(nodes),
			FileCount:    len(fileSet),
			PackageCount: len(packageSet),
		},
		Results: queryNodes,
	}
}

// FormatQueryWithClicky formats AST query results using clicky formatters
func FormatQueryWithClicky(nodes []*models.ASTNode, query string, formatter interface{}) (string, error) {
	// Create the query result struct
	result := CreateQueryResult(nodes, query)
	
	// Use the formatter interface to format the result directly
	switch fmtr := formatter.(type) {
	case interface{ Format(interface{}) (string, error) }:
		return fmtr.Format(result)
	default:
		return "", fmt.Errorf("unsupported formatter type: %T", formatter)
	}
}