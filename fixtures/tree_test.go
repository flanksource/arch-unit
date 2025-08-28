package fixtures

import (
	"testing"
)

func TestFixtureTree(t *testing.T) {
	tree := NewFixtureTree()

	if len(tree.Root) != 0 {
		t.Error("expected empty root")
	}

	if len(tree.AllTests) != 0 {
		t.Error("expected empty AllTests")
	}

	if tree.Stats == nil {
		t.Error("expected stats to be initialized")
	}
}

func TestAddFileNode(t *testing.T) {
	tree := NewFixtureTree()
	fileNode := tree.AddFileNode("test.md", "/path/to/test.md")

	if len(tree.Root) != 1 {
		t.Errorf("expected 1 root node, got %d", len(tree.Root))
	}

	if fileNode.Name != "test.md" {
		t.Errorf("expected name %q, got %q", "test.md", fileNode.Name)
	}

	if fileNode.Type != FileNode {
		t.Errorf("expected FileNode type, got %v", fileNode.Type)
	}

	if fileNode.Level != 0 {
		t.Errorf("expected level 0, got %d", fileNode.Level)
	}
}

func TestFixtureNodeHierarchy(t *testing.T) {
	tree := NewFixtureTree()
	fileNode := tree.AddFileNode("test.md", "/path/to/test.md")

	// Add section node
	sectionNode := &FixtureNode{
		Name:  "Basic Tests",
		Type:  SectionNode,
		Level: 1,
	}
	fileNode.AddChild(sectionNode)

	// Add test node
	testNode := &FixtureNode{
		Name:  "Test 1",
		Type:  TestNode,
		Level: 2,
		Test:  &FixtureTest{Name: "Test 1"},
	}
	sectionNode.AddChild(testNode)

	// Verify hierarchy
	if len(fileNode.Children) != 1 {
		t.Errorf("expected 1 child for file node, got %d", len(fileNode.Children))
	}

	if sectionNode.Parent != fileNode {
		t.Error("expected section parent to be file node")
	}

	if len(sectionNode.Children) != 1 {
		t.Errorf("expected 1 child for section node, got %d", len(sectionNode.Children))
	}

	if testNode.Parent != sectionNode {
		t.Error("expected test parent to be section node")
	}
}

func TestGetSectionPath(t *testing.T) {
	tree := NewFixtureTree()
	fileNode := tree.AddFileNode("test.md", "/path/to/test.md")

	sectionNode := &FixtureNode{
		Name:  "Section A",
		Type:  SectionNode,
		Level: 1,
	}
	fileNode.AddChild(sectionNode)

	subsectionNode := &FixtureNode{
		Name:  "Subsection B",
		Type:  SectionNode,
		Level: 2,
	}
	sectionNode.AddChild(subsectionNode)

	testNode := &FixtureNode{
		Name:  "Test 1",
		Type:  TestNode,
		Level: 3,
	}
	subsectionNode.AddChild(testNode)

	// Test section paths
	if sectionNode.GetSectionPath() != "Section A" {
		t.Errorf("expected section path %q, got %q", "Section A", sectionNode.GetSectionPath())
	}

	if subsectionNode.GetSectionPath() != "Section A > Subsection B" {
		t.Errorf("expected subsection path %q, got %q", "Section A > Subsection B", subsectionNode.GetSectionPath())
	}

	if testNode.GetSectionPath() != "Section A > Subsection B > Test 1" {
		t.Errorf("expected test path %q, got %q", "Section A > Subsection B > Test 1", testNode.GetSectionPath())
	}
}

func TestGetAllTests(t *testing.T) {
	tree := NewFixtureTree()
	fileNode := tree.AddFileNode("test.md", "/path/to/test.md")

	// Add section with multiple tests
	sectionNode := &FixtureNode{
		Name:  "Section A",
		Type:  SectionNode,
		Level: 1,
	}
	fileNode.AddChild(sectionNode)

	test1 := &FixtureNode{
		Name: "Test 1",
		Type: TestNode,
		Test: &FixtureTest{Name: "Test 1"},
	}
	sectionNode.AddChild(test1)

	test2 := &FixtureNode{
		Name: "Test 2",
		Type: TestNode,
		Test: &FixtureTest{Name: "Test 2"},
	}
	sectionNode.AddChild(test2)

	// Add another section with one test
	section2Node := &FixtureNode{
		Name:  "Section B",
		Type:  SectionNode,
		Level: 1,
	}
	fileNode.AddChild(section2Node)

	test3 := &FixtureNode{
		Name: "Test 3",
		Type: TestNode,
		Test: &FixtureTest{Name: "Test 3"},
	}
	section2Node.AddChild(test3)

	// Get all tests from file node
	allTests := fileNode.GetAllTests()

	if len(allTests) != 3 {
		t.Errorf("expected 3 tests, got %d", len(allTests))
	}

	// Verify test order and content
	expectedNames := []string{"Test 1", "Test 2", "Test 3"}
	for i, test := range allTests {
		if test.Name != expectedNames[i] {
			t.Errorf("expected test name %q, got %q", expectedNames[i], test.Name)
		}
		if test.Type != TestNode {
			t.Errorf("expected TestNode type, got %v", test.Type)
		}
	}
}

func TestUpdateStats(t *testing.T) {
	tree := NewFixtureTree()
	fileNode := tree.AddFileNode("test.md", "/path/to/test.md")

	sectionNode := &FixtureNode{
		Name:  "Section A",
		Type:  SectionNode,
		Level: 1,
	}
	fileNode.AddChild(sectionNode)

	// Add test nodes with results
	test1 := &FixtureNode{
		Name: "Test 1",
		Type: TestNode,
		Results: &FixtureResult{
			Name:   "Test 1",
			Status: "PASS",
		},
	}
	sectionNode.AddChild(test1)

	test2 := &FixtureNode{
		Name: "Test 2",
		Type: TestNode,
		Results: &FixtureTestResult{
			Name:   "Test 2",
			Status: "FAIL",
		},
	}
	sectionNode.AddChild(test2)

	test3 := &FixtureNode{
		Name: "Test 3",
		Type: TestNode,
		Results: &FixtureTestResult{
			Name:   "Test 3",
			Status: "SKIP",
		},
	}
	sectionNode.AddChild(test3)

	// Update stats
	fileNode.UpdateStats()

	// Check section stats
	if sectionNode.Stats == nil {
		t.Fatal("expected section stats to be set")
	}

	if sectionNode.Stats.Total != 3 {
		t.Errorf("expected total 3, got %d", sectionNode.Stats.Total)
	}

	if sectionNode.Stats.Passed != 1 {
		t.Errorf("expected passed 1, got %d", sectionNode.Stats.Passed)
	}

	if sectionNode.Stats.Failed != 1 {
		t.Errorf("expected failed 1, got %d", sectionNode.Stats.Failed)
	}

	if sectionNode.Stats.Skipped != 1 {
		t.Errorf("expected skipped 1, got %d", sectionNode.Stats.Skipped)
	}

	// Check file stats (should aggregate from sections)
	if fileNode.Stats == nil {
		t.Fatal("expected file stats to be set")
	}

	if fileNode.Stats.Total != 3 {
		t.Errorf("expected file total 3, got %d", fileNode.Stats.Total)
	}

	if fileNode.Stats.Passed != 1 {
		t.Errorf("expected file passed 1, got %d", fileNode.Stats.Passed)
	}
}

func TestBuildAllTestsList(t *testing.T) {
	tree := NewFixtureTree()

	// File 1
	file1 := tree.AddFileNode("test1.md", "/path/to/test1.md")
	section1 := &FixtureNode{Name: "Section 1", Type: SectionNode}
	file1.AddChild(section1)
	test1 := &FixtureNode{Name: "Test 1", Type: TestNode, Test: &FixtureTest{}}
	section1.AddChild(test1)

	// File 2
	file2 := tree.AddFileNode("test2.md", "/path/to/test2.md")
	test2 := &FixtureNode{Name: "Test 2", Type: TestNode, Test: &FixtureTest{}}
	file2.AddChild(test2)
	test3 := &FixtureNode{Name: "Test 3", Type: TestNode, Test: &FixtureTest{}}
	file2.AddChild(test3)

	// Build all tests list
	tree.BuildAllTestsList()

	if len(tree.AllTests) != 3 {
		t.Errorf("expected 3 tests in AllTests, got %d", len(tree.AllTests))
	}

	// Verify all nodes are test nodes
	for i, testNode := range tree.AllTests {
		if testNode.Type != TestNode {
			t.Errorf("AllTests[%d] should be TestNode, got %v", i, testNode.Type)
		}
		if testNode.Test == nil {
			t.Errorf("AllTests[%d] should have Test field set", i)
		}
	}
}

func TestNodeTypeString(t *testing.T) {
	tests := []struct {
		nodeType NodeType
		expected string
	}{
		{FileNode, "file"},
		{SectionNode, "section"},
		{TestNode, "test"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.nodeType.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.nodeType.String())
			}
		})
	}
}
