package output

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/flanksource/arch-unit/models"
)

type OutputManager struct {
	format  string
	output  string
	compact bool
}

func NewOutputManager(format string) *OutputManager {
	return &OutputManager{
		format: format,
	}
}

func (o *OutputManager) SetOutputFile(file string) {
	o.output = file
}

func (o *OutputManager) SetCompact(compact bool) {
	o.compact = compact
}

func (o *OutputManager) Output(result *models.AnalysisResult) error {
	switch o.format {
	case "json":
		return o.outputJSON(result)
	case "csv":
		return o.outputCSV(result)
	case "html":
		return o.outputHTML(result)
	case "excel":
		return o.outputExcel(result)
	case "markdown":
		return o.outputMarkdown(result)
	default:
		return o.outputTable(result)
	}
}

func (o *OutputManager) outputTable(result *models.AnalysisResult) error {
	if len(result.Violations) == 0 {
		return nil
	}

	// Group violations by file
	if o.compact {
		o.outputCompact(result)
	} else {
		o.outputTree(result)
	}
	return nil
}

func (o *OutputManager) outputCompact(result *models.AnalysisResult) {
	// Define styles
	fileStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	// Group violations by file
	fileMap := make(map[string][]models.Violation)
	for _, v := range result.Violations {
		fileMap[v.File] = append(fileMap[v.File], v)
	}

	// Sort files for consistent output
	var files []string
	for file := range fileMap {
		files = append(files, file)
	}
	sort.Strings(files)

	fmt.Println("\n📋 Architecture Violations (Compact)")
	fmt.Println(strings.Repeat("─", 80))

	for _, file := range files {
		violations := fileMap[file]
		relPath := getRelativePath(file)

		// Count violations by rule
		ruleCount := make(map[string]int)
		for _, v := range violations {
			ruleStr := ""
			if v.Rule != nil {
				ruleStr = v.Rule.String()
			}
			ruleCount[ruleStr]++
		}

		// Build rule summary
		var ruleSummary []string
		for rule, count := range ruleCount {
			if count > 1 {
				ruleSummary = append(ruleSummary, fmt.Sprintf("%s×%d", rule, count))
			} else {
				ruleSummary = append(ruleSummary, rule)
			}
		}
		sort.Strings(ruleSummary)

		fmt.Printf("  %s %s %s\n",
			fileStyle.Render(relPath),
			countStyle.Render(fmt.Sprintf("(%d)", len(violations))),
			ruleStyle.Render(strings.Join(ruleSummary, ", ")))
	}

	fmt.Println(strings.Repeat("─", 80))
}

func (o *OutputManager) outputTree(result *models.AnalysisResult) {
	// Define styles
	fileStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	violationStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	// Group violations by file
	fileMap := make(map[string][]models.Violation)
	for _, v := range result.Violations {
		fileMap[v.File] = append(fileMap[v.File], v)
	}

	// Sort files for consistent output
	var files []string
	for file := range fileMap {
		files = append(files, file)
	}
	sort.Strings(files)

	fmt.Println("\n📋 Architecture Violations")
	fmt.Println(strings.Repeat("─", 80))

	for i, file := range files {
		violations := fileMap[file]

		// File header
		relPath := getRelativePath(file)
		isLast := i == len(files)-1

		if isLast {
			fmt.Printf("└── %s (%d violations)\n", fileStyle.Render(relPath), len(violations))
		} else {
			fmt.Printf("├── %s (%d violations)\n", fileStyle.Render(relPath), len(violations))
		}

		// Group violations by rule type
		ruleMap := make(map[string][]models.Violation)
		for _, v := range violations {
			ruleStr := ""
			if v.Rule != nil {
				ruleStr = v.Rule.String()
			}
			ruleMap[ruleStr] = append(ruleMap[ruleStr], v)
		}

		// Sort rules for consistent output
		var rules []string
		for rule := range ruleMap {
			rules = append(rules, rule)
		}
		sort.Strings(rules)

		prefix := "│   "
		if isLast {
			prefix = "    "
		}

		for j, rule := range rules {
			ruleViolations := ruleMap[rule]
			isLastRule := j == len(rules)-1

			// Rule header
			if isLastRule {
				fmt.Printf("%s└── %s\n", prefix, ruleStyle.Render(rule))
			} else {
				fmt.Printf("%s├── %s\n", prefix, ruleStyle.Render(rule))
			}

			rulePrefix := prefix + "│   "
			if isLastRule {
				rulePrefix = prefix + "    "
			}

			// Show violations for this rule
			for k, v := range ruleViolations {
				isLastViolation := k == len(ruleViolations)-1

				call := v.CalledPackage
				if v.CalledMethod != "" {
					call = fmt.Sprintf("%s.%s", v.CalledPackage, v.CalledMethod)
				}

				lineInfo := fmt.Sprintf("line %d", v.Line)

				if isLastViolation {
					fmt.Printf("%s└── %s %s\n",
						rulePrefix,
						violationStyle.Render(call),
						lineStyle.Render("("+lineInfo+")"))
				} else {
					fmt.Printf("%s├── %s %s\n",
						rulePrefix,
						violationStyle.Render(call),
						lineStyle.Render("("+lineInfo+")"))
				}
			}
		}

		if !isLast {
			fmt.Println("│")
		}
	}

	fmt.Println(strings.Repeat("─", 80))
}

func getRelativePath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}

	relPath, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}

	if strings.HasPrefix(relPath, "../") {
		return path
	}

	return relPath
}

func (o *OutputManager) createBorder(widths []int, left, mid, right, fill string) string {
	var border strings.Builder
	border.WriteString(left)
	for i, width := range widths {
		border.WriteString(strings.Repeat(fill, width+2))
		if i < len(widths)-1 {
			border.WriteString(mid)
		}
	}
	border.WriteString(right)
	return border.String()
}

func (o *OutputManager) padString(s string, width int) string {
	if len(s) > width {
		return s[:width-3] + "..."
	}
	return s + strings.Repeat(" ", width-len(s))
}

func (o *OutputManager) outputJSON(result *models.AnalysisResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	output := map[string]interface{}{
		"summary": map[string]int{
			"files_analyzed": result.FileCount,
			"rules_applied":  result.RuleCount,
			"violations":     len(result.Violations),
		},
		"violations": result.Violations,
	}

	return encoder.Encode(output)
}

func (o *OutputManager) outputCSV(result *models.AnalysisResult) error {
	writer := os.Stdout
	if o.output != "" {
		file, err := os.Create(o.output)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}

	// Write header
	fmt.Fprintln(writer, "File,Line,Column,Caller,Called,Rule,RuleFile")

	// Write violations
	for _, v := range result.Violations {
		call := v.CalledPackage
		if v.CalledMethod != "" {
			call = fmt.Sprintf("%s.%s", v.CalledPackage, v.CalledMethod)
		}

		ruleStr := ""
		ruleFile := ""
		if v.Rule != nil {
			ruleStr = v.Rule.String()
			ruleFile = fmt.Sprintf("%s:%d", v.Rule.SourceFile, v.Rule.LineNumber)
		}

		fmt.Fprintf(writer, "%s,%d,%d,%s,%s,%s,%s\n",
			v.File, v.Line, v.Column, v.CallerMethod, call, ruleStr, ruleFile)
	}

	return nil
}

func (o *OutputManager) outputHTML(result *models.AnalysisResult) error {
	if o.output == "" {
		return fmt.Errorf("output file required for HTML format")
	}

	file, err := os.Create(o.output)
	if err != nil {
		return err
	}
	defer file.Close()

	html := `<!DOCTYPE html>
<html>
<head>
	<title>Architecture Violations Report</title>
	<style>
		body { font-family: Arial, sans-serif; margin: 20px; }
		h1 { color: #333; }
		.summary { background: #f0f0f0; padding: 10px; border-radius: 5px; margin-bottom: 20px; }
		table { border-collapse: collapse; width: 100%; }
		th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
		th { background-color: #f2f2f2; }
		tr:nth-child(even) { background-color: #f9f9f9; }
		.violation { color: #d9534f; }
		.no-violations { color: #5cb85c; font-size: 1.2em; padding: 20px; }
	</style>
</head>
<body>
	<h1>Architecture Violations Report</h1>
	<div class="summary">
		<p><strong>Files Analyzed:</strong> %d</p>
		<p><strong>Rules Applied:</strong> %d</p>
		<p><strong>Violations Found:</strong> <span class="violation">%d</span></p>
	</div>
`

	fmt.Fprintf(file, html, result.FileCount, result.RuleCount, len(result.Violations))

	if len(result.Violations) == 0 {
		fmt.Fprintln(file, `<div class="no-violations">✓ No architecture violations found!</div>`)
	} else {
		fmt.Fprintln(file, `<table>
		<thead>
			<tr>
				<th>File</th>
				<th>Line</th>
				<th>Caller</th>
				<th>Violation</th>
				<th>Rule</th>
				<th>Rule Source</th>
			</tr>
		</thead>
		<tbody>`)

		for _, v := range result.Violations {
			call := v.CalledPackage
			if v.CalledMethod != "" {
				call = fmt.Sprintf("%s.%s", v.CalledPackage, v.CalledMethod)
			}

			ruleStr := ""
			ruleFile := ""
			if v.Rule != nil {
				ruleStr = v.Rule.String()
				ruleFile = fmt.Sprintf("%s:%d", v.Rule.SourceFile, v.Rule.LineNumber)
			}

			fmt.Fprintf(file, `<tr>
				<td>%s:%d:%d</td>
				<td>%d</td>
				<td>%s</td>
				<td class="violation">%s</td>
				<td>%s</td>
				<td>%s</td>
			</tr>`, v.File, v.Line, v.Column, v.Line, v.CallerMethod, call, ruleStr, ruleFile)
		}

		fmt.Fprintln(file, `</tbody></table>`)
	}

	fmt.Fprintln(file, `</body></html>`)

	return nil
}

func (o *OutputManager) outputMarkdown(result *models.AnalysisResult) error {
	writer := os.Stdout
	if o.output != "" {
		file, err := os.Create(o.output)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}

	fmt.Fprintln(writer, "# Architecture Violations Report")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "## Summary")
	fmt.Fprintf(writer, "- **Files Analyzed:** %d\n", result.FileCount)
	fmt.Fprintf(writer, "- **Rules Applied:** %d\n", result.RuleCount)
	fmt.Fprintf(writer, "- **Violations Found:** %d\n", len(result.Violations))
	fmt.Fprintln(writer)

	if len(result.Violations) == 0 {
		fmt.Fprintln(writer, "✓ **No architecture violations found!**")
		return nil
	}

	fmt.Fprintln(writer, "## Violations")
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "| File | Line | Caller | Violation | Rule | Source |")
	fmt.Fprintln(writer, "|------|------|--------|-----------|------|--------|")

	for _, v := range result.Violations {
		call := v.CalledPackage
		if v.CalledMethod != "" {
			call = fmt.Sprintf("%s.%s", v.CalledPackage, v.CalledMethod)
		}

		ruleStr := ""
		ruleFile := ""
		if v.Rule != nil {
			ruleStr = strings.ReplaceAll(v.Rule.String(), "|", "\\|")
			ruleFile = fmt.Sprintf("%s:%d", v.Rule.SourceFile, v.Rule.LineNumber)
		}

		fmt.Fprintf(writer, "| %s:%d:%d | %d | %s | %s | %s | %s |\n",
			v.File, v.Line, v.Column, v.Line, v.CallerMethod, call, ruleStr, ruleFile)
	}

	return nil
}

func (o *OutputManager) outputExcel(result *models.AnalysisResult) error {
	if o.output == "" {
		return fmt.Errorf("output file required for Excel format")
	}

	// For now, we'll create a CSV that can be opened in Excel
	// A full Excel implementation would use a library like excelize
	o.format = "csv"
	return o.outputCSV(result)
}
