package ast

import (
	"fmt"

	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/api"
)

// RenderComplexity renders cyclomatic complexity with color coding
func RenderComplexity(value interface{}, field api.PrettyField, theme api.Theme) api.Text {
	var complexity int
	switch v := value.(type) {
	case int:
		complexity = v
	case int64:
		complexity = int(v)
	case float64:
		complexity = int(v)
	default:
		return clicky.Textf("%v", value)
	}

	// Color based on complexity thresholds
	var style string
	switch {
	case complexity > 15:
		style = "text-red-600 font-bold"
	case complexity > 10:
		style = "text-red-600"
	case complexity > 5:
		style = "text-yellow-600"
	case complexity > 0:
		style = "text-green-600"
	default:
		style = "text-gray-400"
	}

	text := clicky.Textf("%d", complexity)
	text.Style = style
	return text
}

// // RenderMembers renders a list of members in compact format
// func RenderMembers(value interface{}, field api.PrettyField, theme api.Theme) api.Text {
// 	switch v := value.(type) {
// 	case []MemberInfo:
// 		if len(v) == 0 {
// 			return clicky.Text("")
// 		}

// 		parts := make([]api.Text, 0, len(v)*2) // Pre-allocate for members + separators
// 		for i, member := range v {
// 			if i > 0 {
// 				parts = append(parts, clicky.Text(", "))
// 			}

// 			// Add member name and line
// 			memberText := clicky.Textf("%s:%d", member.Name, member.Line)
// 			parts = append(parts, memberText)

// 			// Add complexity with appropriate styling
// 			if member.Complexity > 0 {
// 				complexityStyle := ""
// 				switch {
// 				case member.Complexity > 10:
// 					complexityStyle = "text-red-600"
// 				case member.Complexity > 5:
// 					complexityStyle = "text-yellow-600"
// 				default:
// 					complexityStyle = "text-green-600"
// 				}

// 				complexityText := clicky.Textf("(c:%d)", member.Complexity)
// 				complexityText.Style = complexityStyle
// 				parts = append(parts, complexityText)
// 			}
// 		}

// 		// Combine all parts
// 		result := parts[0]
// 		for _, part := range parts[1:] {
// 			result = result.Append(part)
// 		}
// 		return result

// 	case []interface{}:
// 		var parts []api.Text
// 		for i, item := range v {
// 			if i > 0 {
// 				parts = append(parts, clicky.Text(", "))
// 			}

// 			if m, ok := item.(map[string]interface{}); ok {
// 				name := fmt.Sprintf("%v", m["name"])
// 				line := 0
// 				if l, ok := m["line"].(int); ok {
// 					line = l
// 				}
// 				complexity := 0
// 				if c, ok := m["complexity"].(int); ok {
// 					complexity = c
// 				}

// 				memberText := clicky.Textf("%s:%d", name, line)
// 				parts = append(parts, memberText)

// 				if complexity > 0 {
// 					complexityStyle := ""
// 					switch {
// 					case complexity > 10:
// 						complexityStyle = "text-red-600"
// 					case complexity > 5:
// 						complexityStyle = "text-yellow-600"
// 					default:
// 						complexityStyle = "text-green-600"
// 					}

// 					complexityText := clicky.Textf("(c:%d)", complexity)
// 					complexityText.Style = complexityStyle
// 					parts = append(parts, complexityText)
// 				}
// 			} else {
// 				parts = append(parts, clicky.Textf("%v", item))
// 			}
// 		}

// 		if len(parts) == 0 {
// 			return clicky.Text("")
// 		}

// 		result := parts[0]
// 		for _, part := range parts[1:] {
// 			result = result.Append(part)
// 		}
// 		return result

// 	default:
// 		return clicky.Textf("%v", value)
// 	}
// }

// RenderFixtureStatus renders fixture test status with color coding and icons
func RenderFixtureStatus(value interface{}, field api.PrettyField, theme api.Theme) api.Text {
	status, ok := value.(string)
	if !ok {
		return clicky.Textf("%v", value)
	}

	var styleClass string
	var icon string

	switch status {
	case "PASS":
		styleClass = "text-green-600 font-bold"
		icon = "✅ "
	case "FAIL":
		styleClass = "text-red-600 font-bold"
		icon = "❌ "
	case "SKIP":
		styleClass = "text-yellow-600 font-bold"
		icon = "⏭️ "
	default:
		styleClass = "text-gray-400"
		icon = "🔍 "
	}

	return clicky.Text(icon).Append(status, styleClass)
}

// RenderRelationship renders a call relationship
func RenderRelationship(value interface{}, field api.PrettyField, theme api.Theme) api.Text {
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

		return clicky.Text(arrow+" ").Append(target, "text-blue-600").Printf(" (line %d)", line)

	case string:
		return clicky.Text(v)

	default:
		return clicky.Textf("%v", value)
	}
}

// RenderLibrary renders an external library dependency
func RenderLibrary(value interface{}, field api.PrettyField, theme api.Theme) api.Text {
	switch v := value.(type) {
	case map[string]interface{}:
		lib := fmt.Sprintf("%v", v["library"])
		count := 0
		if c, ok := v["count"].(int); ok {
			count = c
		}

		// Style library name
		libText := clicky.Text(lib)
		libText.Style = "text-purple-600 italic"

		if count > 0 {
			return libText.PrintfWithStyle(" (%d calls)", "text-gray-400", count)
		}

		return libText

	case string:
		return clicky.Text(v, "text-purple-600 italic")

	default:
		return clicky.Textf("%v", value)
	}
}
