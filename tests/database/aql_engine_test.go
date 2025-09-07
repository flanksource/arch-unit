package database_test_suite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	flanksourceContext "github.com/flanksource/commons/context"
)

var _ = Describe("AQL Engine", func() {
	var astCache *cache.ASTCache
	var engine *query.AQLEngine

	BeforeEach(func() {
		// For AQL tests, we need to use the cache singleton but with our test DB
		// Reset the cache first to ensure clean state
		cache.ResetGormDB()

		// Override the cache directory to use our test DB path
		var err error
		astCache, err = cache.NewASTCacheWithPath(testDB.TempDir())
		Expect(err).ToNot(HaveOccurred())

		engine = query.NewAQLEngine(astCache)

		// Setup test data for AQL tests directly in the ASTCache
		testDB.SetupAQLTestDataInCache(astCache)
	})

	Context("LIMIT Statements", func() {
		It("should find methods with complexity greater than 10", func() {
			yaml := `
rules:
  - name: "High Complexity"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "*"
            metric: "cyclomatic"
          operator: ">"
          value: 10
`

			ruleSet, err := parser.LoadAQLFromYAML(yaml)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(1)) // Only ComplexController.ProcessOrder has complexity > 10

			// Verify violation details
			for _, violation := range violations {
				Expect(violation.File).ToNot(BeEmpty())
				Expect(violation.Line).To(BeNumerically(">", 0))
				Expect(violation.Message).ToNot(BeEmpty())
				Expect(violation.Source).To(Equal("aql"))
			}
		})

		It("should find methods with complexity less than 5", func() {
			aql := `RULE "Low Complexity" {
				LIMIT(*.cyclomatic < 5)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(3)) // SimpleController.GetUser(2), UserRepository.Save(1), and UserService.CreateUser should not match (5)
		})

		It("should find complex controller methods", func() {
			aql := `RULE "Complex Controllers" {
				LIMIT(*Controller*.cyclomatic > 5)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(1)) // Only ComplexController.ProcessOrder
		})

		It("should find service methods meeting complexity threshold", func() {
			aql := `RULE "Service Complexity" {
				LIMIT(*Service*.cyclomatic >= 5)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(1)) // UserService.CreateUser has complexity 5
		})

		It("should find methods with too many parameters", func() {
			aql := `RULE "Too Many Parameters" {
				LIMIT(*.params > 2)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(1)) // ComplexController.ProcessOrder has 3 parameters
		})
	})

	Context("FORBID Statements", func() {
		It("should not find direct controller to repository calls", func() {
			aql := `RULE "Layer Violation" {
				FORBID(*Controller* -> *Repository*)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(0)) // No direct controller->repository relationships in test data
		})

		It("should not find service to controller calls", func() {
			aql := `RULE "Reverse Dependency" {
				FORBID(*Service* -> *Controller*)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(0)) // No service->controller relationships in test data
		})

		It("should enforce model isolation", func() {
			aql := `RULE "Model Isolation" {
				FORBID(*model* -> *)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(0)) // Model layer should not have outgoing calls
		})
	})

	Context("REQUIRE Statements", func() {
		It("should validate controllers use services", func() {
			aql := `RULE "Controller Service Dependency" {
				REQUIRE(*Controller* -> *Service*)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(0)) // Both controllers use services, so no violations
		})

		It("should validate services use repositories", func() {
			aql := `RULE "Service Repository Dependency" {
				REQUIRE(*Service* -> *Repository*)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(0)) // UserService uses UserRepository, so no violations
		})

		It("should create violations for missing controller-model dependencies", func() {
			aql := `RULE "Controller Model Dependency" {
				REQUIRE(*Controller* -> *model*)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(2)) // Neither controller directly uses models

			// Verify violation structure for missing dependencies
			for _, violation := range violations {
				Expect(violation.File).ToNot(BeEmpty())
				Expect(violation.Line).To(BeNumerically(">", 0))
				Expect(violation.Message).To(ContainSubstring("Required relationship from"))
				Expect(violation.Source).To(Equal("aql"))
			}
		})
	})

	Context("Complex Rules", func() {
		It("should handle multiple rule types in one ruleset", func() {
			aql := `RULE "Architecture Compliance" {
				LIMIT(*Controller*.cyclomatic > 15)
				FORBID(*Controller* -> *Repository*)
				REQUIRE(*Controller* -> *Service*)
				REQUIRE(*Service* -> *Repository*)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())

			// Should have violations for:
			// 1. ComplexController.ProcessOrder exceeds complexity limit
			Expect(violations).ToNot(BeEmpty())

			// Verify we have the complexity violation
			hasComplexityViolation := false
			for _, violation := range violations {
				if strings.Contains(violation.Message, "violated limit") {
					hasComplexityViolation = true
				}
			}
			Expect(hasComplexityViolation).To(BeTrue(), "Should have complexity violation")
		})
	})

	Context("Pattern Matching", func() {
		DescribeTable("Pattern matching tests",
			func(pattern string, expectedMin int, description string) {
				aql := `RULE "Pattern Test" {
					LIMIT(` + pattern + `.cyclomatic >= 0)
				}`

				ruleSet, err := parser.ParseAQLFile(aql)
				Expect(err).ToNot(HaveOccurred())

				violations, err := engine.ExecuteRuleSet(ruleSet)
				Expect(err).ToNot(HaveOccurred())

				Expect(len(violations)).To(BeNumerically(">=", expectedMin), description)
			},
			Entry("Wildcard should match all methods", "*", 4, "Wildcard should match all methods"),
			Entry("Should match controller methods", "*Controller*", 2, "Should match controller methods"),
			Entry("Should match service methods", "*Service*", 1, "Should match service methods"),
			Entry("Should match repository methods", "*Repository*", 1, "Should match repository methods"),
			Entry("Should match methods in controller package", "controller.*", 2, "Should match methods in controller package"),
			Entry("Should match UserService type", "*.UserService", 1, "Should match UserService type"),
			Entry("Should match specific method", "*.UserService:CreateUser", 1, "Should match specific method"),
		)
	})

	Context("Error Handling", func() {
		It("should handle empty rule set", func() {
			emptyRuleSet := &models.AQLRuleSet{Rules: []*models.AQLRule{}}
			violations, err := engine.ExecuteRuleSet(emptyRuleSet)
			Expect(err).ToNot(HaveOccurred())
			Expect(violations).To(HaveLen(0))
		})

		It("should handle nil rule set", func() {
			violations, err := engine.ExecuteRuleSet(nil)
			Expect(err).To(HaveOccurred())
			Expect(violations).To(BeNil())
		})
	})

	Context("Real Go Code Integration", func() {
		It("should work with extracted AST from real Go code", func() {
			// Create real Go files for testing
			tmpDir := GinkgoT().TempDir()

			// Controller file
			controllerFile := filepath.Join(tmpDir, "controller.go")
			controllerContent := `package controller

import "fmt"

type UserController struct{}

func (c *UserController) GetUser(id string) (*User, error) {
	if id == "" {
		return nil, fmt.Errorf("invalid id")
	}
	// Simple logic - complexity should be 2
	return &User{ID: id}, nil
}

func (c *UserController) ComplexMethod(a, b, c int) int {
	result := 0
	for i := 0; i < a; i++ {
		if i%2 == 0 {
			switch b {
			case 1:
				result += i
			case 2:
				result += i * 2
			default:
				result += i * c
			}
		} else {
			result += i
		}
	}
	// This should have high complexity
	return result
}`

			err := os.WriteFile(controllerFile, []byte(controllerContent), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Extract AST using real extractor
			extractor := analysis.NewGoASTExtractor(astCache)
			err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), controllerFile)
			Expect(err).ToNot(HaveOccurred())

			// Test AQL rules on real extracted data
			aql := `RULE "Real Code Test" {
				LIMIT(*Controller*.cyclomatic > 5)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())

			// Should find the ComplexMethod as a violation
			Expect(violations).ToNot(BeEmpty())

			// Verify violation details
			found := false
			for _, violation := range violations {
				if strings.Contains(violation.Message, "ComplexMethod") || strings.Contains(violation.File, controllerFile) {
					found = true
					Expect(violation.Line).To(BeNumerically(">", 0))
					Expect(violation.Source).To(Equal("aql"))
				}
			}
			Expect(found).To(BeTrue(), "Should find ComplexMethod violation")
		})
	})

	Context("Performance", func() {
		It("should handle large datasets efficiently", func() {
			// Create many test nodes for performance testing
			nodeCount := 1000
			testDB.CreateManyTestASTNodesInCache(astCache, nodeCount)

			// Test complex rule performance
			aql := `RULE "Performance Test" {
				LIMIT(*.cyclomatic > 15)
				FORBID(Type1* -> Type2*)
				REQUIRE(Type3* -> Type4*)
			}`

			start := time.Now()
			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).ToNot(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).ToNot(HaveOccurred())
			elapsed := time.Since(start)

			GinkgoWriter.Printf("Processed %d nodes in %v, found %d violations", nodeCount, elapsed, len(violations))

			// Performance should be reasonable (less than 5 seconds for 1000 nodes)
			Expect(elapsed).To(BeNumerically("<", 5*time.Second), "Query should complete within 5 seconds")
			Expect(violations).ToNot(BeEmpty(), "Should find some violations")
		})
	})
})
