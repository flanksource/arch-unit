package performance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Performance benchmarks and tests for AST system

func BenchmarkASTCache_StoreNode(b *testing.B) {
	astCache, err := cache.NewASTCache()
	require.NoError(b, err)
	defer astCache.Close()

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

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		node.ID = 0 // Reset ID for each iteration
		node.MethodName = fmt.Sprintf("TestMethod%d", i)
		_, err := astCache.StoreASTNode(node)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkASTCache_RetrieveNode(b *testing.B) {
	astCache, err := cache.NewASTCache()
	require.NoError(b, err)
	defer astCache.Close()

	// Pre-populate with nodes
	nodeIDs := make([]int64, 1000)
	for i := 0; i < 1000; i++ {
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
		require.NoError(b, err)
		nodeIDs[i] = id
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		nodeID := nodeIDs[i%1000]
		_, err := astCache.GetASTNode(nodeID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoASTExtractor_SmallFile(b *testing.B) {
	astCache, err := cache.NewASTCache()
	require.NoError(b, err)
	defer astCache.Close()

	extractor := analysis.NewGoASTExtractor(astCache)

	// Create small Go file
	tmpDir := b.TempDir()
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

	err = os.WriteFile(smallFile, []byte(content), 0644)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Clear cache before each extraction
		err := astCache.DeleteASTForFile(smallFile)
		require.NoError(b, err)

		err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), smallFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoASTExtractor_MediumFile(b *testing.B) {
	astCache, err := cache.NewASTCache()
	require.NoError(b, err)
	defer astCache.Close()

	extractor := analysis.NewGoASTExtractor(astCache)

	// Create medium-sized Go file
	tmpDir := b.TempDir()
	mediumFile := filepath.Join(tmpDir, "medium.go")
	content := generateMediumGoFile()

	err = os.WriteFile(mediumFile, []byte(content), 0644)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Clear cache before each extraction
		err := astCache.DeleteASTForFile(mediumFile)
		require.NoError(b, err)

		err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), mediumFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAQLParser_SimpleRule(b *testing.B) {
	aql := `RULE "Benchmark Rule" {
		LIMIT(*.cyclomatic > 10)
	}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := parser.ParseAQLFile(aql)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAQLParser_ComplexRule(b *testing.B) {
	aql := `RULE "Complex Rule" {
		LIMIT(Controller*.cyclomatic > 15)
		FORBID(Controller* -> Repository*)
		REQUIRE(Controller* -> Service*)
		ALLOW(Service* -> Repository*)
		LIMIT(Service*.params > 5)
		FORBID(Repository* -> Controller*)
	}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := parser.ParseAQLFile(aql)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAQLEngine_SimpleQuery(b *testing.B) {
	astCache, err := cache.NewASTCache()
	require.NoError(b, err)
	defer astCache.Close()

	// Pre-populate cache with test data
	populateTestData(b, astCache, 100)

	engine := query.NewAQLEngine(astCache)
	aql := `RULE "Simple Query" {
		LIMIT(*.cyclomatic > 5)
	}`

	ruleSet, err := parser.ParseAQLFile(aql)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.ExecuteRuleSet(ruleSet)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAQLEngine_ComplexQuery(b *testing.B) {
	astCache, err := cache.NewASTCache()
	require.NoError(b, err)
	defer astCache.Close()

	// Pre-populate cache with test data
	populateTestData(b, astCache, 500)

	engine := query.NewAQLEngine(astCache)
	aql := `RULE "Complex Query" {
		LIMIT(*.cyclomatic > 10)
		LIMIT(*.params > 3)
		FORBID(Controller* -> Repository*)
		REQUIRE(Service* -> Repository*)
	}`

	ruleSet, err := parser.ParseAQLFile(aql)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.ExecuteRuleSet(ruleSet)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Performance tests for large codebases

func TestASTPerformance_LargeCodebase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large codebase performance test in short mode")
	}

	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Generate large codebase
	tmpDir := t.TempDir()
	fileCount := 200
	methodsPerFile := 25

	t.Logf("Generating %d files with %d methods each", fileCount, methodsPerFile)

	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
		content := generateLargeGoFile(i, methodsPerFile)

		err := os.WriteFile(filename, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Extract AST from all files
	extractor := analysis.NewGoASTExtractor(astCache)
	totalNodes := 0

	startTime := time.Now()
	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
		err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
		require.NoError(t, err)

		nodes, err := astCache.GetASTNodesByFile(filename)
		require.NoError(t, err)
		totalNodes += len(nodes)

		if i%50 == 0 {
			t.Logf("Processed %d/%d files (%d nodes so far)", i, fileCount, totalNodes)
		}
	}
	extractionTime := time.Since(startTime)

	t.Logf("Extracted %d nodes from %d files in %v", totalNodes, fileCount, extractionTime)
	t.Logf("Average: %.2f nodes/file, %.2f ms/file",
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
	require.NoError(t, err)

	violations, err := engine.ExecuteRuleSet(ruleSet)
	require.NoError(t, err)
	queryTime := time.Since(queryStartTime)

	t.Logf("Found %d violations in %v", len(violations), queryTime)
	t.Logf("Query performance: %.2f nodes/ms",
		float64(totalNodes)/float64(queryTime.Milliseconds()))

	// Performance assertions
	assert.Less(t, extractionTime, 30*time.Second,
		"AST extraction should complete within 30 seconds")
	assert.Less(t, queryTime, 10*time.Second,
		"Complex query should complete within 10 seconds")
	assert.GreaterOrEqual(t, totalNodes, fileCount*methodsPerFile,
		"Should have extracted expected number of nodes")
}

func TestASTPerformance_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	// Get initial memory stats
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Generate moderate-sized codebase
	tmpDir := t.TempDir()
	fileCount := 50
	methodsPerFile := 20

	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("memory_test_%d.go", i))
		content := generateLargeGoFile(i, methodsPerFile)

		err := os.WriteFile(filename, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Extract AST and measure memory
	extractor := analysis.NewGoASTExtractor(astCache)
	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("memory_test_%d.go", i))
		err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
		require.NoError(t, err)
	}

	// Get final memory stats
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)

	memoryUsed := m2.Alloc - m1.Alloc
	totalAllocs := m2.TotalAlloc - m1.TotalAlloc

	t.Logf("Memory used: %d bytes (%.2f MB)", memoryUsed, float64(memoryUsed)/(1024*1024))
	t.Logf("Total allocations: %d bytes (%.2f MB)", totalAllocs, float64(totalAllocs)/(1024*1024))
	t.Logf("Memory per file: %.2f KB", float64(memoryUsed)/(1024*float64(fileCount)))

	// Memory usage assertions (should be reasonable)
	maxMemoryPerFile := 500 * 1024 // 500 KB per file
	assert.Less(t, int(memoryUsed), fileCount*maxMemoryPerFile,
		"Memory usage should be reasonable")
}

func TestASTPerformance_CacheEfficiency(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Create test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "cache_test.go")
	content := generateMediumGoFile()

	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	extractor := analysis.NewGoASTExtractor(astCache)

	// First extraction (cold cache)
	start1 := time.Now()
	err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), testFile)
	require.NoError(t, err)
	firstTime := time.Since(start1)

	// Second extraction (should be cached)
	start2 := time.Now()
	err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), testFile)
	require.NoError(t, err)
	secondTime := time.Since(start2)

	t.Logf("First extraction: %v", firstTime)
	t.Logf("Second extraction (cached): %v", secondTime)
	t.Logf("Cache speedup: %.2fx", float64(firstTime)/float64(secondTime))

	// Cache should provide significant speedup
	assert.Less(t, secondTime, firstTime/2,
		"Cached extraction should be at least 2x faster")
}

func TestASTPerformance_ConcurrentAccess(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Create test files
	tmpDir := t.TempDir()
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
		require.NoError(t, err)
	}

	// Test concurrent extraction
	startTime := time.Now()

	done := make(chan error, fileCount)
	for i := 0; i < fileCount; i++ {
		go func(fileIndex int) {
			extractor := analysis.NewGoASTExtractor(astCache)
			filename := filepath.Join(tmpDir, fmt.Sprintf("concurrent_%d.go", fileIndex))
			done <- extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
		}(i)
	}

	// Wait for all extractions to complete
	for i := 0; i < fileCount; i++ {
		err := <-done
		require.NoError(t, err)
	}

	concurrentTime := time.Since(startTime)

	// Test sequential extraction for comparison
	astCache2, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache2.Close()

	sequentialStart := time.Now()
	extractor := analysis.NewGoASTExtractor(astCache2)

	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("concurrent_%d.go", i))
		err := extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), filename)
		require.NoError(t, err)
	}

	sequentialTime := time.Since(sequentialStart)

	t.Logf("Concurrent extraction: %v", concurrentTime)
	t.Logf("Sequential extraction: %v", sequentialTime)
	t.Logf("Concurrent speedup: %.2fx", float64(sequentialTime)/float64(concurrentTime))

	// Verify both caches have same number of nodes
	totalNodes1 := 0
	totalNodes2 := 0

	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("concurrent_%d.go", i))

		nodes1, err := astCache.GetASTNodesByFile(filename)
		require.NoError(t, err)
		totalNodes1 += len(nodes1)

		nodes2, err := astCache2.GetASTNodesByFile(filename)
		require.NoError(t, err)
		totalNodes2 += len(nodes2)
	}

	assert.Equal(t, totalNodes1, totalNodes2,
		"Concurrent and sequential extraction should yield same results")
}

// Helper functions

func populateTestData(b *testing.B, astCache *cache.ASTCache, nodeCount int) {
	b.Helper()

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
		require.NoError(b, err)

		// Add some relationships
		if i > 0 && i%10 == 0 {
			prevNodeID := nodeID - int64(i%5+1)
			err = astCache.StoreASTRelationship(nodeID, &prevNodeID,
				i%20+1, models.RelationshipCall, fmt.Sprintf("call%d", i))
			require.NoError(b, err)
		}
	}
}

func generateMediumGoFile() string {
	var content strings.Builder

	content.WriteString(`package medium

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

func (um *UserManager) DeleteUser(id string) error {
	if _, exists := um.users[id]; !exists {
		return fmt.Errorf("user with ID %s not found", id)
	}

	delete(um.users, id)
	return nil
}

func (um *UserManager) ListUsers(filters map[string]interface{}) []*User {
	var result []*User

	for _, user := range um.users {
		include := true

		if nameFilter, ok := filters["name"]; ok {
			if name, ok := nameFilter.(string); ok {
				if !strings.Contains(strings.ToLower(user.Name), strings.ToLower(name)) {
					include = false
				}
			}
		}

		if emailFilter, ok := filters["email"]; ok {
			if email, ok := emailFilter.(string); ok {
				if !strings.Contains(strings.ToLower(user.Email), strings.ToLower(email)) {
					include = false
				}
			}
		}

		if ageFilter, ok := filters["min_age"]; ok {
			if minAge, ok := ageFilter.(int); ok {
				if user.Age < minAge {
					include = false
				}
			}
		}

		if ageFilter, ok := filters["max_age"]; ok {
			if maxAge, ok := ageFilter.(int); ok {
				if user.Age > maxAge {
					include = false
				}
			}
		}

		if activeFilter, ok := filters["active"]; ok {
			if active, ok := activeFilter.(bool); ok {
				if user.Active != active {
					include = false
				}
			}
		}

		if include {
			result = append(result, user)
		}
	}

	return result
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
}`)

	return content.String()
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
		if i%5 == 0 {
			content.WriteString("\tstrings.TrimSpace(fmt.Sprintf(\"%d\", result))\n")
		}

		content.WriteString("\treturn result\n")
		content.WriteString("}\n\n")
	}

	return content.String()
}
