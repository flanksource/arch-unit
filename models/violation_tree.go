package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flanksource/clicky/api"
)

// ViolationRootNode represents the root node of the violations tree
type ViolationRootNode struct {
	directoryNodes []*ViolationDirectoryNode
	fileNodes      []*ViolationFileNode
	total          int
}

// ViolationDirectoryNode represents a directory containing files or subdirectories
type ViolationDirectoryNode struct {
	path           string
	name           string
	childDirs      []*ViolationDirectoryNode
	fileNodes      []*ViolationFileNode
	total          int
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

	// Build directory tree
	directoryNodes, rootFileNodes := buildDirectoryTree(files, fileMap)

	return &ViolationRootNode{
		directoryNodes: directoryNodes,
		fileNodes:      rootFileNodes,
		total:          len(violations),
	}
}

// buildDirectoryTree creates a directory tree structure from file paths
func buildDirectoryTree(files []string, fileMap map[string][]Violation) ([]*ViolationDirectoryNode, []*ViolationFileNode) {
	// Map to track directory nodes by their path
	dirMap := make(map[string]*ViolationDirectoryNode)
	var rootFileNodes []*ViolationFileNode

	// Get current working directory for relative path conversion
	cwd, err := os.Getwd()
	if err != nil {
		// If we can't get the working directory, proceed with original paths
		cwd = ""
	}

	// Process each file to build directory structure
	for _, file := range files {
		// Convert to relative path if possible
		relativeFile := file
		if cwd != "" {
			if rel, err := filepath.Rel(cwd, file); err == nil && !strings.HasPrefix(rel, "../") {
				relativeFile = rel
			}
		}

		dir := filepath.Dir(relativeFile)

		// If file is in current directory (dir == "."), add to root
		if dir == "." {
			fileNode := NewViolationFileNode(file, fileMap[file]) // Use original path for file node
			rootFileNodes = append(rootFileNodes, fileNode)
			continue
		}

		// Ensure all parent directories exist (using relative path for tree structure)
		createDirectoryPath(dir, dirMap)

		// Add file to its parent directory
		parentDir := dirMap[dir]
		fileNode := NewViolationFileNode(file, fileMap[file]) // Use original path for file node
		parentDir.fileNodes = append(parentDir.fileNodes, fileNode)
		parentDir.total += len(fileMap[file])
	}

	// Build hierarchy and find root directories
	var rootDirs []*ViolationDirectoryNode
	for path, dirNode := range dirMap {
		parentPath := filepath.Dir(path)
		if parentPath == "." || parentPath == path {
			// This is a root directory
			rootDirs = append(rootDirs, dirNode)
		} else if parentDir, exists := dirMap[parentPath]; exists {
			// Add as child to parent directory
			parentDir.childDirs = append(parentDir.childDirs, dirNode)
		}
	}

	// Calculate totals for directory nodes (bottom-up)
	for _, dirNode := range dirMap {
		calculateDirectoryTotal(dirNode)
	}

	// Sort directories and files
	sort.Slice(rootDirs, func(i, j int) bool {
		return rootDirs[i].name < rootDirs[j].name
	})

	for _, dirNode := range dirMap {
		sort.Slice(dirNode.childDirs, func(i, j int) bool {
			return dirNode.childDirs[i].name < dirNode.childDirs[j].name
		})
		sort.Slice(dirNode.fileNodes, func(i, j int) bool {
			return filepath.Base(dirNode.fileNodes[i].path) < filepath.Base(dirNode.fileNodes[j].path)
		})
	}

	return rootDirs, rootFileNodes
}

// createDirectoryPath ensures all directories in a path exist in dirMap
func createDirectoryPath(path string, dirMap map[string]*ViolationDirectoryNode) {
	if path == "." || dirMap[path] != nil {
		return
	}

	// Recursively create parent directories
	parentPath := filepath.Dir(path)
	if parentPath != "." && parentPath != path {
		createDirectoryPath(parentPath, dirMap)
	}

	// Create this directory node
	dirName := filepath.Base(path)
	dirMap[path] = &ViolationDirectoryNode{
		path:      path,
		name:      dirName,
		childDirs: []*ViolationDirectoryNode{},
		fileNodes: []*ViolationFileNode{},
		total:     0,
	}
}

// calculateDirectoryTotal calculates the total violations for a directory
func calculateDirectoryTotal(dirNode *ViolationDirectoryNode) {
	total := 0
	
	// Add violations from direct files
	for _, fileNode := range dirNode.fileNodes {
		total += fileNode.total
	}
	
	// Add violations from child directories (recursively)
	for _, childDir := range dirNode.childDirs {
		calculateDirectoryTotal(childDir)
		total += childDir.total
	}
	
	dirNode.total = total
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
	var nodes []api.TreeNode
	
	// Add directory nodes first
	for _, dirNode := range vrt.root.directoryNodes {
		nodes = append(nodes, dirNode.Tree())
	}
	
	// Add direct file nodes (files not in any directory)
	for _, fileNode := range vrt.root.fileNodes {
		nodes = append(nodes, fileNode.Tree())
	}
	
	return nodes
}

// MarshalJSON implements JSON marshaling for ViolationRootTreeNode
func (vrt *ViolationRootTreeNode) MarshalJSON() ([]byte, error) {
	// Collect all violations from the tree in a flat structure for JSON
	var violations []Violation
	
	// Collect from directory nodes
	for _, dirNode := range vrt.root.directoryNodes {
		violations = append(violations, vrt.collectViolationsFromDirectory(dirNode)...)
	}
	
	// Collect from direct file nodes
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

// collectViolationsFromDirectory recursively collects violations from a directory node
func (vrt *ViolationRootTreeNode) collectViolationsFromDirectory(dirNode *ViolationDirectoryNode) []Violation {
	var violations []Violation
	
	// Collect from child directories
	for _, childDir := range dirNode.childDirs {
		violations = append(violations, vrt.collectViolationsFromDirectory(childDir)...)
	}
	
	// Collect from files in this directory
	for _, fileNode := range dirNode.fileNodes {
		for _, sourceNode := range fileNode.sourceNodes {
			for _, violationNode := range sourceNode.violationNodes {
				violations = append(violations, violationNode.violation)
			}
		}
	}
	
	return violations
}

// Tree returns a TreeNode representation of this ViolationDirectoryNode
func (v *ViolationDirectoryNode) Tree() api.TreeNode {
	return &ViolationDirectoryTreeNode{directory: v}
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
	// In tree view, use basename only since directory structure is already shown
	fileName := filepath.Base(vft.file.path)

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

	content := fmt.Sprintf("%s %s (%d violations)", icon, fileName, vft.file.total)
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

// ViolationDirectoryTreeNode is a TreeNode wrapper for ViolationDirectoryNode
type ViolationDirectoryTreeNode struct {
	directory *ViolationDirectoryNode
}

func (vdt *ViolationDirectoryTreeNode) Pretty() api.Text {
	content := fmt.Sprintf("📁 %s (%d violations)", vdt.directory.name, vdt.directory.total)
	return api.Text{
		Content: content,
		Style:   "text-blue-600 font-bold",
	}
}

func (vdt *ViolationDirectoryTreeNode) GetChildren() []api.TreeNode {
	var nodes []api.TreeNode
	
	// Add subdirectories first
	for _, childDir := range vdt.directory.childDirs {
		nodes = append(nodes, childDir.Tree())
	}
	
	// Add files
	for _, fileNode := range vdt.directory.fileNodes {
		nodes = append(nodes, fileNode.Tree())
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