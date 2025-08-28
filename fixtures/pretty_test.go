package fixtures

import (
	"strings"
	"testing"
	"time"

	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
)

func TestFixtureTestResultPretty(t *testing.T) {
	tests := []struct {
		name     string
		result   FixtureResult
		contains []string
	}{
		{
			name: "passing test",
			result: FixtureResult{
				Name:     "Simple Test",
				Status:   task.StatusPASS,
				Duration: 1200 * time.Millisecond,
			},
			contains: []string{"✓", "Simple Test", "1.2s"},
		},
		{
			name: "failing test with error",
			result: FixtureResult{
				Name:     "Failed Test",
				Status:   task.StatusFailed,
				Duration: 500 * time.Millisecond,
				Error:    "assertion failed",
			},
			contains: []string{"✗", "Failed Test", "0.5s", "assertion failed"},
		},
		{
			name: "skipped test",
			result: FixtureResult{
				Name:   "Skipped Test",
				Status: task.StatusSKIP,
			},
			contains: []string{"○", "Skipped Test"},
		},
		{
			name: "test with details",
			result: FixtureResult{
				Name:     "Detailed Test",
				Status:   task.StatusPASS,
				Duration: 2100 * time.Millisecond,
				Metadata: map[string]interface{}{"details": "all checks passed"},
			},
			contains: []string{"✓", "Detailed Test", "2.1s", "all checks passed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test clicky formatting
			output, err := clicky.Format(tt.result, clicky.FormatOptions{Format: "tree"})
			if err != nil {
				t.Fatalf("formatting failed: %v", err)
			}

			// Basic check that output contains the test name
			if !strings.Contains(output, tt.result.Name) {
				t.Errorf("expected output to contain test name %q, got %q", tt.result.Name, output)
			}
		})
	}
}

func TestFixtureNodePretty(t *testing.T) {
	tests := []struct {
		name     string
		node     *FixtureNode
		contains []string
	}{
		{
			name: "file node",
			node: &FixtureNode{
				Name: "test.md",
				Type: FileNode,
				Stats: &Stats{
					Total:  5,
					Passed: 3,
					Failed: 2,
				},
			},
			contains: []string{"📁", "test.md", "3/5 passed"},
		},
		{
			name: "section node with all passing",
			node: &FixtureNode{
				Name: "Basic Tests",
				Type: SectionNode,
				Stats: &Stats{
					Total:  3,
					Passed: 3,
					Failed: 0,
				},
			},
			contains: []string{"✓", "Basic Tests", "3/3 passed"},
		},
		{
			name: "section node with failures",
			node: &FixtureNode{
				Name: "Advanced Tests",
				Type: SectionNode,
				Stats: &Stats{
					Total:  4,
					Passed: 2,
					Failed: 2,
				},
			},
			contains: []string{"✗", "Advanced Tests", "2/4 passed"},
		},
		{
			name: "test node with result",
			node: &FixtureNode{
				Name: "Test Case 1",
				Type: TestNode,
				Results: &FixtureResult{
					Name:     "Test Case 1",
					Status:   task.StatusPASS,
					Duration: 1500 * time.Millisecond,
				},
			},
			contains: []string{"✓", "Test Case 1", "1.5s"},
		},
		{
			name: "test node without result",
			node: &FixtureNode{
				Name: "Pending Test",
				Type: TestNode,
			},
			contains: []string{"○", "Pending Test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test clicky formatting
			output, err := clicky.Format(tt.node, clicky.FormatOptions{Format: "tree"})
			if err != nil {
				t.Fatalf("formatting failed: %v", err)
			}

			// Basic check that output contains the node name
			if !strings.Contains(output, tt.node.Name) {
				t.Errorf("expected output to contain node name %q, got %q", tt.node.Name, output)
			}
		})
	}
}

func TestStatsHasFailures(t *testing.T) {
	tests := []struct {
		name     string
		results  Stats
		expected bool
	}{
		{
			name: "no failures",
			results: Stats{
				Total:  3,
				Passed: 3,
				Failed: 0,
			},
			expected: false,
		},
		{
			name: "has failures",
			results: Stats{
				Total:  3,
				Passed: 2,
				Failed: 1,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.results.HasFailures() != tt.expected {
				t.Errorf("expected HasFailures() to return %v, got %v", tt.expected, tt.results.HasFailures())
			}
		})
	}
}
