package database_test_suite

import (
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"

	"github.com/flanksource/arch-unit/git"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

// TestDB manages the test database lifecycle
type TestDB struct {
	db      *gorm.DB
	tempDir string
}

// NewTestDB creates a new test database instance
func NewTestDB() (*TestDB, error) {
	// Create temporary directory for test database
	tempDir, err := os.MkdirTemp("", "arch-unit-db-test-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Create GORM database instance
	db, err := cache.NewGormDBWithPath(tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to create test database: %w", err)
	}

	return &TestDB{
		db:      db,
		tempDir: tempDir,
	}, nil
}

// DB returns the GORM database instance
func (tdb *TestDB) DB() *gorm.DB {
	return tdb.db
}

// TempDir returns the temporary directory path for this test database
func (tdb *TestDB) TempDir() string {
	return tdb.tempDir
}

// ASTCache creates and returns an AST cache instance using the test database
func (tdb *TestDB) ASTCache() *cache.ASTCache {
	astCache, err := cache.NewASTCacheWithPath(tdb.tempDir)
	if err != nil {
		panic(fmt.Sprintf("Failed to create AST cache: %v", err))
	}
	return astCache
}

// ClearAllData removes all data from all tables
func (tdb *TestDB) ClearAllData() error {
	// Clear all tables in proper order (dependencies first, then referenced tables)
	tables := []interface{}{
		&models.Violation{},           // Has foreign keys to ASTNode
		&models.ASTRelationship{},     // Has foreign keys to ASTNode
		&models.LibraryRelationship{}, // Has foreign keys to ASTNode and LibraryNode
		&models.ASTNode{},              // Referenced by Violation, ASTRelationship, LibraryRelationship
		&models.LibraryNode{},          // Referenced by LibraryRelationship
		&models.FileScan{},
		&models.FileMetadata{},
		&models.DependencyAlias{},
	}

	return tdb.db.Transaction(func(tx *gorm.DB) error {
		for _, table := range tables {
			if err := tx.Unscoped().Delete(table, "1 = 1").Error; err != nil {
				return fmt.Errorf("failed to clear table %T: %w", table, err)
			}
		}
		return nil
	})
}

// Close closes the database connection and cleans up temp directory
func (tdb *TestDB) Close() error {
	if tdb.db != nil {
		sqlDB, err := tdb.db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}

	if tdb.tempDir != "" {
		os.RemoveAll(tdb.tempDir)
	}

	return nil
}

// CreateTestASTNode creates a test AST node with default values
func (tdb *TestDB) CreateTestASTNode(overrides ...func(*models.ASTNode)) *models.ASTNode {
	node := &models.ASTNode{
		FilePath:    "/test/file.go",
		PackageName: "main",
		TypeName:    "TestStruct",
		NodeType:    models.NodeTypeType,
		StartLine:   10,
		EndLine:     20,
		LineCount:   11,
		FileHash:    "abc123",
	}

	// Apply overrides
	for _, override := range overrides {
		override(node)
	}

	result := tdb.db.Create(node)
	if result.Error != nil {
		panic(fmt.Sprintf("Failed to create test AST node: %v", result.Error))
	}

	return node
}

// CreateTestViolation creates a test violation with default values
func (tdb *TestDB) CreateTestViolation(overrides ...func(*models.Violation)) *models.Violation {
	violation := &models.Violation{
		File:    "/test/file.go",
		Line:    42,
		Column:  10,
		Source:  "arch-unit",
		Message: "Test violation",
		// Note: CallerID and CalledID should be set when AST nodes are available
		// For test purposes, the caller/called information is in the message
	}

	// Apply overrides first to get the final file path
	for _, override := range overrides {
		override(violation)
	}

	// Ensure we have a FileScan record that matches the violation's file path
	fileScan := &models.FileScan{
		FilePath:     violation.File,
		LastScanTime: 1640995200, // Unix timestamp
		FileModTime:  1640995200,
		FileHash:     "def456",
	}
	
	// Create or update the FileScan record
	tdb.db.Where("file_path = ?", fileScan.FilePath).FirstOrCreate(fileScan)

	result := tdb.db.Create(violation)
	if result.Error != nil {
		panic(fmt.Sprintf("Failed to create test violation: %v", result.Error))
	}

	return violation
}

// CreateTestLibraryNode creates a test library node with default values
func (tdb *TestDB) CreateTestLibraryNode(overrides ...func(*models.LibraryNode)) *models.LibraryNode {
	node := &models.LibraryNode{
		Package:   "github.com/gin-gonic/gin",
		Class:     "Engine",
		Method:    "Run",
		NodeType:  "method",
		Language:  "go",
		Framework: "gin",
	}

	// Apply overrides
	for _, override := range overrides {
		override(node)
	}

	result := tdb.db.Create(node)
	if result.Error != nil {
		panic(fmt.Sprintf("Failed to create test library node: %v", result.Error))
	}

	return node
}

// CreateTestDependencyAlias creates a test dependency alias with default values
func (tdb *TestDB) CreateTestDependencyAlias(overrides ...func(*models.DependencyAlias)) *models.DependencyAlias {
	alias := &models.DependencyAlias{
		PackageName: "github.com/gin-gonic/gin",
		PackageType: "go",
		GitURL:      "https://github.com/gin-gonic/gin.git",
		LastChecked: 1640995200,
		CreatedAt:   1640995200,
	}

	// Apply overrides
	for _, override := range overrides {
		override(alias)
	}

	result := tdb.db.Create(alias)
	if result.Error != nil {
		panic(fmt.Sprintf("Failed to create test dependency alias: %v", result.Error))
	}

	return alias
}

// SetupAQLTestData creates the standard test data used by AQL engine tests
func (tdb *TestDB) SetupAQLTestData() {
	// Create test AST nodes
	nodes := []*models.ASTNode{
		// Simple controller with low complexity
		{
			FilePath:             "/test/SimpleController.go",
			PackageName:          "controller",
			TypeName:             "SimpleController",
			MethodName:           "GetUser",
			NodeType:             models.NodeTypeMethod,
			StartLine:            10,
			EndLine:              15,
			CyclomaticComplexity: 2,
			LastModified:         time.Now(),
		},
		// Complex controller with high complexity
		{
			FilePath:             "/test/ComplexController.go",
			PackageName:          "controller",
			TypeName:             "ComplexController",
			MethodName:           "ProcessOrder",
			NodeType:             models.NodeTypeMethod,
			StartLine:            20,
			EndLine:              80,
			CyclomaticComplexity: 25,
			ParameterCount:       3, // Has 3 parameters to trigger the parameter test
			LastModified:         time.Now(),
		},
		// Service layer
		{
			FilePath:             "/test/UserService.go",
			PackageName:          "service",
			TypeName:             "UserService",
			MethodName:           "CreateUser",
			NodeType:             models.NodeTypeMethod,
			StartLine:            5,
			EndLine:              20,
			CyclomaticComplexity: 5,
			ParameterCount:       2,
			LastModified:         time.Now(),
		},
		// Repository layer
		{
			FilePath:             "/test/UserRepository.go",
			PackageName:          "repository",
			TypeName:             "UserRepository",
			MethodName:           "Save",
			NodeType:             models.NodeTypeMethod,
			StartLine:            8,
			EndLine:              12,
			CyclomaticComplexity: 1,
			ParameterCount:       1,
			LastModified:         time.Now(),
		},
		// Model layer
		{
			FilePath:             "/test/User.go",
			PackageName:          "model",
			TypeName:             "User",
			MethodName:           "",
			NodeType:             models.NodeTypeType,
			StartLine:            1,
			EndLine:              10,
			CyclomaticComplexity: 0,
			ParameterCount:       0,
			LastModified:         time.Now(),
		},
	}

	// Store nodes and get their IDs
	nodeIDs := make([]int64, len(nodes))
	for i, node := range nodes {
		result := tdb.db.Create(node)
		if result.Error != nil {
			panic(fmt.Sprintf("Failed to create AQL test AST node: %v", result.Error))
		}
		nodeIDs[i] = node.ID
	}

	// Create relationships
	// SimpleController -> UserService
	relationship1 := &models.ASTRelationship{
		FromASTID:        nodeIDs[0],
		ToASTID:          &nodeIDs[2],
		LineNo:           12,
		RelationshipType: models.RelationshipCall,
		Text:             "userService.CreateUser()",
	}
	tdb.db.Create(relationship1)

	// ComplexController -> UserService
	relationship2 := &models.ASTRelationship{
		FromASTID:        nodeIDs[1],
		ToASTID:          &nodeIDs[2],
		LineNo:           45,
		RelationshipType: models.RelationshipCall,
		Text:             "userService.CreateUser()",
	}
	tdb.db.Create(relationship2)

	// UserService -> UserRepository
	relationship3 := &models.ASTRelationship{
		FromASTID:        nodeIDs[2],
		ToASTID:          &nodeIDs[3],
		LineNo:           18,
		RelationshipType: models.RelationshipCall,
		Text:             "userRepo.Save()",
	}
	tdb.db.Create(relationship3)

	// Store library relationships
	libNode := &models.LibraryNode{
		Package:   "fmt",
		Class:     "",
		Method:    "Printf",
		NodeType:  "method",
		Language:  "go",
		Framework: "stdlib",
	}
	tdb.db.Create(libNode)

	libRelationship := &models.LibraryRelationship{
		ASTID:            nodeIDs[0],
		LibraryID:        libNode.ID,
		LineNo:           11,
		RelationshipType: string(models.RelationshipCall),
		Text:             "fmt.Printf()",
	}
	tdb.db.Create(libRelationship)
}

// SetupAQLTestDataInCache creates the standard test data used by AQL engine tests using ASTCache methods
func (tdb *TestDB) SetupAQLTestDataInCache(astCache *cache.ASTCache) {
	// Clear the cache first to ensure clean state
	err := astCache.ClearAllData()
	if err != nil {
		panic(fmt.Sprintf("Failed to clear AST cache: %v", err))
	}

	// Create test AST nodes using direct field values instead of struct literals
	// This avoids GORM field serialization issues with the ASTCache
	
	// Simple controller with low complexity
	node1 := &models.ASTNode{}
	node1.FilePath = "/test/SimpleController.go"
	node1.PackageName = "controller"
	node1.TypeName = "SimpleController"
	node1.MethodName = "GetUser"
	node1.NodeType = models.NodeTypeMethod
	node1.StartLine = 10
	node1.EndLine = 15
	node1.CyclomaticComplexity = 2
	node1.ParameterCount = 1
	node1.LastModified = time.Now()

	// Complex controller with high complexity
	node2 := &models.ASTNode{}
	node2.FilePath = "/test/ComplexController.go"
	node2.PackageName = "controller"
	node2.TypeName = "ComplexController"
	node2.MethodName = "ProcessOrder"
	node2.NodeType = models.NodeTypeMethod
	node2.StartLine = 20
	node2.EndLine = 80
	node2.CyclomaticComplexity = 25
	node2.ParameterCount = 3
	node2.LastModified = time.Now()

	// Service layer
	node3 := &models.ASTNode{}
	node3.FilePath = "/test/UserService.go"
	node3.PackageName = "service"
	node3.TypeName = "UserService"
	node3.MethodName = "CreateUser"
	node3.NodeType = models.NodeTypeMethod
	node3.StartLine = 5
	node3.EndLine = 20
	node3.CyclomaticComplexity = 5
	node3.ParameterCount = 2
	node3.LastModified = time.Now()

	// Repository layer
	node4 := &models.ASTNode{}
	node4.FilePath = "/test/UserRepository.go"
	node4.PackageName = "repository"
	node4.TypeName = "UserRepository"
	node4.MethodName = "Save"
	node4.NodeType = models.NodeTypeMethod
	node4.StartLine = 8
	node4.EndLine = 12
	node4.CyclomaticComplexity = 1
	node4.ParameterCount = 1
	node4.LastModified = time.Now()

	// Model layer
	node5 := &models.ASTNode{}
	node5.FilePath = "/test/User.go"
	node5.PackageName = "model"
	node5.TypeName = "User"
	node5.MethodName = ""
	node5.NodeType = models.NodeTypeType
	node5.StartLine = 1
	node5.EndLine = 10
	node5.CyclomaticComplexity = 0
	node5.ParameterCount = 0
	node5.LastModified = time.Now()

	nodes := []*models.ASTNode{node1, node2, node3, node4, node5}

	// Store nodes and get their IDs using ASTCache methods
	nodeIDs := make([]int64, len(nodes))
	for i, node := range nodes {
		id, err := astCache.StoreASTNode(node)
		if err != nil {
			panic(fmt.Sprintf("Failed to create AQL test AST node: %v", err))
		}
		nodeIDs[i] = id
		node.ID = id // Update the node with the ID for relationship creation
	}

	// Create relationships using ASTCache methods
	// SimpleController -> UserService
	err = astCache.StoreASTRelationship(nodeIDs[0], &nodeIDs[2], 12, models.RelationshipCall, "userService.CreateUser()")
	if err != nil {
		panic(fmt.Sprintf("Failed to create relationship: %v", err))
	}

	// ComplexController -> UserService
	err = astCache.StoreASTRelationship(nodeIDs[1], &nodeIDs[2], 45, models.RelationshipCall, "userService.CreateUser()")
	if err != nil {
		panic(fmt.Sprintf("Failed to create relationship: %v", err))
	}

	// UserService -> UserRepository
	err = astCache.StoreASTRelationship(nodeIDs[2], &nodeIDs[3], 18, models.RelationshipCall, "userRepo.Save()")
	if err != nil {
		panic(fmt.Sprintf("Failed to create relationship: %v", err))
	}

	// Store library relationships
	var fmtLibID int64
	fmtLibID, err = astCache.StoreLibraryNode("fmt", "", "Printf", "", models.NodeTypeMethod, "go", "stdlib")
	if err != nil {
		panic(fmt.Sprintf("Failed to create library node: %v", err))
	}

	err = astCache.StoreLibraryRelationship(nodeIDs[0], fmtLibID, 11, models.RelationshipCall, "fmt.Printf()")
	if err != nil {
		panic(fmt.Sprintf("Failed to create library relationship: %v", err))
	}
}

// CreateManyTestASTNodes creates many test nodes for performance testing
func (tdb *TestDB) CreateManyTestASTNodes(count int) {
	for i := 0; i < count; i++ {
		node := &models.ASTNode{
			FilePath:             fmt.Sprintf("/test/file%d.go", i),
			PackageName:          fmt.Sprintf("pkg%d", i%10),
			TypeName:             fmt.Sprintf("Type%d", i),
			MethodName:           fmt.Sprintf("Method%d", i),
			NodeType:             models.NodeTypeMethod,
			StartLine:            i % 100,
			EndLine:              i%100 + 10,
			CyclomaticComplexity: i % 20, // 0-19 complexity
			LastModified:         time.Now(),
		}
		result := tdb.db.Create(node)
		if result.Error != nil {
			panic(fmt.Sprintf("Failed to create performance test AST node: %v", result.Error))
		}
	}
}

// CreateManyTestASTNodesInCache creates many test nodes for performance testing using ASTCache
func (tdb *TestDB) CreateManyTestASTNodesInCache(astCache *cache.ASTCache, count int) {
	for i := 0; i < count; i++ {
		node := &models.ASTNode{
			FilePath:             fmt.Sprintf("/test/file%d.go", i),
			PackageName:          fmt.Sprintf("pkg%d", i%10),
			TypeName:             fmt.Sprintf("Type%d", i),
			MethodName:           fmt.Sprintf("Method%d", i),
			NodeType:             models.NodeTypeMethod,
			StartLine:            i % 100,
			EndLine:              i%100 + 10,
			CyclomaticComplexity: i % 20, // 0-19 complexity
			LastModified:         time.Now(),
		}
		_, err := astCache.StoreASTNode(node)
		if err != nil {
			panic(fmt.Sprintf("Failed to create performance test AST node: %v", err))
		}
	}
}

// CreateGitManager creates a git repository manager using a subdirectory of the test temp dir
func (tdb *TestDB) CreateGitManager() git.GitRepositoryManager {
	gitCacheDir, err := os.MkdirTemp(tdb.tempDir, "git-cache-*")
	if err != nil {
		panic(fmt.Sprintf("Failed to create git cache directory: %v", err))
	}
	return git.NewGitRepositoryManager(gitCacheDir)
}
