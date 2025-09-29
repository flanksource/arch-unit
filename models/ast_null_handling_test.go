package models

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestASTNodeNullHandling(t *testing.T) {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Auto-migrate the schema
	err = db.AutoMigrate(&ASTNode{})
	assert.NoError(t, err)

	// Test case 1: Insert node with null values in nullable fields
	nodeWithNulls := &ASTNode{
		FilePath:     "/test/file.go",
		PackageName:  "test",
		NodeType:     NodeTypeMethod,
		StartLine:    10,
		LastModified: time.Now(),
		// Leave Language, Summary, FieldType, DefaultValue as nil
	}

	err = db.Create(nodeWithNulls).Error
	assert.NoError(t, err)

	// Test case 2: Insert node with values in nullable fields
	nodeWithValues := &ASTNode{
		FilePath:     "/test/file2.go",
		PackageName:  "test2",
		NodeType:     NodeTypeField,
		StartLine:    20,
		LastModified: time.Now(),
		Language:     StringPtr("go"),
		Summary:      StringPtr("Test summary"),
		FieldType:    StringPtr("string"),
		DefaultValue: StringPtr("default"),
	}

	err = db.Create(nodeWithValues).Error
	assert.NoError(t, err)

	// Test case 3: Read back the nodes and verify NULL handling
	var retrievedNodes []ASTNode
	err = db.Find(&retrievedNodes).Error
	assert.NoError(t, err)
	assert.Len(t, retrievedNodes, 2)

	// Find the node with null values
	var nullNode *ASTNode
	var valueNode *ASTNode
	for i := range retrievedNodes {
		if retrievedNodes[i].FilePath == "/test/file.go" {
			nullNode = &retrievedNodes[i]
		} else if retrievedNodes[i].FilePath == "/test/file2.go" {
			valueNode = &retrievedNodes[i]
		}
	}

	assert.NotNil(t, nullNode)
	assert.NotNil(t, valueNode)

	// Verify null values are properly handled
	assert.Nil(t, nullNode.Language)
	assert.Nil(t, nullNode.Summary)
	assert.Nil(t, nullNode.FieldType)
	assert.Nil(t, nullNode.DefaultValue)

	// Verify non-null values are properly handled
	assert.NotNil(t, valueNode.Language)
	assert.NotNil(t, valueNode.Summary)
	assert.NotNil(t, valueNode.FieldType)
	assert.NotNil(t, valueNode.DefaultValue)

	assert.Equal(t, "go", *valueNode.Language)
	assert.Equal(t, "Test summary", *valueNode.Summary)
	assert.Equal(t, "string", *valueNode.FieldType)
	assert.Equal(t, "default", *valueNode.DefaultValue)

	// Test case 4: Direct SQL scan to verify database NULL handling
	var scanTest struct {
		ID           int64
		Language     sql.NullString
		Summary      sql.NullString
		FieldType    sql.NullString
		DefaultValue sql.NullString
	}

	err = db.Raw("SELECT id, language, summary, field_type, default_value FROM ast_nodes WHERE file_path = ?", "/test/file.go").Scan(&scanTest).Error
	assert.NoError(t, err)

	// Verify that NULL values in database are properly handled
	assert.False(t, scanTest.Language.Valid)
	assert.False(t, scanTest.Summary.Valid)
	assert.False(t, scanTest.FieldType.Valid)
	assert.False(t, scanTest.DefaultValue.Valid)

	// Test case 5: Test accessing nullable fields safely
	// This should not panic even with nil values
	assert.NotPanics(t, func() {
		if nullNode.Language != nil {
			_ = *nullNode.Language
		}
		if nullNode.Summary != nil {
			_ = *nullNode.Summary
		}
		if nullNode.FieldType != nil {
			_ = *nullNode.FieldType
		}
		if nullNode.DefaultValue != nil {
			_ = *nullNode.DefaultValue
		}
	})
}

func TestViolationNullHandling(t *testing.T) {
	// Create in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Auto-migrate the schema
	err = db.AutoMigrate(&Violation{})
	assert.NoError(t, err)

	// Test case 1: Insert violation with null Code and Message
	violationWithNulls := &Violation{
		File:      "/test/file.go",
		Line:      10,
		Column:    5,
		Source:    "test",
		CreatedAt: time.Now(),
		// Leave Code and Message as nil
	}

	err = db.Create(violationWithNulls).Error
	assert.NoError(t, err)

	// Test case 2: Insert violation with values in nullable fields
	violationWithValues := &Violation{
		File:      "/test/file2.go",
		Line:      20,
		Column:    10,
		Source:    "test2",
		CreatedAt: time.Now(),
		Code:      StringPtr("example code"),
		Message:   StringPtr("Test violation message"),
	}

	err = db.Create(violationWithValues).Error
	assert.NoError(t, err)

	// Test case 3: Read back the violations and verify NULL handling
	var retrievedViolations []Violation
	err = db.Find(&retrievedViolations).Error
	assert.NoError(t, err)
	assert.Len(t, retrievedViolations, 2)

	// Find the violations
	var nullViolation *Violation
	var valueViolation *Violation
	for i := range retrievedViolations {
		if retrievedViolations[i].File == "/test/file.go" {
			nullViolation = &retrievedViolations[i]
		} else if retrievedViolations[i].File == "/test/file2.go" {
			valueViolation = &retrievedViolations[i]
		}
	}

	assert.NotNil(t, nullViolation)
	assert.NotNil(t, valueViolation)

	// Verify null values are properly handled
	assert.Nil(t, nullViolation.Code)
	assert.Nil(t, nullViolation.Message)

	// Verify non-null values are properly handled
	assert.NotNil(t, valueViolation.Code)
	assert.NotNil(t, valueViolation.Message)

	assert.Equal(t, "example code", *valueViolation.Code)
	assert.Equal(t, "Test violation message", *valueViolation.Message)
}

func TestStringPtrHelper(t *testing.T) {
	// Test the StringPtr helper function
	testString := "test value"
	ptr := StringPtr(testString)

	assert.NotNil(t, ptr)
	assert.Equal(t, testString, *ptr)

	// Test with empty string
	emptyPtr := StringPtr("")
	assert.NotNil(t, emptyPtr)
	assert.Equal(t, "", *emptyPtr)
}