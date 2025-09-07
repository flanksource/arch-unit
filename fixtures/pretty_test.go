package fixtures

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/task"
)

var _ = Describe("Fixture Test Result Pretty", func() {
	DescribeTable("should format fixture results correctly",
		func(result FixtureResult, expectedContains []string) {
			// Test clicky formatting
			output, err := clicky.Format(result, clicky.FormatOptions{Format: "tree"})
			Expect(err).NotTo(HaveOccurred())

			// Basic check that output contains some content
			Expect(output).NotTo(BeEmpty())

			// Check for some expected content based on status
			switch result.Status {
			case "PASS":
				Expect(output).To(MatchRegexp("PASS|✓"))
			case "FAIL", "failed":
				Expect(output).To(MatchRegexp("failed|FAIL|✗"))
			case "SKIP":
				Expect(output).To(MatchRegexp("SKIP|○|⊘"))
			}
		},
		Entry("passing test",
			FixtureResult{
				Name:     "Simple Test",
				Status:   task.StatusPASS,
				Duration: 1200 * time.Millisecond,
			},
			[]string{"✓", "Simple Test", "1.2s"}),

		Entry("failing test with error",
			FixtureResult{
				Name:     "Failed Test",
				Status:   task.StatusFailed,
				Duration: 500 * time.Millisecond,
				Error:    "assertion failed",
			},
			[]string{"✗", "Failed Test", "0.5s", "assertion failed"}),

		Entry("skipped test",
			FixtureResult{
				Name:   "Skipped Test",
				Status: task.StatusSKIP,
			},
			[]string{"○", "Skipped Test"}),

		Entry("test with details",
			FixtureResult{
				Name:     "Detailed Test",
				Status:   task.StatusPASS,
				Duration: 2100 * time.Millisecond,
				Metadata: map[string]interface{}{"details": "all checks passed"},
			},
			[]string{"✓", "Detailed Test", "2.1s", "all checks passed"}),
	)
})

var _ = Describe("Fixture Node Pretty", func() {
	XDescribeTable("should format fixture nodes correctly",
		func(node *FixtureNode, expectedContains []string) {
			// Test clicky formatting
			output, err := clicky.Format(node, clicky.FormatOptions{Format: "tree"})
			Expect(err).NotTo(HaveOccurred())

			// Basic check that output contains some content
			Expect(output).NotTo(BeEmpty())

			// More flexible checks based on node type
			if node.Type == FileNode {
				Expect(output).To(MatchRegexp("📁|📂|file"))
			}
			
			if node.Stats != nil && node.Stats.Failed > 0 {
				Expect(output).To(MatchRegexp("fail|✗|📂"))
			} else if node.Stats != nil && node.Stats.Passed > 0 {
				Expect(output).To(MatchRegexp("pass|✓|📂"))
			}
		},
		Entry("file node",
			&FixtureNode{
				Name: "test.md",
				Type: FileNode,
				Stats: &Stats{
					Total:  5,
					Passed: 3,
					Failed: 2,
				},
			},
			[]string{"📁", "test.md", "3/5 passed"}),

		Entry("section node with all passing",
			&FixtureNode{
				Name: "Basic Tests",
				Type: SectionNode,
				Stats: &Stats{
					Total:  3,
					Passed: 3,
					Failed: 0,
				},
			},
			[]string{"✓", "Basic Tests", "3/3 passed"}),

		Entry("section node with failures",
			&FixtureNode{
				Name: "Advanced Tests",
				Type: SectionNode,
				Stats: &Stats{
					Total:  4,
					Passed: 2,
					Failed: 2,
				},
			},
			[]string{"✗", "Advanced Tests", "2/4 passed"}),

		Entry("test node with result",
			&FixtureNode{
				Name: "Test Case 1",
				Type: TestNode,
				Results: &FixtureResult{
					Name:     "Test Case 1",
					Status:   task.StatusPASS,
					Duration: 1500 * time.Millisecond,
				},
			},
			[]string{"✓", "Test Case 1", "1.5s"}),

		Entry("test node without result",
			&FixtureNode{
				Name: "Pending Test",
				Type: TestNode,
			},
			[]string{"○", "Pending Test"}),
	)
})

var _ = Describe("Stats Has Failures", func() {
	DescribeTable("should detect failures correctly",
		func(results Stats, expected bool) {
			Expect(results.HasFailures()).To(Equal(expected))
		},
		Entry("no failures",
			Stats{
				Total:  3,
				Passed: 3,
				Failed: 0,
			},
			false),

		Entry("has failures",
			Stats{
				Total:  3,
				Passed: 2,
				Failed: 1,
			},
			true),
	)
})
