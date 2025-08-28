package ast

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzer_AnalyzeFiles(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test Go file
	goFile := filepath.Join(tmpDir, "test.go")
	goContent := `package main

import "fmt"

type Calculator struct {
	value int
}

func (c *Calculator) Add(x int) {
	if x > 0 {
		c.value += x
	} else {
		c.value -= x
	}
}

func main() {
	calc := &Calculator{}
	calc.Add(10)
	fmt.Println(calc.value)
}
`
	err := os.WriteFile(goFile, []byte(goContent), 0644)
	require.NoError(t, err)

	// Create test Python file
	pyFile := filepath.Join(tmpDir, "test.py")
	pyContent := `
class Calculator:
    def __init__(self):
        self.value = 0
    
    def add(self, x):
        if x > 0:
            self.value += x
        else:
            self.value -= abs(x)
        return self.value

def main():
    calc = Calculator()
    calc.add(10)
    print(calc.value)

if __name__ == "__main__":
    main()
`
	err = os.WriteFile(pyFile, []byte(pyContent), 0644)
	require.NoError(t, err)

	// Create AST cache
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Create analyzer
	analyzer := NewAnalyzer(astCache, tmpDir)

	// Analyze files
	err = analyzer.AnalyzeFiles()
	assert.NoError(t, err)

	// Verify nodes were created
	nodes, err := analyzer.QueryPattern("*")
	require.NoError(t, err)
	assert.NotEmpty(t, nodes)

	// Check for specific nodes
	var foundGoCalculator bool
	for _, node := range nodes {
		if node.TypeName == "Calculator" && filepath.Ext(node.FilePath) == ".go" {
			foundGoCalculator = true
		}
	}

	assert.True(t, foundGoCalculator, "Should find Go Calculator type")
}

func TestAnalyzer_QueryPattern(t *testing.T) {
	// Create test cache with sample data
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Insert test data
	_, err = astCache.StoreASTNode(&models.ASTNode{
		FilePath:    "/test/src/controller.go",
		PackageName: "controllers",
		TypeName:    "UserController",
		MethodName:  "GetUser",
		NodeType:    "method",
		StartLine:   10,
		EndLine:     20,
	})
	require.NoError(t, err)

	_, err = astCache.StoreASTNode(&models.ASTNode{
		FilePath:    "/test/src/service.go",
		PackageName: "services",
		TypeName:    "UserService",
		MethodName:  "CreateUser",
		NodeType:    "method",
		StartLine:   15,
		EndLine:     30,
	})
	require.NoError(t, err)

	// Create analyzer
	analyzer := NewAnalyzer(astCache, "/test")

	tests := []struct {
		name     string
		pattern  string
		expected int
	}{
		{
			name:     "All nodes",
			pattern:  "*",
			expected: 2,
		},
		{
			name:     "Controller pattern",
			pattern:  "*Controller*",
			expected: 1,
		},
		{
			name:     "Service pattern",
			pattern:  "*Service*",
			expected: 1,
		},
		{
			name:     "Specific method",
			pattern:  "controllers:UserController:GetUser",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := analyzer.QueryPattern(tt.pattern)
			require.NoError(t, err)
			assert.Len(t, nodes, tt.expected)
		})
	}
}

func TestFilterByComplexity(t *testing.T) {
	nodes := []*models.ASTNode{
		{MethodName: "simple", CyclomaticComplexity: 2},
		{MethodName: "medium", CyclomaticComplexity: 5},
		{MethodName: "complex", CyclomaticComplexity: 10},
		{MethodName: "veryComplex", CyclomaticComplexity: 20},
	}

	filtered := FilterByComplexity(nodes, 10)
	assert.Len(t, filtered, 2)
	assert.Equal(t, "complex", filtered[0].MethodName)
	assert.Equal(t, "veryComplex", filtered[1].MethodName)
}

func TestFilterByNodeType(t *testing.T) {
	nodes := []*models.ASTNode{
		{MethodName: "method1", NodeType: "method"},
		{TypeName: "Type1", NodeType: "type"},
		{MethodName: "method2", NodeType: "method"},
		{FieldName: "field1", NodeType: "field"},
	}

	filtered := FilterByNodeType(nodes, "method")
	assert.Len(t, filtered, 2)
	for _, node := range filtered {
		assert.Equal(t, "method", string(node.NodeType))
	}
}

func TestAnalyzer_ExecuteMetricQuery(t *testing.T) {
	// Create test cache with sample data
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Insert test data with various line counts and complexity
	testNodes := []models.ASTNode{
		{
			FilePath:             "/test/src/small.go",
			PackageName:          "test",
			MethodName:           "SmallMethod",
			NodeType:             "method",
			LineCount:            50,
			CyclomaticComplexity: 3,
			ParameterCount:       2,
		},
		{
			FilePath:             "/test/src/large.go",
			PackageName:          "test",
			MethodName:           "LargeMethod",
			NodeType:             "method",
			LineCount:            150,
			CyclomaticComplexity: 15,
			ParameterCount:       5,
		},
		{
			FilePath:             "/test/src/complex.go",
			PackageName:          "test",
			MethodName:           "ComplexMethod",
			NodeType:             "method",
			LineCount:            80,
			CyclomaticComplexity: 20,
			ParameterCount:       3,
		},
	}

	for _, node := range testNodes {
		_, err = astCache.StoreASTNode(&node)
		require.NoError(t, err)
	}

	// Create analyzer
	analyzer := NewAnalyzer(astCache, "/test")

	tests := []struct {
		name     string
		query    string
		expected []string // Expected method names
	}{
		{
			name:     "Lines greater than 100",
			query:    "lines(*) > 100",
			expected: []string{"LargeMethod"},
		},
		{
			name:     "Complexity greater than or equal to 15",
			query:    "cyclomatic(*) >= 15",
			expected: []string{"LargeMethod", "ComplexMethod"},
		},
		{
			name:     "Parameters greater than 3",
			query:    "parameters(*) > 3",
			expected: []string{"LargeMethod"},
		},
		{
			name:     "Lines less than 100",
			query:    "lines(*) < 100",
			expected: []string{"SmallMethod", "ComplexMethod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := analyzer.ExecuteAQLQuery(tt.query)
			require.NoError(t, err)

			var methodNames []string
			for _, node := range nodes {
				if node.MethodName != "" {
					methodNames = append(methodNames, node.MethodName)
				}
			}

			assert.ElementsMatch(t, tt.expected, methodNames)
		})
	}
}
