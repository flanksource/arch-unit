package ast

import (
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteMetricQuery_NewSyntax(t *testing.T) {
	// Create a test cache
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

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
	require.NoError(t, err)
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
	require.NoError(t, err)
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
	require.NoError(t, err)
	node3.ID = id3

	// Create analyzer
	analyzer := NewAnalyzer(astCache, "/test")

	tests := []struct {
		name          string
		query         string
		expectedCount int
		checkNodes    func(t *testing.T, nodes []*models.ASTNode)
	}{
		{
			name:          "New syntax - lines metric",
			query:         "lines(*) > 100",
			expectedCount: 1,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				assert.Equal(t, "GetUserByIDWithComplexValidation", nodes[0].MethodName)
			},
		},
		{
			name:          "New syntax - cyclomatic metric",
			query:         "cyclomatic(*) >= 5",
			expectedCount: 2,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				methods := []string{nodes[0].MethodName, nodes[1].MethodName}
				assert.Contains(t, methods, "GetUserByIDWithComplexValidation")
				assert.Contains(t, methods, "Validate")
			},
		},
		{
			name:          "New syntax - params alias",
			query:         "params(*) > 4",
			expectedCount: 1,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				assert.Equal(t, 6, nodes[0].ParameterCount)
			},
		},
		{
			name:          "New syntax - len metric for long names",
			query:         "len(*) > 40",
			expectedCount: 1,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				// controllers:UserController:GetUserByIDWithComplexValidation is > 40 chars
				assert.Equal(t, "GetUserByIDWithComplexValidation", nodes[0].MethodName)
			},
		},
		{
			name:          "New syntax with pattern",
			query:         "lines(services:*) < 100",
			expectedCount: 1,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				assert.Equal(t, "services", nodes[0].PackageName)
				assert.Equal(t, "Send", nodes[0].MethodName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := analyzer.ExecuteAQLQuery(tt.query)
			require.NoError(t, err)
			assert.Len(t, nodes, tt.expectedCount)
			if tt.checkNodes != nil && len(nodes) > 0 {
				tt.checkNodes(t, nodes)
			}
		})
	}
}

func TestExecuteMetricQuery_InvalidQueries(t *testing.T) {
	// Create a test cache
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	// Add a test node so pattern matching works
	node := &models.ASTNode{
		FilePath:    "/test/file.go",
		PackageName: "test",
		MethodName:  "TestMethod",
		NodeType:    models.NodeTypeMethod,
		LineCount:   10,
	}
	_, err = astCache.StoreASTNode(node)
	require.NoError(t, err)

	analyzer := NewAnalyzer(astCache, "/test")

	tests := []struct {
		name        string
		query       string
		expectedErr string
	}{
		{
			name:        "Invalid metric name",
			query:       "unknown(*) > 10",
			expectedErr: "unknown metric: unknown",
		},
		{
			name:        "Invalid value",
			query:       "lines(*) > abc",
			expectedErr: "invalid numeric value",
		},
		{
			name:        "Old dot syntax no longer supported",
			query:       "*.lines > 100",
			expectedErr: "invalid metric query format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := analyzer.ExecuteAQLQuery(tt.query)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestExecuteMetricQuery_EdgeCases(t *testing.T) {
	// Create a test cache
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	analyzer := NewAnalyzer(astCache, "/test")

	tests := []struct {
		name          string
		query         string
		expectedCount int
		description   string
	}{
		{
			name:          "Function syntax without operator treated as pattern",
			query:         "lines(*)",
			expectedCount: 0,
			description:   "Parsed as method name pattern, returns no results",
		},
		{
			name:          "Empty parentheses defaults to wildcard",
			query:         "lines() > 50",
			expectedCount: 0,
			description:   "Empty pattern defaults to *, no nodes in empty cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := analyzer.ExecuteAQLQuery(tt.query)
			require.NoError(t, err, tt.description)
			assert.Len(t, nodes, tt.expectedCount, tt.description)
		})
	}
}

func TestExecuteMetricQuery_RelationshipMetrics(t *testing.T) {
	// Create a test cache
	astCache, err := cache.NewASTCacheWithPath(t.TempDir())
	require.NoError(t, err)
	defer astCache.Close()

	// Insert test nodes
	node1 := &models.ASTNode{
		FilePath:    "/test/file1.go",
		PackageName: "controllers",
		TypeName:    "UserController",
		MethodName:  "GetUser",
		NodeType:    models.NodeTypeMethod,
	}
	id1, err := astCache.StoreASTNode(node1)
	require.NoError(t, err)

	node2 := &models.ASTNode{
		FilePath:    "/test/file2.go",
		PackageName: "services",
		TypeName:    "UserService",
		MethodName:  "FindUser",
		NodeType:    models.NodeTypeMethod,
	}
	id2, err := astCache.StoreASTNode(node2)
	require.NoError(t, err)

	// Add import relationships
	err = astCache.StoreASTRelationship(id1, nil, 10, "import", "import services")
	require.NoError(t, err)
	err = astCache.StoreASTRelationship(id1, nil, 11, "import", "import models")
	require.NoError(t, err)
	err = astCache.StoreASTRelationship(id1, nil, 12, "import", "import utils")
	require.NoError(t, err)

	// Add call relationships (external calls)
	err = astCache.StoreASTRelationship(id1, &id2, 20, "call", "services.UserService.FindUser()")
	require.NoError(t, err)
	err = astCache.StoreASTRelationship(id1, nil, 21, "call", "fmt.Println()")
	require.NoError(t, err)

	analyzer := NewAnalyzer(astCache, "/test")

	tests := []struct {
		name          string
		query         string
		expectedCount int
		checkNodes    func(t *testing.T, nodes []*models.ASTNode)
	}{
		{
			name:          "Count imports",
			query:         "imports(*) > 2",
			expectedCount: 1,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				assert.Equal(t, "GetUser", nodes[0].MethodName)
			},
		},
		{
			name:          "Count external calls",
			query:         "calls(*) >= 2",
			expectedCount: 1,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				assert.Equal(t, "GetUser", nodes[0].MethodName)
			},
		},
		{
			name:          "No imports",
			query:         "imports(*) == 0",
			expectedCount: 1,
			checkNodes: func(t *testing.T, nodes []*models.ASTNode) {
				assert.Equal(t, "FindUser", nodes[0].MethodName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := analyzer.ExecuteAQLQuery(tt.query)
			require.NoError(t, err)
			assert.Len(t, nodes, tt.expectedCount)
			if tt.checkNodes != nil && len(nodes) > 0 {
				tt.checkNodes(t, nodes)
			}
		})
	}
}
