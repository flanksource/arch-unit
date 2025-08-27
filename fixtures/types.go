package fixtures

import ()

// TODO: Register custom renderer for status icons when clicky supports it
// For now, the pretty tags with color mapping should handle this

// FixtureResults represents the results of running fixture tests
type FixtureResults struct {
	Summary ResultSummary   `json:"summary" pretty:"summary,struct"`
	Tests   []FixtureResult `json:"tests" pretty:"tests,tree"`
}

// FixtureResult represents the result of a single fixture test (unified type)
type FixtureResult struct {
	// Core fields
	Name      string                 `json:"name" pretty:"label=Test Name,style=text-blue-600"`
	Type      string                 `json:"type" pretty:"label=Type,style=text-gray-500"`
	Status    string                 `json:"status" pretty:"label=Status,render=status_icon,color:PASS=green:FAIL=red:SKIP=yellow"`
	Duration  string                 `json:"duration,omitempty" pretty:"label=Duration,style=text-yellow-600,omitempty"`
	
	// Result data
	Error     string                 `json:"error,omitempty" pretty:"label=Error,style=text-red-600,omitempty"`
	Expected  interface{}            `json:"expected,omitempty" pretty:"label=Expected,omitempty"`
	Actual    interface{}            `json:"actual,omitempty" pretty:"label=Actual,omitempty"`
	CELResult bool                   `json:"cel_result,omitempty" pretty:"label=CEL Result,omitempty"`
	Details   string                 `json:"details,omitempty" pretty:"label=Details,omitempty"`
	Output    string                 `json:"output,omitempty" pretty:"label=Output,style=text-gray-400,omitempty"`
	
	// Execution metadata
	Command   string                 `json:"command,omitempty" pretty:"label=Command,style=text-cyan-600,omitempty"`
	CWD       string                 `json:"cwd,omitempty" pretty:"label=Working Dir,style=text-purple-500,omitempty"`
	Stdout    string                 `json:"stdout,omitempty" pretty:"label=Stdout,omitempty"`
	Stderr    string                 `json:"stderr,omitempty" pretty:"label=Stderr,omitempty"`
	ExitCode  int                    `json:"exit_code,omitempty" pretty:"label=Exit Code,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" pretty:"label=Metadata,omitempty"`
}

// Out returns the combined stderr and stdout output
func (f FixtureResult) Out() string {
	return f.Stderr + f.Stdout
}

// FixtureTestResult is an alias for backwards compatibility
type FixtureTestResult = FixtureResult



// ResultSummary provides summary statistics
type ResultSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// HasFailures returns true if any tests failed
func (r *FixtureResults) HasFailures() bool {
	return r.Summary.Failed > 0
}

// NodeType represents the type of fixture node
type NodeType int

const (
	FileNode NodeType = iota
	SectionNode
	TestNode
)

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
	Name      string         `json:"name" pretty:"name,tree"`                     // Node name (file, section, or test)
	Type      NodeType       `json:"type" pretty:"type"`                          // File, Section, or Test
	Path      string         `json:"path" pretty:"path,omitempty"`                // Full path for files, section path for sections
	Level     int            `json:"level" pretty:"level,omitempty"`              // Nesting level (0=file, 1=section, 2=subsection, etc.)
	Parent    *FixtureNode   `json:"-"`                                           // Parent node reference
	Children  []*FixtureNode `json:"children" pretty:"children,tree"`            // Child nodes (sections or tests)
	Test      *FixtureTest   `json:"test,omitempty" pretty:"test,omitempty"`      // Only populated for Test nodes
	Results   *FixtureResult `json:"results,omitempty" pretty:"results,omitempty"` // Only populated after execution
	Stats     *NodeStats     `json:"stats,omitempty" pretty:"stats,omitempty"`    // Aggregated statistics for sections/files
}

// NodeStats represents aggregated statistics for a node
type NodeStats struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// FixtureTree represents the hierarchical structure of fixtures
type FixtureTree struct {
	Root     []*FixtureNode `json:"root" pretty:"root,tree"`           // Top-level file nodes
	AllTests []*FixtureNode `json:"all_tests,omitempty" pretty:"-"`    // Flat list of all test nodes for execution (not displayed)
	Stats    *NodeStats     `json:"stats" pretty:"stats,struct"`       // Overall statistics
}

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

// UpdateStats calculates and updates statistics for this node and its children
func (fn *FixtureNode) UpdateStats() {
	if fn.Type == TestNode {
		// For test nodes, stats come from the test result
		fn.Stats = &NodeStats{Total: 1}
		if fn.Results != nil {
			switch fn.Results.Status {
			case "PASS":
				fn.Stats.Passed = 1
			case "FAIL":
				fn.Stats.Failed = 1
			case "SKIP":
				fn.Stats.Skipped = 1
			}
		}
		return
	}
	
	// For section and file nodes, aggregate from children
	fn.Stats = &NodeStats{}
	for _, child := range fn.Children {
		child.UpdateStats() // Recursive update
		if child.Stats != nil {
			fn.Stats.Total += child.Stats.Total
			fn.Stats.Passed += child.Stats.Passed
			fn.Stats.Failed += child.Stats.Failed
			fn.Stats.Skipped += child.Stats.Skipped
		}
	}
}

// Pretty returns a formatted representation of the fixture node
func (fn *FixtureNode) Pretty() api.Text {
	builder := api.NewText("")
	
	switch fn.Type {
	case FileNode:
		// File node: 📁 filename.md (stats)
		iconText := api.NewText("📁 ").Info()
		nameText := api.NewText(fn.Name).Bold()
		builder.ChildBuilder(iconText).ChildBuilder(nameText)
		
		if fn.Stats != nil && fn.Stats.Total > 0 {
			statsText := fmt.Sprintf(" (%d/%d passed)", fn.Stats.Passed, fn.Stats.Total)
			var statsColor *api.TextBuilder
			if fn.Stats.Failed > 0 {
				statsColor = api.NewText(statsText).Error()
			} else if fn.Stats.Passed == fn.Stats.Total {
				statsColor = api.NewText(statsText).Success()
			} else {
				statsColor = api.NewText(statsText).Warning()
			}
			builder.ChildBuilder(statsColor)
		}
		
	case SectionNode:
		// Section node: status icon + section name (stats)
		var iconText *api.TextBuilder
		if fn.Stats != nil {
			if fn.Stats.Failed > 0 {
				iconText = api.NewText("✗ ").Error()
			} else if fn.Stats.Total > 0 && fn.Stats.Passed == fn.Stats.Total {
				iconText = api.NewText("✓ ").Success()
			} else {
				iconText = api.NewText("○ ").Warning()
			}
		} else {
			iconText = api.NewText("○ ").Muted()
		}
		
		nameText := api.NewText(fn.Name).Bold()
		builder.ChildBuilder(iconText).ChildBuilder(nameText)
		
		if fn.Stats != nil && fn.Stats.Total > 0 {
			statsText := fmt.Sprintf(" (%d/%d passed)", fn.Stats.Passed, fn.Stats.Total)
			var statsColor *api.TextBuilder
			if fn.Stats.Failed > 0 {
				statsColor = api.NewText(statsText).Error()
			} else if fn.Stats.Passed == fn.Stats.Total {
				statsColor = api.NewText(statsText).Success()
			} else {
				statsColor = api.NewText(statsText).Warning()
			}
			builder.ChildBuilder(statsColor)
		}
		
	case TestNode:
		// For test nodes, use the test result's Pretty method if available
		if fn.Results != nil {
			return fn.Results.Pretty()
		}
		// Fallback for tests without results
		nameText := api.NewText(fn.Name).Muted()
		builder.ChildBuilder(api.NewText("○ ").Muted()).ChildBuilder(nameText)
	}
	
	return builder.Build()
}

// IsLeaf returns true if this node has no children
func (fn *FixtureNode) IsLeaf() bool {
	return len(fn.Children) == 0
}

// GetAllTests returns all test nodes in this subtree
func (fn *FixtureNode) GetAllTests() []*FixtureNode {
	var tests []*FixtureNode
	if fn.Type == TestNode {
		tests = append(tests, fn)
	}
	for _, child := range fn.Children {
		tests = append(tests, child.GetAllTests()...)
	}
	return tests
}

// NewFixtureTree creates a new fixture tree
func NewFixtureTree() *FixtureTree {
	return &FixtureTree{
		Root:     make([]*FixtureNode, 0),
		AllTests: make([]*FixtureNode, 0),
		Stats:    &NodeStats{},
	}
}

// AddFileNode adds a file node to the tree
func (ft *FixtureTree) AddFileNode(name, path string) *FixtureNode {
	node := &FixtureNode{
		Name:     name,
		Type:     FileNode,
		Path:     path,
		Level:    0,
		Children: make([]*FixtureNode, 0),
	}
	ft.Root = append(ft.Root, node)
	return node
}

// BuildAllTestsList builds the flat list of all test nodes
func (ft *FixtureTree) BuildAllTestsList() {
	ft.AllTests = make([]*FixtureNode, 0)
	for _, root := range ft.Root {
		ft.AllTests = append(ft.AllTests, root.GetAllTests()...)
	}
}

// UpdateStats updates statistics for the entire tree
func (ft *FixtureTree) UpdateStats() {
	ft.Stats = &NodeStats{}
	for _, root := range ft.Root {
		root.UpdateStats()
		if root.Stats != nil {
			ft.Stats.Total += root.Stats.Total
			ft.Stats.Passed += root.Stats.Passed
			ft.Stats.Failed += root.Stats.Failed
			ft.Stats.Skipped += root.Stats.Skipped
		}
	}
}

// Pretty returns a formatted representation of the fixture node
func (fn *FixtureNode) Pretty() api.Text {
	builder := api.NewText("")
	
	switch fn.Type {
	case FileNode:
		// File node: 📁 filename.md (stats)
		iconText := api.NewText("📁 ").Info()
		nameText := api.NewText(fn.Name).Bold()
		builder.ChildBuilder(iconText).ChildBuilder(nameText)
		
		if fn.Stats != nil && fn.Stats.Total > 0 {
			statsText := fmt.Sprintf(" (%d/%d passed)", fn.Stats.Passed, fn.Stats.Total)
			var statsColor *api.TextBuilder
			if fn.Stats.Failed > 0 {
				statsColor = api.NewText(statsText).Error()
			} else if fn.Stats.Passed == fn.Stats.Total {
				statsColor = api.NewText(statsText).Success()
			} else {
				statsColor = api.NewText(statsText).Warning()
			}
			builder.ChildBuilder(statsColor)
		}
		
	case SectionNode:
		// Section node: status icon + section name (stats)
		var iconText *api.TextBuilder
		if fn.Stats != nil {
			if fn.Stats.Failed > 0 {
				iconText = api.NewText("✗ ").Error()
			} else if fn.Stats.Total > 0 && fn.Stats.Passed == fn.Stats.Total {
				iconText = api.NewText("✓ ").Success()
			} else {
				iconText = api.NewText("○ ").Warning()
			}
		} else {
			iconText = api.NewText("○ ").Muted()
		}
		
		nameText := api.NewText(fn.Name).Bold()
		builder.ChildBuilder(iconText).ChildBuilder(nameText)
		
		if fn.Stats != nil && fn.Stats.Total > 0 {
			statsText := fmt.Sprintf(" (%d/%d passed)", fn.Stats.Passed, fn.Stats.Total)
			var statsColor *api.TextBuilder
			if fn.Stats.Failed > 0 {
				statsColor = api.NewText(statsText).Error()
			} else if fn.Stats.Passed == fn.Stats.Total {
				statsColor = api.NewText(statsText).Success()
			} else {
				statsColor = api.NewText(statsText).Warning()
			}
			builder.ChildBuilder(statsColor)
		}
		
	case TestNode:
		// For test nodes, use the test result's Pretty method if available
		if fn.Results != nil {
			return fn.Results.Pretty()
		}
		// Fallback for tests without results
		nameText := api.NewText(fn.Name).Muted()
		builder.ChildBuilder(api.NewText("○ ").Muted()).ChildBuilder(nameText)
	}
	
	return builder.Build()
}