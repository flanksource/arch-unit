package cache_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("ASTCache", func() {
	var (
		astCache *cache.ASTCache
		tmpDir   string
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		var err error
		astCache, err = cache.NewASTCacheWithPath(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if astCache != nil {
			astCache.Close()
		}
	})

	Describe("NewASTCache", func() {
		It("should create a new cache successfully", func() {
			Expect(astCache).NotTo(BeNil())
		})
	})

	Describe("StoreAndRetrieveNodes", func() {
		var testNode *models.ASTNode
		var nodeID int64

		BeforeEach(func() {
			testNode = &models.ASTNode{
				FilePath:             "/test/example.go",
				PackageName:          "main",
				TypeName:             "TestStruct",
				MethodName:           "TestMethod",
				NodeType:             models.NodeTypeMethod,
				StartLine:            10,
				EndLine:              20,
				LineCount:            11,
				CyclomaticComplexity: 5,
				LastModified:         time.Now(),
				FileHash:             "abc123",
			}

			var err error
			nodeID, err = astCache.StoreASTNode(testNode)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should store node and return valid ID", func() {
			Expect(nodeID).To(BeNumerically(">", 0))
		})

		It("should retrieve stored node correctly", func() {
			retrieved, err := astCache.GetASTNode(nodeID)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.FilePath).To(Equal(testNode.FilePath))
			Expect(retrieved.PackageName).To(Equal(testNode.PackageName))
			Expect(retrieved.TypeName).To(Equal(testNode.TypeName))
			Expect(retrieved.MethodName).To(Equal(testNode.MethodName))
			Expect(retrieved.CyclomaticComplexity).To(Equal(testNode.CyclomaticComplexity))
		})
	})

	Describe("FileHashValidation", func() {
		var testFile string

		BeforeEach(func() {
			testFileDir := GinkgoT().TempDir()
			testFile = filepath.Join(testFileDir, "test.go")
			content := `package main
func main() {
	println("hello")
}`
			err := os.WriteFile(testFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should detect when file needs analysis", func() {
			// First analysis - file needs analysis
			needs, err := astCache.NeedsReanalysis(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(needs).To(BeTrue())
		})

		It("should not need reanalysis after updating metadata", func() {
			// Update file metadata
			err := astCache.UpdateFileMetadata(testFile)
			Expect(err).NotTo(HaveOccurred())

			// Second check - file shouldn't need reanalysis
			needs, err := astCache.NeedsReanalysis(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(needs).To(BeFalse())
		})

		It("should need reanalysis after file modification", func() {
			// Update file metadata first
			err := astCache.UpdateFileMetadata(testFile)
			Expect(err).NotTo(HaveOccurred())

			// Modify file
			time.Sleep(10 * time.Millisecond) // Ensure different mtime
			modifiedContent := `package main
func main() {
	println("hello world")
}`
			err = os.WriteFile(testFile, []byte(modifiedContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			// File should need reanalysis after modification
			needs, err := astCache.NeedsReanalysis(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(needs).To(BeTrue())
		})
	})

	Describe("Relationships", func() {
		var fromNodeID, toNodeID int64

		BeforeEach(func() {
			// Create two nodes
			fromNode := &models.ASTNode{
				FilePath:    "/test/example.go",
				PackageName: "main",
				MethodName:  "caller",
				NodeType:    models.NodeTypeMethod,
				StartLine:   1,
				EndLine:     5,
			}
			toNode := &models.ASTNode{
				FilePath:    "/test/example.go",
				PackageName: "main",
				MethodName:  "callee",
				NodeType:    models.NodeTypeMethod,
				StartLine:   10,
				EndLine:     15,
			}

			var err error
			fromNodeID, err = astCache.StoreASTNode(fromNode)
			Expect(err).NotTo(HaveOccurred())
			toNodeID, err = astCache.StoreASTNode(toNode)
			Expect(err).NotTo(HaveOccurred())

			// Store relationship
			err = astCache.StoreASTRelationship(fromNodeID, &toNodeID, 3, models.RelationshipCall, "callee()")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should store and retrieve relationships correctly", func() {
			// Retrieve relationships
			rels, err := astCache.GetASTRelationships(fromNodeID, models.RelationshipCall)
			Expect(err).NotTo(HaveOccurred())
			Expect(rels).To(HaveLen(1))
			Expect(rels[0].LineNo).To(Equal(3))
			Expect(rels[0].Text).To(Equal("callee()"))
			Expect(*rels[0].ToASTID).To(Equal(toNodeID))
		})
	})

	Describe("LibraryNodes", func() {
		var libID, nodeID int64

		BeforeEach(func() {
			// Store library node
			var err error
			libID, err = astCache.StoreLibraryNode("fmt", "", "Printf", "", models.NodeTypeMethod, "go", "stdlib")
			Expect(err).NotTo(HaveOccurred())
			Expect(libID).To(BeNumerically(">", 0))

			// Store AST node that uses library
			node := &models.ASTNode{
				FilePath:    "/test/example.go",
				PackageName: "main",
				MethodName:  "main",
				NodeType:    models.NodeTypeMethod,
				StartLine:   1,
				EndLine:     5,
			}
			nodeID, err = astCache.StoreASTNode(node)
			Expect(err).NotTo(HaveOccurred())

			// Store library relationship
			err = astCache.StoreLibraryRelationship(nodeID, libID, 3, models.RelationshipCall, "fmt.Printf()")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should store and retrieve library relationships correctly", func() {
			// Retrieve library relationships
			libRels, err := astCache.GetLibraryRelationships(nodeID, models.RelationshipCall)
			Expect(err).NotTo(HaveOccurred())
			Expect(libRels).To(HaveLen(1))
			Expect(libRels[0].LineNo).To(Equal(3))
			Expect(libRels[0].Text).To(Equal("fmt.Printf()"))
			Expect(libRels[0].LibraryNode.Package).To(Equal("fmt"))
			Expect(libRels[0].LibraryNode.Method).To(Equal("Printf"))
		})
	})

	Describe("DeleteASTForFile", func() {
		var filePath string
		var nodeID1, nodeID2 int64

		BeforeEach(func() {
			filePath = "/test/example.go"

			// Store multiple nodes for the same file
			node1 := &models.ASTNode{
				FilePath:    filePath,
				PackageName: "main",
				MethodName:  "method1",
				NodeType:    models.NodeTypeMethod,
				StartLine:   1,
				EndLine:     5,
			}
			node2 := &models.ASTNode{
				FilePath:    filePath,
				PackageName: "main",
				MethodName:  "method2",
				NodeType:    models.NodeTypeMethod,
				StartLine:   10,
				EndLine:     15,
			}

			var err error
			nodeID1, err = astCache.StoreASTNode(node1)
			Expect(err).NotTo(HaveOccurred())
			nodeID2, err = astCache.StoreASTNode(node2)
			Expect(err).NotTo(HaveOccurred())

			// Store relationship between nodes
			err = astCache.StoreASTRelationship(nodeID1, &nodeID2, 3, models.RelationshipCall, "method2()")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should verify nodes exist before deletion", func() {
			nodes, err := astCache.GetASTNodesByFile(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(2))
		})

		It("should delete all AST data for file", func() {
			err := astCache.DeleteASTForFile(filePath)
			Expect(err).NotTo(HaveOccurred())

			// Verify nodes are deleted
			nodes, err := astCache.GetASTNodesByFile(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(0))

			// Verify relationships are also deleted
			rels, err := astCache.GetASTRelationships(nodeID1, models.RelationshipCall)
			Expect(err).NotTo(HaveOccurred())
			Expect(rels).To(HaveLen(0))
		})
	})

	Describe("QueryRaw", func() {
		BeforeEach(func() {
			// Store test nodes with different complexity levels
			nodes := []*models.ASTNode{
				{
					FilePath:             "/test/low.go",
					MethodName:           "simple",
					NodeType:             models.NodeTypeMethod,
					CyclomaticComplexity: 1,
					StartLine:            1,
					EndLine:              5,
				},
				{
					FilePath:             "/test/high.go",
					MethodName:           "complex",
					NodeType:             models.NodeTypeMethod,
					CyclomaticComplexity: 15,
					StartLine:            1,
					EndLine:              30,
				},
			}

			for _, node := range nodes {
				_, err := astCache.StoreASTNode(node)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should query for high complexity methods", func() {
			rows, err := astCache.QueryRaw("SELECT method_name, cyclomatic_complexity FROM ast_nodes WHERE cyclomatic_complexity > ?", 10)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			var results []struct {
				Method     string
				Complexity int
			}
			for rows.Next() {
				var result struct {
					Method     string
					Complexity int
				}
				err := rows.Scan(&result.Method, &result.Complexity)
				Expect(err).NotTo(HaveOccurred())
				results = append(results, result)
			}

			Expect(results).To(HaveLen(1))
			Expect(results[0].Method).To(Equal("complex"))
			Expect(results[0].Complexity).To(Equal(15))
		})
	})

	Describe("ConcurrentAccess", func() {
		It("should handle concurrent node storage", func() {
			// Test concurrent node storage
			done := make(chan bool, 10)
			for i := 0; i < 10; i++ {
				go func(id int) {
					defer GinkgoRecover()
					node := &models.ASTNode{
						FilePath:   "/test/concurrent.go",
						MethodName: fmt.Sprintf("method%d", id),
						NodeType:   models.NodeTypeMethod,
						StartLine:  id * 10,
						EndLine:    id*10 + 5,
					}
					_, err := astCache.StoreASTNode(node)
					Expect(err).NotTo(HaveOccurred())
					done <- true
				}(i)
			}

			// Wait for all goroutines to complete
			for i := 0; i < 10; i++ {
				<-done
			}

			// Verify all nodes were stored
			nodes, err := astCache.GetASTNodesByFile("/test/concurrent.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(10))
		})
	})
})
