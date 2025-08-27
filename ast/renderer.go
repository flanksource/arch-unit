package ast

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/flanksource/clicky/api"
)

func init() {
	// Register AST-specific renderers
	api.RegisterRenderFunc("ast_complexity", RenderComplexity)
	api.RegisterRenderFunc("ast_members", RenderMembers)
	api.RegisterRenderFunc("ast_relationship", RenderRelationship)
	api.RegisterRenderFunc("ast_library", RenderLibrary)
	api.RegisterRenderFunc("fixture_status", RenderFixtureStatus)
}

// RenderComplexity renders cyclomatic complexity with color coding
func RenderComplexity(value interface{}, field api.PrettyField, theme api.Theme) string {
	var complexity int
	switch v := value.(type) {
	case int:
		complexity = v
	case int64:
		complexity = int(v)
	case float64:
		complexity = int(v)
	default:
		return fmt.Sprintf("%v", value)
	}
	
	// Color based on complexity thresholds
	style := lipgloss.NewStyle()
	switch {
	case complexity > 15:
		style = style.Foreground(theme.Error).Bold(true)
	case complexity > 10:
		style = style.Foreground(theme.Error)
	case complexity > 5:
		style = style.Foreground(theme.Warning)
	case complexity > 0:
		style = style.Foreground(theme.Success)
	default:
		style = style.Foreground(theme.Muted)
	}
	
	return style.Render(fmt.Sprintf("%d", complexity))
}

// RenderMembers renders a list of members in compact format
func RenderMembers(value interface{}, field api.PrettyField, theme api.Theme) string {
	switch v := value.(type) {
	case []MemberInfo:
		if len(v) == 0 {
			return ""
		}
		
		var items []string
		for _, member := range v {
			item := fmt.Sprintf("%s:%d", member.Name, member.Line)
			if member.Complexity > 0 {
				// Color the complexity part
				complexityStr := fmt.Sprintf("(c:%d)", member.Complexity)
				style := lipgloss.NewStyle()
				if member.Complexity > 10 {
					style = style.Foreground(theme.Error)
				} else if member.Complexity > 5 {
					style = style.Foreground(theme.Warning)
				} else {
					style = style.Foreground(theme.Success)
				}
				item += style.Render(complexityStr)
			}
			items = append(items, item)
		}
		
		// Join with comma and space
		result := ""
		for i, item := range items {
			if i > 0 {
				result += ", "
			}
			result += item
		}
		return result
		
	case []interface{}:
		// Handle generic slice
		var items []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				name := fmt.Sprintf("%v", m["name"])
				line := 0
				if l, ok := m["line"].(int); ok {
					line = l
				}
				complexity := 0
				if c, ok := m["complexity"].(int); ok {
					complexity = c
				}
				
				itemStr := fmt.Sprintf("%s:%d", name, line)
				if complexity > 0 {
					itemStr += fmt.Sprintf("(c:%d)", complexity)
				}
				items = append(items, itemStr)
			} else {
				items = append(items, fmt.Sprintf("%v", item))
			}
		}
		
		result := ""
		for i, item := range items {
			if i > 0 {
				result += ", "
			}
			result += item
		}
		return result
		
	default:
		return fmt.Sprintf("%v", value)
	}
}

// RenderFixtureStatus renders fixture test status with color coding and icons
func RenderFixtureStatus(value interface{}, field api.PrettyField, theme api.Theme) string {
	status, ok := value.(string)
	if !ok {
		return fmt.Sprintf("%v", value)
	}
	
	var style lipgloss.Style
	var icon string
	
	switch status {
	case "PASS":
		style = lipgloss.NewStyle().Foreground(theme.Success).Bold(true)
		icon = "✅ "
	case "FAIL":
		style = lipgloss.NewStyle().Foreground(theme.Error).Bold(true)
		icon = "❌ "
	case "SKIP":
		style = lipgloss.NewStyle().Foreground(theme.Warning).Bold(true)
		icon = "⏭️ "
	default:
		style = lipgloss.NewStyle().Foreground(theme.Muted)
		icon = "🔍 "
	}
	
	return icon + style.Render(status)
}

// RenderRelationship renders a call relationship
func RenderRelationship(value interface{}, field api.PrettyField, theme api.Theme) string {
	switch v := value.(type) {
	case map[string]interface{}:
		target := fmt.Sprintf("%v", v["target"])
		line := 0
		if l, ok := v["line"].(int); ok {
			line = l
		}
		relType := "calls"
		if t, ok := v["type"].(string); ok {
			relType = t
		}
		
		// Format with arrow and line number
		arrow := "→"
		if relType == "implements" {
			arrow = "⇒"
		} else if relType == "inherits" {
			arrow = "↗"
		}
		
		style := lipgloss.NewStyle().Foreground(theme.Info)
		return fmt.Sprintf("%s %s (line %d)", arrow, style.Render(target), line)
		
	case string:
		return v
		
	default:
		return fmt.Sprintf("%v", value)
	}
}

// RenderLibrary renders an external library dependency
func RenderLibrary(value interface{}, field api.PrettyField, theme api.Theme) string {
	switch v := value.(type) {
	case map[string]interface{}:
		lib := fmt.Sprintf("%v", v["library"])
		count := 0
		if c, ok := v["count"].(int); ok {
			count = c
		}
		
		// Style library name
		style := lipgloss.NewStyle().Foreground(theme.Secondary).Italic(true)
		result := style.Render(lib)
		
		if count > 0 {
			countStyle := lipgloss.NewStyle().Foreground(theme.Muted)
			result += countStyle.Render(fmt.Sprintf(" (%d calls)", count))
		}
		
		return result
		
	case string:
		style := lipgloss.NewStyle().Foreground(theme.Secondary).Italic(true)
		return style.Render(v)
		
	default:
		return fmt.Sprintf("%v", value)
	}
}

