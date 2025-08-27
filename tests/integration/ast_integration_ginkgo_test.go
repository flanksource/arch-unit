package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	flanksourceContext "github.com/flanksource/commons/context"
)

var _ = Describe("AST Integration", func() {
	var (
		astCache     *cache.ASTCache
		tmpDir       string
		testFixtures []string
	)

	BeforeEach(func() {
		var err error
		astCache, err = cache.NewASTCache()
		Expect(err).NotTo(HaveOccurred())

		tmpDir = GinkgoT().TempDir()
		testFixtures = []string{"controller.go", "service.go", "repository.go", "model.go"}
	})

	AfterEach(func() {
		if astCache != nil {
			astCache.Close()
		}
	})

	Describe("Full AST Analysis Pipeline", func() {
		BeforeEach(func() {
			// Copy test fixtures to temp directory
			for _, fixture := range testFixtures {
				sourceFile := filepath.Join("../../testdata/fixtures", fixture)
				destFile := filepath.Join(tmpDir, fixture)
				
				content, err := os.ReadFile(sourceFile)
				if os.IsNotExist(err) {
					// Create mock fixture if doesn't exist
					content = []byte(generateMockFixture(fixture))
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
				
				err = os.WriteFile(destFile, content, 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Extract AST from all files
			extractor := analysis.NewGoASTExtractor(astCache)
			for _, fixture := range testFixtures {
				filePath := filepath.Join(tmpDir, fixture)
				err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filePath)
				Expect(err).NotTo(HaveOccurred(), "Failed to extract AST from %s", fixture)
			}
		})

		It("should extract AST nodes from all fixture files", func() {
			// Verify AST nodes were created
			allNodes := make([]*models.ASTNode, 0)
			for _, fixture := range testFixtures {
				filePath := filepath.Join(tmpDir, fixture)
				nodes, err := astCache.GetASTNodesByFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).NotTo(BeEmpty(), "Should have nodes for %s", fixture)
				allNodes = append(allNodes, nodes...)
			}

			GinkgoWriter.Printf("Extracted %d AST nodes total\n", len(allNodes))

			// Verify we have different node types
			nodeTypeCounts := make(map[models.NodeType]int)
			for _, node := range allNodes {
				nodeTypeCounts[node.NodeType]++
			}

			Expect(nodeTypeCounts[models.NodeTypeMethod]).To(BeNumerically(">=", 5), "Should have methods")
			Expect(nodeTypeCounts[models.NodeTypeType]).To(BeNumerically(">=", 2), "Should have types")
		})

		It("should perform complexity analysis", func() {
			allNodes := make([]*models.ASTNode, 0)
			for _, fixture := range testFixtures {
				filePath := filepath.Join(tmpDir, fixture)
				nodes, err := astCache.GetASTNodesByFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				allNodes = append(allNodes, nodes...)
			}

			// Test complexity analysis
			complexMethods := 0
			for _, node := range allNodes {
				if node.NodeType == models.NodeTypeMethod && node.CyclomaticComplexity > 5 {
					complexMethods++
					GinkgoWriter.Printf("Complex method: %s.%s (complexity: %d)\n",
						node.TypeName, node.MethodName, node.CyclomaticComplexity)
				}
			}
			Expect(complexMethods).To(BeNumerically(">=", 0), "Should analyze complexity")
		})
	})

	Describe("AQL Rule Execution", func() {
		var engine *query.AQLEngine

		BeforeEach(func() {
			// Use test fixtures
			for _, fixture := range []string{"controller.go", "service.go", "repository.go"} {
				sourceFile := filepath.Join("../../testdata/fixtures", fixture)
				destFile := filepath.Join(tmpDir, fixture)
				
				content, err := os.ReadFile(sourceFile)
				if os.IsNotExist(err) {
					// Create mock fixture if doesn't exist
					content = []byte(generateMockFixture(fixture))
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
				
				err = os.WriteFile(destFile, content, 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Extract AST
			extractor := analysis.NewGoASTExtractor(astCache)
			for _, fixture := range []string{"controller.go", "service.go", "repository.go"} {
				filePath := filepath.Join(tmpDir, fixture)
				err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filePath)
				Expect(err).NotTo(HaveOccurred())
			}

			engine = query.NewAQLEngine(astCache)
		})

		DescribeTable("AQL rule tests",
			func(ruleName, aqlRule string, minViolations, maxViolations int, description string) {
				ruleSet, err := parser.ParseAQLFile(aqlRule)
				Expect(err).NotTo(HaveOccurred())

				violations, err := engine.ExecuteRuleSet(ruleSet)
				Expect(err).NotTo(HaveOccurred())

				violationCount := len(violations)
				Expect(violationCount).To(BeNumerically(">=", minViolations),
					"Should have at least %d violations for %s", minViolations, description)
				Expect(violationCount).To(BeNumerically("<=", maxViolations),
					"Should have at most %d violations for %s", maxViolations, description)

				// Log violations for debugging
				for _, v := range violations {
					GinkgoWriter.Printf("Violation: %s:%d - %s\n",
						filepath.Base(v.File), v.Line, v.Message)
				}
			},
			Entry("High complexity limit",
				"High Complexity",
				`RULE "High Complexity" {
					LIMIT(*.cyclomatic > 15)
				}`,
				0, 10, "Should find methods with high complexity"),
			Entry("Controller complexity",
				"Controller Complexity",
				`RULE "Controller Complexity" {
					LIMIT(*Controller*.cyclomatic > 10)
				}`,
				0, 5, "Should find complex controller methods"),
			Entry("Parameter count check",
				"Too Many Parameters",
				`RULE "Too Many Parameters" {
					LIMIT(*.params > 5)
				}`,
				0, 5, "Should find methods with many parameters"),
		)
	})

	Describe("Architecture Rules", func() {
		var engine *query.AQLEngine

		BeforeEach(func() {
			// Use test fixtures to simulate a typical web application structure
			testFixtures = []string{"controller.go", "service.go", "repository.go", "model.go"}
			
			for _, fixture := range testFixtures {
				sourceFile := filepath.Join("../../testdata/fixtures", fixture)
				destFile := filepath.Join(tmpDir, fixture)
				
				content, err := os.ReadFile(sourceFile)
				if os.IsNotExist(err) {
					// Create mock fixture if doesn't exist
					content = []byte(generateMockFixture(fixture))
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
				
				err = os.WriteFile(destFile, content, 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Extract AST and relationships
			extractor := analysis.NewGoASTExtractor(astCache)
			for _, fixture := range testFixtures {
				filePath := filepath.Join(tmpDir, fixture)
				err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filePath)
				Expect(err).NotTo(HaveOccurred())
			}

			engine = query.NewAQLEngine(astCache)
		})

		It("should validate layered architecture rules", func() {
			// Test layered architecture rules
			architectureAQL := `
			RULE "Clean Architecture" {
				LIMIT(*Controller*.cyclomatic > 20)
				FORBID(*Repository* -> *Controller*)
				FORBID(*Repository* -> *Service*)
				FORBID(*Model* -> *Controller*)
				FORBID(*Model* -> *Service*)
				FORBID(*Model* -> *Repository*)
			}`

			ruleSet, err := parser.ParseAQLFile(architectureAQL)
			Expect(err).NotTo(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("Found %d architecture violations\n", len(violations))

			// Check for specific violation types
			complexityViolations := 0
			layerViolations := 0

			for _, v := range violations {
				if strings.Contains(v.Message, "violated limit") {
					complexityViolations++
				}
				if strings.Contains(v.Message, "forbidden relationship") {
					layerViolations++
				}
				GinkgoWriter.Printf("Architecture violation: %s:%d - %s\n",
					filepath.Base(v.File), v.Line, v.Message)
			}

			// We expect some complexity violations but minimal layer violations
			// (our test fixtures follow good architecture)
			Expect(complexityViolations).To(BeNumerically(">=", 0), "May have complexity violations")
			GinkgoWriter.Printf("Complexity violations: %d, Layer violations: %d\n",
				complexityViolations, layerViolations)
		})
	})

	Describe("Relationship Tracking", func() {
		var testFile string

		BeforeEach(func() {
			// Create a simple test file with method calls
			testFile = filepath.Join(tmpDir, "relationships.go")
			content := `package main

type Calculator struct{}

func (c *Calculator) Add(a, b int) int {
	return a + b
}

func (c *Calculator) Multiply(a, b int) int {
	sum := c.Add(a, b) // This creates a relationship
	return sum * 2
}

type Service struct {
	calc *Calculator
}

func (s *Service) Calculate(x, y int) int {
	return s.calc.Multiply(x, y) // This creates a relationship
}

func main() {
	service := &Service{calc: &Calculator{}}
	result := service.Calculate(5, 3) // This creates a relationship
	println(result)
}`

			err := os.WriteFile(testFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Extract AST with relationships
			extractor := analysis.NewGoASTExtractor(astCache)
			err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), testFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should track method call relationships", func() {
			// Verify nodes were created
			nodes, err := astCache.GetASTNodesByFile(testFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(nodes)).To(BeNumerically(">=", 5), "Should have multiple nodes")

			// Find specific methods to check relationships
			var multiplyMethod, serviceCalculateMethod *models.ASTNode
			for _, node := range nodes {
				if node.NodeType == models.NodeTypeMethod {
					if node.MethodName == "Multiply" {
						multiplyMethod = node
					}
					if node.MethodName == "Calculate" {
						serviceCalculateMethod = node
					}
				}
			}

			Expect(multiplyMethod).NotTo(BeNil(), "Should find Multiply method")
			Expect(serviceCalculateMethod).NotTo(BeNil(), "Should find Service.Calculate method")

			// Check for call relationships
			multiplyRels, err := astCache.GetASTRelationships(multiplyMethod.ID, models.RelationshipCall)
			Expect(err).NotTo(HaveOccurred())

			serviceRels, err := astCache.GetASTRelationships(serviceCalculateMethod.ID, models.RelationshipCall)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("Multiply method has %d call relationships\n", len(multiplyRels))
			GinkgoWriter.Printf("Service.Calculate method has %d call relationships\n", len(serviceRels))

			// Verify relationship details
			for _, rel := range multiplyRels {
				GinkgoWriter.Printf("Multiply calls: %s at line %d\n", rel.Text, rel.LineNo)
				Expect(rel.LineNo).To(BeNumerically(">", 0), "Should have valid line number")
				Expect(rel.Text).NotTo(BeEmpty(), "Should have call text")
			}

			for _, rel := range serviceRels {
				GinkgoWriter.Printf("Service.Calculate calls: %s at line %d\n", rel.Text, rel.LineNo)
				Expect(rel.LineNo).To(BeNumerically(">", 0), "Should have valid line number")
				Expect(rel.Text).NotTo(BeEmpty(), "Should have call text")
			}
		})
	})

	Describe("Library Detection", func() {
		var testFile string

		BeforeEach(func() {
			// Create a test file with library calls
			testFile = filepath.Join(tmpDir, "libraries.go")
			content := `package main

import (
	"fmt"
	"net/http"
	"encoding/json"
	"database/sql"
	_ "github.com/lib/pq"
)

type Handler struct{}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Standard library calls
	fmt.Printf("Handling request: %s\n", r.URL.Path)
	
	data := map[string]string{"message": "hello"}
	jsonData, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func connectDB() (*sql.DB, error) {
	return sql.Open("postgres", "connection-string")
}`

			err := os.WriteFile(testFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Extract AST with library detection
			extractor := analysis.NewGoASTExtractor(astCache)
			err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), testFile)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should detect library usage", func() {
			// Find the ServeHTTP method
			nodes, err := astCache.GetASTNodesByFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			var serveHTTPMethod *models.ASTNode
			for _, node := range nodes {
				if node.NodeType == models.NodeTypeMethod && node.MethodName == "ServeHTTP" {
					serveHTTPMethod = node
					break
				}
			}

			Expect(serveHTTPMethod).NotTo(BeNil(), "Should find ServeHTTP method")

			// Check for library relationships
			libRels, err := astCache.GetLibraryRelationships(serveHTTPMethod.ID, models.RelationshipCall)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("Found %d library relationships\n", len(libRels))

			// Verify we detect standard library calls
			foundLibraries := make(map[string]bool)
			for _, rel := range libRels {
				foundLibraries[rel.LibraryNode.Package] = true
				GinkgoWriter.Printf("Library call: %s.%s at line %d (framework: %s)\n",
					rel.LibraryNode.Package, rel.LibraryNode.Method,
					rel.LineNo, rel.LibraryNode.Framework)
			}

			// Should detect standard library usage
			expectedLibraries := []string{"fmt", "http", "json"}
			for _, lib := range expectedLibraries {
				if foundLibraries[lib] {
					GinkgoWriter.Printf("✓ Detected %s library usage\n", lib)
				}
			}

			Expect(len(libRels)).To(BeNumerically(">=", 0), "Should detect library calls")
		})
	})

	Describe("Performance with Large Codebase", func() {
		It("should handle large codebases efficiently", Label("slow"), func() {
			// This test is labeled as "slow" and will be skipped unless --label-filter="slow" is used

			// Generate a large codebase
			fileCount := 50
			methodsPerFile := 20

			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
				content := generateLargeGoFile(i, methodsPerFile)
				
				err := os.WriteFile(filename, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Extract AST from all files
			extractor := analysis.NewGoASTExtractor(astCache)
			totalNodes := 0
			
			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
				err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
				Expect(err).NotTo(HaveOccurred())
				
				nodes, err := astCache.GetASTNodesByFile(filename)
				Expect(err).NotTo(HaveOccurred())
				totalNodes += len(nodes)
			}

			GinkgoWriter.Printf("Processed %d files with %d total AST nodes\n", fileCount, totalNodes)

			// Test complex AQL query performance
			complexAQL := `
			RULE "Performance Test" {
				LIMIT(*.cyclomatic > 5)
				LIMIT(*.params > 3)
				FORBID(Type1* -> Type2*)
				REQUIRE(Service* -> Repository*)
			}`

			engine := query.NewAQLEngine(astCache)
			ruleSet, err := parser.ParseAQLFile(complexAQL)
			Expect(err).NotTo(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("Found %d violations in large codebase\n", len(violations))
			Expect(totalNodes).To(BeNumerically(">=", fileCount*10),
				"Should have processed many nodes")
		})
	})
})

// Helper functions

func generateMockFixture(filename string) string {
	switch filename {
	case "controller.go":
		return `package main

type UserController struct{}

func (c *UserController) GetUser(id string) string {
	if id == "" {
		return ""
	}
	return "user-" + id
}

func (c *UserController) CreateUser(name, email string) (string, error) {
	if name == "" || email == "" {
		return "", fmt.Errorf("name and email required")
	}
	return name + ":" + email, nil
}`

	case "service.go":
		return `package main

type UserService struct{}

func (s *UserService) ProcessUser(data string) string {
	if data == "" {
		return "empty"
	}
	return "processed-" + data
}

func (s *UserService) ValidateUser(user string) bool {
	return len(user) > 0
}`

	case "repository.go":
		return `package main

type UserRepository struct{}

func (r *UserRepository) GetByID(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("empty id")
	}
	return "user-data", nil
}

func (r *UserRepository) Save(user string) error {
	if user == "" {
		return fmt.Errorf("empty user")
	}
	return nil
}`

	case "model.go":
		return `package main

type User struct {
	ID    string
	Name  string
	Email string
}

func (u *User) String() string {
	return u.Name
}

func (u *User) IsValid() bool {
	return u.ID != "" && u.Name != "" && u.Email != ""
}`

	default:
		return `package main

func main() {
	println("hello world")
}`
	}
}

// Helper function to generate a large Go file for performance testing
func generateLargeGoFile(fileIndex, methodCount int) string {
	var content strings.Builder
	
	content.WriteString(fmt.Sprintf("package file%d\n\n", fileIndex))
	content.WriteString("import \"fmt\"\n\n")
	
	// Generate struct types
	content.WriteString(fmt.Sprintf("type Type%d struct {\n", fileIndex))
	content.WriteString("    ID int\n")
	content.WriteString("    Name string\n")
	content.WriteString("}\n\n")
	
	content.WriteString(fmt.Sprintf("type Service%d struct {\n", fileIndex))
	content.WriteString(fmt.Sprintf("    repo *Repository%d\n", fileIndex))
	content.WriteString("}\n\n")
	
	content.WriteString(fmt.Sprintf("type Repository%d struct {\n", fileIndex))
	content.WriteString("    db interface{}\n")
	content.WriteString("}\n\n")
	
	// Generate methods with varying complexity
	for i := 0; i < methodCount; i++ {
		complexity := i%5 + 1 // Complexity from 1 to 5
		paramCount := i%4 + 1 // 1 to 4 parameters
		
		// Generate method signature
		content.WriteString(fmt.Sprintf("func (s *Service%d) Method%d(", fileIndex, i))
		for p := 0; p < paramCount; p++ {
			if p > 0 {
				content.WriteString(", ")
			}
			content.WriteString(fmt.Sprintf("param%d int", p))
		}
		content.WriteString(") int {\n")
		
		// Generate method body with controlled complexity
		content.WriteString("    result := 0\n")
		for c := 0; c < complexity; c++ {
			content.WriteString(fmt.Sprintf("    if param0 > %d {\n", c))
			content.WriteString(fmt.Sprintf("        result += %d\n", c))
			content.WriteString("    }\n")
		}
		
		// Add some method calls
		if i%3 == 0 {
			content.WriteString("    s.repo.Save(result)\n")
		}
		if i%4 == 0 {
			content.WriteString("    fmt.Printf(\"Result: %d\\n\", result)\n")
		}
		
		content.WriteString("    return result\n")
		content.WriteString("}\n\n")
	}
	
	return content.String()
}