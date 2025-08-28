package ast_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/tests/fixtures"
)

var _ = Describe("Analyzer with Fixtures", func() {
	var (
		evaluator *fixtures.CELEvaluator
		tmpDir    string
	)

	BeforeEach(func() {
		var err error
		evaluator, err = fixtures.NewCELEvaluator()
		Expect(err).NotTo(HaveOccurred())

		tmpDir = GinkgoT().TempDir()
	})

	Context("AnalyzeFiles with fixtures", func() {
		DescribeTable("analyzing different code patterns",
			func(files map[string]string, expectedCEL string) {
				// Create test files
				for filename, content := range files {
					filePath := filepath.Join(tmpDir, filename)
					err := os.WriteFile(filePath, []byte(content), 0644)
					Expect(err).NotTo(HaveOccurred())
				}

				// Create AST cache and analyzer
				astCache, err := cache.NewASTCache()
				Expect(err).NotTo(HaveOccurred())
				defer astCache.Close()

				analyzer := ast.NewAnalyzer(astCache, tmpDir)

				// Analyze files
				err = analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())

				// Query all nodes
				nodes, err := analyzer.QueryPattern("*")
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).NotTo(BeEmpty())

				// Evaluate CEL expression
				if expectedCEL != "" && expectedCEL != "true" {
					valid, err := evaluator.EvaluateNodes(expectedCEL, nodes)
					Expect(err).NotTo(HaveOccurred())
					Expect(valid).To(BeTrue(), "CEL validation failed: %s", expectedCEL)
				}
			},
			Entry("Go struct with methods",
				map[string]string{
					"user.go": `package models
type User struct {
	ID   string
	Name string
}

func (u *User) GetID() string {
	return u.ID
}

func (u *User) SetName(name string) {
	u.Name = name
}`,
				},
				`nodes.exists(n, n.type_name == "User") && nodes.exists(n, n.method_name == "GetID")`),

			Entry("Go service with complexity",
				map[string]string{
					"service.go": `package service
type OrderService struct{}

func (s *OrderService) ProcessOrder(order *Order) error {
	if order == nil {
		return errors.New("nil order")
	}
	if order.Items == nil || len(order.Items) == 0 {
		return errors.New("no items")
	}
	for _, item := range order.Items {
		if item.Quantity <= 0 {
			return errors.New("invalid quantity")
		}
		if item.Price < 0 {
			return errors.New("invalid price")
		}
	}
	return nil
}`,
				},
				`nodes.exists(n, n.type_name == "OrderService" && n.cyclomatic_complexity > 1)`),

			Entry("Python class",
				map[string]string{
					"calculator.py": `
class Calculator:
    def __init__(self):
        self.result = 0
    
    def add(self, x, y):
        return x + y
    
    def multiply(self, x, y):
        return x * y`,
				},
				`nodes.exists(n, n.type_name == "Calculator") && nodes.filter(n, n.method_name != "").size() >= 3`),
		)
	})

	Context("QueryPattern with fixtures", func() {
		BeforeEach(func() {
			// Create test files for pattern matching
			files := map[string]string{
				"user_controller.go": `package controllers
type UserController struct{}
func (c *UserController) GetUser(id string) {}
func (c *UserController) CreateUser(name string) {}`,

				"order_controller.go": `package controllers
type OrderController struct{}
func (c *OrderController) GetOrder(id string) {}`,

				"user_service.go": `package services
type UserService struct{}
func (s *UserService) FindUser(id string) {}`,
			}

			for filename, content := range files {
				err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		DescribeTable("pattern matching queries",
			func(pattern string, expectedCount int, celValidation string) {
				// Create AST cache and analyzer
				astCache, err := cache.NewASTCache()
				Expect(err).NotTo(HaveOccurred())
				defer astCache.Close()

				analyzer := ast.NewAnalyzer(astCache, tmpDir)

				// Analyze files
				err = analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())

				// Query with pattern
				nodes, err := analyzer.QueryPattern(pattern)
				Expect(err).NotTo(HaveOccurred())

				// Check count
				Expect(nodes).To(HaveLen(expectedCount))

				// Validate with CEL
				if celValidation != "" && celValidation != "true" {
					valid, err := evaluator.EvaluateNodes(celValidation, nodes)
					Expect(err).NotTo(HaveOccurred())
					Expect(valid).To(BeTrue())
				}
			},
			Entry("All controllers", "*Controller*", 2,
				`nodes.all(n, n.type_name.endsWith("Controller"))`),
			Entry("All services", "*Service*", 1,
				`nodes.all(n, n.type_name.endsWith("Service"))`),
			Entry("Get methods", "*:*:Get*", 2,
				`nodes.all(n, n.method_name.startsWith("Get"))`),
			Entry("User types", "*User*", 2,
				`nodes.all(n, n.type_name.contains("User"))`),
			Entry("All methods", "*:*:*", 5,
				`nodes.all(n, n.node_type == "method")`),
		)
	})

	Context("ExecuteAQLQuery with fixtures", func() {
		BeforeEach(func() {
			// Create test files with various metrics
			files := map[string]string{
				"simple.go": `package main
func simple() { return }`,

				"complex.go": `package main
func complex(x, y, z int) int {
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				switch y {
				case 1: return i
				case 2: return i * 2
				default: return i * z
				}
			}
		}
	}
	return -1
}`,

				"large.go": `package main
func large() {
` + strings.Repeat("\t// Line\n", 150) + `
}`,
			}

			for filename, content := range files {
				err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		DescribeTable("metric queries",
			func(query string, expectedCount int, celValidation string) {
				// Create AST cache and analyzer
				astCache, err := cache.NewASTCache()
				Expect(err).NotTo(HaveOccurred())
				defer astCache.Close()

				analyzer := ast.NewAnalyzer(astCache, tmpDir)

				// Analyze files
				err = analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())

				// Execute AQL query
				nodes, err := analyzer.ExecuteAQLQuery(query)
				Expect(err).NotTo(HaveOccurred())

				// Check count
				Expect(nodes).To(HaveLen(expectedCount))

				// Validate with CEL
				if celValidation != "" && celValidation != "true" {
					valid, err := evaluator.EvaluateNodes(celValidation, nodes)
					Expect(err).NotTo(HaveOccurred())
					Expect(valid).To(BeTrue())
				}
			},
			Entry("High complexity", "cyclomatic(*) > 5", 1,
				`nodes.all(n, n.cyclomatic_complexity > 5)`),
			Entry("Many parameters", "params(*) >= 3", 1,
				`nodes.all(n, n.parameter_count >= 3)`),
			Entry("Large methods", "lines(*) > 100", 1,
				`nodes.all(n, n.line_count > 100)`),
			Entry("Simple methods", "cyclomatic(*) == 1", 2,
				`nodes.all(n, n.cyclomatic_complexity == 1)`),
		)
	})
})
