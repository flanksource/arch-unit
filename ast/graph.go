package ast

import (
	"fmt"
	"sort"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

// CallGraph represents a call graph structure
type CallGraph struct {
	Nodes         []*models.ASTNode            `json:"nodes"`
	Relationships []*models.ASTRelationship    `json:"relationships"`
	LibraryRels   []*models.LibraryRelationship `json:"library_relationships,omitempty"`
	RootNodes     []*models.ASTNode            `json:"root_nodes"` // Entry points
}

// GraphNode represents a node in the call graph for visualization
type GraphNode struct {
	Node         *models.ASTNode               `json:"node"`
	Callers      []*GraphNode                  `json:"callers,omitempty"`
	Callees      []*GraphNode                  `json:"callees,omitempty"`
	LibraryCalls []*models.LibraryRelationship `json:"library_calls,omitempty"`
	Depth        int                           `json:"depth"`
	Visited      bool                          `json:"-"` // For traversal
}

// GraphBuilder builds call graphs from AST relationships
type GraphBuilder struct {
	cache map[int64]*GraphNode // Cache for graph nodes by AST ID
}

// NewGraphBuilder creates a new graph builder
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{
		cache: make(map[int64]*GraphNode),
	}
}

// BuildCallGraph builds a call graph from the given AST nodes
func (gb *GraphBuilder) BuildCallGraph(nodes []*models.ASTNode, relationships []*models.ASTRelationship, libraryRels []*models.LibraryRelationship) *CallGraph {
	// Create graph nodes for all AST nodes
	for _, node := range nodes {
		gb.getOrCreateGraphNode(node)
	}

	// Build relationships
	gb.buildRelationships(relationships, libraryRels)

	// Find root nodes (nodes with no callers or minimal callers)
	rootNodes := gb.findRootNodes()

	return &CallGraph{
		Nodes:         nodes,
		Relationships: relationships,
		LibraryRels:   libraryRels,
		RootNodes:     rootNodes,
	}
}

// BuildCallGraphFromRoots builds a call graph starting from specific root nodes with depth limit
func (gb *GraphBuilder) BuildCallGraphFromRoots(rootNodes []*models.ASTNode, allRelationships []*models.ASTRelationship, libraryRels []*models.LibraryRelationship, maxDepth int) *CallGraph {
	// Initialize root graph nodes
	var roots []*models.ASTNode
	for _, node := range rootNodes {
		graphNode := gb.getOrCreateGraphNode(node)
		gb.buildSubgraphFromNode(graphNode, allRelationships, libraryRels, 0, maxDepth)
		roots = append(roots, node)
	}

	// Collect all nodes that were visited
	var visitedNodes []*models.ASTNode
	var usedRelationships []*models.ASTRelationship
	var usedLibraryRels []*models.LibraryRelationship

	for _, graphNode := range gb.cache {
		if graphNode.Visited {
			visitedNodes = append(visitedNodes, graphNode.Node)
		}
	}

	// Filter relationships to only include those between visited nodes
	for _, rel := range allRelationships {
		if gb.cache[rel.FromASTID] != nil && gb.cache[rel.FromASTID].Visited &&
			rel.ToASTID != nil && gb.cache[*rel.ToASTID] != nil && gb.cache[*rel.ToASTID].Visited {
			usedRelationships = append(usedRelationships, rel)
		}
	}

	// Filter library relationships
	for _, rel := range libraryRels {
		if gb.cache[rel.ASTID] != nil && gb.cache[rel.ASTID].Visited {
			usedLibraryRels = append(usedLibraryRels, rel)
		}
	}

	return &CallGraph{
		Nodes:         visitedNodes,
		Relationships: usedRelationships,
		LibraryRels:   usedLibraryRels,
		RootNodes:     roots,
	}
}

// getOrCreateGraphNode gets or creates a graph node for an AST node
func (gb *GraphBuilder) getOrCreateGraphNode(astNode *models.ASTNode) *GraphNode {
	if existing, exists := gb.cache[astNode.ID]; exists {
		return existing
	}

	graphNode := &GraphNode{
		Node:         astNode,
		Callers:      make([]*GraphNode, 0),
		Callees:      make([]*GraphNode, 0),
		LibraryCalls: make([]*models.LibraryRelationship, 0),
	}

	gb.cache[astNode.ID] = graphNode
	return graphNode
}

// buildRelationships builds the graph relationships
func (gb *GraphBuilder) buildRelationships(relationships []*models.ASTRelationship, libraryRels []*models.LibraryRelationship) {
	// Build AST relationships
	for _, rel := range relationships {
		if rel.RelationshipType != models.RelationshipCall {
			continue // Only process call relationships for now
		}

		fromNode, exists := gb.cache[rel.FromASTID]
		if !exists {
			continue
		}

		if rel.ToASTID != nil {
			toNode, exists := gb.cache[*rel.ToASTID]
			if exists {
				fromNode.Callees = append(fromNode.Callees, toNode)
				toNode.Callers = append(toNode.Callers, fromNode)
			}
		}
	}

	// Build library relationships
	for _, rel := range libraryRels {
		if rel.RelationshipType != models.RelationshipCall {
			continue
		}

		fromNode, exists := gb.cache[rel.ASTID]
		if exists {
			fromNode.LibraryCalls = append(fromNode.LibraryCalls, rel)
		}
	}
}

// buildSubgraphFromNode builds subgraph from a node with depth limit
func (gb *GraphBuilder) buildSubgraphFromNode(node *GraphNode, allRelationships []*models.ASTRelationship, libraryRels []*models.LibraryRelationship, currentDepth, maxDepth int) {
	if currentDepth > maxDepth || node.Visited {
		return
	}

	node.Visited = true
	node.Depth = currentDepth

	// Find relationships where this node is the caller
	for _, rel := range allRelationships {
		if rel.RelationshipType != models.RelationshipCall {
			continue
		}

		if rel.FromASTID == node.Node.ID && rel.ToASTID != nil {
			// Find the target node - we need to create it if it doesn't exist
			var targetNode *GraphNode
			if existing, exists := gb.cache[*rel.ToASTID]; exists {
				targetNode = existing
			} else {
				// We need the AST node data - this is a limitation of this approach
				// In a full implementation, we'd need access to all AST nodes
				continue
			}

			// Add relationship
			node.Callees = append(node.Callees, targetNode)
			targetNode.Callers = append(targetNode.Callers, node)

			// Recursively build subgraph
			gb.buildSubgraphFromNode(targetNode, allRelationships, libraryRels, currentDepth+1, maxDepth)
		}
	}

	// Add library calls
	for _, rel := range libraryRels {
		if rel.RelationshipType == models.RelationshipCall && rel.ASTID == node.Node.ID {
			node.LibraryCalls = append(node.LibraryCalls, rel)
		}
	}
}

// findRootNodes identifies root nodes (entry points) in the graph
func (gb *GraphBuilder) findRootNodes() []*models.ASTNode {
	var roots []*models.ASTNode

	for _, graphNode := range gb.cache {
		// A root node has no callers or very few callers
		if len(graphNode.Callers) == 0 {
			roots = append(roots, graphNode.Node)
		}
	}

	// Sort by node name for consistent output
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].GetFullName() < roots[j].GetFullName()
	})

	return roots
}

// FormatCallGraph formats a call graph for display
func (gb *GraphBuilder) FormatCallGraph(graph *CallGraph, format string, maxDepth int) (string, error) {
	switch format {
	case "dot":
		return gb.formatAsDot(graph)
	case "json":
		return gb.formatAsJSON(graph)
	case "tree":
		return gb.formatAsTree(graph, maxDepth)
	default:
		return gb.formatAsTree(graph, maxDepth)
	}
}

// formatAsTree formats the call graph as a tree structure
func (gb *GraphBuilder) formatAsTree(graph *CallGraph, maxDepth int) (string, error) {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("📊 Call Graph (%d nodes, %d relationships)\n", 
		len(graph.Nodes), len(graph.Relationships)))
	result.WriteString("═══════════════════════════════════════════════════════════════\n\n")

	// Show each root node and its call tree
	for i, rootNode := range graph.RootNodes {
		if i > 0 {
			result.WriteString("\n")
		}

		graphNode := gb.cache[rootNode.ID]
		if graphNode != nil {
			gb.formatNodeTree(graphNode, &result, "", true, maxDepth, 0, make(map[int64]bool))
		}
	}

	return result.String(), nil
}

// formatNodeTree recursively formats a node and its callees as a tree
func (gb *GraphBuilder) formatNodeTree(node *GraphNode, result *strings.Builder, prefix string, isLast bool, maxDepth, currentDepth int, visited map[int64]bool) {
	if currentDepth > maxDepth {
		return
	}

	// Prevent infinite recursion
	if visited[node.Node.ID] {
		nodeStr := fmt.Sprintf("%s%s %s (circular reference)\n",
			prefix, gb.getTreeSymbol(isLast), node.Node.GetFullName())
		result.WriteString(nodeStr)
		return
	}

	visited[node.Node.ID] = true
	defer delete(visited, node.Node.ID) // Clean up for other paths

	// Format the current node
	complexity := ""
	if node.Node.CyclomaticComplexity > 0 {
		complexity = fmt.Sprintf(" (complexity: %d)", node.Node.CyclomaticComplexity)
	}

	nodeStr := fmt.Sprintf("%s%s %s%s\n",
		prefix, gb.getTreeSymbol(isLast), node.Node.GetFullName(), complexity)
	result.WriteString(nodeStr)

	// Prepare prefix for children
	var childPrefix string
	if isLast {
		childPrefix = prefix + "    "
	} else {
		childPrefix = prefix + "│   "
	}

	// Show library calls first
	for i, libCall := range node.LibraryCalls {
		isLastLibCall := i == len(node.LibraryCalls)-1 && len(node.Callees) == 0
		libStr := fmt.Sprintf("%s%s 📚 %s.%s\n",
			childPrefix, gb.getTreeSymbol(isLastLibCall), 
			libCall.LibraryNode.Package, libCall.LibraryNode.Method)
		result.WriteString(libStr)
	}

	// Show callees
	for i, callee := range node.Callees {
		isLastCallee := i == len(node.Callees)-1
		gb.formatNodeTree(callee, result, childPrefix, isLastCallee, maxDepth, currentDepth+1, visited)
	}
}

// getTreeSymbol returns the appropriate tree symbol for the position
func (gb *GraphBuilder) getTreeSymbol(isLast bool) string {
	if isLast {
		return "└── "
	}
	return "├── "
}

// formatAsDot formats the call graph as DOT notation for Graphviz
func (gb *GraphBuilder) formatAsDot(graph *CallGraph) (string, error) {
	var result strings.Builder

	result.WriteString("digraph CallGraph {\n")
	result.WriteString("    rankdir=TB;\n")
	result.WriteString("    node [shape=box, style=rounded];\n\n")

	// Add nodes
	for _, node := range graph.Nodes {
		label := strings.ReplaceAll(node.GetFullName(), "\"", "\\\"")
		result.WriteString(fmt.Sprintf("    \"n%d\" [label=\"%s\"];\n", node.ID, label))
	}

	result.WriteString("\n")

	// Add edges
	for _, rel := range graph.Relationships {
		if rel.ToASTID != nil {
			result.WriteString(fmt.Sprintf("    \"n%d\" -> \"n%d\";\n", rel.FromASTID, *rel.ToASTID))
		}
	}

	// Add library calls as external nodes
	libraryNodes := make(map[string]bool)
	for _, rel := range graph.LibraryRels {
		if rel.LibraryNode != nil {
			libName := rel.LibraryNode.GetFullName()
			if !libraryNodes[libName] {
				label := strings.ReplaceAll(libName, "\"", "\\\"")
				result.WriteString(fmt.Sprintf("    \"lib_%s\" [label=\"%s\", style=\"filled,rounded\", fillcolor=lightblue];\n", 
					libName, label))
				libraryNodes[libName] = true
			}
			result.WriteString(fmt.Sprintf("    \"n%d\" -> \"lib_%s\";\n", rel.ASTID, libName))
		}
	}

	result.WriteString("}\n")
	return result.String(), nil
}

// formatAsJSON formats the call graph as JSON
func (gb *GraphBuilder) formatAsJSON(graph *CallGraph) (string, error) {
	// This would use JSON marshaling - simplified for now
	return fmt.Sprintf(`{
		"nodes": %d,
		"relationships": %d,
		"library_relationships": %d,
		"root_nodes": %d
	}`, len(graph.Nodes), len(graph.Relationships), len(graph.LibraryRels), len(graph.RootNodes)), nil
}