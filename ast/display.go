package ast

import (
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky/api"
)

// TreeDisplay represents the tree view of AST nodes
type TreeDisplay struct {
	Pattern string     `json:"pattern" pretty:"label,style=text-blue-600 bold"`
	Count   int        `json:"count" pretty:"int,label=Nodes Found"`
	Files   []FileNode `json:"files" pretty:"tree"`
}

// FileNode represents a file in the tree
type FileNode struct {
	Path       string      `json:"path"`
	Classes    []ClassNode `json:"classes"`
	workingDir string      // internal use
}

// ClassNode represents a class/type in the tree
type ClassNode struct {
	Name     string       `json:"name"`
	Icon     string       `json:"icon,omitempty"`
	Members  []MemberInfo `json:"members,omitempty"`
	nodeType string       // internal: method, field, type, variable
}

// MemberInfo represents a member (method/field) with metadata
type MemberInfo struct {
	Name       string `json:"name"`
	Line       int    `json:"line"`
	Complexity int    `json:"complexity,omitempty"`
}

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

// Implement api.TreeNode interface for FileNode
func (f *FileNode) GetLabel() string {
	// Make path relative to working directory
	path := f.Path
	if f.workingDir != "" && strings.HasPrefix(path, f.workingDir+"/") {
		path = strings.TrimPrefix(path, f.workingDir+"/")
	}
	return path
}

func (f *FileNode) GetChildren() []api.TreeNode {
	nodes := make([]api.TreeNode, len(f.Classes))
	for i, class := range f.Classes {
		nodes[i] = &class
	}
	return nodes
}

func (f *FileNode) GetIcon() string {
	// Determine icon based on file extension
	switch {
	case strings.HasSuffix(f.Path, ".go"):
		return "🐹"
	case strings.HasSuffix(f.Path, ".py"):
		return "🐍"
	case strings.HasSuffix(f.Path, ".js"), strings.HasSuffix(f.Path, ".jsx"):
		return "📜"
	case strings.HasSuffix(f.Path, ".ts"), strings.HasSuffix(f.Path, ".tsx"):
		return "📘"
	case strings.HasSuffix(f.Path, ".md"):
		return "📝"
	default:
		return "📁"
	}
}

func (f *FileNode) GetStyle() string {
	return "text-blue-600 font-bold"
}

func (f *FileNode) IsLeaf() bool {
	return len(f.Classes) == 0
}

// Implement api.TreeNode interface for ClassNode
func (c *ClassNode) GetLabel() string {
	label := c.Name
	if c.Name == "" {
		label = "package-level"
	}

	// Add member count summary
	if len(c.Members) > 0 {
		label = fmt.Sprintf("%s (%d)", label, len(c.Members))
	}

	return label
}

func (c *ClassNode) GetChildren() []api.TreeNode {
	// For compact display, members are shown inline
	return nil
}

func (c *ClassNode) GetIcon() string {
	if c.Icon != "" {
		return c.Icon
	}

	// Default icons based on type
	switch c.nodeType {
	case "method":
		return "⚡"
	case "field":
		return "📊"
	case "type":
		return "🏷️"
	case "variable":
		return "📝"
	default:
		if c.Name == "package-level" {
			return "📦"
		}
		return "🏗️"
	}
}

func (c *ClassNode) GetStyle() string {
	if c.Name == "package-level" {
		return "text-green-500"
	}
	return "text-green-600 font-semibold"
}

func (c *ClassNode) IsLeaf() bool {
	return true
}

// GetCompactMembers returns a formatted string of members for inline display
func (c *ClassNode) GetCompactMembers() string {
	if len(c.Members) == 0 {
		return ""
	}

	var items []string
	for _, member := range c.Members {
		item := fmt.Sprintf("%s:%d", member.Name, member.Line)
		if member.Complexity > 0 {
			item += fmt.Sprintf("(c:%d)", member.Complexity)
		}
		items = append(items, item)
	}

	return strings.Join(items, ", ")
}

// BuildTreeDisplay converts AST nodes to tree display structure
func BuildTreeDisplay(nodes []*models.ASTNode, pattern string, workingDir string) *TreeDisplay {
	display := &TreeDisplay{
		Pattern: pattern,
		Count:   len(nodes),
		Files:   []FileNode{},
	}

	// Group nodes by file -> class -> members
	fileMap := make(map[string]*FileNode)

	for _, node := range nodes {
		// Get or create file node
		relPath := node.FilePath
		if strings.HasPrefix(node.FilePath, workingDir+"/") {
			relPath = strings.TrimPrefix(node.FilePath, workingDir+"/")
		}

		fileNode, exists := fileMap[relPath]
		if !exists {
			fileNode = &FileNode{
				Path:       node.FilePath,
				Classes:    []ClassNode{},
				workingDir: workingDir,
			}
			fileMap[relPath] = fileNode
		}

		// Find or create class node
		className := node.TypeName
		if className == "" {
			className = "package-level"
		}

		var classNode *ClassNode
		for i := range fileNode.Classes {
			if fileNode.Classes[i].Name == className {
				classNode = &fileNode.Classes[i]
				break
			}
		}

		if classNode == nil {
			newClass := ClassNode{
				Name:     className,
				Members:  []MemberInfo{},
				nodeType: string(node.NodeType),
			}
			fileNode.Classes = append(fileNode.Classes, newClass)
			classNode = &fileNode.Classes[len(fileNode.Classes)-1]
		}

		// Add member info
		memberName := node.MethodName
		if memberName == "" {
			memberName = node.FieldName
		}
		if memberName == "" && node.TypeName != "" {
			memberName = node.TypeName
		}

		if memberName != "" {
			classNode.Members = append(classNode.Members, MemberInfo{
				Name:       memberName,
				Line:       node.StartLine,
				Complexity: node.CyclomaticComplexity,
			})
		}
	}

	// Convert map to slice
	for _, fileNode := range fileMap {
		display.Files = append(display.Files, *fileNode)
	}

	return display
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
