package performance_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

var _ = Describe("AST Performance", Label("slow"), func() {
	var (
		astCache *cache.ASTCache
	)

	BeforeEach(func() {
		var err error
		astCache, err = cache.NewASTCache()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if astCache != nil {
			astCache.Close()
		}
	})

	Describe("Cache Operations", func() {
		It("should store AST nodes efficiently", func() {
			node := &models.ASTNode{
				FilePath:             "/test/benchmark.go",
				PackageName:          "test",
				TypeName:             "TestType",
				MethodName:           "TestMethod",
				NodeType:             models.NodeTypeMethod,
				StartLine:            1,
				EndLine:              10,
				CyclomaticComplexity: 5,
				LastModified:         time.Now(),
			}

			startTime := time.Now()
			for i := 0; i < 1000; i++ {
				node.ID = 0 // Reset ID for each iteration
				node.MethodName = fmt.Sprintf("TestMethod%d", i)
				_, err := astCache.StoreASTNode(node)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 5.0), "Should store 1000 nodes within 5 seconds")
		})

		It("should retrieve AST nodes efficiently", func() {
			// Pre-populate with nodes
			nodeIDs := make([]int64, 100)
			for i := 0; i < 100; i++ {
				node := &models.ASTNode{
					FilePath:             fmt.Sprintf("/test/file%d.go", i),
					PackageName:          "test",
					MethodName:           fmt.Sprintf("Method%d", i),
					NodeType:             models.NodeTypeMethod,
					StartLine:            i,
					EndLine:              i + 10,
					CyclomaticComplexity: i % 20,
					LastModified:         time.Now(),
				}
				id, err := astCache.StoreASTNode(node)
				Expect(err).NotTo(HaveOccurred())
				nodeIDs[i] = id
			}

			startTime := time.Now()
			for i := 0; i < 100; i++ {
				nodeID := nodeIDs[i%100]
				_, err := astCache.GetASTNode(nodeID)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 1.0), "Should retrieve 100 nodes within 1 second")
		})
	})

	Describe("AST Extraction", func() {
		It("should extract small Go files efficiently", func() {
			extractor := analysis.NewGoASTExtractor(astCache)

			// Create small Go file
			tmpDir := GinkgoT().TempDir()
			smallFile := filepath.Join(tmpDir, "small.go")
			content := `package test

type User struct {
	Name string
	Age  int
}

func (u *User) String() string {
	return u.Name
}

func NewUser(name string, age int) *User {
	return &User{Name: name, Age: age}
}`

			err := os.WriteFile(smallFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Clear cache before extraction
			err = astCache.DeleteASTForFile(smallFile)
			Expect(err).NotTo(HaveOccurred())

			startTime := time.Now()
			err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), smallFile)
			Expect(err).NotTo(HaveOccurred())
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 1.0), "Should extract small file within 1 second")
		})

		It("should extract medium Go files efficiently", func() {
			extractor := analysis.NewGoASTExtractor(astCache)

			// Create medium-sized Go file
			tmpDir := GinkgoT().TempDir()
			mediumFile := filepath.Join(tmpDir, "medium.go")
			content := generateMediumGoFile()

			err := os.WriteFile(mediumFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Clear cache before extraction
			err = astCache.DeleteASTForFile(mediumFile)
			Expect(err).NotTo(HaveOccurred())

			startTime := time.Now()
			err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), mediumFile)
			Expect(err).NotTo(HaveOccurred())
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 2.0), "Should extract medium file within 2 seconds")
		})
	})

	Describe("AQL Parser", func() {
		It("should parse simple rules efficiently", func() {
			aql := `RULE "Benchmark Rule" {
				LIMIT(*.cyclomatic > 10)
			}`

			startTime := time.Now()
			for i := 0; i < 100; i++ {
				_, err := parser.ParseAQLFile(aql)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 0.5), "Should parse 100 simple rules within 0.5 seconds")
		})

		It("should parse complex rules efficiently", func() {
			aql := `RULE "Complex Rule" {
				LIMIT(Controller*.cyclomatic > 15)
				FORBID(Controller* -> Repository*)
				REQUIRE(Controller* -> Service*)
				ALLOW(Service* -> Repository*)
				LIMIT(Service*.params > 5)
				FORBID(Repository* -> Controller*)
			}`

			startTime := time.Now()
			for i := 0; i < 50; i++ {
				_, err := parser.ParseAQLFile(aql)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 1.0), "Should parse 50 complex rules within 1 second")
		})
	})

	Describe("AQL Query Execution", func() {
		It("should execute simple queries efficiently", func() {
			// Pre-populate cache with test data
			populateTestData(astCache, 100)

			engine := query.NewAQLEngine(astCache)
			aql := `RULE "Simple Query" {
				LIMIT(*.cyclomatic > 5)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).NotTo(HaveOccurred())

			startTime := time.Now()
			for i := 0; i < 10; i++ {
				_, err := engine.ExecuteRuleSet(ruleSet)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 2.0), "Should execute 10 simple queries within 2 seconds")
		})

		It("should execute complex queries efficiently", func() {
			// Pre-populate cache with test data
			populateTestData(astCache, 200)

			engine := query.NewAQLEngine(astCache)
			aql := `RULE "Complex Query" {
				LIMIT(*.cyclomatic > 10)
				LIMIT(*.params > 3)
				FORBID(Controller* -> Repository*)
				REQUIRE(Service* -> Repository*)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			Expect(err).NotTo(HaveOccurred())

			startTime := time.Now()
			for i := 0; i < 5; i++ {
				_, err := engine.ExecuteRuleSet(ruleSet)
				Expect(err).NotTo(HaveOccurred())
			}
			runtime := time.Since(startTime)

			Expect(runtime.Seconds()).To(BeNumerically("<", 5.0), "Should execute 5 complex queries within 5 seconds")
		})
	})

	Describe("Large Codebase Performance", func() {
		It("should handle large codebases within reasonable time", Label("slow"), func() {
			// This test is labeled as "slow" and will only run when --label-filter="slow" is used

			// Generate large codebase
			tmpDir := GinkgoT().TempDir()
			fileCount := 200
			methodsPerFile := 25

			GinkgoWriter.Printf("Generating %d files with %d methods each\n", fileCount, methodsPerFile)

			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
				content := generateLargeGoFile(i, methodsPerFile)

				err := os.WriteFile(filename, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Extract AST from all files
			extractor := analysis.NewGoASTExtractor(astCache)
			totalNodes := 0

			startTime := time.Now()
			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
				err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
				Expect(err).NotTo(HaveOccurred())

				nodes, err := astCache.GetASTNodesByFile(filename)
				Expect(err).NotTo(HaveOccurred())
				totalNodes += len(nodes)

				if i%50 == 0 {
					GinkgoWriter.Printf("Processed %d/%d files (%d nodes so far)\n", i, fileCount, totalNodes)
				}
			}
			extractionTime := time.Since(startTime)

			GinkgoWriter.Printf("Extracted %d nodes from %d files in %v\n", totalNodes, fileCount, extractionTime)
			GinkgoWriter.Printf("Average: %.2f nodes/file, %.2f ms/file\n",
				float64(totalNodes)/float64(fileCount),
				float64(extractionTime.Milliseconds())/float64(fileCount))

			// Test complex AQL query performance
			engine := query.NewAQLEngine(astCache)
			complexAQL := `
			RULE "Large Codebase Test" {
				LIMIT(*.cyclomatic > 10)
				LIMIT(*.params > 4)
				FORBID(Type1* -> Type2*)
				REQUIRE(Service* -> Repository*)
				LIMIT(*.lines > 50)
			}`

			queryStartTime := time.Now()
			ruleSet, err := parser.ParseAQLFile(complexAQL)
			Expect(err).NotTo(HaveOccurred())

			violations, err := engine.ExecuteRuleSet(ruleSet)
			Expect(err).NotTo(HaveOccurred())
			queryTime := time.Since(queryStartTime)

			GinkgoWriter.Printf("Found %d violations in %v\n", len(violations), queryTime)
			GinkgoWriter.Printf("Query performance: %.2f nodes/ms\n",
				float64(totalNodes)/float64(queryTime.Milliseconds()))

			// Performance assertions
			Expect(extractionTime).To(BeNumerically("<", 30*time.Second),
				"AST extraction should complete within 30 seconds")
			Expect(queryTime).To(BeNumerically("<", 10*time.Second),
				"Complex query should complete within 10 seconds")
			Expect(totalNodes).To(BeNumerically(">=", fileCount*methodsPerFile),
				"Should have extracted expected number of nodes")
		})
	})

	Describe("Memory Usage", func() {
		It("should use memory efficiently", Label("slow"), func() {
			// This test is labeled as "slow" and will only run when --label-filter="slow" is used

			// Get initial memory stats
			var m1 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			// Generate moderate-sized codebase
			tmpDir := GinkgoT().TempDir()
			fileCount := 50
			methodsPerFile := 20

			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("memory_test_%d.go", i))
				content := generateLargeGoFile(i, methodsPerFile)

				err := os.WriteFile(filename, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Extract AST and measure memory
			extractor := analysis.NewGoASTExtractor(astCache)
			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("memory_test_%d.go", i))
				err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
				Expect(err).NotTo(HaveOccurred())
			}

			// Get final memory stats
			var m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m2)

			memoryUsed := m2.Alloc - m1.Alloc
			totalAllocs := m2.TotalAlloc - m1.TotalAlloc

			GinkgoWriter.Printf("Memory used: %d bytes (%.2f MB)\n", memoryUsed, float64(memoryUsed)/(1024*1024))
			GinkgoWriter.Printf("Total allocations: %d bytes (%.2f MB)\n", totalAllocs, float64(totalAllocs)/(1024*1024))
			GinkgoWriter.Printf("Memory per file: %.2f KB\n", float64(memoryUsed)/(1024*float64(fileCount)))

			// Memory usage assertions (should be reasonable)
			maxMemoryPerFile := 500 * 1024 // 500 KB per file
			Expect(int(memoryUsed)).To(BeNumerically("<", fileCount*maxMemoryPerFile),
				"Memory usage should be reasonable")
		})
	})

	Describe("Cache Efficiency", func() {
		It("should provide significant cache speedup", func() {
			// Create test file
			tmpDir := GinkgoT().TempDir()
			testFile := filepath.Join(tmpDir, "cache_test.go")
			content := generateMediumGoFile()

			err := os.WriteFile(testFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			extractor := analysis.NewGoASTExtractor(astCache)

			// First extraction (cold cache)
			start1 := time.Now()
			err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), testFile)
			Expect(err).NotTo(HaveOccurred())
			firstTime := time.Since(start1)

			// Second extraction (should be cached)
			start2 := time.Now()
			err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), testFile)
			Expect(err).NotTo(HaveOccurred())
			secondTime := time.Since(start2)

			GinkgoWriter.Printf("First extraction: %v\n", firstTime)
			GinkgoWriter.Printf("Second extraction (cached): %v\n", secondTime)
			GinkgoWriter.Printf("Cache speedup: %.2fx\n", float64(firstTime)/float64(secondTime))

			// Cache should provide significant speedup
			Expect(secondTime).To(BeNumerically("<", firstTime/2),
				"Cached extraction should be at least 2x faster")
		})
	})

	Describe("Concurrent Access", func() {
		It("should handle concurrent extraction efficiently", func() {
			// Create test files
			tmpDir := GinkgoT().TempDir()
			fileCount := 20

			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("concurrent_%d.go", i))
				content := fmt.Sprintf(`package test%d

type Struct%d struct {
	Field1 string
	Field2 int
}

func (s *Struct%d) Method1() string {
	return s.Field1
}

func (s *Struct%d) Method2(x int) int {
	return s.Field2 + x
}`, i, i, i, i)

				err := os.WriteFile(filename, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			// Test concurrent extraction
			startTime := time.Now()

			done := make(chan error, fileCount)
			for i := 0; i < fileCount; i++ {
				go func(fileIndex int) {
					defer GinkgoRecover()
					extractor := analysis.NewGoASTExtractor(astCache)
					filename := filepath.Join(tmpDir, fmt.Sprintf("concurrent_%d.go", fileIndex))
					done <- extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
				}(i)
			}

			// Wait for all extractions to complete
			for i := 0; i < fileCount; i++ {
				err := <-done
				Expect(err).NotTo(HaveOccurred())
			}

			concurrentTime := time.Since(startTime)

			// Test sequential extraction for comparison
			astCache2, err := cache.NewASTCache()
			Expect(err).NotTo(HaveOccurred())
			defer astCache2.Close()

			sequentialStart := time.Now()
			extractor := analysis.NewGoASTExtractor(astCache2)

			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("concurrent_%d.go", i))
				err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
				Expect(err).NotTo(HaveOccurred())
			}

			sequentialTime := time.Since(sequentialStart)

			GinkgoWriter.Printf("Concurrent extraction: %v\n", concurrentTime)
			GinkgoWriter.Printf("Sequential extraction: %v\n", sequentialTime)
			if sequentialTime > concurrentTime {
				GinkgoWriter.Printf("Concurrent speedup: %.2fx\n", float64(sequentialTime)/float64(concurrentTime))
			}

			// Verify both caches have same number of nodes
			totalNodes1 := 0
			totalNodes2 := 0

			for i := 0; i < fileCount; i++ {
				filename := filepath.Join(tmpDir, fmt.Sprintf("concurrent_%d.go", i))

				nodes1, err := astCache.GetASTNodesByFile(filename)
				Expect(err).NotTo(HaveOccurred())
				totalNodes1 += len(nodes1)

				nodes2, err := astCache2.GetASTNodesByFile(filename)
				Expect(err).NotTo(HaveOccurred())
				totalNodes2 += len(nodes2)
			}

			Expect(totalNodes1).To(Equal(totalNodes2),
				"Concurrent and sequential extraction should yield same results")
		})
	})
})

// Helper functions

func populateTestData(astCache *cache.ASTCache, nodeCount int) {
	for i := 0; i < nodeCount; i++ {
		node := &models.ASTNode{
			FilePath:             fmt.Sprintf("/test/file%d.go", i%10),
			PackageName:          fmt.Sprintf("pkg%d", i%5),
			TypeName:             fmt.Sprintf("Type%d", i%20),
			MethodName:           fmt.Sprintf("Method%d", i),
			NodeType:             models.NodeTypeMethod,
			StartLine:            i % 100,
			EndLine:              i%100 + 10,
			CyclomaticComplexity: i % 25,
			LineCount:            i%50 + 1,
			LastModified:         time.Now(),
		}

		nodeID, err := astCache.StoreASTNode(node)
		Expect(err).NotTo(HaveOccurred())

		// Add some relationships
		if i > 0 && i%10 == 0 {
			prevNodeID := nodeID - int64(i%5+1)
			err = astCache.StoreASTRelationship(nodeID, &prevNodeID,
				i%20+1, models.RelationshipCall, fmt.Sprintf("call%d", i))
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func generateMediumGoFile() string {
	return `package medium

import (
	"fmt"
	"strings"
	"strconv"
)

type UserManager struct {
	users map[string]*User
}

type User struct {
	ID       string
	Name     string
	Email    string
	Age      int
	Active   bool
	Settings map[string]interface{}
}

func NewUserManager() *UserManager {
	return &UserManager{
		users: make(map[string]*User),
	}
}

func (um *UserManager) AddUser(user *User) error {
	if user == nil {
		return fmt.Errorf("user cannot be nil")
	}

	if user.ID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	if _, exists := um.users[user.ID]; exists {
		return fmt.Errorf("user with ID %s already exists", user.ID)
	}

	um.users[user.ID] = user
	return nil
}

func (um *UserManager) GetUser(id string) (*User, error) {
	if id == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	user, exists := um.users[id]
	if !exists {
		return nil, fmt.Errorf("user with ID %s not found", id)
	}

	return user, nil
}

func (um *UserManager) UpdateUser(id string, updates map[string]interface{}) error {
	user, err := um.GetUser(id)
	if err != nil {
		return err
	}

	for key, value := range updates {
		switch key {
		case "name":
			if name, ok := value.(string); ok && name != "" {
				user.Name = name
			}
		case "email":
			if email, ok := value.(string); ok && strings.Contains(email, "@") {
				user.Email = email
			}
		case "age":
			if age, ok := value.(int); ok && age >= 0 && age <= 150 {
				user.Age = age
			}
		case "active":
			if active, ok := value.(bool); ok {
				user.Active = active
			}
		}
	}

	return nil
}

func (u *User) Validate() []string {
	var errors []string

	if u.ID == "" {
		errors = append(errors, "ID is required")
	}

	if u.Name == "" {
		errors = append(errors, "Name is required")
	}

	if u.Email == "" {
		errors = append(errors, "Email is required")
	} else if !strings.Contains(u.Email, "@") {
		errors = append(errors, "Email must be valid")
	}

	if u.Age < 0 || u.Age > 150 {
		errors = append(errors, "Age must be between 0 and 150")
	}

	return errors
}`
}

func generateLargeGoFile(fileIndex, methodCount int) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf("package file%d\n\n", fileIndex))
	content.WriteString("import (\n\t\"fmt\"\n\t\"strings\"\n)\n\n")

	// Generate struct types
	content.WriteString(fmt.Sprintf("type Service%d struct {\n", fileIndex))
	content.WriteString(fmt.Sprintf("\trepo *Repository%d\n", fileIndex))
	content.WriteString("\tconfig map[string]interface{}\n")
	content.WriteString("}\n\n")

	content.WriteString(fmt.Sprintf("type Repository%d struct {\n", fileIndex))
	content.WriteString("\tdb interface{}\n")
	content.WriteString("\tcache map[string]interface{}\n")
	content.WriteString("}\n\n")

	// Generate methods with varying complexity
	for i := 0; i < methodCount; i++ {
		complexity := (i%10 + 1) * 2 // Complexity from 2 to 20
		paramCount := i%5 + 1        // 1 to 5 parameters

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
		content.WriteString("\tresult := 0\n")

		for c := 0; c < complexity; c++ {
			switch c % 4 {
			case 0:
				content.WriteString(fmt.Sprintf("\tif param0 > %d {\n", c))
				content.WriteString(fmt.Sprintf("\t\tresult += %d\n", c))
				content.WriteString("\t}\n")
			case 1:
				content.WriteString(fmt.Sprintf("\tfor j := 0; j < %d; j++ {\n", c+1))
				content.WriteString("\t\tresult += j\n")
				content.WriteString("\t}\n")
			case 2:
				content.WriteString(fmt.Sprintf("\tswitch param0 %% %d {\n", c+2))
				for sc := 0; sc <= c%3+1; sc++ {
					content.WriteString(fmt.Sprintf("\tcase %d:\n", sc))
					content.WriteString(fmt.Sprintf("\t\tresult += %d\n", sc*c))
				}
				content.WriteString("\tdefault:\n")
				content.WriteString(fmt.Sprintf("\t\tresult += %d\n", c))
				content.WriteString("\t}\n")
			case 3:
				content.WriteString(fmt.Sprintf("\tif param0 %% %d == 0 {\n", c+1))
				content.WriteString(fmt.Sprintf("\t\tif result > %d {\n", c*10))
				content.WriteString(fmt.Sprintf("\t\t\tresult *= %d\n", c+1))
				content.WriteString("\t\t} else {\n")
				content.WriteString(fmt.Sprintf("\t\t\tresult += %d\n", c*2))
				content.WriteString("\t\t}\n")
				content.WriteString("\t}\n")
			}
		}

		// Add some method calls for relationship testing
		if i%3 == 0 {
			content.WriteString("\ts.repo.Save(result)\n")
		}
		if i%4 == 0 {
			content.WriteString("\tfmt.Printf(\"Method%d result: %d\\n\", result)\n")
		}

		content.WriteString("\treturn result\n")
		content.WriteString("}\n\n")
	}

	return content.String()
}
