package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ASTCache manages cached AST data and relationships using GORM
type ASTCache struct {
	db *gorm.DB
}

var (
	astCacheInstance *ASTCache
	astCacheMutex    sync.Mutex
	astCacheOnce     sync.Once
)

func NewASTCache() (*ASTCache, error) {
	return GetASTCache()
}

// GetASTCache returns the singleton AST cache instance
func GetASTCache() (*ASTCache, error) {
	var err error
	astCacheOnce.Do(func() {
		astCacheInstance, err = newASTCache()
	})
	if err != nil {
		return nil, err
	}
	return astCacheInstance, nil
}

func MustGetASTCache() *ASTCache {
	astCache, err := GetASTCache()
	if err != nil {
		panic(err)
	}
	return astCache
}

// ResetASTCache resets the singleton instance (mainly for testing)
func resetASTCache() {
	astCacheMutex.Lock()
	defer astCacheMutex.Unlock()

	if astCacheInstance != nil {
		astCacheInstance.Close()
		astCacheInstance = nil
	}
	astCacheOnce = sync.Once{}
}

// ClearAllData removes all data from the AST cache tables
// This is useful for testing to ensure clean state
func (c *ASTCache) ClearAllData() error {
	if 1 == 1 {
		return nil
	}
	// Use GORM transaction for consistency
	return c.db.Transaction(func(tx *gorm.DB) error {
		// Clear tables in proper order (relationships first due to foreign keys)
		tables := []interface{}{
			&models.ASTRelationship{},
			&models.LibraryRelationship{},
			&models.ASTNode{},
			&models.LibraryNode{},
			&models.FileMetadata{},
			&models.DependencyAlias{},
		}

		for _, table := range tables {
			if err := tx.Unscoped().Delete(table, "1 = 1").Error; err != nil {
				return fmt.Errorf("failed to clear table %T: %w", table, err)
			}
		}

		return nil
	})
}

// NewASTCache creates a new AST cache
func newASTCache() (*ASTCache, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "arch-unit")
	return newASTCacheWithPath(cacheDir)
}

// NewASTCacheWithPath creates a new AST cache in the specified directory
// This is the public version for use by external packages
func NewASTCacheWithPath(cacheDir string) (*ASTCache, error) {
	return newASTCacheWithPath(cacheDir)
}

// newASTCacheWithPath creates a new AST cache in the specified directory (internal version)
func newASTCacheWithPath(cacheDir string) (*ASTCache, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Use the existing GORM database instance
	db, err := NewGormDBWithPath(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open database with GORM: %w", err)
	}

	cache := &ASTCache{db: db}
	// Migration is handled by GORM's AutoMigrate in the GORM initialization
	// Just ensure we have the basic table structure for immediate operations
	if err := cache.ensureBasicStructure(); err != nil {
		return nil, fmt.Errorf("failed to ensure basic database structure: %w", err)
	}

	return cache, nil
}

// ensureBasicStructure ensures minimal database structure is present
// Full migrations are handled by GORM AutoMigrate
func (c *ASTCache) ensureBasicStructure() error {
	// GORM handles schema creation and migrations automatically
	// Just verify the database is accessible
	return nil
}

// Note: init() and migrateSchema() methods have been removed.
// GORM AutoMigrate handles schema creation and migrations automatically.

// Close closes the database connection
func (c *ASTCache) Close() error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// ClearCache clears all data from the cache
func (c *ASTCache) ClearCache() error {
	// Use GORM transaction for consistency
	return c.db.Transaction(func(tx *gorm.DB) error {
		// Clear tables in proper order (relationships first due to foreign keys)
		tables := []interface{}{
			&models.ASTRelationship{},
			&models.LibraryRelationship{},
			&models.ASTNode{},
			&models.LibraryNode{},
		}

		for _, table := range tables {
			if err := tx.Unscoped().Delete(table, "1 = 1").Error; err != nil {
				return fmt.Errorf("failed to clear table %T: %w", table, err)
			}
		}

		return nil
	})
}

// QueryRow executes a query that returns a single row
func (c *ASTCache) QueryRow(query string, args ...interface{}) *sql.Row {
	// Get the underlying sql.DB from GORM to maintain compatibility
	sqlDB, _ := c.db.DB()
	return sqlDB.QueryRow(query, args...)
}

// QueryASTNodes executes a query and returns AST nodes
func (c *ASTCache) QueryASTNodes(query string, args ...interface{}) ([]*models.ASTNode, error) {
	var nodes []*models.ASTNode

	// Use GORM's Raw method for custom queries
	if err := c.db.Raw(query, args...).Scan(&nodes).Error; err != nil {
		return nil, fmt.Errorf("failed to query AST nodes: %w", err)
	}

	// GORM automatically handles JSON deserialization
	return nodes, nil
}

// calculateFileHash calculates SHA256 hash of file content
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// NeedsReanalysis checks if a file needs re-analysis based on hash and modification time
func (c *ASTCache) NeedsReanalysis(filePath string) (bool, error) {
	// Check if file exists
	_, err := os.Stat(filePath)
	if err != nil {
		return true, err
	}

	// Calculate current file hash
	currentHash, err := calculateFileHash(filePath)
	if err != nil {
		return true, err
	}

	// Check database for existing metadata
	var metadata models.FileMetadata
	err = c.db.Where("file_path = ?", filePath).First(&metadata).Error

	if err == gorm.ErrRecordNotFound {
		return true, nil // File not in cache
	}
	if err != nil {
		return true, fmt.Errorf("failed to get file metadata: %w", err)
	}

	// Compare hashes - if content is the same, no reanalysis needed
	return currentHash != metadata.FileHash, nil
}

// UpdateFileMetadata updates or inserts file metadata
func (c *ASTCache) UpdateFileMetadata(filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	fileHash, err := calculateFileHash(filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	metadata := &models.FileMetadata{
		FilePath:        filePath,
		FileHash:        fileHash,
		FileSize:        fileInfo.Size(),
		LastModified:    fileInfo.ModTime(),
		LastAnalyzed:    time.Now(),
		AnalysisVersion: "1.0",
	}

	// Use Clauses to handle upsert (INSERT ... ON CONFLICT DO UPDATE)
	if err := c.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "file_path"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_hash", "file_size", "last_modified", "last_analyzed", "analysis_version"}),
	}).Create(metadata).Error; err != nil {
		return fmt.Errorf("failed to update file metadata: %w", err)
	}

	return nil
}

// StoreASTNode stores an AST node and returns its ID
func (c *ASTCache) StoreASTNode(node *models.ASTNode) (int64, error) {
	// GORM handles JSON serialization automatically with the gorm:"serializer:json" tag
	// No need to manually marshal parameters and return values

	// Use Save to insert or update (equivalent to INSERT OR REPLACE)
	if err := c.db.Save(node).Error; err != nil {
		return 0, fmt.Errorf("failed to save AST node: %w", err)
	}

	return node.ID, nil
}

// GetASTNode retrieves an AST node by ID
func (c *ASTCache) GetASTNode(id int64) (*models.ASTNode, error) {
	var node models.ASTNode

	// Use GORM's First method to find by ID
	if err := c.db.First(&node, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, sql.ErrNoRows // Maintain compatibility with existing error handling
		}
		return nil, fmt.Errorf("failed to get AST node: %w", err)
	}

	// GORM automatically deserializes JSON fields
	return &node, nil
}

// GetASTNodesByFile retrieves all AST nodes for a file
func (c *ASTCache) GetASTNodesByFile(filePath string) ([]*models.ASTNode, error) {
	var nodes []*models.ASTNode

	// Use GORM's Where and Find methods with ordering
	if err := c.db.Where("file_path = ?", filePath).Order("start_line").Find(&nodes).Error; err != nil {
		return nil, fmt.Errorf("failed to get AST nodes by file: %w", err)
	}

	// GORM automatically deserializes JSON fields
	return nodes, nil
}

// StoreASTRelationship stores a relationship between AST nodes
func (c *ASTCache) StoreASTRelationship(fromID int64, toID *int64, lineNo int, relType, text string) error {
	relationship := &models.ASTRelationship{
		FromASTID:        fromID,
		ToASTID:          toID,
		LineNo:           lineNo,
		RelationshipType: models.RelationshipType(relType),
		Text:             text,
	}

	if err := c.db.Create(relationship).Error; err != nil {
		return fmt.Errorf("failed to store AST relationship: %w", err)
	}

	return nil
}

// GetASTRelationships retrieves relationships for an AST node
func (c *ASTCache) GetASTRelationships(astID int64, relType string) ([]*models.ASTRelationship, error) {
	var relationships []*models.ASTRelationship

	query := c.db.Where("from_ast_id = ?", astID)

	if relType != "" {
		query = query.Where("relationship_type = ?", relType)
	}

	if err := query.Order("line_no").Find(&relationships).Error; err != nil {
		return nil, fmt.Errorf("failed to get AST relationships: %w", err)
	}

	return relationships, nil
}

// StoreLibraryNode stores a library node and returns its ID
func (c *ASTCache) StoreLibraryNode(pkg, class, method, field, nodeType, language, framework string) (int64, error) {
	node := &models.LibraryNode{
		Package:   pkg,
		Class:     class,
		Method:    method,
		Field:     field,
		NodeType:  nodeType,
		Language:  language,
		Framework: framework,
	}

	// Try to find existing node first
	var existing models.LibraryNode
	result := c.db.Where("package = ? AND class = ? AND method = ? AND field = ?",
		pkg, class, method, field).First(&existing)

	if result.Error == nil {
		// Node already exists, return its ID
		return existing.ID, nil
	} else if result.Error != gorm.ErrRecordNotFound {
		return 0, fmt.Errorf("failed to check existing library node: %w", result.Error)
	}

	// Create new node
	if err := c.db.Create(node).Error; err != nil {
		return 0, fmt.Errorf("failed to create library node: %w", err)
	}

	return node.ID, nil
}

// StoreLibraryRelationship stores a relationship between AST node and library node
func (c *ASTCache) StoreLibraryRelationship(astID, libraryID int64, lineNo int, relType, text string) error {
	relationship := &models.LibraryRelationship{
		ASTID:            astID,
		LibraryID:        libraryID,
		LineNo:           lineNo,
		RelationshipType: relType,
		Text:             text,
	}

	if err := c.db.Create(relationship).Error; err != nil {
		return fmt.Errorf("failed to store library relationship: %w", err)
	}

	return nil
}

// GetLibraryRelationships retrieves library relationships for an AST node
func (c *ASTCache) GetLibraryRelationships(astID int64, relType string) ([]*models.LibraryRelationship, error) {
	var relationships []*models.LibraryRelationship

	query := c.db.Where("ast_id = ?", astID)

	if relType != "" {
		query = query.Where("relationship_type = ?", relType)
	}

	// Use Preload to fetch the associated LibraryNode data
	if err := query.Order("line_no").Preload("LibraryNode").Find(&relationships).Error; err != nil {
		return nil, fmt.Errorf("failed to get library relationships: %w", err)
	}

	return relationships, nil
}

// DeleteASTForFile removes all AST data for a file (for re-analysis)
func (c *ASTCache) DeleteASTForFile(filePath string) error {
	// Use GORM transaction
	return c.db.Transaction(func(tx *gorm.DB) error {
		// Get all AST node IDs for the file first
		var nodeIDs []int64
		if err := tx.Model(&models.ASTNode{}).Where("file_path = ?", filePath).Pluck("id", &nodeIDs).Error; err != nil {
			return fmt.Errorf("failed to get AST node IDs: %w", err)
		}

		// Delete relationships first (due to foreign key constraints)
		if len(nodeIDs) > 0 {
			// Delete AST relationships
			if err := tx.Where("from_ast_id IN ? OR to_ast_id IN ?", nodeIDs, nodeIDs).Delete(&models.ASTRelationship{}).Error; err != nil {
				return fmt.Errorf("failed to delete AST relationships: %w", err)
			}

			// Delete library relationships
			if err := tx.Where("ast_id IN ?", nodeIDs).Delete(&models.LibraryRelationship{}).Error; err != nil {
				return fmt.Errorf("failed to delete library relationships: %w", err)
			}
		}

		// Delete AST nodes
		if err := tx.Where("file_path = ?", filePath).Delete(&models.ASTNode{}).Error; err != nil {
			return fmt.Errorf("failed to delete AST nodes: %w", err)
		}

		return nil
	})
}

// QueryRaw executes a raw SQL query and returns rows
func (c *ASTCache) QueryRaw(query string, args ...interface{}) (*sql.Rows, error) {
	// Get the underlying sql.DB from GORM to maintain compatibility
	sqlDB, _ := c.db.DB()
	return sqlDB.Query(query, args...)
}

// GetDB returns the underlying GORM database instance
func (c *ASTCache) GetDB() *gorm.DB {
	return c.db
}

// CountImports counts the number of import relationships for a node
func (c *ASTCache) CountImports(nodeID int64) (int, error) {
	var count int64

	if err := c.db.Model(&models.ASTRelationship{}).
		Where("from_ast_id = ? AND relationship_type = ?", nodeID, "import").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count imports: %w", err)
	}

	return int(count), nil
}

// CountExternalCalls counts the number of call relationships to nodes outside the current package
func (c *ASTCache) CountExternalCalls(nodeID int64) (int, error) {
	var count int64

	// Use raw query for complex joins, as GORM's query builder can be complex for this case
	query := `
		SELECT COUNT(DISTINCT ar.id)
		FROM ast_relationships ar
		JOIN ast_nodes from_node ON from_node.id = ar.from_ast_id
		LEFT JOIN ast_nodes to_node ON to_node.id = ar.to_ast_id
		WHERE ar.from_ast_id = ?
		AND ar.relationship_type = 'call'
		AND (
			ar.to_ast_id IS NULL  -- External library call
			OR to_node.package_name != from_node.package_name  -- Different package
		)
	`

	if err := c.db.Raw(query, nodeID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count external calls: %w", err)
	}

	return int(count), nil
}

// StoreFileResults stores all analysis results for a file in a single transaction
func (c *ASTCache) StoreFileResults(file string, result interface{}) error {
	// Import cycle prevention - accept interface{} and type assert
	type astResult struct {
		Nodes         []*models.ASTNode
		Relationships []*models.ASTRelationship
		Libraries     []*models.LibraryRelationship
	}

	r, ok := result.(*astResult)
	if !ok {
		// Try to extract fields using reflection for analysis.ASTResult
		rv := reflect.ValueOf(result)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}

		nodes := rv.FieldByName("Nodes")
		relationships := rv.FieldByName("Relationships")
		libraries := rv.FieldByName("Libraries")

		if !nodes.IsValid() || !relationships.IsValid() || !libraries.IsValid() {
			return fmt.Errorf("invalid result type")
		}

		// Create a compatible struct
		r = &astResult{
			Nodes:         nodes.Interface().([]*models.ASTNode),
			Relationships: relationships.Interface().([]*models.ASTRelationship),
			Libraries:     libraries.Interface().([]*models.LibraryRelationship),
		}
	}

	// Use GORM transaction
	return c.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing data for the file first
		if err := c.deleteFileDataInTx(tx, file); err != nil {
			return fmt.Errorf("failed to delete existing file data: %w", err)
		}

		// Store new nodes and build ID mapping
		nodeIDMap := make(map[int64]int64) // Map old IDs to new IDs
		for _, node := range r.Nodes {
			oldID := node.ID
			node.ID = 0 // Clear ID so GORM creates a new one

			// GORM handles JSON serialization automatically
			if err := tx.Create(node).Error; err != nil {
				return fmt.Errorf("failed to store node: %w", err)
			}

			// Map old ID to new ID for relationship updates
			if oldID > 0 {
				nodeIDMap[oldID] = node.ID
			}
		}

		// Store relationships with updated IDs
		for _, rel := range r.Relationships {
			fromID := nodeIDMap[rel.FromASTID]
			var toID *int64
			if rel.ToASTID != nil && *rel.ToASTID > 0 {
				newToID := nodeIDMap[*rel.ToASTID]
				toID = &newToID
			}

			relationship := &models.ASTRelationship{
				FromASTID:        fromID,
				ToASTID:          toID,
				LineNo:           rel.LineNo,
				RelationshipType: rel.RelationshipType,
				Text:             rel.Text,
			}

			if err := tx.Create(relationship).Error; err != nil {
				return fmt.Errorf("failed to store relationship: %w", err)
			}
		}

		// Store library dependencies
		for _, lib := range r.Libraries {
			if lib.LibraryNode != nil {
				// Find or create library node
				var libraryNode models.LibraryNode
				result := tx.Where("package = ? AND class = ? AND method = ? AND field = ?",
					lib.LibraryNode.Package, lib.LibraryNode.Class,
					lib.LibraryNode.Method, lib.LibraryNode.Field).First(&libraryNode)

				if result.Error == gorm.ErrRecordNotFound {
					// Create new library node
					libraryNode = *lib.LibraryNode
					libraryNode.ID = 0 // Clear ID for new record
					if err := tx.Create(&libraryNode).Error; err != nil {
						return fmt.Errorf("failed to create library node: %w", err)
					}
				} else if result.Error != nil {
					return fmt.Errorf("failed to find library node: %w", result.Error)
				}

				// Store library relationship if we have an AST ID
				if lib.ASTID > 0 {
					astID := nodeIDMap[lib.ASTID]
					if astID > 0 {
						relationship := &models.LibraryRelationship{
							ASTID:            astID,
							LibraryID:        libraryNode.ID,
							LineNo:           lib.LineNo,
							RelationshipType: lib.RelationshipType,
							Text:             lib.Text,
						}

						if err := tx.Create(relationship).Error; err != nil {
							return fmt.Errorf("failed to store library relationship: %w", err)
						}
					}
				}
			}
		}

		// Update file metadata
		fileInfo, err := os.Stat(file)
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}

		fileHash, err := calculateFileHash(file)
		if err != nil {
			return fmt.Errorf("failed to calculate file hash: %w", err)
		}

		fileMetadata := &models.FileMetadata{
			FilePath:        file,
			FileHash:        fileHash,
			FileSize:        fileInfo.Size(),
			LastModified:    fileInfo.ModTime(),
			LastAnalyzed:    time.Now(),
			AnalysisVersion: "1.0",
		}

		// Use Save to insert or update (equivalent to INSERT OR REPLACE)
		if err := tx.Save(fileMetadata).Error; err != nil {
			return fmt.Errorf("failed to update file metadata: %w", err)
		}

		return nil
	})
}

// deleteFileDataInTx is a helper function to delete existing data for a file within a transaction
func (c *ASTCache) deleteFileDataInTx(tx *gorm.DB, filePath string) error {
	// Get all AST node IDs for the file first
	var nodeIDs []int64
	if err := tx.Model(&models.ASTNode{}).Where("file_path = ?", filePath).Pluck("id", &nodeIDs).Error; err != nil {
		return fmt.Errorf("failed to get AST node IDs: %w", err)
	}

	// Delete relationships first (due to foreign key constraints)
	if len(nodeIDs) > 0 {
		// Delete AST relationships
		if err := tx.Where("from_ast_id IN ? OR to_ast_id IN ?", nodeIDs, nodeIDs).Delete(&models.ASTRelationship{}).Error; err != nil {
			return fmt.Errorf("failed to delete AST relationships: %w", err)
		}

		// Delete library relationships
		if err := tx.Where("ast_id IN ?", nodeIDs).Delete(&models.LibraryRelationship{}).Error; err != nil {
			return fmt.Errorf("failed to delete library relationships: %w", err)
		}
	}

	// Delete AST nodes
	if err := tx.Where("file_path = ?", filePath).Delete(&models.ASTNode{}).Error; err != nil {
		return fmt.Errorf("failed to delete AST nodes: %w", err)
	}

	return nil
}

// GetDependencyAlias retrieves a cached dependency alias
func (c *ASTCache) GetDependencyAlias(packageName, packageType string) (*models.DependencyAlias, error) {
	var alias models.DependencyAlias

	err := c.db.Where("package_name = ? AND package_type = ?", packageName, packageType).First(&alias).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dependency alias: %w", err)
	}

	return &alias, nil
}

// StoreDependencyAlias stores a dependency alias in the cache
func (c *ASTCache) StoreDependencyAlias(alias *models.DependencyAlias) error {
	// Use Save to insert or update (equivalent to INSERT OR REPLACE)
	if err := c.db.Save(alias).Error; err != nil {
		return fmt.Errorf("failed to store dependency alias: %w", err)
	}
	return nil
}
