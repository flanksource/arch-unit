package ast

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

// Formatter handles formatting of AST query results (legacy)
type Formatter struct {
	format     string
	template   string
	noColor    bool
	workingDir string
}

// FormatOverview formats cache overview statistics
func (f *Formatter) FormatOverview(overview *Overview) (string, error) {
	if f.format == "json" {
		data, err := json.MarshalIndent(overview, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString("=== AST Cache Overview ===\n")
	sb.WriteString(fmt.Sprintf("Directory: %s\n", overview.Directory))
	sb.WriteString(fmt.Sprintf("Total: %d\n\n", overview.Total))
	for key, value := range overview.Stats {
		sb.WriteString(fmt.Sprintf("%s: %d\n", key, value))
	}
	return sb.String(), nil
}

// FormatCacheStats formats cache statistics
func (f *Formatter) FormatCacheStats(stats *CacheStats) (string, error) {
	if f.format == "json" {
		data, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString("=== Cache Statistics ===\n")
	sb.WriteString(fmt.Sprintf("Total Files: %d\n", stats.TotalFiles))
	sb.WriteString(fmt.Sprintf("Cached Files: %d\n", stats.CachedFiles))
	sb.WriteString(fmt.Sprintf("Total Nodes: %d\n", stats.TotalNodes))
	sb.WriteString(fmt.Sprintf("Last Updated: %s\n", stats.LastUpdated))
	return sb.String(), nil
}

// NewFormatter creates a new formatter (legacy)
func NewFormatter(format, template string, noColor bool, workingDir string) *Formatter {
	return &Formatter{
		format:     format,
		template:   template,
		noColor:    noColor,
		workingDir: workingDir,
	}
}

// FormatNodes formats AST nodes for display
func (f *Formatter) FormatNodes(nodes []*models.ASTNode, query string) (string, error) {
	if len(nodes) == 0 {
		return "No results found", nil
	}

	switch f.format {
	case "json":
		data, err := json.MarshalIndent(nodes, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil

	case "table":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d nodes:\n\n", len(nodes)))
		sb.WriteString(fmt.Sprintf("%-30s %-20s %-30s %-10s %-10s\n", "File", "Type", "Name", "Lines", "Complexity"))
		sb.WriteString(strings.Repeat("-", 100) + "\n")

		for _, node := range nodes {
			name := f.getNodeName(node)
			file := node.FilePath
			if idx := strings.LastIndex(file, "/"); idx >= 0 && idx < len(file)-1 {
				file = file[idx+1:]
			}
			
			sb.WriteString(fmt.Sprintf("%-30s %-20s %-30s %-10d %-10d\n",
				file,
				node.NodeType,
				name,
				node.LineCount,
				node.CyclomaticComplexity))
		}
		return sb.String(), nil

	default:
		// Simple list format
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d nodes:\n", len(nodes)))
		for _, node := range nodes {
			name := f.getNodeName(node)
			sb.WriteString(fmt.Sprintf("  %s:%d - %s (%s)\n",
				node.FilePath,
				node.StartLine,
				name,
				node.NodeType))
		}
		return sb.String(), nil
	}
}

// FormatRelationships formats AST relationships for display
func (f *Formatter) FormatRelationships(rels []*models.ASTRelationship) (string, error) {
	if len(rels) == 0 {
		return "No relationships found", nil
	}

	switch f.format {
	case "json":
		data, err := json.MarshalIndent(rels, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil

	default:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d relationships:\n", len(rels)))
		for _, rel := range rels {
			sb.WriteString(fmt.Sprintf("  [%d] --%s--> ", rel.FromASTID, rel.RelationshipType))
			if rel.ToASTID != nil {
				sb.WriteString(fmt.Sprintf("[%d]", *rel.ToASTID))
			} else {
				sb.WriteString("external")
			}
			sb.WriteString(fmt.Sprintf(" (%s)\n", rel.Text))
		}
		return sb.String(), nil
	}
}

// getNodeName returns the display name for a node
func (f *Formatter) getNodeName(node *models.ASTNode) string {
	switch node.NodeType {
	case models.NodeTypeMethod:
		if node.TypeName != "" {
			return fmt.Sprintf("%s.%s", node.TypeName, node.MethodName)
		}
		return node.MethodName
	case models.NodeTypeType:
		return node.TypeName
	case models.NodeTypeField:
		if node.TypeName != "" {
			return fmt.Sprintf("%s.%s", node.TypeName, node.FieldName)
		}
		return node.FieldName
	case models.NodeTypeVariable:
		return node.FieldName
	default:
		return node.PackageName
	}
}

// FormatOverview formats cache overview statistics
func (f *Formatter) FormatOverview(overview map[string]interface{}) (string, error) {
	if f.format == "json" {
		data, err := json.MarshalIndent(overview, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString("=== AST Cache Overview ===\n")
	if files, ok := overview["total_files"].(int); ok {
		sb.WriteString(fmt.Sprintf("Total Files: %d\n", files))
	}
	if nodes, ok := overview["total_nodes"].(int); ok {
		sb.WriteString(fmt.Sprintf("Total Nodes: %d\n", nodes))
	}
	if methods, ok := overview["total_methods"].(int); ok {
		sb.WriteString(fmt.Sprintf("Total Methods: %d\n", methods))
	}
	if types, ok := overview["total_types"].(int); ok {
		sb.WriteString(fmt.Sprintf("Total Types: %d\n", types))
	}
	return sb.String(), nil
}

// FormatCacheStats formats cache statistics
func (f *Formatter) FormatCacheStats(stats map[string]interface{}) (string, error) {
	if f.format == "json" {
		data, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	var sb strings.Builder
	sb.WriteString("=== Cache Statistics ===\n")
	for key, value := range stats {
		sb.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}
	return sb.String(), nil
}