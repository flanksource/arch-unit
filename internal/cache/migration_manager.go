package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flanksource/commons/logger"
	_ "modernc.org/sqlite"
)

// MigrationManager coordinates migrations across all cache systems
type MigrationManager struct {
	cacheDir string
	db       *DB
}

// Migration represents a single migration operation
type Migration struct {
	Version     int
	Name        string
	Description string
	Up          func(m *MigrationManager) error
	Down        func(m *MigrationManager) error
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager() (*MigrationManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "arch-unit")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Use a central migration database to coordinate all cache migrations
	dbPath := filepath.Join(cacheDir, "migrations.db")
	db, err := NewDB("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open migration database: %w", err)
	}

	return &MigrationManager{
		cacheDir: cacheDir,
		db:       db,
	}, nil
}

// Close closes the migration manager
func (m *MigrationManager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// RunMigrations executes all pending migrations for all cache systems
func (m *MigrationManager) RunMigrations() error {
	// Initialize migration tracking table
	if err := m.initMigrationTracking(); err != nil {
		return fmt.Errorf("failed to initialize migration tracking: %w", err)
	}

	// Get current schema version
	currentVersion, err := m.getCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// Get all available migrations
	migrations := m.getAllMigrations()

	// Filter migrations that need to run
	var pendingMigrations []Migration
	for _, migration := range migrations {
		if migration.Version > currentVersion {
			pendingMigrations = append(pendingMigrations, migration)
		}
	}

	if len(pendingMigrations) == 0 {
		logger.Debugf("No migrations to run (current version: %d)", currentVersion)
		return nil
	}

	logger.Infof("Running %d migrations (from version %d)", len(pendingMigrations), currentVersion)

	// Begin transaction for all migrations
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migrations in order
	for _, migration := range pendingMigrations {
		logger.Infof("Executing migration %d: %s", migration.Version, migration.Name)

		if err := migration.Up(m); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", migration.Version, migration.Name, err)
		}

		// Record migration execution
		_, err = tx.Exec(`
			INSERT INTO schema_migrations (version, name, description, executed_at) 
			VALUES (?, ?, ?, ?)`,
			migration.Version, migration.Name, migration.Description, time.Now().Unix())
		if err != nil {
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}
	}

	// Commit all migrations
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migrations: %w", err)
	}

	logger.Infof("Successfully completed %d migrations", len(pendingMigrations))
	return nil
}

// initMigrationTracking creates the migration tracking table
func (m *MigrationManager) initMigrationTracking() error {
	schema := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		executed_at INTEGER NOT NULL,
		created_at INTEGER DEFAULT (strftime('%s', 'now'))
	);
	
	CREATE INDEX IF NOT EXISTS idx_schema_migrations_executed ON schema_migrations(executed_at);
	`

	if _, err := m.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create migration tracking table: %w", err)
	}

	return nil
}

// getCurrentVersion gets the current schema version
func (m *MigrationManager) getCurrentVersion() (int, error) {
	var version int
	err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// getAllMigrations returns all available migrations in order
func (m *MigrationManager) getAllMigrations() []Migration {
	return []Migration{
		{
			Version:     1,
			Name:        "initialize_ast_cache",
			Description: "Initialize AST cache with all tables and indexes",
			Up:          m.migrateASTCacheV1,
		},
		{
			Version:     2,
			Name:        "initialize_violation_cache",
			Description: "Initialize violation cache with all tables and indexes",
			Up:          m.migrateViolationCacheV1,
		},
		{
			Version:     3,
			Name:        "ast_cache_dependency_aliases",
			Description: "Add dependency aliases table to AST cache",
			Up:          m.migrateASTCacheDependencyAliases,
		},
		{
			Version:     4,
			Name:        "violation_cache_stored_at",
			Description: "Add stored_at column to violations table",
			Up:          m.migrateViolationCacheStoredAt,
		},
		{
			Version:     5,
			Name:        "initialize_gorm",
			Description: "Initialize GORM migration support and additional columns",
			Up:          m.migrateToGORM,
		},
	}
}

// migrateASTCacheV1 initializes the AST cache database
func (m *MigrationManager) migrateASTCacheV1(mgr *MigrationManager) error {
	// Get AST cache database path
	astCacheDB := filepath.Join(m.cacheDir, "ast.db")

	// Open AST cache database
	astDB, err := NewDB("sqlite", astCacheDB)
	if err != nil {
		return fmt.Errorf("failed to open AST cache: %w", err)
	}
	defer func() { _ = astDB.Close() }()

	// Begin transaction on AST database
	astTx, err := astDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin AST cache transaction: %w", err)
	}
	defer astTx.Rollback()

	// Create AST cache schema
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
	`

	if _, err := astTx.Exec(schema); err != nil {
		return fmt.Errorf("failed to create AST cache schema: %w", err)
	}

	// Create indexes
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
	`

	if _, err := astTx.Exec(indexes); err != nil {
		return fmt.Errorf("failed to create AST cache indexes: %w", err)
	}

	return astTx.Commit()
}

// migrateViolationCacheV1 initializes the violation cache database
func (m *MigrationManager) migrateViolationCacheV1(mgr *MigrationManager) error {
	// Get violation cache database path
	violationCacheDB := filepath.Join(m.cacheDir, "violations.db")

	// Open violation cache database
	violationDB, err := NewDB("sqlite", violationCacheDB)
	if err != nil {
		return fmt.Errorf("failed to open violation cache: %w", err)
	}
	defer func() { _ = violationDB.Close() }()

	// Begin transaction on violation database
	violationTx, err := violationDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin violation cache transaction: %w", err)
	}
	defer violationTx.Rollback()

	// Create violation cache schema
	schema := `
	CREATE TABLE IF NOT EXISTS file_scans (
		file_path TEXT PRIMARY KEY,
		last_scan_time INTEGER NOT NULL,
		file_mod_time INTEGER NOT NULL,
		file_hash TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS violations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL,
		line INTEGER NOT NULL,
		column INTEGER NOT NULL,
		source TEXT NOT NULL,
		message TEXT,
		rule_json TEXT,
		caller_package TEXT,
		caller_method TEXT,
		called_package TEXT,
		called_method TEXT,
		fixable INTEGER DEFAULT 0,
		fix_applicability TEXT DEFAULT '',
		FOREIGN KEY (file_path) REFERENCES file_scans(file_path) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_violations_file ON violations(file_path);
	CREATE INDEX IF NOT EXISTS idx_violations_source ON violations(source);
	`

	if _, err := violationTx.Exec(schema); err != nil {
		return fmt.Errorf("failed to create violation cache schema: %w", err)
	}

	return violationTx.Commit()
}

// migrateASTCacheDependencyAliases adds dependency aliases table
func (m *MigrationManager) migrateASTCacheDependencyAliases(mgr *MigrationManager) error {
	astCacheDB := filepath.Join(m.cacheDir, "ast.db")

	astDB, err := NewDB("sqlite", astCacheDB)
	if err != nil {
		return fmt.Errorf("failed to open AST cache: %w", err)
	}
	defer func() { _ = astDB.Close() }()

	astTx, err := astDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin AST cache transaction: %w", err)
	}
	defer astTx.Rollback()

	// Add dependency aliases table
	schema := `
	CREATE TABLE IF NOT EXISTS dependency_aliases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		package_name TEXT NOT NULL,
		package_type TEXT NOT NULL,
		git_url TEXT NOT NULL,
		last_checked INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		UNIQUE(package_name, package_type)
	);

	CREATE INDEX IF NOT EXISTS idx_dependency_aliases_package ON dependency_aliases(package_name);
	CREATE INDEX IF NOT EXISTS idx_dependency_aliases_type ON dependency_aliases(package_type);
	`

	if _, err := astTx.Exec(schema); err != nil {
		return fmt.Errorf("failed to create dependency aliases table: %w", err)
	}

	return astTx.Commit()
}

// migrateViolationCacheStoredAt adds stored_at column to violations
func (m *MigrationManager) migrateViolationCacheStoredAt(mgr *MigrationManager) error {
	violationCacheDB := filepath.Join(m.cacheDir, "violations.db")

	violationDB, err := NewDB("sqlite", violationCacheDB)
	if err != nil {
		return fmt.Errorf("failed to open violation cache: %w", err)
	}
	defer func() { _ = violationDB.Close() }()

	violationTx, err := violationDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin violation cache transaction: %w", err)
	}
	defer violationTx.Rollback()

	// Check if stored_at column exists
	rows, err := violationTx.Query("PRAGMA table_info(violations)")
	if err != nil {
		return fmt.Errorf("failed to get table info: %w", err)
	}

	hasStoredAt := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan table info: %w", err)
		}

		if name == "stored_at" {
			hasStoredAt = true
			break
		}
	}
	_ = rows.Close()

	// Add stored_at column if it doesn't exist
	if !hasStoredAt {
		_, err = violationTx.Exec("ALTER TABLE violations ADD COLUMN stored_at INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return fmt.Errorf("failed to add stored_at column: %w", err)
		}

		// Update existing records to current timestamp
		_, err = violationTx.Exec("UPDATE violations SET stored_at = strftime('%s', 'now') WHERE stored_at = 0")
		if err != nil {
			return fmt.Errorf("failed to update stored_at values: %w", err)
		}

		// Create index
		_, err = violationTx.Exec("CREATE INDEX IF NOT EXISTS idx_violations_stored_at ON violations(stored_at)")
		if err != nil {
			return fmt.Errorf("failed to create stored_at index: %w", err)
		}
	}

	return violationTx.Commit()
}

// migrateToGORM adds GORM-specific columns and prepares for GORM transition
func (m *MigrationManager) migrateToGORM(mgr *MigrationManager) error {
	// Migrate AST cache for GORM compatibility
	if err := m.migrateASTCacheForGORM(); err != nil {
		return fmt.Errorf("failed to migrate AST cache for GORM: %w", err)
	}

	// Migrate violation cache for GORM compatibility  
	if err := m.migrateViolationCacheForGORM(); err != nil {
		return fmt.Errorf("failed to migrate violation cache for GORM: %w", err)
	}

	return nil
}

// migrateASTCacheForGORM adds GORM-specific columns to AST cache
func (m *MigrationManager) migrateASTCacheForGORM() error {
	astCacheDB := filepath.Join(m.cacheDir, "ast.db")

	astDB, err := NewDB("sqlite", astCacheDB)
	if err != nil {
		return fmt.Errorf("failed to open AST cache: %w", err)
	}
	defer func() { _ = astDB.Close() }()

	astTx, err := astDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin AST cache transaction: %w", err)
	}
	defer astTx.Rollback()

	// Add parent_id column to ast_nodes if it doesn't exist
	if err := m.addColumnIfNotExists(astTx, "ast_nodes", "parent_id", "INTEGER DEFAULT 0"); err != nil {
		return fmt.Errorf("failed to add parent_id column: %w", err)
	}

	// Add dependency_id column to ast_nodes if it doesn't exist
	if err := m.addColumnIfNotExists(astTx, "ast_nodes", "dependency_id", "INTEGER"); err != nil {
		return fmt.Errorf("failed to add dependency_id column: %w", err)
	}

	// Add summary column to ast_nodes if it doesn't exist
	if err := m.addColumnIfNotExists(astTx, "ast_nodes", "summary", "TEXT"); err != nil {
		return fmt.Errorf("failed to add summary column: %w", err)
	}

	// Add comments column to ast_relationships if it doesn't exist
	if err := m.addColumnIfNotExists(astTx, "ast_relationships", "comments", "TEXT"); err != nil {
		return fmt.Errorf("failed to add comments column: %w", err)
	}

	// Create indexes for new columns
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_ast_nodes_parent_id ON ast_nodes(parent_id)",
		"CREATE INDEX IF NOT EXISTS idx_ast_nodes_dependency_id ON ast_nodes(dependency_id)",
	}

	for _, indexSQL := range indexes {
		if _, err := astTx.Exec(indexSQL); err != nil {
			// Log but don't fail on index creation errors
			logger.Debugf("Warning: Failed to create index: %v", err)
		}
	}

	return astTx.Commit()
}

// migrateViolationCacheForGORM adds GORM-specific columns to violation cache
func (m *MigrationManager) migrateViolationCacheForGORM() error {
	violationCacheDB := filepath.Join(m.cacheDir, "violations.db")

	violationDB, err := NewDB("sqlite", violationCacheDB)
	if err != nil {
		return fmt.Errorf("failed to open violation cache: %w", err)
	}
	defer func() { _ = violationDB.Close() }()

	violationTx, err := violationDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin violation cache transaction: %w", err)
	}
	defer violationTx.Rollback()

	// The violations table should already have stored_at from migration 4
	// Just ensure it's there and has the right index
	if err := m.addColumnIfNotExists(violationTx, "violations", "stored_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("failed to ensure stored_at column: %w", err)
	}

	return violationTx.Commit()
}

// addColumnIfNotExists adds a column to a table if it doesn't already exist
func (m *MigrationManager) addColumnIfNotExists(tx *Tx, tableName, columnName, columnDef string) error {
	// Check if column exists
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return fmt.Errorf("failed to get table info for %s: %w", tableName, err)
	}
	defer func() { _ = rows.Close() }()

	hasColumn := false
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString

		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			continue
		}

		if name == columnName {
			hasColumn = true
			break
		}
	}
	_ = rows.Close()

	// Add column if it doesn't exist
	if !hasColumn {
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef)
		if _, err := tx.Exec(alterSQL); err != nil {
			return fmt.Errorf("failed to add column %s to %s: %w", columnName, tableName, err)
		}
	}

	return nil
}
