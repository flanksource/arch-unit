package fixtures

import (
	"fmt"
	"time"

	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/api"
	"github.com/flanksource/clicky/task"
	"github.com/flanksource/commons/logger"
)

// TODO: Register custom renderer for status icons when clicky supports it
// For now, the pretty tags with color mapping should handle this

type Text api.Text

// NodeType represents the type of fixture node
type NodeType int

const (
	FileNode NodeType = iota
	SectionNode
	TestNode
)

// FixtureResult represents the result of a single fixture test (unified type)
type FixtureResult struct {
	// Core fields
	Name     string        `json:"name" pretty:"label=Test Name,style=text-blue-600"`
	Type     string        `json:"type,omitempty" pretty:"label=Type,style=text-gray-500"`
	Status   task.Status   `json:"status,omitempty" `
	Duration time.Duration `json:"duration,omitempty" pretty:"label=Duration,style=text-yellow-600,omitempty"`
	Test     FixtureTest   `json:"-"` // Only populated for Test nodes

	// Result data
	Error     string      `json:"error,omitempty" pretty:"label=Error,style=text-red-600,omitempty"`
	Expected  interface{} `json:"expected,omitempty" pretty:"label=Expected,omitempty"`
	Actual    interface{} `json:"actual,omitempty" pretty:"label=Actual,omitempty"`
	CELResult bool        `json:"cel_result,omitempty" pretty:"label=CEL Result,omitempty"`

	// Execution metadata
	Command  string                 `json:"command,omitempty" pretty:"label=Command,style=text-cyan-600,omitempty"`
	CWD      string                 `json:"cwd,omitempty" pretty:"label=Working Dir,style=text-purple-500,omitempty"`
	Stdout   string                 `json:"stdout,omitempty" pretty:"label=Stdout,omitempty"`
	Stderr   string                 `json:"stderr,omitempty" pretty:"label=Stderr,omitempty"`
	ExitCode int                    `json:"exit_code,omitempty" pretty:"label=Exit Code,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty" pretty:"label=Metadata,omitempty"`
}

func (f FixtureResult) Stats() Stats {
	switch f.Status {
	case task.StatusFailed:
		return Stats{Failed: 1, Total: 1}
	case task.StatusPASS, task.StatusSuccess:
		return Stats{Passed: 1, Total: 1}
	case task.StatusSKIP:
		return Stats{Skipped: 1, Total: 1}
	case task.StatusERR, task.StatusCancelled:
		return Stats{Error: 1, Total: 1}
	default:
		return Stats{}
	}
}

func (f FixtureResult) String() string {
	return fmt.Sprintf("%s - %s", f.Test.Name, f.Status.String())
}

func (f FixtureResult) Pretty() api.Text {
	t := f.Status.Pretty().Append(" ").Add(f.Test.Pretty())

	t = t.PrintfWithStyle(" (%s)", "text-yellow-600", f.Duration)
	if f.Error != "" {
		t = t.Add(clicky.Text(fmt.Sprintf("\n%s", f.Error), "text-red-600").Indent(2))
	}
	if logger.IsLevelEnabled(int(logger.Debug)) {
		trace := logger.IsLevelEnabled(2)
		d := api.Text{}
		// add command, exit code, and first line of stderr
		t = t.Append(fmt.Sprintf(" %s", f.Command), "text-gray-500")
		t = t.Append(fmt.Sprintf(" exit=%d", f.ExitCode), "text-gray-500")
		if trace {
			t = t.Append(fmt.Sprintf("dir= %s", f.CWD), "text-gray-500")
			if f.Stderr != "" {
				d = d.Append(fmt.Sprintf("%s\n", f.Stderr), "text-red-500 ")
			}
			if f.Stdout != "" {
				d = d.Append(fmt.Sprintf("%s\n", f.Stdout), "text-gray-500 ")
			}

		} else if f.Stderr != "" {
			d = d.Append(fmt.Sprintf("%s\n", f.Stderr), "text-red-500 max-w-[50ch]")
		}
		if !d.IsEmpty() {
			t = t.Append("\n").Add(d.Indent(2))
		}
	}
	return t
}

// Out returns the combined stderr and stdout output
func (f FixtureResult) Out() string {
	return f.Stderr + f.Stdout
}

func (f FixtureResult) IsOK() bool {
	return f.Status.Health() == task.HealthOK
}

type Visitor interface {
	Visit(test *FixtureResult)
}

// ResultSummary provides summary statistics
type Stats struct {
	Total   int `json:"total,omitempty"`
	Passed  int `json:"passed,omitempty"`
	Failed  int `json:"failed,omitempty"`
	Skipped int `json:"skipped,omitempty"`
	Error   int `json:"error,omitempty"`
}

func (s Stats) Merge(o Stats) Stats {
	return Stats{
		Total:   s.Total + o.Total,
		Passed:  s.Passed + o.Passed,
		Failed:  s.Failed + o.Failed,
		Skipped: s.Skipped + o.Skipped,
		Error:   s.Error + o.Error,
	}
}

func (s Stats) Add(result *FixtureResult) Stats {
	if result == nil {
		return s
	}
	s.Total++
	switch result.Status {
	case task.StatusFailed:
		s.Failed++
	case task.StatusPASS, task.StatusSuccess:
		s.Passed++
	case task.StatusSKIP:
		s.Skipped++
	case task.StatusERR, task.StatusCancelled:
		s.Error++
	}
	return s
}

func (f FixtureNode) GetStats() Stats {
	s := Stats{}

	s = s.Add(f.Results)
	for _, child := range f.Children {
		s = s.Merge(child.GetStats())
	}
	return s
}

func (s Stats) IsOK() bool {
	return s.Failed == 0 && s.Error == 0
}

func (f *FixtureNode) AddFileNode(path string) *FixtureNode {
	node := &FixtureNode{
		Name:   path,
		Type:   FileNode,
		Parent: f,
	}
	f.Children = append(f.Children, node)
	return node
}

// Pretty prints status, with green for passed red for failed and yellow for skipped
func (s Stats) Pretty() api.Text {

	t := api.Text{}
	if s.Passed > 0 {
		t = t.Append(string(s.Passed), "text-green-500")
	}
	if s.Failed > 0 {
		if !t.IsEmpty() {
			t = t.Append("/", "text-gray-500")
		}
		t = t.Append(string(s.Failed), "text-red-500")

	}
	if s.Skipped > 0 {
		t = t.Append(fmt.Sprintf(" %d skipped", s.Skipped), "text-yellow-500")
	}
	if s.Error > 0 {
		t = t.Append(fmt.Sprintf("%d errors", s.Error), "text-red-500")
	}
	return t
}

func (s Stats) String() string {
	if s.Total == 0 {
		return "-"
	}
	str := fmt.Sprintf("%d/%d", s.Passed, s.Failed+s.Passed)

	if s.Skipped > 0 {
		str += fmt.Sprintf(" %d skipped", s.Skipped)
	}

	if s.Error > 0 {
		str += fmt.Sprintf(" %d error", s.Error)
	}

	return str
}

func (s *Stats) Visit(node *FixtureNode) {
	test := node.Results
	if test == nil {
		return
	}
	s.Total++

	switch test.Status {
	case task.StatusFailed, task.StatusERR, task.StatusCancelled:
		s.Failed++
	case task.StatusPASS, task.StatusSuccess:
		s.Passed++
	case task.StatusSKIP:
		s.Skipped++
	}
}

func (s *Stats) Health() task.Health {
	if s.Failed+s.Error > 0 {
		return task.HealthError
	}
	if s.Total == 0 || s.Skipped > 0 {
		return task.HealthWarning
	}
	return task.HealthOK
}

// String returns a string representation of NodeType
func (nt NodeType) String() string {
	switch nt {
	case FileNode:
		return "file"
	case SectionNode:
		return "section"
	case TestNode:
		return "test"
	default:
		return "unknown"
	}
}

// FixtureNode represents a node in the hierarchical fixture tree
type FixtureNode struct {
	Name     string         `json:"name" pretty:"label"` // Node name (file, section, or test)
	Type     NodeType       `json:"type" pretty:"type"`  // File, Section, or Test
	Children []*FixtureNode `json:"children,omitempty" pretty:"format=tree"`
	Parent   *FixtureNode   `json:"-"`                                            // Parent node reference
	Test     *FixtureTest   `json:"test,omitempty" pretty:"test,omitempty"`       // Only populated for Test nodes
	Results  *FixtureResult `json:"results,omitempty" pretty:"results,omitempty"` // Only populated after execution
	Stats    *Stats         `json:"stats,omitempty" pretty:"stats,omitempty"`     // Aggregated statistics for sections/files
}

// FixtureTree represents the hierarchical structure of fixtures

// AddChild adds a child node to this node
func (fn *FixtureNode) AddChild(child *FixtureNode) {
	child.Parent = fn
	fn.Children = append(fn.Children, child)
}

// GetSectionPath returns the full section path (e.g., "File > Section > Subsection")
func (fn *FixtureNode) GetSectionPath() string {
	if fn.Parent == nil {
		return fn.Name
	}
	if fn.Parent.Type == FileNode {
		return fn.Name
	}
	return fn.Parent.GetSectionPath() + " > " + fn.Name
}

// Implement api.TreeNode interface
// GetLabel returns the display label for this node
func (fn *FixtureNode) GetLabel() string {
	return fn.Name
}

// GetChildren returns the children as TreeNode interface
func (fn *FixtureNode) GetChildren() []api.TreeNode {
	if fn.Children == nil || len(fn.Children) == 0 {
		return nil
	}
	nodes := make([]api.TreeNode, len(fn.Children))
	for i, child := range fn.Children {
		nodes[i] = child
	}
	return nodes
}

// GetIcon returns an icon based on node type and status
func (fn *FixtureNode) GetIcon() string {
	switch fn.Type {
	case FileNode:
		return "📁"
	case SectionNode:
		return "📂"
	case TestNode:
		if fn.Results != nil {
			return fn.Results.Status.Icon()
		}
	}
	return ""
}

// GetStyle returns the style based on test results
func (fn *FixtureNode) GetStyle() string {
	if fn.Results != nil {
		return fn.Results.Status.Style()
	}

	// Style for nodes with stats
	if fn.Stats != nil {
		return fn.Stats.Health().Style()
	}

	// Default styles by type
	switch fn.Type {
	case FileNode:
		return "text-blue-600 font-bold"
	case SectionNode:
		return "text-blue-500"
	default:
		return ""
	}
}

// IsLeaf returns true if this node has no children
func (fn *FixtureNode) IsLeaf() bool {
	return len(fn.Children) == 0
}

// Pretty returns a formatted Text with rich formatting
func (fn *FixtureNode) Pretty() api.Text {
	if fn.Results != nil {
		return fn.Results.Pretty()
	}
	// Start with icon
	icon := fn.GetIcon()
	content := fn.Name

	// Add icon to content
	if icon != "" {
		content = fmt.Sprintf("%s %s", icon, content)
	}

	// Add stats for section/file nodes (not individual tests)
	if fn.Type != TestNode && fn.Stats != nil && fn.Stats.Total > 0 {
		content = fmt.Sprintf("%s (%d/%d passed)",
			content, fn.Stats.Passed, fn.Stats.Total)
	}

	// Add test details if available
	if fn.Type == TestNode && fn.Results != nil {
		if fn.Results.Duration > 0 {
			content = fmt.Sprintf("%s (%s)", content, fn.Results.Duration)
		}
	}

	// Get style
	style := fn.GetStyle()

	// Create the text object
	text := api.Text{
		Content: content,
		Style:   style,
	}

	// Add error message as child if present
	if fn.Results != nil && fn.Results.Error != "" {
		text.Children = []api.Text{
			{
				Content: fmt.Sprintf("  ❌ Error: %s", fn.Results.Error),
				Style:   "text-red-500 text-sm italic",
			},
		}
	}

	return text
}

// Walk visits all nodes in the tree, calling visitor for test nodes
func (fn *FixtureNode) Walk(vistor func(f *FixtureNode)) {
	// Visit test nodes (nodes that have a Test to execute)
	if fn.Test != nil {
		vistor(fn)
	}
	// Recursively visit children
	for _, child := range fn.Children {
		child.Walk(vistor)
	}
}

// GetAllTests returns all test nodes in this subtree
func (fn *FixtureNode) GetAllTests() []*FixtureResult {
	var tests []*FixtureResult
	fn.Walk(func(f *FixtureNode) {
		tests = append(tests, f.Results)
	})
	return tests
}
