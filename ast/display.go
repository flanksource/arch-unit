package ast

import (
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky/api"
)


// TableDisplay represents the table view of AST nodes
type TableDisplay struct {
	Rows []NodeRow `json:"rows" pretty:"table,sort=file,header_style=bold"`
}

// NodeRow represents a row in the table view
type NodeRow struct {
	File       string `json:"file" pretty:"style=text-blue-500"`
	Package    string `json:"package"`
	Type       string `json:"type"`
	Method     string `json:"method"`
	Imports    int    `json:"imports"`
	Complexity int    `json:"complexity" pretty:"render=ast_complexity"`
	Lines      int    `json:"lines"`
}

// Overview represents AST statistics overview
type Overview struct {
	Directory string         `json:"directory" pretty:"label,style=text-blue-600 bold"`
	Stats     map[string]int `json:"stats" pretty:"table"`
	Total     int            `json:"total" pretty:"int,style=bold"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalFiles  int    `json:"total_files" pretty:"int"`
	CachedFiles int    `json:"cached_files" pretty:"int"`
	TotalNodes  int    `json:"total_nodes" pretty:"int"`
	LastUpdated string `json:"last_updated" pretty:"date"`
}


// BuildTreeDisplay converts AST nodes to tree display structure using ASTNode tree
func BuildTreeDisplay(nodes []*models.ASTNode, pattern string, workingDir string) api.TreeNode {
	// Use the existing BuildASTNodeTree function from models package
	return models.BuildASTNodeTree(nodes)
}

// BuildTableDisplay converts AST nodes to table display structure
func BuildTableDisplay(nodes []*models.ASTNode, workingDir string) *TableDisplay {
	display := &TableDisplay{
		Rows: make([]NodeRow, len(nodes)),
	}

	for i, node := range nodes {
		relPath := node.FilePath
		if strings.HasPrefix(node.FilePath, workingDir+"/") {
			relPath = strings.TrimPrefix(node.FilePath, workingDir+"/")
		}

		display.Rows[i] = NodeRow{
			File:       relPath,
			Package:    node.PackageName,
			Type:       node.TypeName,
			Method:     node.MethodName,
			Complexity: node.CyclomaticComplexity,
			Lines:      node.LineCount,
		}
	}

	return display
}

// BuildOverview creates an overview from node statistics
func BuildOverview(stats map[string]int, workingDir string) *Overview {
	total := 0
	for _, count := range stats {
		total += count
	}

	return &Overview{
		Directory: workingDir,
		Stats:     stats,
		Total:     total,
	}
}

// FixtureTreeDisplay represents fixture test results in tree format
type FixtureTreeDisplay struct {
	Summary FixtureSummary    `json:"summary" pretty:"label,style=text-blue-600 bold"`
	Tests   []FixtureTestNode `json:"tests" pretty:"tree"`
}

// FixtureTableDisplay represents fixture test results in table format
type FixtureTableDisplay struct {
	Summary FixtureSummary      `json:"summary" pretty:"label,style=text-blue-600 bold"`
	Rows    []FixtureTestResult `json:"rows" pretty:"table,sort=status,header_style=bold"`
}

// FixtureSummary represents test execution summary
type FixtureSummary struct {
	Total   int `json:"total" pretty:"int,label=Total Tests"`
	Passed  int `json:"passed" pretty:"int,label=Passed,style=text-green-600"`
	Failed  int `json:"failed" pretty:"int,label=Failed,style=text-red-600"`
	Skipped int `json:"skipped" pretty:"int,label=Skipped,style=text-yellow-600"`
}

// FixtureTestNode represents a test node in tree display
type FixtureTestNode struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Expected int    `json:"expected,omitempty"`
	Actual   int    `json:"actual,omitempty"`
	Error    string `json:"error,omitempty"`
	Details  string `json:"details,omitempty"`
}

// FixtureTestResult represents a test result row in table display
type FixtureTestResult struct {
	Name     string `json:"name" pretty:"style=text-blue-500"`
	Type     string `json:"type"`
	Status   string `json:"status" pretty:"render=fixture_status"`
	Expected int    `json:"expected,omitempty"`
	Actual   int    `json:"actual,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Implement api.TreeNode interface for FixtureTestNode
func (f *FixtureTestNode) GetLabel() string {
	label := fmt.Sprintf("%s [%s]", f.Name, f.Type)
	if f.Expected > 0 || f.Actual > 0 {
		label = fmt.Sprintf("%s (%d/%d)", label, f.Actual, f.Expected)
	}
	return label
}

func (f *FixtureTestNode) GetChildren() []api.TreeNode {
	return nil // Leaf node
}

func (f *FixtureTestNode) GetIcon() string {
	switch f.Status {
	case "PASS":
		return "✅"
	case "FAIL":
		return "❌"
	case "SKIP":
		return "⏭️"
	default:
		return "🔍"
	}
}

func (f *FixtureTestNode) GetStyle() string {
	switch f.Status {
	case "PASS":
		return "text-green-600"
	case "FAIL":
		return "text-red-600"
	case "SKIP":
		return "text-yellow-600"
	default:
		return "text-gray-600"
	}
}

func (f *FixtureTestNode) IsLeaf() bool {
	return true
}
