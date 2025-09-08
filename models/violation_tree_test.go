package models

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/flanksource/clicky"
)

var _ = Describe("ViolationTree", func() {
	var testViolations []Violation
	
	BeforeEach(func() {
		testRule := &Rule{
			OriginalLine: "no calls to database from controllers",
			SourceFile:   "arch.md",
			LineNumber:   5,
		}
		
		testViolations = []Violation{
			{
				File:          "controller.go",
				Line:          30,
				Column:        8,
				CallerMethod:  "HandleRequest",
				CalledPackage: "database",
				CalledMethod:  "Query",
				Rule:          testRule,
				Source:        "arch-unit",
				CreatedAt:     time.Now(),
			},
			{
				File:          "controller.go",
				Line:          45,
				Column:        12,
				CallerMethod:  "ProcessData",
				CalledPackage: "external.api",
				CalledMethod:  "Call",
				Message:       "Direct external API calls are not allowed",
				Source:        "arch-unit",
				CreatedAt:     time.Now(),
			},
			{
				File:          "service.go",
				Line:          20,
				Column:        5,
				CallerMethod:  "ServiceMethod",
				CalledPackage: "forbidden.pkg",
				CalledMethod:  "BadMethod",
				Source:        "golangci-lint",
				CreatedAt:     time.Now(),
			},
		}
	})

	Describe("BuildViolationTree", func() {
		It("should create an empty tree for no violations", func() {
			tree := BuildViolationTree([]Violation{})
			
			rootNode, ok := tree.(*ViolationRootNode)
			Expect(ok).To(BeTrue())
			Expect(rootNode.total).To(Equal(0))
			Expect(len(rootNode.fileNodes)).To(Equal(0))
			Expect(rootNode.IsLeaf()).To(BeTrue())
		})

		It("should create a properly structured tree for violations", func() {
			tree := BuildViolationTree(testViolations)
			
			rootNode, ok := tree.(*ViolationRootNode)
			Expect(ok).To(BeTrue())
			Expect(rootNode.total).To(Equal(3))
			Expect(len(rootNode.fileNodes)).To(Equal(2)) // controller.go and service.go
			
			// Check that files are sorted
			Expect(rootNode.fileNodes[0].path).To(Equal("controller.go"))
			Expect(rootNode.fileNodes[1].path).To(Equal("service.go"))
		})
		
		It("should group violations by file and then by source", func() {
			tree := BuildViolationTree(testViolations)
			rootNode := tree.(*ViolationRootNode)
			
			// Check controller.go file node
			controllerNode := rootNode.fileNodes[0]
			Expect(controllerNode.path).To(Equal("controller.go"))
			Expect(controllerNode.total).To(Equal(2))
			Expect(len(controllerNode.sourceNodes)).To(Equal(1)) // Only arch-unit
			
			archUnitNode := controllerNode.sourceNodes[0]
			Expect(archUnitNode.source).To(Equal("arch-unit"))
			Expect(len(archUnitNode.violationNodes)).To(Equal(2))
			
			// Check service.go file node
			serviceNode := rootNode.fileNodes[1]
			Expect(serviceNode.path).To(Equal("service.go"))
			Expect(serviceNode.total).To(Equal(1))
			Expect(len(serviceNode.sourceNodes)).To(Equal(1)) // Only golangci-lint
			
			lintNode := serviceNode.sourceNodes[0]
			Expect(lintNode.source).To(Equal("golangci-lint"))
			Expect(len(lintNode.violationNodes)).To(Equal(1))
		})
	})

	Describe("ViolationRootNode", func() {
		var rootNode *ViolationRootNode
		
		BeforeEach(func() {
			tree := BuildViolationTree(testViolations)
			rootNode = tree.(*ViolationRootNode)
		})
		
		It("should implement TreeNode interface correctly", func() {
			Expect(rootNode.GetLabel()).To(Equal("Violations (3)"))
			Expect(rootNode.GetIcon()).To(Equal("📋"))
			Expect(rootNode.GetStyle()).To(Equal("text-red-600 font-bold"))
			Expect(rootNode.IsLeaf()).To(BeFalse())
			
			children := rootNode.GetChildren()
			Expect(len(children)).To(Equal(2))
		})
		
		It("should implement PrettyNode interface correctly", func() {
			pretty := rootNode.Pretty()
			Expect(pretty.Content).To(Equal("📋 Violations (3)"))
			Expect(pretty.Style).To(Equal("text-red-600 font-bold"))
		})
	})

	Describe("ViolationFileNode", func() {
		var fileNode *ViolationFileNode
		
		BeforeEach(func() {
			tree := BuildViolationTree(testViolations)
			rootNode := tree.(*ViolationRootNode)
			fileNode = rootNode.fileNodes[0] // controller.go
		})
		
		It("should implement TreeNode interface correctly", func() {
			Expect(fileNode.GetLabel()).To(ContainSubstring("controller.go (2 violations)"))
			Expect(fileNode.GetIcon()).To(Equal("🐹")) // Go file icon
			Expect(fileNode.GetStyle()).To(Equal("text-cyan-600 font-bold"))
			Expect(fileNode.IsLeaf()).To(BeFalse())
			
			children := fileNode.GetChildren()
			Expect(len(children)).To(Equal(1)) // Only arch-unit source
		})
		
		It("should display correct file icons based on extension", func() {
			pyFileNode := NewViolationFileNode("test.py", testViolations)
			Expect(pyFileNode.GetIcon()).To(Equal("🐍"))
			
			jsFileNode := NewViolationFileNode("test.js", testViolations)
			Expect(jsFileNode.GetIcon()).To(Equal("📜"))
			
			tsFileNode := NewViolationFileNode("test.ts", testViolations)
			Expect(tsFileNode.GetIcon()).To(Equal("📘"))
		})
		
		It("should implement PrettyNode interface correctly", func() {
			pretty := fileNode.Pretty()
			Expect(pretty.Content).To(ContainSubstring("🐹"))
			Expect(pretty.Content).To(ContainSubstring("controller.go (2 violations)"))
			Expect(pretty.Style).To(Equal("text-cyan-600 font-bold"))
		})
	})

	Describe("ViolationSourceNode", func() {
		var sourceNode *ViolationSourceNode
		
		BeforeEach(func() {
			tree := BuildViolationTree(testViolations)
			rootNode := tree.(*ViolationRootNode)
			fileNode := rootNode.fileNodes[0] // controller.go
			sourceNode = fileNode.sourceNodes[0] // arch-unit
		})
		
		It("should implement TreeNode interface correctly", func() {
			Expect(sourceNode.GetLabel()).To(Equal("arch-unit (2)"))
			Expect(sourceNode.GetIcon()).To(Equal("🏛️"))
			Expect(sourceNode.GetStyle()).To(Equal("text-purple-600"))
			Expect(sourceNode.IsLeaf()).To(BeFalse())
			
			children := sourceNode.GetChildren()
			Expect(len(children)).To(Equal(2)) // Two violations
		})
		
		It("should display correct source icons and styles", func() {
			golangciNode := NewViolationSourceNode("golangci-lint", testViolations)
			Expect(golangciNode.GetIcon()).To(Equal("🔍"))
			Expect(golangciNode.GetStyle()).To(Equal("text-yellow-600"))
			
			eslintNode := NewViolationSourceNode("eslint", testViolations)
			Expect(eslintNode.GetIcon()).To(Equal("⚡"))
			Expect(eslintNode.GetStyle()).To(Equal("text-yellow-600"))
		})
		
		It("should implement PrettyNode interface correctly", func() {
			pretty := sourceNode.Pretty()
			Expect(pretty.Content).To(Equal("🏛️ arch-unit (2)"))
			Expect(pretty.Style).To(Equal("text-purple-600"))
		})
	})

	Describe("ViolationNode", func() {
		var violationNode *ViolationNode
		
		BeforeEach(func() {
			tree := BuildViolationTree(testViolations)
			rootNode := tree.(*ViolationRootNode)
			fileNode := rootNode.fileNodes[0] // controller.go
			sourceNode := fileNode.sourceNodes[0] // arch-unit
			violationNode = sourceNode.violationNodes[0] // First violation
		})
		
		It("should implement TreeNode interface correctly", func() {
			Expect(violationNode.GetLabel()).To(ContainSubstring("Line 30:8: HandleRequest calls forbidden database.Query"))
			Expect(violationNode.GetIcon()).To(Equal("❌"))
			Expect(violationNode.GetStyle()).To(Equal("text-red-600"))
			Expect(violationNode.IsLeaf()).To(BeTrue())
			
			children := violationNode.GetChildren()
			Expect(children).To(BeNil())
		})
		
		It("should include message in label when available", func() {
			tree := BuildViolationTree(testViolations)
			rootNode := tree.(*ViolationRootNode)
			fileNode := rootNode.fileNodes[0] // controller.go
			sourceNode := fileNode.sourceNodes[0] // arch-unit
			violationWithMessage := sourceNode.violationNodes[1] // Second violation has message
			
			label := violationWithMessage.GetLabel()
			Expect(label).To(ContainSubstring("Line 45:12: ProcessData calls forbidden external.api.Call - Direct external API calls are not allowed"))
		})
		
		It("should implement PrettyNode interface correctly", func() {
			pretty := violationNode.Pretty()
			// Should delegate to violation's Pretty() method
			expectedPretty := violationNode.violation.Pretty()
			Expect(pretty.Content).To(Equal(expectedPretty.Content))
			Expect(pretty.Style).To(Equal(expectedPretty.Style))
		})
	})

	Describe("Violation.Tree", func() {
		It("should return a ViolationNode", func() {
			violation := testViolations[0]
			treeNode := violation.Tree()
			
			violationNode, ok := treeNode.(*ViolationNode)
			Expect(ok).To(BeTrue())
			Expect(violationNode.violation).To(Equal(violation))
		})
	})

	Describe("Integration with clicky.Format", func() {
		It("should format tree as string successfully", func() {
			tree := BuildViolationTree(testViolations)
			
			output, err := clicky.Format(tree, clicky.FormatOptions{Format: "tree"})
			Expect(err).ToNot(HaveOccurred())
			Expect(output).ToNot(BeEmpty())
			
			// Debug output
			GinkgoWriter.Printf("Formatted output:\n%s\n", output)
			
			// Check that output contains expected elements
			Expect(output).To(ContainSubstring("Violations (3)"))
			// Let's check if the tree is actually expanding properly
		})
		
		It("should format empty tree without error", func() {
			tree := BuildViolationTree([]Violation{})
			
			output, err := clicky.Format(tree, clicky.FormatOptions{Format: "tree"})
			Expect(err).ToNot(HaveOccurred())
			Expect(output).ToNot(BeEmpty())
			Expect(output).To(ContainSubstring("Violations (0)"))
		})
		
		It("should work with different format options", func() {
			tree := BuildViolationTree(testViolations)
			
			// Test JSON format
			jsonOutput, err := clicky.Format(tree, clicky.FormatOptions{Format: "json"})
			Expect(err).ToNot(HaveOccurred())
			Expect(jsonOutput).ToNot(BeEmpty())
			
			// Test compact tree format
			compactOutput, err := clicky.Format(tree, clicky.FormatOptions{
				Format:  "tree",
				NoColor: true,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(compactOutput).ToNot(BeEmpty())
		})
	})

	Describe("Edge cases", func() {
		It("should handle violations with missing method names", func() {
			violationWithoutMethod := Violation{
				File:          "test.go",
				Line:          10,
				Column:        5,
				CallerMethod:  "TestMethod",
				CalledPackage: "pkg",
				Source:        "arch-unit",
			}
			
			tree := BuildViolationTree([]Violation{violationWithoutMethod})
			rootNode := tree.(*ViolationRootNode)
			fileNode := rootNode.fileNodes[0]
			sourceNode := fileNode.sourceNodes[0]
			violationNode := sourceNode.violationNodes[0]
			
			label := violationNode.GetLabel()
			Expect(label).To(ContainSubstring("TestMethod calls forbidden pkg"))
			Expect(label).ToNot(ContainSubstring("pkg."))
		})
		
		It("should handle violations with empty source", func() {
			violationWithoutSource := Violation{
				File:          "test.go",
				Line:          10,
				Column:        5,
				CallerMethod:  "TestMethod",
				CalledPackage: "pkg",
				CalledMethod:  "Method",
				Source:        "", // Empty source
			}
			
			tree := BuildViolationTree([]Violation{violationWithoutSource})
			rootNode := tree.(*ViolationRootNode)
			fileNode := rootNode.fileNodes[0]
			sourceNode := fileNode.sourceNodes[0]
			
			Expect(sourceNode.source).To(Equal("arch-unit")) // Default
		})
		
		It("should sort files and sources consistently", func() {
			unsortedViolations := []Violation{
				{File: "z.go", Source: "z-linter"},
				{File: "a.go", Source: "a-linter"},
				{File: "m.go", Source: "m-linter"},
				{File: "a.go", Source: "z-linter"},
			}
			
			tree := BuildViolationTree(unsortedViolations)
			rootNode := tree.(*ViolationRootNode)
			
			// Files should be sorted
			Expect(rootNode.fileNodes[0].path).To(Equal("a.go"))
			Expect(rootNode.fileNodes[1].path).To(Equal("m.go"))
			Expect(rootNode.fileNodes[2].path).To(Equal("z.go"))
			
			// Sources within a file should be sorted
			aFileNode := rootNode.fileNodes[0] // a.go has 2 sources
			if len(aFileNode.sourceNodes) > 1 {
				Expect(aFileNode.sourceNodes[0].source).To(Equal("a-linter"))
				Expect(aFileNode.sourceNodes[1].source).To(Equal("z-linter"))
			}
		})
	})
})