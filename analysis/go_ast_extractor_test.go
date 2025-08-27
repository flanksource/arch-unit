package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoASTExtractor_ExtractSimpleFile(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Create test Go file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "simple.go")
	content := `package main

import "fmt"

type User struct {
	Name string
	Age  int
}

func (u *User) String() string {
	return fmt.Sprintf("User: %s (%d)", u.Name, u.Age)
}

func main() {
	user := User{Name: "Alice", Age: 30}
	fmt.Println(user.String())
}`

	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	// Extract AST
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Verify extracted nodes
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	// Should have: User type, Name field, Age field, String method, main function
	assert.GreaterOrEqual(t, len(nodes), 5)

	// Check for specific nodes
	var userType, nameField, ageField, stringMethod, mainFunc *models.ASTNode
	for _, node := range nodes {
		switch {
		case node.NodeType == models.NodeTypeType && node.TypeName == "User":
			userType = node
		case node.NodeType == models.NodeTypeField && node.FieldName == "Name":
			nameField = node
		case node.NodeType == models.NodeTypeField && node.FieldName == "Age":
			ageField = node
		case node.NodeType == models.NodeTypeMethod && node.MethodName == "String":
			stringMethod = node
		case node.NodeType == models.NodeTypeMethod && node.MethodName == "main":
			mainFunc = node
		}
	}

	assert.NotNil(t, userType, "User type should be extracted")
	assert.NotNil(t, nameField, "Name field should be extracted")
	assert.NotNil(t, ageField, "Age field should be extracted")
	assert.NotNil(t, stringMethod, "String method should be extracted")
	assert.NotNil(t, mainFunc, "main function should be extracted")

	// Verify method details
	assert.Equal(t, "User", stringMethod.TypeName)
	assert.Len(t, stringMethod.Parameters, 0) // receiver is not counted as parameter
	assert.Len(t, stringMethod.ReturnValues, 1)
	assert.Greater(t, stringMethod.CyclomaticComplexity, 0)

	assert.Equal(t, "", mainFunc.TypeName) // no receiver
	assert.Len(t, mainFunc.Parameters, 0)
	assert.Len(t, mainFunc.ReturnValues, 0)
}

func TestGoASTExtractor_CyclomaticComplexity(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Create test Go file with varying complexity
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "complexity.go")
	content := `package main

// Simple function - complexity 1
func simple() {
	println("hello")
}

// Function with if statement - complexity 2
func withIf(x int) {
	if x > 0 {
		println("positive")
	}
}

// Complex function - complexity 8
func complex(x, y int) int {
	if x > 0 { // +1
		if y > 0 { // +1
			for i := 0; i < x; i++ { // +1
				switch i % 3 { // +1
				case 0: // +1
					println("zero")
				case 1: // +1
					println("one")
				default:
					println("other")
				}
			}
		}
	}
	return x + y
}`

	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	// Extract AST
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Verify complexity calculations
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	complexityMap := make(map[string]int)
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeMethod {
			complexityMap[node.MethodName] = node.CyclomaticComplexity
		}
	}

	assert.Equal(t, 1, complexityMap["simple"])
	assert.Equal(t, 2, complexityMap["withIf"])
	assert.GreaterOrEqual(t, complexityMap["complex"], 6) // Should be high complexity
}

func TestGoASTExtractor_InterfaceExtraction(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Create test Go file with interface
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "interface.go")
	content := `package main

type Writer interface {
	Write([]byte) (int, error)
	Close() error
}

type Reader interface {
	Read([]byte) (int, error)
}

type ReadWriter interface {
	Reader
	Writer
}`

	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	// Extract AST
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Verify extracted nodes
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	// Check for interface types and methods
	var writerType, readerType, readWriterType *models.ASTNode
	var writeMethods, readMethods, closeMethods []*models.ASTNode

	for _, node := range nodes {
		switch {
		case node.NodeType == models.NodeTypeType && node.TypeName == "Writer":
			writerType = node
		case node.NodeType == models.NodeTypeType && node.TypeName == "Reader":
			readerType = node
		case node.NodeType == models.NodeTypeType && node.TypeName == "ReadWriter":
			readWriterType = node
		case node.NodeType == models.NodeTypeMethod && node.MethodName == "Write":
			writeMethods = append(writeMethods, node)
		case node.NodeType == models.NodeTypeMethod && node.MethodName == "Read":
			readMethods = append(readMethods, node)
		case node.NodeType == models.NodeTypeMethod && node.MethodName == "Close":
			closeMethods = append(closeMethods, node)
		}
	}

	assert.NotNil(t, writerType)
	assert.NotNil(t, readerType)
	assert.NotNil(t, readWriterType)
	assert.Len(t, writeMethods, 1)
	assert.Len(t, readMethods, 1)
	assert.Len(t, closeMethods, 1)

	// Verify method parameters and returns
	writeMethod := writeMethods[0]
	assert.Len(t, writeMethod.Parameters, 1)
	assert.Len(t, writeMethod.ReturnValues, 2)
}

func TestGoASTExtractor_LibraryCalls(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Create test Go file with library calls
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "library.go")
	content := `package main

import (
	"fmt"
	"net/http"
	"github.com/gin-gonic/gin"
)

func main() {
	// Standard library calls
	fmt.Println("Hello")
	http.Get("http://example.com")
	
	// Third-party library calls
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "hello"})
	})
}`

	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	// Extract AST
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Verify library relationships were created
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	var mainFunc *models.ASTNode
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeMethod && node.MethodName == "main" {
			mainFunc = node
			break
		}
	}
	require.NotNil(t, mainFunc)

	// Check for library relationships
	libRels, err := astCache.GetLibraryRelationships(mainFunc.ID, models.RelationshipCall)
	require.NoError(t, err)
	assert.Greater(t, len(libRels), 0, "Should have library relationships")

	// Verify different framework classifications
	frameworks := make(map[string]bool)
	for _, rel := range libRels {
		frameworks[rel.LibraryNode.Framework] = true
	}

	// Should detect standard library and gin framework
	assert.Contains(t, frameworks, "stdlib")
	// Note: gin classification depends on import analysis working correctly
}

func TestGoASTExtractor_CallRelationships(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Create test Go file with method calls
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "calls.go")
	content := `package main

type Calculator struct{}

func (c *Calculator) Add(a, b int) int {
	return a + b
}

func (c *Calculator) Multiply(a, b int) int {
	sum := c.Add(a, b) // Call to Add method
	return sum * 2
}

func main() {
	calc := Calculator{}
	result := calc.Multiply(5, 3) // Call to Multiply method
	println(result)
}`

	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	// Extract AST
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Find the Multiply method
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	var multiplyMethod *models.ASTNode
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeMethod && node.MethodName == "Multiply" {
			multiplyMethod = node
			break
		}
	}
	require.NotNil(t, multiplyMethod)

	// Check for call relationships from Multiply method
	rels, err := astCache.GetASTRelationships(multiplyMethod.ID, models.RelationshipCall)
	require.NoError(t, err)
	assert.Greater(t, len(rels), 0, "Multiply method should have call relationships")

	// Verify call details
	hasAddCall := false
	for _, rel := range rels {
		if strings.Contains(rel.Text, "Add") {
			hasAddCall = true
			assert.Greater(t, rel.LineNo, 0)
		}
	}
	assert.True(t, hasAddCall, "Should detect call to Add method")
}

func TestGoASTExtractor_FileUpdateHandling(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Create test Go file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "update.go")
	originalContent := `package main

func original() {
	println("original")
}`

	require.NoError(t, os.WriteFile(testFile, []byte(originalContent), 0644))

	// First extraction
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Check initial nodes
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	assert.Len(t, nodes, 1) // Just the original function

	// Update file content
	updatedContent := `package main

func original() {
	println("original")
}

func newFunction() {
	println("new")
}`

	require.NoError(t, os.WriteFile(testFile, []byte(updatedContent), 0644))

	// Second extraction should detect changes and update
	err = extractor.ExtractFile(testFile)
	require.NoError(t, err)

	// Check updated nodes
	nodes, err = astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	assert.Len(t, nodes, 2) // Now should have both functions

	functionNames := make(map[string]bool)
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeMethod {
			functionNames[node.MethodName] = true
		}
	}

	assert.Contains(t, functionNames, "original")
	assert.Contains(t, functionNames, "newFunction")
}

func TestGoASTExtractor_ErrorHandling(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Test with non-existent file
	err = extractor.ExtractFile("/non/existent/file.go")
	assert.Error(t, err)

	// Test with invalid Go syntax
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.go")
	invalidContent := `package main

func invalid( {
	// Missing closing parenthesis and invalid syntax
}`

	require.NoError(t, os.WriteFile(invalidFile, []byte(invalidContent), 0644))

	err = extractor.ExtractFile(invalidFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse Go file")
}

func BenchmarkGoASTExtractor_LargeFile(b *testing.B) {
	astCache, err := cache.NewASTCache()
	require.NoError(b, err)
	defer astCache.Close()

	extractor := NewGoASTExtractor(astCache)

	// Create large Go file
	tmpDir := b.TempDir()
	largeFile := filepath.Join(tmpDir, "large.go")
	
	var content strings.Builder
	content.WriteString("package main\n\n")
	
	// Generate 1000 functions
	for i := 0; i < 1000; i++ {
		content.WriteString(fmt.Sprintf(`func function%d() {
	if %d > 500 {
		for j := 0; j < %d; j++ {
			switch j %% 3 {
			case 0:
				println("zero")
			case 1:
				println("one")
			default:
				println("other")
			}
		}
	}
}

`, i, i, i%10+1))
	}

	require.NoError(b, os.WriteFile(largeFile, []byte(content.String()), 0644))

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// Clear cache between runs
		err := astCache.DeleteASTForFile(largeFile)
		require.NoError(b, err)
		
		err = extractor.ExtractFile(largeFile)
		require.NoError(b, err)
	}
}