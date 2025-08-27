package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB_ConcurrentWrites(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "db-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE test_table (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			value TEXT NOT NULL,
			thread_id INTEGER NOT NULL
		)
	`)
	require.NoError(t, err)

	// Test concurrent writes
	const numGoroutines = 10
	const writesPerGoroutine = 10
	
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	startTime := time.Now()
	
	for i := 0; i < numGoroutines; i++ {
		go func(threadID int) {
			defer wg.Done()
			
			for j := 0; j < writesPerGoroutine; j++ {
				value := fmt.Sprintf("thread-%d-write-%d", threadID, j)
				_, err := db.Exec(
					"INSERT INTO test_table (value, thread_id) VALUES (?, ?)",
					value, threadID,
				)
				assert.NoError(t, err)
			}
		}(i)
	}
	
	wg.Wait()
	
	duration := time.Since(startTime)
	t.Logf("Concurrent writes completed in %v", duration)
	
	// Verify all writes succeeded
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_table").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*writesPerGoroutine, count)
	
	// Verify writes from each thread
	for i := 0; i < numGoroutines; i++ {
		var threadCount int
		err = db.QueryRow(
			"SELECT COUNT(*) FROM test_table WHERE thread_id = ?",
			i,
		).Scan(&threadCount)
		require.NoError(t, err)
		assert.Equal(t, writesPerGoroutine, threadCount)
	}
}

func TestDB_TransactionIsolation(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "db-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Create test table
	_, err = db.Exec(`
		CREATE TABLE test_table (
			id INTEGER PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	require.NoError(t, err)

	// Insert initial data
	_, err = db.Exec("INSERT INTO test_table (id, value) VALUES (1, 'initial')")
	require.NoError(t, err)

	// Start two transactions concurrently
	var wg sync.WaitGroup
	wg.Add(2)
	
	errors := make(chan error, 2)
	
	// Transaction 1: Update value
	go func() {
		defer wg.Done()
		
		tx, err := db.Begin()
		if err != nil {
			errors <- err
			return
		}
		defer tx.Rollback()
		
		// Update value
		_, err = tx.Exec("UPDATE test_table SET value = 'tx1' WHERE id = 1")
		if err != nil {
			errors <- err
			return
		}
		
		// Sleep to ensure tx2 tries to access
		time.Sleep(50 * time.Millisecond)
		
		err = tx.Commit()
		errors <- err
	}()
	
	// Transaction 2: Try to update same value (should wait)
	go func() {
		defer wg.Done()
		
		// Wait a bit to ensure tx1 starts first
		time.Sleep(10 * time.Millisecond)
		
		tx, err := db.Begin()
		if err != nil {
			errors <- err
			return
		}
		defer tx.Rollback()
		
		// This should wait until tx1 completes
		_, err = tx.Exec("UPDATE test_table SET value = 'tx2' WHERE id = 1")
		if err != nil {
			errors <- err
			return
		}
		
		err = tx.Commit()
		errors <- err
	}()
	
	wg.Wait()
	close(errors)
	
	// Check for errors
	for err := range errors {
		assert.NoError(t, err)
	}
	
	// Verify final value (should be tx2 since it committed last)
	var finalValue string
	err = db.QueryRow("SELECT value FROM test_table WHERE id = 1").Scan(&finalValue)
	require.NoError(t, err)
	assert.Equal(t, "tx2", finalValue)
}