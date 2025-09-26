package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/flanksource/arch-unit/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProtectedGormDB_ConcurrentReads(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewGormDBWithPath(tempDir)
	require.NoError(t, err)

	protectedDB := NewProtectedGormDB(db)

	// Create some test data
	testNode := &models.ASTNode{
		FilePath:    "test_concurrent.go",
		PackageName: "main",
		TypeName:    "",
		MethodName:  "testMethod",
		NodeType:    "method",
		StartLine:   1,
		EndLine:     5,
	}

	// Store test data using write lock
	err = protectedDB.WithWriteLock().Create(testNode)
	require.NoError(t, err)

	// Test concurrent reads
	const numReaders = 10
	var wg sync.WaitGroup
	results := make([]models.ASTNode, numReaders)

	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func(index int) {
			defer wg.Done()

			var node models.ASTNode
			err := protectedDB.WithReadLock().Where("method_name = ?", "testMethod").First(&node)
			assert.NoError(t, err)
			results[index] = node
		}(i)
	}

	wg.Wait()

	// Verify all readers got the same result
	for i, result := range results {
		assert.Equal(t, "testMethod", result.MethodName, "Reader %d got incorrect result", i)
		assert.Equal(t, "main", result.PackageName, "Reader %d got incorrect result", i)
	}
}

func TestProtectedGormDB_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewGormDBWithPath(tempDir)
	require.NoError(t, err)

	protectedDB := NewProtectedGormDB(db)

	// Test concurrent writes
	const numWriters = 5
	var wg sync.WaitGroup

	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(index int) {
			defer wg.Done()

			testNode := &models.ASTNode{
				FilePath:    "test_concurrent.go",
				PackageName: "main",
				TypeName:    "",
				MethodName:  "testMethod" + string(rune('A'+index)),
				NodeType:    "method",
				StartLine:   index + 1,
				EndLine:     index + 5,
			}

			err := protectedDB.WithWriteLock().Create(testNode)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all writes succeeded
	var count int64
	err = protectedDB.WithReadLock().Model(&models.ASTNode{}).Count(&count)
	require.NoError(t, err)
	assert.Equal(t, int64(numWriters), count)
}

func TestProtectedGormDB_MixedReadWrite(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewGormDBWithPath(tempDir)
	require.NoError(t, err)

	protectedDB := NewProtectedGormDB(db)

	// Create initial test data
	for i := 0; i < 5; i++ {
		testNode := &models.ASTNode{
			FilePath:    "test_mixed.go",
			PackageName: "main",
			TypeName:    "",
			MethodName:  "initialMethod" + string(rune('A'+i)),
			NodeType:    "method",
			StartLine:   i + 1,
			EndLine:     i + 5,
		}
		err = protectedDB.WithWriteLock().Create(testNode)
		require.NoError(t, err)
	}

	const numReaders = 3
	const numWriters = 2
	var wg sync.WaitGroup

	// Start readers
	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func(index int) {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				var nodes []models.ASTNode
				err := protectedDB.WithReadLock().Where("file_path = ?", "test_mixed.go").Find(&nodes)
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, len(nodes), 5, "Reader %d iteration %d should find at least 5 nodes", index, j)

				// Small delay to allow writers to interleave
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Start writers
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(index int) {
			defer wg.Done()

			for j := 0; j < 5; j++ {
				testNode := &models.ASTNode{
					FilePath:    "test_mixed.go",
					PackageName: "main",
					TypeName:    "",
					MethodName:  "writerMethod" + string(rune('A'+index)) + string(rune('0'+j)),
					NodeType:    "method",
					StartLine:   (index*10 + j) + 1,
					EndLine:     (index*10 + j) + 5,
				}

				err := protectedDB.WithWriteLock().Create(testNode)
				assert.NoError(t, err)

				// Small delay to allow readers to interleave
				time.Sleep(2 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify final state
	var totalCount int64
	err = protectedDB.WithReadLock().Model(&models.ASTNode{}).Where("file_path = ?", "test_mixed.go").Count(&totalCount)
	require.NoError(t, err)

	expectedCount := int64(5 + (numWriters * 5)) // 5 initial + (2 writers * 5 each)
	assert.Equal(t, expectedCount, totalCount)
}

func TestProtectedGormDB_Transaction(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewGormDBWithPath(tempDir)
	require.NoError(t, err)

	protectedDB := NewProtectedGormDB(db)

	// Test successful transaction
	err = protectedDB.Transaction(func(tx *gorm.DB) error {
		testNode1 := &models.ASTNode{
			FilePath:    "test_tx.go",
			PackageName: "main",
			MethodName:  "method1",
			NodeType:    "method",
			StartLine:   1,
			EndLine:     5,
		}
		testNode2 := &models.ASTNode{
			FilePath:    "test_tx.go",
			PackageName: "main",
			MethodName:  "method2",
			NodeType:    "method",
			StartLine:   6,
			EndLine:     10,
		}

		if err := tx.Create(testNode1).Error; err != nil {
			return err
		}
		return tx.Create(testNode2).Error
	})
	require.NoError(t, err)

	// Verify both nodes were created
	var count int64
	err = protectedDB.WithReadLock().Model(&models.ASTNode{}).Where("file_path = ?", "test_tx.go").Count(&count)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestProtectedQuery_AutoUnlock(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewGormDBWithPath(tempDir)
	require.NoError(t, err)

	protectedDB := NewProtectedGormDB(db)

	// Create test data
	testNode := &models.ASTNode{
		FilePath:    "test_unlock.go",
		PackageName: "main",
		MethodName:  "testMethod",
		NodeType:    "method",
		StartLine:   1,
		EndLine:     5,
	}

	// Test that locks are automatically released after operations
	err = protectedDB.WithWriteLock().Create(testNode)
	require.NoError(t, err)

	// This should not deadlock if the previous lock was properly released
	var foundNode models.ASTNode
	err = protectedDB.WithReadLock().Where("method_name = ?", "testMethod").First(&foundNode)
	require.NoError(t, err)
	assert.Equal(t, "testMethod", foundNode.MethodName)

	// Test multiple chained operations
	var nodes []models.ASTNode
	err = protectedDB.WithReadLock().
		Where("file_path = ?", "test_unlock.go").
		Order("start_line").
		Find(&nodes)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
}

func TestProtectedQuery_ManualUnlock(t *testing.T) {
	tempDir := t.TempDir()
	db, err := NewGormDBWithPath(tempDir)
	require.NoError(t, err)

	protectedDB := NewProtectedGormDB(db)

	// Test manual unlock
	query := protectedDB.WithReadLock()

	// Manually unlock without executing a query
	query.Unlock()

	// Should be able to unlock multiple times safely
	query.Unlock()

	// Should still be able to get a new lock
	var count int64
	err = protectedDB.WithReadLock().Model(&models.ASTNode{}).Count(&count)
	require.NoError(t, err)
}