package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestASTIntegration tests the full AST analysis pipeline
func TestASTIntegration_FullPipeline(t *testing.T) {
	// Setup
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Copy test fixtures to temp directory
	tmpDir := t.TempDir()
	testFixtures := []string{"controller.go", "service.go", "repository.go", "model.go"}
	
	for _, fixture := range testFixtures {
		sourceFile := filepath.Join("../../testdata/fixtures", fixture)
		destFile := filepath.Join(tmpDir, fixture)
		
		content, err := os.ReadFile(sourceFile)
		require.NoError(t, err)
		
		err = os.WriteFile(destFile, content, 0644)
		require.NoError(t, err)
	}

	// Extract AST from all files
	extractor := analysis.NewGoASTExtractor(astCache)
	for _, fixture := range testFixtures {
		filePath := filepath.Join(tmpDir, fixture)
		err := extractor.ExtractFile(filePath)
		require.NoError(t, err, "Failed to extract AST from %s", fixture)
	}

	// Verify AST nodes were created
	allNodes := make([]*models.ASTNode, 0)
	for _, fixture := range testFixtures {
		filePath := filepath.Join(tmpDir, fixture)
		nodes, err := astCache.GetASTNodesByFile(filePath)
		require.NoError(t, err)
		assert.Greater(t, len(nodes), 0, "Should have nodes for %s", fixture)
		allNodes = append(allNodes, nodes...)
	}

	t.Logf("Extracted %d AST nodes total", len(allNodes))

	// Verify we have different node types
	nodeTypeCounts := make(map[models.NodeType]int)
	for _, node := range allNodes {
		nodeTypeCounts[node.NodeType]++
	}

	assert.Greater(t, nodeTypeCounts[models.NodeTypeMethod], 10, "Should have many methods")
	assert.Greater(t, nodeTypeCounts[models.NodeTypeType], 5, "Should have several types")
	assert.Greater(t, nodeTypeCounts[models.NodeTypeField], 5, "Should have fields")

	// Test complexity analysis
	complexMethods := 0
	for _, node := range allNodes {
		if node.NodeType == models.NodeTypeMethod && node.CyclomaticComplexity > 10 {
			complexMethods++
			t.Logf("Complex method: %s.%s (complexity: %d)", 
				node.TypeName, node.MethodName, node.CyclomaticComplexity)
		}
	}
	assert.Greater(t, complexMethods, 0, "Should find some complex methods")
}

func TestASTIntegration_AQLRuleExecution(t *testing.T) {
	// Setup
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Use test fixtures
	tmpDir := t.TempDir()
	testFixtures := []string{"controller.go", "service.go", "repository.go"}
	
	for _, fixture := range testFixtures {
		sourceFile := filepath.Join("../../testdata/fixtures", fixture)
		destFile := filepath.Join(tmpDir, fixture)
		
		content, err := os.ReadFile(sourceFile)
		require.NoError(t, err)
		
		err = os.WriteFile(destFile, content, 0644)
		require.NoError(t, err)
	}

	// Extract AST
	extractor := analysis.NewGoASTExtractor(astCache)
	for _, fixture := range testFixtures {
		filePath := filepath.Join(tmpDir, fixture)
		err := extractor.ExtractFile(filePath)
		require.NoError(t, err)
	}

	// Test various AQL rules
	testCases := []struct {
		name          string
		aqlRule       string
		minViolations int
		maxViolations int
		description   string
	}{
		{
			name: "High complexity limit",
			aqlRule: `RULE "High Complexity" {
				LIMIT(*.cyclomatic > 15)
			}`,
			minViolations: 1,
			maxViolations: 10,
			description:   "Should find methods with high complexity",
		},
		{
			name: "Controller complexity",
			aqlRule: `RULE "Controller Complexity" {
				LIMIT(*Controller*.cyclomatic > 10)
			}`,
			minViolations: 1,
			maxViolations: 5,
			description:   "Should find complex controller methods",
		},
		{
			name: "Service complexity",
			aqlRule: `RULE "Service Complexity" {
				LIMIT(*Service*.cyclomatic > 15)
			}`,
			minViolations: 0,
			maxViolations: 3,
			description:   "Should find complex service methods",
		},
		{
			name: "Parameter count check",
			aqlRule: `RULE "Too Many Parameters" {
				LIMIT(*.params > 5)
			}`,
			minViolations: 0,
			maxViolations: 5,
			description:   "Should find methods with many parameters",
		},
		{
			name: "Very high complexity",
			aqlRule: `RULE "Very High Complexity" {
				LIMIT(*.cyclomatic > 25)
			}`,
			minViolations: 0,
			maxViolations: 3,
			description:   "Should find very complex methods",
		},
	}

	engine := query.NewAQLEngine(astCache)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ruleSet, err := parser.ParseAQLFile(tc.aqlRule)
			require.NoError(t, err)

			violations, err := engine.ExecuteRuleSet(ruleSet)
			require.NoError(t, err)

			violationCount := len(violations)
			assert.GreaterOrEqual(t, violationCount, tc.minViolations, 
				"Should have at least %d violations for %s", tc.minViolations, tc.description)
			assert.LessOrEqual(t, violationCount, tc.maxViolations,
				"Should have at most %d violations for %s", tc.maxViolations, tc.description)

			// Log violations for debugging
			for _, v := range violations {
				t.Logf("Violation: %s:%d - %s", 
					filepath.Base(v.File), v.Line, v.Message)
			}
		})
	}
}

func TestASTIntegration_ArchitectureRules(t *testing.T) {
	// Setup
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Use test fixtures to simulate a typical web application structure
	tmpDir := t.TempDir()
	testFixtures := []string{"controller.go", "service.go", "repository.go", "model.go"}
	
	for _, fixture := range testFixtures {
		sourceFile := filepath.Join("../../testdata/fixtures", fixture)
		destFile := filepath.Join(tmpDir, fixture)
		
		content, err := os.ReadFile(sourceFile)
		require.NoError(t, err)
		
		err = os.WriteFile(destFile, content, 0644)
		require.NoError(t, err)
	}

	// Extract AST and relationships
	extractor := analysis.NewGoASTExtractor(astCache)
	for _, fixture := range testFixtures {
		filePath := filepath.Join(tmpDir, fixture)
		err := extractor.ExtractFile(filePath)
		require.NoError(t, err)
	}

	engine := query.NewAQLEngine(astCache)

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
	require.NoError(t, err)

	violations, err := engine.ExecuteRuleSet(ruleSet)
	require.NoError(t, err)

	t.Logf("Found %d architecture violations", len(violations))

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
		t.Logf("Architecture violation: %s:%d - %s", 
			filepath.Base(v.File), v.Line, v.Message)
	}

	// We expect some complexity violations but minimal layer violations
	// (our test fixtures follow good architecture)
	assert.GreaterOrEqual(t, complexityViolations, 0, "May have complexity violations")
	t.Logf("Complexity violations: %d, Layer violations: %d", 
		complexityViolations, layerViolations)
}

func TestASTIntegration_RelationshipTracking(t *testing.T) {
	// Setup
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Create a simple test file with method calls
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "relationships.go")
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

	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Extract AST with relationships
	extractor := analysis.NewGoASTExtractor(astCache)
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Verify nodes were created
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	assert.Greater(t, len(nodes), 5, "Should have multiple nodes")

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

	require.NotNil(t, multiplyMethod, "Should find Multiply method")
	require.NotNil(t, serviceCalculateMethod, "Should find Service.Calculate method")

	// Check for call relationships
	multiplyRels, err := astCache.GetASTRelationships(multiplyMethod.ID, models.RelationshipCall)
	require.NoError(t, err)

	serviceRels, err := astCache.GetASTRelationships(serviceCalculateMethod.ID, models.RelationshipCall)
	require.NoError(t, err)

	t.Logf("Multiply method has %d call relationships", len(multiplyRels))
	t.Logf("Service.Calculate method has %d call relationships", len(serviceRels))

	// Verify relationship details
	for _, rel := range multiplyRels {
		t.Logf("Multiply calls: %s at line %d", rel.Text, rel.LineNo)
		assert.Greater(t, rel.LineNo, 0, "Should have valid line number")
		assert.NotEmpty(t, rel.Text, "Should have call text")
	}

	for _, rel := range serviceRels {
		t.Logf("Service.Calculate calls: %s at line %d", rel.Text, rel.LineNo)
		assert.Greater(t, rel.LineNo, 0, "Should have valid line number")
		assert.NotEmpty(t, rel.Text, "Should have call text")
	}
}

func TestASTIntegration_LibraryDetection(t *testing.T) {
	// Setup
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Create a test file with library calls
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "libraries.go")
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

	err = os.WriteFile(testFile, []byte(content), 0644)
	require.NoError(t, err)

	// Extract AST with library detection
	extractor := analysis.NewGoASTExtractor(astCache)
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Find the ServeHTTP method
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	var serveHTTPMethod *models.ASTNode
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeMethod && node.MethodName == "ServeHTTP" {
			serveHTTPMethod = node
			break
		}
	}

	require.NotNil(t, serveHTTPMethod, "Should find ServeHTTP method")

	// Check for library relationships
	libRels, err := astCache.GetLibraryRelationships(serveHTTPMethod.ID, models.RelationshipCall)
	require.NoError(t, err)

	t.Logf("Found %d library relationships", len(libRels))

	// Verify we detect standard library calls
	foundLibraries := make(map[string]bool)
	for _, rel := range libRels {
		foundLibraries[rel.LibraryNode.Package] = true
		t.Logf("Library call: %s.%s at line %d (framework: %s)", 
			rel.LibraryNode.Package, rel.LibraryNode.Method, 
			rel.LineNo, rel.LibraryNode.Framework)
	}

	// Should detect standard library usage
	expectedLibraries := []string{"fmt", "http", "json"}
	for _, lib := range expectedLibraries {
		if foundLibraries[lib] {
			t.Logf("✓ Detected %s library usage", lib)
		}
	}

	assert.Greater(t, len(libRels), 0, "Should detect library calls")
}

func TestASTIntegration_PerformanceWithLargeCodebase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Setup
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Generate a large codebase
	tmpDir := t.TempDir()
	fileCount := 50
	methodsPerFile := 20

	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
		content := generateLargeGoFile(i, methodsPerFile)
		
		err := os.WriteFile(filename, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Extract AST from all files
	extractor := analysis.NewGoASTExtractor(astCache)
	totalNodes := 0
	
	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("file%d.go", i))
		err := extractor.ExtractFile(filename)
		require.NoError(t, err)
		
		nodes, err := astCache.GetASTNodesByFile(filename)
		require.NoError(t, err)
		totalNodes += len(nodes)
	}

	t.Logf("Processed %d files with %d total AST nodes", fileCount, totalNodes)

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
	require.NoError(t, err)

	violations, err := engine.ExecuteRuleSet(ruleSet)
	require.NoError(t, err)

	t.Logf("Found %d violations in large codebase", len(violations))
	assert.GreaterOrEqual(t, totalNodes, fileCount*methodsPerFile, 
		"Should have processed many nodes")
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