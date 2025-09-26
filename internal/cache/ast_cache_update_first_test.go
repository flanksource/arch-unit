package cache

import (
	"os"
	"testing"

	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStoreFileResults_UpdateFirst(t *testing.T) {
	// Create temporary cache for testing
	tempDir := t.TempDir()
	cache, err := NewASTCacheWithPath(tempDir)
	require.NoError(t, err)
	defer func() { _ = cache.Close() }()

	filePath := "test_file.go"

	// Create test file for metadata calculation
	testFile := tempDir + "/" + filePath
	err = os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
	require.NoError(t, err)

	// Create first analysis result with some nodes
	firstResult := &struct {
		Nodes         []*models.ASTNode
		Relationships []*models.ASTRelationship
		Libraries     []*models.LibraryRelationship
	}{
		Nodes: []*models.ASTNode{
			{
				ID:          1, // Analysis ID
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "main",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   3,
				EndLine:     3,
			},
			{
				ID:          2, // Analysis ID
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "TestStruct",
				MethodName:  "",
				FieldName:   "Field1",
				NodeType:    "field",
				StartLine:   5,
				EndLine:     5,
			},
		},
		Relationships: []*models.ASTRelationship{
			{
				FromASTID:        1,
				ToASTID:          nil,
				LineNo:           3,
				RelationshipType: "call",
				Text:             "main()",
			},
		},
		Libraries: []*models.LibraryRelationship{},
	}

	// Store first result
	err = cache.StoreFileResults(testFile, firstResult)
	require.NoError(t, err)

	// Verify nodes were created
	nodes1, err := cache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	require.Len(t, nodes1, 2)

	// Record the IDs assigned by the database
	var mainMethodID, field1ID int64
	for _, node := range nodes1 {
		if node.MethodName == "main" {
			mainMethodID = node.ID
		} else if node.FieldName == "Field1" {
			field1ID = node.ID
		}
	}

	// Ensure IDs were assigned
	assert.Greater(t, mainMethodID, int64(0))
	assert.Greater(t, field1ID, int64(0))

	// Create second analysis result - simulate re-analysis with changes
	// - Keep the main method (should preserve ID)
	// - Modify Field1 (should preserve ID but update data)
	// - Add a new field (should get new ID)
	// - Remove nothing (test orphan cleanup separately)
	secondResult := &struct {
		Nodes         []*models.ASTNode
		Relationships []*models.ASTRelationship
		Libraries     []*models.LibraryRelationship
	}{
		Nodes: []*models.ASTNode{
			{
				ID:          1, // Same analysis ID for main method
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "main",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   3,
				EndLine:     3,
				LineCount:   5, // Updated line count
			},
			{
				ID:          2, // Same analysis ID for Field1 but modified
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "TestStruct",
				MethodName:  "",
				FieldName:   "Field1",
				NodeType:    "field",
				StartLine:   5,
				EndLine:     5,
				LineCount:   10, // Updated line count
			},
			{
				ID:          3, // New field
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "TestStruct",
				MethodName:  "",
				FieldName:   "Field2",
				NodeType:    "field",
				StartLine:   6,
				EndLine:     6,
			},
		},
		Relationships: []*models.ASTRelationship{
			{
				FromASTID:        1,
				ToASTID:          nil,
				LineNo:           3,
				RelationshipType: "call",
				Text:             "main()",
			},
		},
		Libraries: []*models.LibraryRelationship{},
	}

	// Store second result (re-analysis)
	err = cache.StoreFileResults(testFile, secondResult)
	require.NoError(t, err)

	// Verify the results
	nodes2, err := cache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	require.Len(t, nodes2, 3) // Should have 3 nodes now

	// Verify ID preservation and updates
	var mainMethod2, field1_2, field2 *models.ASTNode
	for _, node := range nodes2 {
		switch {
		case node.MethodName == "main":
			mainMethod2 = node
		case node.FieldName == "Field1":
			field1_2 = node
		case node.FieldName == "Field2":
			field2 = node
		}
	}

	require.NotNil(t, mainMethod2)
	require.NotNil(t, field1_2)
	require.NotNil(t, field2)

	// Verify ID preservation
	assert.Equal(t, mainMethodID, mainMethod2.ID, "main method ID should be preserved")
	assert.Equal(t, field1ID, field1_2.ID, "Field1 ID should be preserved")

	// Verify data updates
	assert.Equal(t, 5, mainMethod2.LineCount, "main method LineCount should be updated")
	assert.Equal(t, 10, field1_2.LineCount, "Field1 LineCount should be updated")

	// Verify new node got a new ID
	assert.Greater(t, field2.ID, int64(0))
	assert.NotEqual(t, mainMethodID, field2.ID)
	assert.NotEqual(t, field1ID, field2.ID)
}

func TestStoreFileResults_OrphanCleanup(t *testing.T) {
	// Create temporary cache for testing
	tempDir := t.TempDir()
	cache, err := NewASTCacheWithPath(tempDir)
	require.NoError(t, err)
	defer func() { _ = cache.Close() }()

	filePath := "test_orphan.go"

	// Create test file for metadata calculation
	testFile := tempDir + "/" + filePath
	err = os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
	require.NoError(t, err)

	// Create first analysis result with 3 nodes
	firstResult := &struct {
		Nodes         []*models.ASTNode
		Relationships []*models.ASTRelationship
		Libraries     []*models.LibraryRelationship
	}{
		Nodes: []*models.ASTNode{
			{
				ID:          1,
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "func1",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   3,
			},
			{
				ID:          2,
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "func2",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   5,
			},
			{
				ID:          3,
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "func3",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   7,
			},
		},
		Relationships: []*models.ASTRelationship{},
		Libraries:     []*models.LibraryRelationship{},
	}

	// Store first result
	err = cache.StoreFileResults(testFile, firstResult)
	require.NoError(t, err)

	// Verify 3 nodes exist
	nodes1, err := cache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	require.Len(t, nodes1, 3)

	// Create second analysis result with only 2 nodes (func2 removed)
	secondResult := &struct {
		Nodes         []*models.ASTNode
		Relationships []*models.ASTRelationship
		Libraries     []*models.LibraryRelationship
	}{
		Nodes: []*models.ASTNode{
			{
				ID:          1, // Keep func1
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "func1",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   3,
			},
			{
				ID:          3, // Keep func3 (func2 is being removed)
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "func3",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   7,
			},
		},
		Relationships: []*models.ASTRelationship{},
		Libraries:     []*models.LibraryRelationship{},
	}

	// Store second result (should remove func2)
	err = cache.StoreFileResults(testFile, secondResult)
	require.NoError(t, err)

	// Verify only 2 nodes remain
	nodes2, err := cache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	require.Len(t, nodes2, 2)

	// Verify the right nodes remain
	methodNames := make([]string, len(nodes2))
	for i, node := range nodes2 {
		methodNames[i] = node.MethodName
	}

	assert.Contains(t, methodNames, "func1")
	assert.Contains(t, methodNames, "func3")
	assert.NotContains(t, methodNames, "func2")
}

func TestStoreFileResults_RelationshipUpdate(t *testing.T) {
	// Create temporary cache for testing
	tempDir := t.TempDir()
	cache, err := NewASTCacheWithPath(tempDir)
	require.NoError(t, err)
	defer func() { _ = cache.Close() }()

	filePath := "test_relationships.go"

	// Create test file for metadata calculation
	testFile := tempDir + "/" + filePath
	err = os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
	require.NoError(t, err)

	// Create first analysis result with nodes and relationships
	firstResult := &struct {
		Nodes         []*models.ASTNode
		Relationships []*models.ASTRelationship
		Libraries     []*models.LibraryRelationship
	}{
		Nodes: []*models.ASTNode{
			{
				ID:          1,
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "caller",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   3,
			},
			{
				ID:          2,
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "callee",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   5,
			},
		},
		Relationships: []*models.ASTRelationship{
			{
				FromASTID:        1, // caller calls callee
				ToASTID:          &[]int64{2}[0],
				LineNo:           4,
				RelationshipType: "call",
				Text:             "callee()",
			},
		},
		Libraries: []*models.LibraryRelationship{},
	}

	// Store first result
	err = cache.StoreFileResults(testFile, firstResult)
	require.NoError(t, err)

	// Get the actual database IDs
	nodes1, err := cache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	require.Len(t, nodes1, 2)

	var callerID, calleeID int64
	for _, node := range nodes1 {
		if node.MethodName == "caller" {
			callerID = node.ID
		} else if node.MethodName == "callee" {
			calleeID = node.ID
		}
	}

	// Verify relationship was created
	relationships1, err := cache.GetASTRelationships(callerID, "call")
	require.NoError(t, err)
	require.Len(t, relationships1, 1)
	assert.Equal(t, calleeID, *relationships1[0].ToASTID)

	// Create second analysis result with updated relationships
	secondResult := &struct {
		Nodes         []*models.ASTNode
		Relationships []*models.ASTRelationship
		Libraries     []*models.LibraryRelationship
	}{
		Nodes: []*models.ASTNode{
			{
				ID:          1, // Same nodes
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "caller",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   3,
			},
			{
				ID:          2,
				FilePath:    testFile,
				PackageName: "main",
				TypeName:    "",
				MethodName:  "callee",
				FieldName:   "",
				NodeType:    "method",
				StartLine:   5,
			},
		},
		Relationships: []*models.ASTRelationship{
			{
				FromASTID:        1, // Same relationship but different line
				ToASTID:          &[]int64{2}[0],
				LineNo:           6, // Updated line number
				RelationshipType: "call",
				Text:             "callee()",
			},
		},
		Libraries: []*models.LibraryRelationship{},
	}

	// Store second result
	err = cache.StoreFileResults(testFile, secondResult)
	require.NoError(t, err)

	// Verify nodes still have same IDs
	nodes2, err := cache.GetASTNodesByFile(testFile)
	require.NoError(t, err)
	require.Len(t, nodes2, 2)

	for _, node := range nodes2 {
		if node.MethodName == "caller" {
			assert.Equal(t, callerID, node.ID, "caller ID should be preserved")
		} else if node.MethodName == "callee" {
			assert.Equal(t, calleeID, node.ID, "callee ID should be preserved")
		}
	}

	// Verify relationship was updated
	relationships2, err := cache.GetASTRelationships(callerID, "call")
	require.NoError(t, err)
	require.Len(t, relationships2, 1)
	assert.Equal(t, 6, relationships2[0].LineNo, "relationship line number should be updated")
}

func TestFindExistingNodeByNaturalKey(t *testing.T) {
	// Create temporary cache for testing
	tempDir := t.TempDir()
	cache, err := NewASTCacheWithPath(tempDir)
	require.NoError(t, err)
	defer func() { _ = cache.Close() }()

	// Create a test node
	testNode := &models.ASTNode{
		FilePath:    "test.go",
		PackageName: "main",
		TypeName:    "TestStruct",
		MethodName:  "TestMethod",
		FieldName:   "",
		NodeType:    "method",
		StartLine:   10,
		EndLine:     15,
	}

	// Store the node
	id, err := cache.StoreASTNode(testNode)
	require.NoError(t, err)
	testNode.ID = id

	// Test finding the node by natural key
	cache.db.Transaction(func(tx *gorm.DB) error {
		// Create a search node with same natural key but different metadata
		searchNode := &models.ASTNode{
			FilePath:    "test.go",
			PackageName: "main",
			TypeName:    "TestStruct",
			MethodName:  "TestMethod",
			FieldName:   "",
			StartLine:   20, // Different line number
			EndLine:     25, // Different line number
		}

		found, err := cache.findExistingNodeByNaturalKey(tx, searchNode)
		require.NoError(t, err)
		require.NotNil(t, found)

		assert.Equal(t, testNode.ID, found.ID)
		assert.Equal(t, "test.go", found.FilePath)
		assert.Equal(t, "main", found.PackageName)
		assert.Equal(t, "TestStruct", found.TypeName)
		assert.Equal(t, "TestMethod", found.MethodName)
		assert.Equal(t, "", found.FieldName)

		return nil
	})

	// Test not finding a node with different natural key
	cache.db.Transaction(func(tx *gorm.DB) error {
		differentNode := &models.ASTNode{
			FilePath:    "test.go",
			PackageName: "main",
			TypeName:    "TestStruct",
			MethodName:  "DifferentMethod", // Different method name
			FieldName:   "",
		}

		found, err := cache.findExistingNodeByNaturalKey(tx, differentNode)
		require.NoError(t, err)
		assert.Nil(t, found) // Should not find anything

		return nil
	})
}