package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestASTCache_NewASTCache(t *testing.T) {
	// Use temporary directory for isolated database
	tmpDir := t.TempDir()
	cache, err := newASTCacheWithPath(tmpDir)
	require.NoError(t, err)
	defer cache.Close()

	assert.NotNil(t, cache)
	assert.NotNil(t, cache.db)
}

func TestASTCache_StoreAndRetrieveNodes(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := newASTCacheWithPath(tmpDir)
	require.NoError(t, err)
	defer cache.Close()

	// Create test node
	node := &models.ASTNode{
		FilePath:             "/test/example.go",
		PackageName:          "main",
		TypeName:             "TestStruct",
		MethodName:           "TestMethod",
		NodeType:             models.NodeTypeMethod,
		StartLine:            10,
		EndLine:              20,
		LineCount:            11,
		CyclomaticComplexity: 5,
		LastModified:         time.Now(),
		FileHash:             "abc123",
	}

	// Store node
	nodeID, err := cache.StoreASTNode(node)
	require.NoError(t, err)
	assert.Greater(t, nodeID, int64(0))

	// Retrieve node
	retrieved, err := cache.GetASTNode(nodeID)
	require.NoError(t, err)
	assert.Equal(t, node.FilePath, retrieved.FilePath)
	assert.Equal(t, node.PackageName, retrieved.PackageName)
	assert.Equal(t, node.TypeName, retrieved.TypeName)
	assert.Equal(t, node.MethodName, retrieved.MethodName)
	assert.Equal(t, node.CyclomaticComplexity, retrieved.CyclomaticComplexity)
}

func TestASTCache_FileHashValidation(t *testing.T) {
	cacheDir := t.TempDir()
	cache, err := newASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer cache.Close()

	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main
func main() {
	println("hello")
}`
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0644))

	// First analysis - file needs analysis
	needs, err := cache.NeedsReanalysis(testFile)
	require.NoError(t, err)
	assert.True(t, needs)

	// Update file metadata
	require.NoError(t, cache.UpdateFileMetadata(testFile))

	// Second check - file shouldn't need reanalysis
	needs, err = cache.NeedsReanalysis(testFile)
	require.NoError(t, err)
	assert.False(t, needs)

	// Modify file
	time.Sleep(10 * time.Millisecond) // Ensure different mtime
	modifiedContent := `package main
func main() {
	println("hello world")
}`
	require.NoError(t, os.WriteFile(testFile, []byte(modifiedContent), 0644))

	// File should need reanalysis after modification
	needs, err = cache.NeedsReanalysis(testFile)
	require.NoError(t, err)
	assert.True(t, needs)
}

func TestASTCache_Relationships(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := newASTCacheWithPath(tmpDir)
	require.NoError(t, err)
	defer cache.Close()

	// Create two nodes
	fromNode := &models.ASTNode{
		FilePath:    "/test/example.go",
		PackageName: "main",
		MethodName:  "caller",
		NodeType:    models.NodeTypeMethod,
		StartLine:   1,
		EndLine:     5,
	}
	toNode := &models.ASTNode{
		FilePath:    "/test/example.go",
		PackageName: "main",
		MethodName:  "callee",
		NodeType:    models.NodeTypeMethod,
		StartLine:   10,
		EndLine:     15,
	}

	fromID, err := cache.StoreASTNode(fromNode)
	require.NoError(t, err)
	toID, err := cache.StoreASTNode(toNode)
	require.NoError(t, err)

	// Store relationship
	err = cache.StoreASTRelationship(fromID, &toID, 3, models.RelationshipCall, "callee()")
	require.NoError(t, err)

	// Retrieve relationships
	rels, err := cache.GetASTRelationships(fromID, models.RelationshipCall)
	require.NoError(t, err)
	assert.Len(t, rels, 1)
	assert.Equal(t, int64(3), int64(rels[0].LineNo))
	assert.Equal(t, "callee()", rels[0].Text)
	assert.Equal(t, toID, *rels[0].ToASTID)
}

func TestASTCache_LibraryNodes(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := newASTCacheWithPath(tmpDir)
	require.NoError(t, err)
	defer cache.Close()

	// Store library node
	libID, err := cache.StoreLibraryNode("fmt", "", "Printf", "", models.NodeTypeMethod, "go", "stdlib")
	require.NoError(t, err)
	assert.Greater(t, libID, int64(0))

	// Store AST node that uses library
	node := &models.ASTNode{
		FilePath:    "/test/example.go",
		PackageName: "main",
		MethodName:  "main",
		NodeType:    models.NodeTypeMethod,
		StartLine:   1,
		EndLine:     5,
	}
	nodeID, err := cache.StoreASTNode(node)
	require.NoError(t, err)

	// Store library relationship
	err = cache.StoreLibraryRelationship(nodeID, libID, 3, models.RelationshipCall, "fmt.Printf()")
	require.NoError(t, err)

	// Retrieve library relationships
	libRels, err := cache.GetLibraryRelationships(nodeID, models.RelationshipCall)
	require.NoError(t, err)
	assert.Len(t, libRels, 1)
	assert.Equal(t, int64(3), int64(libRels[0].LineNo))
	assert.Equal(t, "fmt.Printf()", libRels[0].Text)
	assert.Equal(t, "fmt", libRels[0].LibraryNode.Package)
	assert.Equal(t, "Printf", libRels[0].LibraryNode.Method)
}

func TestASTCache_DeleteASTForFile(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := newASTCacheWithPath(tmpDir)
	require.NoError(t, err)
	defer cache.Close()

	filePath := "/test/example.go"

	// Store multiple nodes for the same file
	node1 := &models.ASTNode{
		FilePath:    filePath,
		PackageName: "main",
		MethodName:  "method1",
		NodeType:    models.NodeTypeMethod,
		StartLine:   1,
		EndLine:     5,
	}
	node2 := &models.ASTNode{
		FilePath:    filePath,
		PackageName: "main",
		MethodName:  "method2",
		NodeType:    models.NodeTypeMethod,
		StartLine:   10,
		EndLine:     15,
	}

	nodeID1, err := cache.StoreASTNode(node1)
	require.NoError(t, err)
	nodeID2, err := cache.StoreASTNode(node2)
	require.NoError(t, err)

	// Store relationship between nodes
	err = cache.StoreASTRelationship(nodeID1, &nodeID2, 3, models.RelationshipCall, "method2()")
	require.NoError(t, err)

	// Verify nodes exist
	nodes, err := cache.GetASTNodesByFile(filePath)
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	// Delete all AST data for file
	err = cache.DeleteASTForFile(filePath)
	require.NoError(t, err)

	// Verify nodes are deleted
	nodes, err = cache.GetASTNodesByFile(filePath)
	require.NoError(t, err)
	assert.Len(t, nodes, 0)

	// Verify relationships are also deleted
	rels, err := cache.GetASTRelationships(nodeID1, models.RelationshipCall)
	require.NoError(t, err)
	assert.Len(t, rels, 0)
}

func TestASTCache_QueryRaw(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := newASTCacheWithPath(tmpDir)
	require.NoError(t, err)
	defer cache.Close()

	// Store test nodes with different complexity levels
	nodes := []*models.ASTNode{
		{
			FilePath:             "/test/low.go",
			MethodName:           "simple",
			NodeType:             models.NodeTypeMethod,
			CyclomaticComplexity: 1,
			StartLine:            1,
			EndLine:              5,
		},
		{
			FilePath:             "/test/high.go",
			MethodName:           "complex",
			NodeType:             models.NodeTypeMethod,
			CyclomaticComplexity: 15,
			StartLine:            1,
			EndLine:              30,
		},
	}

	for _, node := range nodes {
		_, err := cache.StoreASTNode(node)
		require.NoError(t, err)
	}

	// Query for high complexity methods
	rows, err := cache.QueryRaw("SELECT method_name, cyclomatic_complexity FROM ast_nodes WHERE cyclomatic_complexity > ?", 10)
	require.NoError(t, err)
	defer rows.Close()

	var results []struct {
		Method     string
		Complexity int
	}
	for rows.Next() {
		var result struct {
			Method     string
			Complexity int
		}
		err := rows.Scan(&result.Method, &result.Complexity)
		require.NoError(t, err)
		results = append(results, result)
	}

	assert.Len(t, results, 1)
	assert.Equal(t, "complex", results[0].Method)
	assert.Equal(t, 15, results[0].Complexity)
}

func TestASTCache_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := newASTCacheWithPath(tmpDir)
	require.NoError(t, err)
	defer cache.Close()

	// Test concurrent node storage
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			node := &models.ASTNode{
				FilePath:   "/test/concurrent.go",
				MethodName: fmt.Sprintf("method%d", id),
				NodeType:   models.NodeTypeMethod,
				StartLine:  id * 10,
				EndLine:    id*10 + 5,
			}
			_, err := cache.StoreASTNode(node)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all nodes were stored
	nodes, err := cache.GetASTNodesByFile("/test/concurrent.go")
	require.NoError(t, err)
	assert.Len(t, nodes, 10)
}
