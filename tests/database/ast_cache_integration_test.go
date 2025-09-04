package database_test_suite

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

const (
	testFilePath        = "/test/example.go"
	testPackageNameAST  = "main"
	testMethodNameAST   = "TestMethod"
	testComplexityAST   = 5
	testLibraryPackage  = "fmt"
	testLibraryMethod   = "Printf"
)

var _ = Describe("AST Cache Integration with GORM", func() {
	Context("AST Node GORM Operations", func() {
		It("should store and retrieve AST nodes through GORM", func() {
			node := testDB.CreateTestASTNode(func(n *models.ASTNode) {
				n.FilePath = testFilePath
				n.PackageName = testPackageNameAST
				n.MethodName = testMethodNameAST
				n.NodeType = models.NodeTypeMethod
				n.CyclomaticComplexity = testComplexityAST
			})

			Expect(node.ID).To(BeNumerically(">", 0))

			var retrieved models.ASTNode
			result := testDB.DB().First(&retrieved, node.ID)
			Expect(result.Error).ToNot(HaveOccurred())
			
			Expect(retrieved.FilePath).To(Equal(testFilePath))
			Expect(retrieved.MethodName).To(Equal(testMethodNameAST))
			Expect(retrieved.CyclomaticComplexity).To(Equal(testComplexityAST))
		})

		It("should handle batch AST node creation", func() {
			const numNodes = 5
			var nodes []*models.ASTNode

			for i := 0; i < numNodes; i++ {
				node := testDB.CreateTestASTNode(func(n *models.ASTNode) {
					n.FilePath = fmt.Sprintf("/test/file%d.go", i)
					n.MethodName = fmt.Sprintf("method%d", i)
					n.StartLine = i * 10
					n.EndLine = i*10 + 5
				})
				nodes = append(nodes, node)
			}

			// Verify all nodes were created
			var count int64
			testDB.DB().Model(&models.ASTNode{}).Count(&count)
			Expect(count).To(Equal(int64(numNodes)))

			// Verify each node can be retrieved
			for _, originalNode := range nodes {
				var retrieved models.ASTNode
				result := testDB.DB().First(&retrieved, originalNode.ID)
				Expect(result.Error).ToNot(HaveOccurred())
				Expect(retrieved.MethodName).To(Equal(originalNode.MethodName))
			}
		})
	})

	Context("Library Node Integration", func() {
		It("should create library nodes and relationships", func() {
			// Create library node
			libNode := testDB.CreateTestLibraryNode(func(ln *models.LibraryNode) {
				ln.Package = testLibraryPackage
				ln.Method = testLibraryMethod
				ln.NodeType = "method"
				ln.Language = "go"
			})

			Expect(libNode.ID).To(BeNumerically(">", 0))

			// Create AST node
			astNode := testDB.CreateTestASTNode(func(n *models.ASTNode) {
				n.FilePath = testFilePath
				n.MethodName = "main"
			})

			// Create library relationship
			relationship := &models.LibraryRelationship{
				ASTID:            astNode.ID,
				LibraryID:        libNode.ID,
				LineNo:           3,
				RelationshipType: string(models.RelationshipCall),
				Text:             fmt.Sprintf("%s.%s()", testLibraryPackage, testLibraryMethod),
			}

			result := testDB.DB().Create(relationship)
			Expect(result.Error).ToNot(HaveOccurred())

			// Verify relationship exists
			var retrievedRel models.LibraryRelationship
			result = testDB.DB().Preload("LibraryNode").First(&retrievedRel, relationship.ID)
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(retrievedRel.Text).To(Equal(fmt.Sprintf("%s.%s()", testLibraryPackage, testLibraryMethod)))
			Expect(retrievedRel.LibraryNode.Package).To(Equal(testLibraryPackage))
		})
	})

	Context("Complex Queries", func() {
		BeforeEach(func() {
			// Set up test data for complex queries
			complexityLevels := []int{1, 5, 10, 15, 20}
			
			for i, complexity := range complexityLevels {
				testDB.CreateTestASTNode(func(n *models.ASTNode) {
					n.FilePath = fmt.Sprintf("/test/complexity%d.go", i)
					n.MethodName = fmt.Sprintf("method%d", i)
					n.CyclomaticComplexity = complexity
					n.NodeType = models.NodeTypeMethod
				})
			}
		})

		It("should query nodes by complexity range", func() {
			var highComplexityNodes []models.ASTNode
			result := testDB.DB().Where("cyclomatic_complexity > ?", 10).Find(&highComplexityNodes)
			
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(highComplexityNodes).To(HaveLen(2)) // complexity 15 and 20
			
			for _, node := range highComplexityNodes {
				Expect(node.CyclomaticComplexity).To(BeNumerically(">", 10))
			}
		})

		It("should aggregate complexity statistics", func() {
			var result struct {
				MaxComplexity float64
				AvgComplexity float64
				Count         int64
			}

			err := testDB.DB().Model(&models.ASTNode{}).
				Select("MAX(cyclomatic_complexity) as max_complexity, AVG(cyclomatic_complexity) as avg_complexity, COUNT(*) as count").
				Scan(&result).Error

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Count).To(Equal(int64(5)))
			Expect(result.MaxComplexity).To(Equal(float64(20)))
			Expect(result.AvgComplexity).To(Equal(float64(10.2))) // (1+5+10+15+20)/5
		})
	})

	Context("Data Relationships", func() {
		It("should maintain referential integrity", func() {
			// Create two AST nodes
			node1 := testDB.CreateTestASTNode(func(n *models.ASTNode) {
				n.FilePath = testFilePath
				n.MethodName = "caller"
			})

			node2 := testDB.CreateTestASTNode(func(n *models.ASTNode) {
				n.FilePath = testFilePath
				n.MethodName = "callee"
			})

			// Create relationship between them
			relationship := &models.ASTRelationship{
				FromASTID:        node1.ID,
				ToASTID:          &node2.ID,
				LineNo:           5,
				RelationshipType: models.RelationshipCall,
				Text:             "callee()",
			}

			result := testDB.DB().Create(relationship)
			Expect(result.Error).ToNot(HaveOccurred())

			// Verify relationship can be queried with joins
			var relationships []models.ASTRelationship
			result = testDB.DB().Where("from_ast_id = ?", node1.ID).Find(&relationships)
			
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(relationships).To(HaveLen(1))
			Expect(relationships[0].Text).To(Equal("callee()"))
		})
	})
})
