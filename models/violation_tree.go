package models

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flanksource/clicky/api"
)

// ViolationRootNode represents the root node of the violations tree
type ViolationRootNode struct {
	fileNodes []*ViolationFileNode
	total     int
}

// ViolationFileNode represents a file containing violations
type ViolationFileNode struct {
	path        string
	sourceNodes []*ViolationSourceNode
	total       int
}

// ViolationSourceNode represents violations from a specific source (arch-unit, linter, etc.)
type ViolationSourceNode struct {
	source         string
	violationNodes []*ViolationNode
}

// NewViolationRootNode creates a new root node for violations tree
func NewViolationRootNode(violations []Violation) *ViolationRootNode {
	// Group violations by file
	fileMap := make(map[string][]Violation)
	for _, v := range violations {
		fileMap[v.File] = append(fileMap[v.File], v)
	}

	// Sort files for consistent output
	var files []string
	for file := range fileMap {
		files = append(files, file)
	}
	sort.Strings(files)

	// Create file nodes
	var fileNodes []*ViolationFileNode
	for _, file := range files {
		fileViolations := fileMap[file]
		fileNode := NewViolationFileNode(file, fileViolations)
		fileNodes = append(fileNodes, fileNode)
	}

	return &ViolationRootNode{
		fileNodes: fileNodes,
		total:     len(violations),
	}
}

// NewViolationFileNode creates a new file node
func NewViolationFileNode(path string, violations []Violation) *ViolationFileNode {
	// Group violations by source
	sourceMap := make(map[string][]Violation)
	for _, v := range violations {
		source := v.Source
		if source == "" {
			source = "arch-unit"
		}
		sourceMap[source] = append(sourceMap[source], v)
	}

	// Sort sources for consistent output
	var sources []string
	for source := range sourceMap {
		sources = append(sources, source)
	}
	sort.Strings(sources)

	// Create source nodes
	var sourceNodes []*ViolationSourceNode
	for _, source := range sources {
		sourceViolations := sourceMap[source]
		sourceNode := NewViolationSourceNode(source, sourceViolations)
		sourceNodes = append(sourceNodes, sourceNode)
	}

	return &ViolationFileNode{
		path:        path,
		sourceNodes: sourceNodes,
		total:       len(violations),
	}
}

// NewViolationSourceNode creates a new source node
func NewViolationSourceNode(source string, violations []Violation) *ViolationSourceNode {
	var violationNodes []*ViolationNode
	for _, v := range violations {
		violationNodes = append(violationNodes, &ViolationNode{violation: v})
	}

	return &ViolationSourceNode{
		source:         source,
		violationNodes: violationNodes,
	}
}

// Tree returns a TreeNode representation of this ViolationRootNode
func (v *ViolationRootNode) Tree() api.TreeNode {
	return &ViolationRootTreeNode{root: v}
}

// ViolationRootTreeNode is a TreeNode wrapper for ViolationRootNode
type ViolationRootTreeNode struct {
	root *ViolationRootNode
}

func (vrt *ViolationRootTreeNode) Pretty() api.Text {
	content := fmt.Sprintf("📋 Violations (%d)", vrt.root.total)
	return api.Text{
		Content: content,
		Style:   "text-red-600 font-bold",
	}
}

func (vrt *ViolationRootTreeNode) GetChildren() []api.TreeNode {
	nodes := make([]api.TreeNode, len(vrt.root.fileNodes))
	for i, fileNode := range vrt.root.fileNodes {
		nodes[i] = fileNode.Tree()
	}
	return nodes
}

// MarshalJSON implements JSON marshaling for ViolationRootTreeNode
func (vrt *ViolationRootTreeNode) MarshalJSON() ([]byte, error) {
	// Collect all violations from the tree in a flat structure for JSON
	var violations []Violation
	for _, fileNode := range vrt.root.fileNodes {
		for _, sourceNode := range fileNode.sourceNodes {
			for _, violationNode := range sourceNode.violationNodes {
				violations = append(violations, violationNode.violation)
			}
		}
	}
	
	return json.Marshal(map[string]interface{}{
		"total":      vrt.root.total,
		"violations": violations,
	})
}

// Tree returns a TreeNode representation of this ViolationFileNode
func (v *ViolationFileNode) Tree() api.TreeNode {
	return &ViolationFileTreeNode{file: v}
}

// ViolationFileTreeNode is a TreeNode wrapper for ViolationFileNode
type ViolationFileTreeNode struct {
	file *ViolationFileNode
}

func (vft *ViolationFileTreeNode) Pretty() api.Text {
	// Get relative path for display
	relPath := vft.file.path
	if cwd, err := filepath.Abs("."); err == nil {
		if rel, err := filepath.Rel(cwd, vft.file.path); err == nil && !strings.HasPrefix(rel, "../") {
			relPath = rel
		}
	}

	// Determine icon based on file extension
	var icon string
	switch {
	case strings.HasSuffix(vft.file.path, ".go"):
		icon = "🐹"
	case strings.HasSuffix(vft.file.path, ".py"):
		icon = "🐍"
	case strings.HasSuffix(vft.file.path, ".js"), strings.HasSuffix(vft.file.path, ".jsx"):
		icon = "📜"
	case strings.HasSuffix(vft.file.path, ".ts"), strings.HasSuffix(vft.file.path, ".tsx"):
		icon = "📘"
	case strings.HasSuffix(vft.file.path, ".md"):
		icon = "📝"
	default:
		icon = "📁"
	}

	content := fmt.Sprintf("%s %s (%d violations)", icon, relPath, vft.file.total)
	return api.Text{
		Content: content,
		Style:   "text-cyan-600 font-bold",
	}
}

func (vft *ViolationFileTreeNode) GetChildren() []api.TreeNode {
	nodes := make([]api.TreeNode, len(vft.file.sourceNodes))
	for i, sourceNode := range vft.file.sourceNodes {
		nodes[i] = sourceNode.Tree()
	}
	return nodes
}

// Tree returns a TreeNode representation of this ViolationSourceNode
func (v *ViolationSourceNode) Tree() api.TreeNode {
	return &ViolationSourceTreeNode{source: v}
}

// ViolationSourceTreeNode is a TreeNode wrapper for ViolationSourceNode
type ViolationSourceTreeNode struct {
	source *ViolationSourceNode
}

func (vst *ViolationSourceTreeNode) Pretty() api.Text {
	// Determine icon based on source
	var icon string
	switch vst.source.source {
	case "arch-unit":
		icon = "🏛️"
	case "golangci-lint":
		icon = "🔍"
	case "eslint":
		icon = "⚡"
	case "pylint":
		icon = "🐍"
	default:
		icon = "🔧"
	}

	// Determine style based on source
	var style string
	switch vst.source.source {
	case "arch-unit":
		style = "text-purple-600"
	default:
		style = "text-yellow-600"
	}

	content := fmt.Sprintf("%s %s (%d)", icon, vst.source.source, len(vst.source.violationNodes))
	return api.Text{
		Content: content,
		Style:   style,
	}
}

func (vst *ViolationSourceTreeNode) GetChildren() []api.TreeNode {
	nodes := make([]api.TreeNode, len(vst.source.violationNodes))
	for i, violationNode := range vst.source.violationNodes {
		nodes[i] = violationNode.Tree()
	}
	return nodes
}

// BuildViolationTree creates a tree structure for violations that can be formatted with clicky.Format
func BuildViolationTree(violations []Violation) api.TreeNode {
	if len(violations) == 0 {
		return &ViolationRootTreeNode{
			root: &ViolationRootNode{
				fileNodes: []*ViolationFileNode{},
				total:     0,
			},
		}
	}
	
	return NewViolationRootNode(violations).Tree()
}