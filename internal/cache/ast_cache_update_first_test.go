package cache_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("AST Cache Update First", func() {
	var (
		tempDir  string
		astCache *cache.ASTCache
	)

	BeforeEach(func() {
		var err error
		tempDir = GinkgoT().TempDir()
		astCache, err = cache.GetASTCache()
		Expect(err).NotTo(HaveOccurred())

		// Clear all data to ensure test isolation
		err = astCache.ClearAllData()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("StoreFileResults with UpdateFirst behavior", func() {
		Context("when re-analyzing a file", func() {
			It("should preserve existing node IDs and update data", func() {
				filePath := "test_file.go"

				// Create test file for metadata calculation
				testFile := tempDir + "/" + filePath
				err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// Create first analysis result with some nodes
				firstResult := &struct {
					Nodes         []*models.ASTNode
					Relationships []*models.ASTRelationship
					Libraries     []*models.LibraryRelationship
				}{
					Nodes: []*models.ASTNode{
						{
							ID:          1, // Analysis ID
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "main",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   3,
							EndLine:     3,
						},
						{
							ID:          2, // Analysis ID
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "TestStruct",
							MethodName:  "",
							FieldName:   "Field1",
							NodeType:    "field",
							StartLine:   5,
							EndLine:     5,
						},
					},
					Relationships: []*models.ASTRelationship{
						{
							FromASTID:        1,
							ToASTID:          nil,
							LineNo:           3,
							RelationshipType: "call",
							Text:             "main()",
						},
					},
					Libraries: []*models.LibraryRelationship{},
				}

				// Store first result
				err = astCache.StoreFileResults(testFile, firstResult)
				Expect(err).NotTo(HaveOccurred())

				// Verify nodes were created
				nodes1, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes1).To(HaveLen(2))

				// Record the IDs assigned by the database
				var mainMethodID, field1ID int64
				for _, node := range nodes1 {
					if node.MethodName == "main" {
						mainMethodID = node.ID
					} else if node.FieldName == "Field1" {
						field1ID = node.ID
					}
				}

				// Ensure IDs were assigned
				Expect(mainMethodID).To(BeNumerically(">", 0))
				Expect(field1ID).To(BeNumerically(">", 0))

				// Create second analysis result - simulate re-analysis with changes
				// - Keep the main method (should preserve ID)
				// - Modify Field1 (should preserve ID but update data)
				// - Add a new field (should get new ID)
				// - Remove nothing (test orphan cleanup separately)
				secondResult := &struct {
					Nodes         []*models.ASTNode
					Relationships []*models.ASTRelationship
					Libraries     []*models.LibraryRelationship
				}{
					Nodes: []*models.ASTNode{
						{
							ID:          1, // Same analysis ID for main method
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "main",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   3,
							EndLine:     3,
							LineCount:   5, // Updated line count
						},
						{
							ID:          2, // Same analysis ID for Field1 but modified
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "TestStruct",
							MethodName:  "",
							FieldName:   "Field1",
							NodeType:    "field",
							StartLine:   5,
							EndLine:     5,
							LineCount:   10, // Updated line count
						},
						{
							ID:          3, // New field
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "TestStruct",
							MethodName:  "",
							FieldName:   "Field2",
							NodeType:    "field",
							StartLine:   6,
							EndLine:     6,
						},
					},
					Relationships: []*models.ASTRelationship{
						{
							FromASTID:        1,
							ToASTID:          nil,
							LineNo:           3,
							RelationshipType: "call",
							Text:             "main()",
						},
					},
					Libraries: []*models.LibraryRelationship{},
				}

				// Store second result (re-analysis)
				err = astCache.StoreFileResults(testFile, secondResult)
				Expect(err).NotTo(HaveOccurred())

				// Verify the results
				nodes2, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes2).To(HaveLen(3)) // Should have 3 nodes now

				// Verify ID preservation and updates
				var mainMethod2, field1_2, field2 *models.ASTNode
				for _, node := range nodes2 {
					switch {
					case node.MethodName == "main":
						mainMethod2 = node
					case node.FieldName == "Field1":
						field1_2 = node
					case node.FieldName == "Field2":
						field2 = node
					}
				}

				Expect(mainMethod2).NotTo(BeNil())
				Expect(field1_2).NotTo(BeNil())
				Expect(field2).NotTo(BeNil())

				// Verify ID preservation
				Expect(mainMethod2.ID).To(Equal(mainMethodID), "main method ID should be preserved")
				Expect(field1_2.ID).To(Equal(field1ID), "Field1 ID should be preserved")

				// Verify data updates
				Expect(mainMethod2.LineCount).To(Equal(5), "main method LineCount should be updated")
				Expect(field1_2.LineCount).To(Equal(10), "Field1 LineCount should be updated")

				// Verify new node got a new ID
				Expect(field2.ID).To(BeNumerically(">", 0))
				Expect(field2.ID).NotTo(Equal(mainMethodID))
				Expect(field2.ID).NotTo(Equal(field1ID))
			})
		})

		Context("when nodes are removed in re-analysis", func() {
			It("should clean up orphaned nodes", func() {
				filePath := "test_orphan.go"

				// Create test file for metadata calculation
				testFile := tempDir + "/" + filePath
				err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// Create first analysis result with 3 nodes
				firstResult := &struct {
					Nodes         []*models.ASTNode
					Relationships []*models.ASTRelationship
					Libraries     []*models.LibraryRelationship
				}{
					Nodes: []*models.ASTNode{
						{
							ID:          1,
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "func1",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   3,
						},
						{
							ID:          2,
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "func2",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   5,
						},
						{
							ID:          3,
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "func3",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   7,
						},
					},
					Relationships: []*models.ASTRelationship{},
					Libraries:     []*models.LibraryRelationship{},
				}

				// Store first result
				err = astCache.StoreFileResults(testFile, firstResult)
				Expect(err).NotTo(HaveOccurred())

				// Verify 3 nodes exist
				nodes1, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes1).To(HaveLen(3))

				// Create second analysis result with only 2 nodes (func2 removed)
				secondResult := &struct {
					Nodes         []*models.ASTNode
					Relationships []*models.ASTRelationship
					Libraries     []*models.LibraryRelationship
				}{
					Nodes: []*models.ASTNode{
						{
							ID:          1, // Keep func1
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "func1",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   3,
						},
						{
							ID:          3, // Keep func3 (func2 is being removed)
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "func3",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   7,
						},
					},
					Relationships: []*models.ASTRelationship{},
					Libraries:     []*models.LibraryRelationship{},
				}

				// Store second result (should remove func2)
				err = astCache.StoreFileResults(testFile, secondResult)
				Expect(err).NotTo(HaveOccurred())

				// Verify only 2 nodes remain
				nodes2, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes2).To(HaveLen(2))

				// Verify the right nodes remain
				methodNames := make([]string, len(nodes2))
				for i, node := range nodes2 {
					methodNames[i] = node.MethodName
				}

				Expect(methodNames).To(ContainElement("func1"))
				Expect(methodNames).To(ContainElement("func3"))
				Expect(methodNames).NotTo(ContainElement("func2"))
			})
		})

		Context("when relationships change", func() {
			It("should update relationships and clean up orphans", func() {
				filePath := "test_relationships.go"

				// Create test file for metadata calculation
				testFile := tempDir + "/" + filePath
				err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// Create first analysis result with nodes and relationships
				firstResult := &struct {
					Nodes         []*models.ASTNode
					Relationships []*models.ASTRelationship
					Libraries     []*models.LibraryRelationship
				}{
					Nodes: []*models.ASTNode{
						{
							ID:          1,
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "caller",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   3,
						},
						{
							ID:          2,
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "callee",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   5,
						},
					},
					Relationships: []*models.ASTRelationship{
						{
							FromASTID:        1, // caller calls callee
							ToASTID:          &[]int64{2}[0],
							LineNo:           4,
							RelationshipType: "call",
							Text:             "callee()",
						},
					},
					Libraries: []*models.LibraryRelationship{},
				}

				// Store first result
				err = astCache.StoreFileResults(testFile, firstResult)
				Expect(err).NotTo(HaveOccurred())

				// Get the actual database IDs
				nodes1, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes1).To(HaveLen(2))

				var callerID, calleeID int64
				for _, node := range nodes1 {
					if node.MethodName == "caller" {
						callerID = node.ID
					} else if node.MethodName == "callee" {
						calleeID = node.ID
					}
				}

				// Verify relationship was created
				relationships1, err := astCache.GetASTRelationships(callerID, "call")
				Expect(err).NotTo(HaveOccurred())
				Expect(relationships1).To(HaveLen(1))
				Expect(*relationships1[0].ToASTID).To(Equal(calleeID))

				// Create second analysis result with updated relationships
				secondResult := &struct {
					Nodes         []*models.ASTNode
					Relationships []*models.ASTRelationship
					Libraries     []*models.LibraryRelationship
				}{
					Nodes: []*models.ASTNode{
						{
							ID:          1, // Same nodes
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "caller",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   3,
						},
						{
							ID:          2,
							FilePath:    testFile,
							PackageName: "main",
							TypeName:    "",
							MethodName:  "callee",
							FieldName:   "",
							NodeType:    "method",
							StartLine:   5,
						},
					},
					Relationships: []*models.ASTRelationship{
						{
							FromASTID:        1, // Same relationship but different line
							ToASTID:          &[]int64{2}[0],
							LineNo:           6, // Updated line number
							RelationshipType: "call",
							Text:             "callee()",
						},
					},
					Libraries: []*models.LibraryRelationship{},
				}

				// Store second result
				err = astCache.StoreFileResults(testFile, secondResult)
				Expect(err).NotTo(HaveOccurred())

				// Verify nodes still have same IDs
				nodes2, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes2).To(HaveLen(2))

				for _, node := range nodes2 {
					if node.MethodName == "caller" {
						Expect(node.ID).To(Equal(callerID), "caller ID should be preserved")
					} else if node.MethodName == "callee" {
						Expect(node.ID).To(Equal(calleeID), "callee ID should be preserved")
					}
				}

				// Verify relationship was updated
				relationships2, err := astCache.GetASTRelationships(callerID, "call")
				Expect(err).NotTo(HaveOccurred())
				Expect(relationships2).To(HaveLen(1))
				Expect(relationships2[0].LineNo).To(Equal(6), "relationship line number should be updated")
			})
		})
	})

	Describe("Public cache operations", func() {
		It("should store and retrieve nodes correctly", func() {
			// Create a test node
			testNode := &models.ASTNode{
				FilePath:    "test.go",
				PackageName: "main",
				TypeName:    "TestStruct",
				MethodName:  "TestMethod",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   10,
				EndLine:     15,
			}

			// Store the node
			id, err := astCache.StoreASTNode(testNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(BeNumerically(">", 0))

			// Retrieve nodes by file
			nodes, err := astCache.GetASTNodesByFile("test.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))

			found := nodes[0]
			Expect(found.ID).To(Equal(id))
			Expect(found.FilePath).To(Equal("test.go"))
			Expect(found.PackageName).To(Equal("main"))
			Expect(found.TypeName).To(Equal("TestStruct"))
			Expect(found.MethodName).To(Equal("TestMethod"))
			Expect(found.FieldName).To(Equal(""))
		})
	})
})
