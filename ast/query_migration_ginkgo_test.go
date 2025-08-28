package ast_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("AQL Metric Queries", func() {
	var (
		astCache *cache.ASTCache
		analyzer *ast.Analyzer
	)

	BeforeEach(func() {
		astCache = cache.MustGetASTCache()

		analyzer = ast.NewAnalyzer(astCache, "/test")
	})

	AfterEach(func() {
		if astCache != nil {
			astCache.Close()
		}
	})

	Describe("ExecuteMetricQuery - New Syntax", func() {
		BeforeEach(func() {
			// Insert test data
			node1 := &models.ASTNode{
				FilePath:             "/test/file1.go",
				PackageName:          "controllers",
				TypeName:             "UserController",
				MethodName:           "GetUserByIDWithComplexValidation",
				NodeType:             models.NodeTypeMethod,
				LineCount:            150,
				CyclomaticComplexity: 12,
				ParameterCount:       6,
				ReturnCount:          2,
			}
			id1, err := astCache.StoreASTNode(node1)
			Expect(err).NotTo(HaveOccurred())
			node1.ID = id1

			node2 := &models.ASTNode{
				FilePath:             "/test/file2.go",
				PackageName:          "services",
				TypeName:             "EmailService",
				MethodName:           "Send",
				NodeType:             models.NodeTypeMethod,
				LineCount:            50,
				CyclomaticComplexity: 3,
				ParameterCount:       2,
				ReturnCount:          1,
			}
			id2, err := astCache.StoreASTNode(node2)
			Expect(err).NotTo(HaveOccurred())
			node2.ID = id2

			node3 := &models.ASTNode{
				FilePath:             "/test/file3.go",
				PackageName:          "models",
				TypeName:             "User",
				MethodName:           "Validate",
				NodeType:             models.NodeTypeMethod,
				LineCount:            25,
				CyclomaticComplexity: 5,
				ParameterCount:       0,
				ReturnCount:          1,
			}
			id3, err := astCache.StoreASTNode(node3)
			Expect(err).NotTo(HaveOccurred())
			node3.ID = id3
		})

		It("should find nodes by lines metric", func() {
			nodes, err := analyzer.ExecuteAQLQuery("lines(*) > 100")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].MethodName).To(Equal("GetUserByIDWithComplexValidation"))
		})

		It("should find nodes by cyclomatic complexity", func() {
			nodes, err := analyzer.ExecuteAQLQuery("cyclomatic(*) >= 5")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(2))

			methodNames := []string{nodes[0].MethodName, nodes[1].MethodName}
			Expect(methodNames).To(ContainElement("GetUserByIDWithComplexValidation"))
			Expect(methodNames).To(ContainElement("Validate"))
		})

		It("should find nodes by parameter count using params alias", func() {
			nodes, err := analyzer.ExecuteAQLQuery("params(*) > 4")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].ParameterCount).To(Equal(6))
		})

		It("should find nodes with long names using len metric", func() {
			nodes, err := analyzer.ExecuteAQLQuery("len(*) > 40")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			// controllers:UserController:GetUserByIDWithComplexValidation is > 40 chars
			Expect(nodes[0].MethodName).To(Equal("GetUserByIDWithComplexValidation"))
		})

		It("should work with pattern-specific queries", func() {
			nodes, err := analyzer.ExecuteAQLQuery("lines(services:*) < 100")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].PackageName).To(Equal("services"))
			Expect(nodes[0].MethodName).To(Equal("Send"))
		})
	})

	Describe("ExecuteMetricQuery - Invalid Queries", func() {
		BeforeEach(func() {
			// Add a test node so pattern matching works
			node := &models.ASTNode{
				FilePath:    "/test/file.go",
				PackageName: "test",
				MethodName:  "TestMethod",
				NodeType:    models.NodeTypeMethod,
				LineCount:   10,
			}
			_, err := astCache.StoreASTNode(node)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for unknown metric name", func() {
			_, err := analyzer.ExecuteAQLQuery("unknown(*) > 10")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown metric: unknown"))
		})

		It("should return error for invalid value", func() {
			_, err := analyzer.ExecuteAQLQuery("lines(*) > abc")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid numeric value"))
		})

		It("should return error for old dot syntax", func() {
			_, err := analyzer.ExecuteAQLQuery("*.lines > 100")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid metric query format"))
		})
	})

	Describe("ExecuteMetricQuery - Edge Cases", func() {
		It("should treat function syntax without operator as pattern", func() {
			// Function syntax without operator treated as pattern
			nodes, err := analyzer.ExecuteAQLQuery("lines(*)")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(0)) // Parsed as method name pattern, returns no results
		})

		It("should handle empty parentheses as wildcard", func() {
			// Empty parentheses defaults to wildcard
			nodes, err := analyzer.ExecuteAQLQuery("lines() > 50")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(0)) // Empty pattern defaults to *, no nodes in empty cache
		})
	})

	Describe("ExecuteMetricQuery - Relationship Metrics", func() {
		var node1ID, node2ID int64

		BeforeEach(func() {
			// Insert test nodes
			node1 := &models.ASTNode{
				FilePath:    "/test/file1.go",
				PackageName: "controllers",
				TypeName:    "UserController",
				MethodName:  "GetUser",
				NodeType:    models.NodeTypeMethod,
			}
			var err error
			node1ID, err = astCache.StoreASTNode(node1)
			Expect(err).NotTo(HaveOccurred())

			node2 := &models.ASTNode{
				FilePath:    "/test/file2.go",
				PackageName: "services",
				TypeName:    "UserService",
				MethodName:  "FindUser",
				NodeType:    models.NodeTypeMethod,
			}
			node2ID, err = astCache.StoreASTNode(node2)
			Expect(err).NotTo(HaveOccurred())

			// Add import relationships
			err = astCache.StoreASTRelationship(node1ID, nil, 10, "import", "import services")
			Expect(err).NotTo(HaveOccurred())
			err = astCache.StoreASTRelationship(node1ID, nil, 11, "import", "import models")
			Expect(err).NotTo(HaveOccurred())
			err = astCache.StoreASTRelationship(node1ID, nil, 12, "import", "import utils")
			Expect(err).NotTo(HaveOccurred())

			// Add call relationships (external calls)
			err = astCache.StoreASTRelationship(node1ID, &node2ID, 20, "call", "services.UserService.FindUser()")
			Expect(err).NotTo(HaveOccurred())
			err = astCache.StoreASTRelationship(node1ID, nil, 21, "call", "fmt.Println()")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should count imports correctly", func() {
			nodes, err := analyzer.ExecuteAQLQuery("imports(*) > 2")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].MethodName).To(Equal("GetUser"))
		})

		It("should count external calls correctly", func() {
			nodes, err := analyzer.ExecuteAQLQuery("calls(*) >= 2")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].MethodName).To(Equal("GetUser"))
		})

		It("should find nodes with no imports", func() {
			nodes, err := analyzer.ExecuteAQLQuery("imports(*) == 0")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].MethodName).To(Equal("FindUser"))
		})
	})
})
