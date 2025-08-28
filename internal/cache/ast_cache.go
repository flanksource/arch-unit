package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/models"
	_ "modernc.org/sqlite"
)

// ASTCache manages cached AST data and relationships using SQLite
type ASTCache struct {
	db *DB
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
func ResetASTCache() {
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
	queries := []string{
		"DELETE FROM ast_relationships",
		"DELETE FROM ast_nodes",
		"DELETE FROM library_nodes",
		"DELETE FROM library_relationships",
		"DELETE FROM file_metadata",
		"DELETE FROM dependency_aliases",
	}

	for _, query := range queries {
		if _, err := c.db.Exec(query); err != nil {
			return fmt.Errorf("failed to clear table: %w", err)
		}
	}

	return nil
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

	dbPath := filepath.Join(cacheDir, "ast.db")
	db, err := NewDB("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	cache := &ASTCache{db: db}
	// Migration is now handled by the unified migration manager in the PrePersistent hook
	// Just ensure we have the basic table structure for immediate operations
	if err := cache.ensureBasicStructure(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ensure basic database structure: %w", err)
	}

	return cache, nil
}

// ensureBasicStructure ensures minimal database structure is present
// Full migrations are handled by the unified migration manager
func (c *ASTCache) ensureBasicStructure() error {
	// This is a minimal check to ensure the database can be used
	// Full schema creation and migrations are handled by the migration manager
	return c.init()
}

// init creates the necessary tables and indexes (DEPRECATED - use migration manager)
func (c *ASTCache) init() error {
	schema := `
	-- Main AST nodes table
	CREATE TABLE IF NOT EXISTS ast_nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL,
		package_name TEXT,
		type_name TEXT,
		method_name TEXT,
		field_name TEXT,
		node_type TEXT NOT NULL, -- 'package', 'type', 'method', 'field', 'variable'
		start_line INTEGER,
		end_line INTEGER,
		cyclomatic_complexity INTEGER DEFAULT 0,
		parameter_count INTEGER DEFAULT 0,
		return_count INTEGER DEFAULT 0,
		line_count INTEGER DEFAULT 0,
		parameters_json TEXT, -- JSON serialized parameter details
		return_values_json TEXT, -- JSON serialized return values
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_modified TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		file_hash TEXT, -- SHA256 of file content for cache invalidation
		UNIQUE(file_path, package_name, type_name, method_name, field_name)
	);

	-- AST node relationships (calls, references, inheritance, etc.)
	CREATE TABLE IF NOT EXISTS ast_relationships (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_ast_id INTEGER NOT NULL,
		to_ast_id INTEGER,
		line_no INTEGER NOT NULL,
		relationship_type TEXT NOT NULL, -- 'call', 'reference', 'inheritance', 'implements', 'import'
		text TEXT, -- The actual text of the relationship (e.g., method call syntax)
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (from_ast_id) REFERENCES ast_nodes(id) ON DELETE CASCADE,
		FOREIGN KEY (to_ast_id) REFERENCES ast_nodes(id) ON DELETE CASCADE
	);

	-- External library/framework nodes
	CREATE TABLE IF NOT EXISTS library_nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		package TEXT NOT NULL,
		class TEXT,
		method TEXT,
		field TEXT,
		node_type TEXT NOT NULL, -- 'package', 'class', 'method', 'field'
		language TEXT, -- 'go', 'python', 'javascript', etc.
		framework TEXT, -- 'stdlib', 'gin', 'django', 'react', etc.
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(package, class, method, field)
	);

	-- Relationships between AST nodes and library nodes
	CREATE TABLE IF NOT EXISTS library_relationships (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ast_id INTEGER NOT NULL,
		library_id INTEGER NOT NULL,
		line_no INTEGER NOT NULL,
		relationship_type TEXT NOT NULL, -- 'import', 'call', 'reference', 'extends'
		text TEXT, -- The actual usage text
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (ast_id) REFERENCES ast_nodes(id) ON DELETE CASCADE,
		FOREIGN KEY (library_id) REFERENCES library_nodes(id) ON DELETE CASCADE
	);

	-- File metadata for cache management
	CREATE TABLE IF NOT EXISTS file_metadata (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT UNIQUE NOT NULL,
		file_hash TEXT NOT NULL, -- SHA256 of file content
		file_size INTEGER,
		last_modified TIMESTAMP NOT NULL,
		last_analyzed TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		analysis_version TEXT DEFAULT '1.0' -- Schema version for cache invalidation
	);

	-- Dependency aliases table
	CREATE TABLE IF NOT EXISTS dependency_aliases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		package_name TEXT NOT NULL,
		package_type TEXT NOT NULL,
		git_url TEXT NOT NULL,
		last_checked INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		UNIQUE(package_name, package_type)
	);
	`

	indexes := `
	-- AST node indexes
	CREATE INDEX IF NOT EXISTS idx_ast_nodes_file_path ON ast_nodes(file_path);
	CREATE INDEX IF NOT EXISTS idx_ast_nodes_package ON ast_nodes(package_name);
	CREATE INDEX IF NOT EXISTS idx_ast_nodes_type ON ast_nodes(type_name);
	CREATE INDEX IF NOT EXISTS idx_ast_nodes_method ON ast_nodes(method_name);
	CREATE INDEX IF NOT EXISTS idx_ast_nodes_node_type ON ast_nodes(node_type);
	CREATE INDEX IF NOT EXISTS idx_ast_nodes_complexity ON ast_nodes(cyclomatic_complexity);
	CREATE INDEX IF NOT EXISTS idx_ast_nodes_last_modified ON ast_nodes(last_modified);

	-- Relationship indexes
	CREATE INDEX IF NOT EXISTS idx_ast_relationships_from ON ast_relationships(from_ast_id);
	CREATE INDEX IF NOT EXISTS idx_ast_relationships_to ON ast_relationships(to_ast_id);
	CREATE INDEX IF NOT EXISTS idx_ast_relationships_type ON ast_relationships(relationship_type);
	CREATE INDEX IF NOT EXISTS idx_ast_relationships_line ON ast_relationships(line_no);

	-- Library indexes
	CREATE INDEX IF NOT EXISTS idx_library_nodes_package ON library_nodes(package);
	CREATE INDEX IF NOT EXISTS idx_library_nodes_class ON library_nodes(class);
	CREATE INDEX IF NOT EXISTS idx_library_nodes_method ON library_nodes(method);
	CREATE INDEX IF NOT EXISTS idx_library_nodes_type ON library_nodes(node_type);
	CREATE INDEX IF NOT EXISTS idx_library_nodes_framework ON library_nodes(framework);

	-- Library relationship indexes
	CREATE INDEX IF NOT EXISTS idx_library_relationships_ast ON library_relationships(ast_id);
	CREATE INDEX IF NOT EXISTS idx_library_relationships_library ON library_relationships(library_id);
	CREATE INDEX IF NOT EXISTS idx_library_relationships_type ON library_relationships(relationship_type);

	-- File metadata indexes
	CREATE INDEX IF NOT EXISTS idx_file_metadata_path ON file_metadata(file_path);
	CREATE INDEX IF NOT EXISTS idx_file_metadata_hash ON file_metadata(file_hash);
	CREATE INDEX IF NOT EXISTS idx_file_metadata_modified ON file_metadata(last_modified);

	-- Dependency aliases indexes
	CREATE INDEX IF NOT EXISTS idx_dependency_aliases_package ON dependency_aliases(package_name);
	CREATE INDEX IF NOT EXISTS idx_dependency_aliases_type ON dependency_aliases(package_type);
	`

	if _, err := c.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	if _, err := c.db.Exec(indexes); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	// Run migrations
	if err := c.migrateSchema(); err != nil {
		return fmt.Errorf("failed to migrate schema: %w", err)
	}

	return nil
}

// migrateSchema handles schema migrations for existing databases
func (c *ASTCache) migrateSchema() error {
	// Check if dependency_aliases table has the required columns
	rows, err := c.db.Query("PRAGMA table_info(dependency_aliases)")
	if err != nil {
		// Table doesn't exist, nothing to migrate
		return nil
	}
	defer rows.Close()

	hasLastChecked := false
	hasCreatedAtInt := false

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			continue
		}

		if name == "last_checked" {
			hasLastChecked = true
		}
		if name == "created_at" && dataType == "INTEGER" {
			hasCreatedAtInt = true
		}
	}
	rows.Close()

	// Add last_checked column if it doesn't exist
	if !hasLastChecked {
		_, err = c.db.Exec("ALTER TABLE dependency_aliases ADD COLUMN last_checked INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return fmt.Errorf("failed to add last_checked column: %w", err)
		}

		// Update existing records to current timestamp
		_, err = c.db.Exec("UPDATE dependency_aliases SET last_checked = strftime('%s', 'now') WHERE last_checked = 0")
		if err != nil {
			return fmt.Errorf("failed to update last_checked values: %w", err)
		}
	}

	// Check if created_at is TIMESTAMP instead of INTEGER and convert if needed
	if !hasCreatedAtInt {
		// Create a new table with correct schema
		_, err = c.db.Exec(`
			CREATE TABLE dependency_aliases_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				package_name TEXT NOT NULL,
				package_type TEXT NOT NULL,
				git_url TEXT NOT NULL,
				last_checked INTEGER NOT NULL,
				created_at INTEGER NOT NULL,
				UNIQUE(package_name, package_type)
			)`)
		if err != nil {
			return fmt.Errorf("failed to create new dependency_aliases table: %w", err)
		}

		// Copy data from old table, converting TIMESTAMP to INTEGER
		_, err = c.db.Exec(`
			INSERT INTO dependency_aliases_new (id, package_name, package_type, git_url, last_checked, created_at)
			SELECT id, package_name, package_type, git_url,
				   COALESCE(last_checked, strftime('%s', 'now')),
				   strftime('%s', COALESCE(created_at, 'now'))
			FROM dependency_aliases`)
		if err != nil {
			return fmt.Errorf("failed to copy data to new dependency_aliases table: %w", err)
		}

		// Drop old table and rename new one
		_, err = c.db.Exec("DROP TABLE dependency_aliases")
		if err != nil {
			return fmt.Errorf("failed to drop old dependency_aliases table: %w", err)
		}

		_, err = c.db.Exec("ALTER TABLE dependency_aliases_new RENAME TO dependency_aliases")
		if err != nil {
			return fmt.Errorf("failed to rename dependency_aliases table: %w", err)
		}

		// Recreate index
		_, err = c.db.Exec("CREATE INDEX IF NOT EXISTS idx_dependency_aliases_package ON dependency_aliases(package_name)")
		if err != nil {
			return fmt.Errorf("failed to recreate package index: %w", err)
		}

		_, err = c.db.Exec("CREATE INDEX IF NOT EXISTS idx_dependency_aliases_type ON dependency_aliases(package_type)")
		if err != nil {
			return fmt.Errorf("failed to recreate type index: %w", err)
		}
	}

	return nil
}

// Close closes the database connection
func (c *ASTCache) Close() error {
	return c.db.Close()
}

// ClearCache clears all data from the cache
func (c *ASTCache) ClearCache() error {
	tables := []string{
		"ast_nodes",
		"ast_relationships",
		"library_nodes",
		"library_relationships",
	}

	for _, table := range tables {
		if _, err := c.db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("failed to clear table %s: %w", table, err)
		}
	}

	return nil
}

// QueryRow executes a query that returns a single row
func (c *ASTCache) QueryRow(query string, args ...interface{}) *sql.Row {
	return c.db.QueryRow(query, args...)
}

// QueryASTNodes executes a query and returns AST nodes
func (c *ASTCache) QueryASTNodes(query string, args ...interface{}) ([]*models.ASTNode, error) {
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query AST nodes: %w", err)
	}
	defer rows.Close()

	var nodes []*models.ASTNode
	for rows.Next() {
		node := &models.ASTNode{}
		var parametersJSON, returnValuesJSON sql.NullString

		err := rows.Scan(
			&node.ID,
			&node.FilePath,
			&node.PackageName,
			&node.TypeName,
			&node.MethodName,
			&node.FieldName,
			&node.NodeType,
			&node.StartLine,
			&node.EndLine,
			&node.CyclomaticComplexity,
			&node.ParameterCount,
			&node.ReturnCount,
			&node.LineCount,
			&parametersJSON,
			&returnValuesJSON,
			&node.LastModified,
			&node.FileHash,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan AST node: %w", err)
		}

		// Deserialize parameters and return values
		if parametersJSON.Valid && parametersJSON.String != "" {
			if err := json.Unmarshal([]byte(parametersJSON.String), &node.Parameters); err != nil {
				// Log error but don't fail - backward compatibility
				fmt.Printf("Warning: failed to unmarshal parameters: %v\n", err)
			}
		}

		if returnValuesJSON.Valid && returnValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(returnValuesJSON.String), &node.ReturnValues); err != nil {
				// Log error but don't fail - backward compatibility
				fmt.Printf("Warning: failed to unmarshal return values: %v\n", err)
			}
		}

		nodes = append(nodes, node)
	}

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
	var dbHash string
	var lastAnalyzed time.Time
	err = c.db.QueryRow(`
		SELECT file_hash, last_analyzed
		FROM file_metadata
		WHERE file_path = ?`, filePath).Scan(&dbHash, &lastAnalyzed)

	if err == sql.ErrNoRows {
		return true, nil // File not in cache
	}
	if err != nil {
		return true, err
	}

	// Compare hashes - if content is the same, no reanalysis needed
	return currentHash != dbHash, nil
}

// UpdateFileMetadata updates or inserts file metadata
func (c *ASTCache) UpdateFileMetadata(filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	fileHash, err := calculateFileHash(filePath)
	if err != nil {
		return err
	}

	_, err = c.db.Exec(`
		INSERT OR REPLACE INTO file_metadata
		(file_path, file_hash, file_size, last_modified, last_analyzed, analysis_version)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, '1.0')`,
		filePath, fileHash, fileInfo.Size(), fileInfo.ModTime())

	return err
}

// StoreASTNode stores an AST node and returns its ID
func (c *ASTCache) StoreASTNode(node *models.ASTNode) (int64, error) {
	// Serialize parameters and return values to JSON
	var parametersJSON, returnValuesJSON []byte
	var err error

	if len(node.Parameters) > 0 {
		parametersJSON, err = json.Marshal(node.Parameters)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal parameters: %w", err)
		}
	}

	if len(node.ReturnValues) > 0 {
		returnValuesJSON, err = json.Marshal(node.ReturnValues)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal return values: %w", err)
		}
	}

	result, err := c.db.Exec(`
		INSERT OR REPLACE INTO ast_nodes
		(file_path, package_name, type_name, method_name, field_name, node_type,
		 start_line, end_line, cyclomatic_complexity, parameter_count, return_count,
		 line_count, parameters_json, return_values_json, last_modified, file_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		node.FilePath, node.PackageName, node.TypeName, node.MethodName,
		node.FieldName, node.NodeType, node.StartLine, node.EndLine,
		node.CyclomaticComplexity, node.ParameterCount, node.ReturnCount,
		node.LineCount, parametersJSON, returnValuesJSON, node.LastModified, node.FileHash)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// GetASTNode retrieves an AST node by ID
func (c *ASTCache) GetASTNode(id int64) (*models.ASTNode, error) {
	var node models.ASTNode
	var parametersJSON, returnValuesJSON []byte

	err := c.db.QueryRow(`
		SELECT id, file_path, package_name, type_name, method_name, field_name,
		       node_type, start_line, end_line, cyclomatic_complexity,
		       parameter_count, return_count, line_count,
		       parameters_json, return_values_json, last_modified
		FROM ast_nodes WHERE id = ?`, id).Scan(
		&node.ID, &node.FilePath, &node.PackageName, &node.TypeName,
		&node.MethodName, &node.FieldName, &node.NodeType, &node.StartLine,
		&node.EndLine, &node.CyclomaticComplexity, &node.ParameterCount,
		&node.ReturnCount, &node.LineCount,
		&parametersJSON, &returnValuesJSON, &node.LastModified)

	if err != nil {
		return nil, err
	}

	// Deserialize parameters
	if len(parametersJSON) > 0 {
		if err := json.Unmarshal(parametersJSON, &node.Parameters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
		}
	}

	// Deserialize return values
	if len(returnValuesJSON) > 0 {
		if err := json.Unmarshal(returnValuesJSON, &node.ReturnValues); err != nil {
			return nil, fmt.Errorf("failed to unmarshal return values: %w", err)
		}
	}

	return &node, nil
}

// GetASTNodesByFile retrieves all AST nodes for a file
func (c *ASTCache) GetASTNodesByFile(filePath string) ([]*models.ASTNode, error) {
	rows, err := c.db.Query(`
		SELECT id, file_path, package_name, type_name, method_name, field_name,
		       node_type, start_line, end_line, cyclomatic_complexity,
		       parameter_count, return_count, line_count,
		       parameters_json, return_values_json, last_modified
		FROM ast_nodes WHERE file_path = ?
		ORDER BY start_line`, filePath)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*models.ASTNode
	for rows.Next() {
		var node models.ASTNode
		var parametersJSON, returnValuesJSON []byte

		err := rows.Scan(&node.ID, &node.FilePath, &node.PackageName,
			&node.TypeName, &node.MethodName, &node.FieldName, &node.NodeType,
			&node.StartLine, &node.EndLine, &node.CyclomaticComplexity,
			&node.ParameterCount, &node.ReturnCount, &node.LineCount,
			&parametersJSON, &returnValuesJSON, &node.LastModified)
		if err != nil {
			return nil, err
		}

		// Deserialize parameters
		if len(parametersJSON) > 0 {
			if err := json.Unmarshal(parametersJSON, &node.Parameters); err != nil {
				return nil, fmt.Errorf("failed to unmarshal parameters for node %d: %w", node.ID, err)
			}
		}

		// Deserialize return values
		if len(returnValuesJSON) > 0 {
			if err := json.Unmarshal(returnValuesJSON, &node.ReturnValues); err != nil {
				return nil, fmt.Errorf("failed to unmarshal return values for node %d: %w", node.ID, err)
			}
		}

		nodes = append(nodes, &node)
	}

	return nodes, nil
}

// StoreASTRelationship stores a relationship between AST nodes
func (c *ASTCache) StoreASTRelationship(fromID int64, toID *int64, lineNo int, relType, text string) error {
	_, err := c.db.Exec(`
		INSERT INTO ast_relationships
		(from_ast_id, to_ast_id, line_no, relationship_type, text)
		VALUES (?, ?, ?, ?, ?)`,
		fromID, toID, lineNo, relType, text)

	return err
}

// GetASTRelationships retrieves relationships for an AST node
func (c *ASTCache) GetASTRelationships(astID int64, relType string) ([]*models.ASTRelationship, error) {
	query := `
		SELECT id, from_ast_id, to_ast_id, line_no, relationship_type, text
		FROM ast_relationships
		WHERE from_ast_id = ?`

	args := []interface{}{astID}
	if relType != "" {
		query += " AND relationship_type = ?"
		args = append(args, relType)
	}

	query += " ORDER BY line_no"

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []*models.ASTRelationship
	for rows.Next() {
		var rel models.ASTRelationship
		err := rows.Scan(&rel.ID, &rel.FromASTID, &rel.ToASTID, &rel.LineNo,
			&rel.RelationshipType, &rel.Text)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, &rel)
	}

	return relationships, nil
}

// StoreLibraryNode stores a library node and returns its ID
func (c *ASTCache) StoreLibraryNode(pkg, class, method, field, nodeType, language, framework string) (int64, error) {
	result, err := c.db.Exec(`
		INSERT OR IGNORE INTO library_nodes
		(package, class, method, field, node_type, language, framework)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		pkg, class, method, field, nodeType, language, framework)

	if err != nil {
		return 0, err
	}

	// If already exists, get the ID
	if rowsAffected, _ := result.RowsAffected(); rowsAffected == 0 {
		var id int64
		err = c.db.QueryRow(`
			SELECT id FROM library_nodes
			WHERE package = ? AND class = ? AND method = ? AND field = ?`,
			pkg, class, method, field).Scan(&id)
		return id, err
	}

	return result.LastInsertId()
}

// StoreLibraryRelationship stores a relationship between AST node and library node
func (c *ASTCache) StoreLibraryRelationship(astID, libraryID int64, lineNo int, relType, text string) error {
	_, err := c.db.Exec(`
		INSERT INTO library_relationships
		(ast_id, library_id, line_no, relationship_type, text)
		VALUES (?, ?, ?, ?, ?)`,
		astID, libraryID, lineNo, relType, text)

	return err
}

// GetLibraryRelationships retrieves library relationships for an AST node
func (c *ASTCache) GetLibraryRelationships(astID int64, relType string) ([]*models.LibraryRelationship, error) {
	query := `
		SELECT lr.id, lr.ast_id, lr.library_id, lr.line_no, lr.relationship_type, lr.text,
		       ln.package, ln.class, ln.method, ln.field, ln.node_type, ln.language, ln.framework
		FROM library_relationships lr
		JOIN library_nodes ln ON lr.library_id = ln.id
		WHERE lr.ast_id = ?`

	args := []interface{}{astID}
	if relType != "" {
		query += " AND lr.relationship_type = ?"
		args = append(args, relType)
	}

	query += " ORDER BY lr.line_no"

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []*models.LibraryRelationship
	for rows.Next() {
		var rel models.LibraryRelationship
		var libNode models.LibraryNode
		err := rows.Scan(&rel.ID, &rel.ASTID, &rel.LibraryID, &rel.LineNo,
			&rel.RelationshipType, &rel.Text, &libNode.Package, &libNode.Class,
			&libNode.Method, &libNode.Field, &libNode.NodeType, &libNode.Language,
			&libNode.Framework)
		if err != nil {
			return nil, err
		}
		rel.LibraryNode = &libNode
		relationships = append(relationships, &rel)
	}

	return relationships, nil
}

// DeleteASTForFile removes all AST data for a file (for re-analysis)
func (c *ASTCache) DeleteASTForFile(filePath string) error {
	// Get all AST node IDs for the file
	rows, err := c.db.Query("SELECT id FROM ast_nodes WHERE file_path = ?", filePath)
	if err != nil {
		return err
	}
	defer rows.Close()

	var nodeIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		nodeIDs = append(nodeIDs, id)
	}

	// Begin transaction
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete relationships first (due to foreign key constraints)
	for _, nodeID := range nodeIDs {
		_, err = tx.Exec("DELETE FROM ast_relationships WHERE from_ast_id = ? OR to_ast_id = ?", nodeID, nodeID)
		if err != nil {
			return err
		}

		_, err = tx.Exec("DELETE FROM library_relationships WHERE ast_id = ?", nodeID)
		if err != nil {
			return err
		}
	}

	// Delete AST nodes
	_, err = tx.Exec("DELETE FROM ast_nodes WHERE file_path = ?", filePath)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// QueryRaw executes a raw SQL query and returns rows
func (c *ASTCache) QueryRaw(query string, args ...interface{}) (*sql.Rows, error) {
	return c.db.Query(query, args...)
}

// CountImports counts the number of import relationships for a node
func (c *ASTCache) CountImports(nodeID int64) (int, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM ast_relationships
		WHERE from_ast_id = ? AND relationship_type = 'import'
	`
	err := c.db.QueryRow(query, nodeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count imports: %w", err)
	}
	return count, nil
}

// CountExternalCalls counts the number of call relationships to nodes outside the current package
func (c *ASTCache) CountExternalCalls(nodeID int64) (int, error) {
	var count int
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
	err := c.db.QueryRow(query, nodeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count external calls: %w", err)
	}
	return count, nil
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
	// Begin transaction
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing data for the file
	// Get all AST node IDs for the file
	rows, err := tx.Query("SELECT id FROM ast_nodes WHERE file_path = ?", file)
	if err != nil {
		return fmt.Errorf("failed to query existing nodes: %w", err)
	}

	var nodeIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan node ID: %w", err)
		}
		nodeIDs = append(nodeIDs, id)
	}
	rows.Close()

	// Delete relationships first (due to foreign key constraints)
	for _, nodeID := range nodeIDs {
		_, err = tx.Exec("DELETE FROM ast_relationships WHERE from_ast_id = ? OR to_ast_id = ?", nodeID, nodeID)
		if err != nil {
			return fmt.Errorf("failed to delete relationships: %w", err)
		}

		_, err = tx.Exec("DELETE FROM library_relationships WHERE ast_id = ?", nodeID)
		if err != nil {
			return fmt.Errorf("failed to delete library relationships: %w", err)
		}
	}

	// Delete AST nodes
	_, err = tx.Exec("DELETE FROM ast_nodes WHERE file_path = ?", file)
	if err != nil {
		return fmt.Errorf("failed to delete nodes: %w", err)
	}

	// Store new nodes
	nodeIDMap := make(map[int64]int64) // Map old IDs to new IDs
	for _, node := range r.Nodes {
		// Serialize parameters and return values to JSON
		var parametersJSON, returnValuesJSON []byte

		if len(node.Parameters) > 0 {
			parametersJSON, err = json.Marshal(node.Parameters)
			if err != nil {
				return fmt.Errorf("failed to marshal parameters: %w", err)
			}
		}

		if len(node.ReturnValues) > 0 {
			returnValuesJSON, err = json.Marshal(node.ReturnValues)
			if err != nil {
				return fmt.Errorf("failed to marshal return values: %w", err)
			}
		}

		res, err := tx.Exec(`
			INSERT OR REPLACE INTO ast_nodes
			(file_path, package_name, type_name, method_name, field_name, node_type,
			 start_line, end_line, cyclomatic_complexity, parameter_count, return_count,
			 line_count, parameters_json, return_values_json, last_modified, file_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			node.FilePath, node.PackageName, node.TypeName, node.MethodName,
			node.FieldName, node.NodeType, node.StartLine, node.EndLine,
			node.CyclomaticComplexity, node.ParameterCount, node.ReturnCount,
			node.LineCount, parametersJSON, returnValuesJSON, node.LastModified, node.FileHash)

		if err != nil {
			return fmt.Errorf("failed to store node: %w", err)
		}

		newID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get inserted node ID: %w", err)
		}

		// Map old ID to new ID for relationship updates
		if node.ID > 0 {
			nodeIDMap[node.ID] = newID
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

		_, err = tx.Exec(`
			INSERT INTO ast_relationships
			(from_ast_id, to_ast_id, line_no, relationship_type, text)
			VALUES (?, ?, ?, ?, ?)`,
			fromID, toID, rel.LineNo, rel.RelationshipType, rel.Text)

		if err != nil {
			return fmt.Errorf("failed to store relationship: %w", err)
		}
	}

	// Store library dependencies
	for _, lib := range r.Libraries {
		if lib.LibraryNode != nil {
			res, err := tx.Exec(`
				INSERT OR IGNORE INTO library_nodes
				(package, class, method, field, node_type, language, framework)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				lib.LibraryNode.Package,
				lib.LibraryNode.Class,
				lib.LibraryNode.Method,
				lib.LibraryNode.Field,
				lib.LibraryNode.NodeType,
				lib.LibraryNode.Language,
				lib.LibraryNode.Framework)

			if err != nil {
				return fmt.Errorf("failed to store library node: %w", err)
			}

			var libraryID int64
			if rowsAffected, _ := res.RowsAffected(); rowsAffected == 0 {
				// Already exists, get the ID
				err = tx.QueryRow(`
					SELECT id FROM library_nodes
					WHERE package = ? AND class = ? AND method = ? AND field = ?`,
					lib.LibraryNode.Package, lib.LibraryNode.Class,
					lib.LibraryNode.Method, lib.LibraryNode.Field).Scan(&libraryID)
				if err != nil {
					return fmt.Errorf("failed to get library node ID: %w", err)
				}
			} else {
				libraryID, err = res.LastInsertId()
				if err != nil {
					return fmt.Errorf("failed to get inserted library ID: %w", err)
				}
			}

			// Store library relationship if we have an AST ID
			if lib.ASTID > 0 {
				astID := nodeIDMap[lib.ASTID]
				if astID > 0 {
					_, err = tx.Exec(`
						INSERT INTO library_relationships
						(ast_id, library_id, line_no, relationship_type, text)
						VALUES (?, ?, ?, ?, ?)`,
						astID, libraryID, lib.LineNo, lib.RelationshipType, lib.Text)

					if err != nil {
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

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO file_metadata
		(file_path, file_hash, file_size, last_modified, last_analyzed, analysis_version)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, '1.0')`,
		file, fileHash, fileInfo.Size(), fileInfo.ModTime())

	if err != nil {
		return fmt.Errorf("failed to update file metadata: %w", err)
	}

	// Commit transaction
	return tx.Commit()
}

// GetDependencyAlias retrieves a cached dependency alias
func (c *ASTCache) GetDependencyAlias(packageName, packageType string) (*models.DependencyAlias, error) {
	var gitURL string
	var lastChecked, createdAt int64
	err := c.db.QueryRow(`
		SELECT git_url, last_checked, created_at FROM dependency_aliases
		WHERE package_name = ? AND package_type = ?`,
		packageName, packageType).Scan(&gitURL, &lastChecked, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &models.DependencyAlias{
		PackageName: packageName,
		PackageType: packageType,
		GitURL:      gitURL,
		LastChecked: lastChecked,
		CreatedAt:   createdAt,
	}, nil
}

// StoreDependencyAlias stores a dependency alias in the cache
func (c *ASTCache) StoreDependencyAlias(alias *models.DependencyAlias) error {
	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO dependency_aliases
		(package_name, package_type, git_url, last_checked, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		alias.PackageName, alias.PackageType, alias.GitURL, alias.LastChecked, alias.CreatedAt)
	return err
}
