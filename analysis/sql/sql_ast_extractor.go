package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/models"
	"github.com/xo/dbtpl/loader"
	xo "github.com/xo/dbtpl/types"
	"github.com/xo/dburl"

	// Import database drivers
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/sijms/go-ora/v2"
)

// SQLASTExtractor extracts AST information from SQL databases using dbtpl
type SQLASTExtractor struct {
	virtualPathMgr *analysis.VirtualPathManager
}

// NewSQLASTExtractor creates a new SQL AST extractor
func NewSQLASTExtractor() *SQLASTExtractor {
	return &SQLASTExtractor{
		virtualPathMgr: analysis.NewVirtualPathManager(),
	}
}


// ExtractFromConnection extracts AST from a database connection using dbtpl
func (e *SQLASTExtractor) ExtractFromConnection(connectionString string) (*types.ASTResult, error) {
	// Create virtual path for this connection
	virtualPath := e.virtualPathMgr.CreateVirtualPath(analysis.AnalysisSource{
		Type:             "sql_connection",
		ConnectionString: connectionString,
	})

	// Parse connection URL
	u, err := dburl.Parse(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Open database connection
	db, err := sql.Open(u.Driver, u.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create context with database connection info
	ctx := context.Background()
	ctx = context.WithValue(ctx, xo.DriverKey, u.Driver)
	ctx = context.WithValue(ctx, xo.DbKey, db)
	// Note: dburl doesn't have Schema field, we'll get it from loader
	ctx = context.WithValue(ctx, xo.SchemaKey, "")

	// Get schema name
	schemaName, err := loader.Schema(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema name: %w", err)
	}
	if schemaName == "" {
		schemaName = "public" // Default schema
	}

	// Create AST result
	result := types.NewASTResult(virtualPath, "sql")

	// Extract enums
	if err := e.extractEnums(ctx, result, schemaName, virtualPath); err != nil {
		return nil, fmt.Errorf("failed to extract enums: %w", err)
	}

	// Extract tables
	if err := e.extractTables(ctx, result, schemaName, virtualPath, "table"); err != nil {
		return nil, fmt.Errorf("failed to extract tables: %w", err)
	}

	// Extract views
	if err := e.extractTables(ctx, result, schemaName, virtualPath, "view"); err != nil {
		return nil, fmt.Errorf("failed to extract views: %w", err)
	}

	// Extract procedures and functions
	if err := e.extractProcs(ctx, result, schemaName, virtualPath); err != nil {
		return nil, fmt.Errorf("failed to extract procedures: %w", err)
	}

	// Set up parent-child relationships for tree display
	e.setupParentChildRelationships(result.Nodes, result.Relationships)

	return result, nil
}

// setupParentChildRelationships assigns temporary IDs and sets up parent-child relationships for tree display
func (e *SQLASTExtractor) setupParentChildRelationships(nodes []*models.ASTNode, relationships []*models.ASTRelationship) {
	// First pass: assign temporary IDs to all nodes
	for i, node := range nodes {
		node.ID = int64(i + 1) // Start from 1 to avoid zero ID issues
	}

	// Second pass: set up parent IDs based on the Parent field
	for _, node := range nodes {
		if node.Parent != nil {
			node.ParentID = &node.Parent.ID
		}
	}

	// Third pass: set up relationship IDs based on the stored AST references
	relationshipID := int64(len(nodes) + 1) // Start relationship IDs after node IDs
	for _, relationship := range relationships {
		relationship.ID = relationshipID
		relationshipID++

		// Set FromASTID and ToASTID based on stored references
		if relationship.FromAST != nil {
			relationship.FromASTID = relationship.FromAST.ID
		}
		if relationship.ToAST != nil {
			relationship.ToASTID = &relationship.ToAST.ID
		}
	}
}

// extractEnums extracts database enums using dbtpl
func (e *SQLASTExtractor) extractEnums(ctx context.Context, result *types.ASTResult, schemaName, virtualPath string) error {
	enums, err := loader.Enums(ctx)
	if err != nil {
		// Some databases don't support enums, so we can ignore this error
		return nil
	}

	for _, enum := range enums {
		enumNode := &models.ASTNode{
			FilePath:     virtualPath,
			PackageName:  schemaName,
			TypeName:     enum.EnumName,
			NodeType:     models.NodeTypeType, // Use base type for now
			StartLine:    -1,
			LastModified: time.Now(),
			Summary:      models.StringPtr("Database enum"),
		}
		result.AddNode(enumNode)

		// Extract enum values
		enumValues, err := loader.EnumValues(ctx, enum.EnumName)
		if err != nil {
			continue
		}

		for _, value := range enumValues {
			valueNode := &models.ASTNode{
				FilePath:     virtualPath,
				PackageName:  schemaName,
				TypeName:     enum.EnumName,
				FieldName:    value.EnumValue,
				NodeType:     models.NodeTypeField, // Use base field type
				StartLine:    -1,
				LastModified: time.Now(),
				Summary:      models.StringPtr("Enum value"),
			}
			result.AddNode(valueNode)
		}
	}
	return nil
}

// extractTables extracts tables or views using dbtpl
func (e *SQLASTExtractor) extractTables(ctx context.Context, result *types.ASTResult, schemaName, virtualPath, tableType string) error {
	tables, err := loader.Tables(ctx, tableType)
	if err != nil {
		return err
	}

	// Track table nodes for setting parent relationships
	tableNodes := make(map[string]*models.ASTNode)

	for _, table := range tables {
		// Create table/view node
		nodeType := models.NodeTypeTypeTable
		if tableType == "view" {
			nodeType = models.NodeTypeTypeView
		}

		tableNode := &models.ASTNode{
			FilePath:     virtualPath,
			PackageName:  schemaName,
			TypeName:     table.TableName,
			NodeType:     nodeType,
			StartLine:    -1,
			LastModified: time.Now(),
			Summary:      models.StringPtr(fmt.Sprintf("Database %s", tableType)),
		}
		result.AddNode(tableNode)
		tableNodes[table.TableName] = tableNode

		// Extract columns with parent relationship
		if err := e.extractColumns(ctx, result, schemaName, virtualPath, table.TableName, tableNode); err != nil {
			continue // Skip this table if column extraction fails
		}

		// Extract indexes with parent relationship
		if err := e.extractIndexes(ctx, result, schemaName, virtualPath, table.TableName, tableNode); err != nil {
			continue // Skip this table if index extraction fails
		}

		// Extract foreign keys with parent relationship
		if err := e.extractForeignKeys(ctx, result, schemaName, virtualPath, table.TableName, tableNode); err != nil {
			continue // Skip this table if foreign key extraction fails
		}
	}
	return nil
}

// extractColumns extracts columns for a table using dbtpl
func (e *SQLASTExtractor) extractColumns(ctx context.Context, result *types.ASTResult, schemaName, virtualPath, tableName string, parentTable *models.ASTNode) error {
	columns, err := loader.TableColumns(ctx, tableName)
	if err != nil {
		return err
	}

	for _, column := range columns {
		columnNode := &models.ASTNode{
			FilePath:     virtualPath,
			PackageName:  schemaName,
			TypeName:     tableName,
			FieldName:    column.ColumnName,
			NodeType:     models.NodeTypeFieldColumn,
			StartLine:    -1,
			FieldType:    models.StringPtr(column.DataType),
			DefaultValue: models.StringPtr(column.DefaultValue.String),
			LastModified: time.Now(),
			Summary:      models.StringPtr(fmt.Sprintf("%s column", column.DataType)),
		}

		// Set parent relationship after the table node has been added and has an ID
		// For now, we'll store the parent reference and set IDs after all nodes are created
		columnNode.Parent = parentTable

		result.AddNode(columnNode)
	}
	return nil
}

// extractIndexes extracts indexes for a table using dbtpl
func (e *SQLASTExtractor) extractIndexes(ctx context.Context, result *types.ASTResult, schemaName, virtualPath, tableName string, parentTable *models.ASTNode) error {
	indexes, err := loader.TableIndexes(ctx, tableName)
	if err != nil {
		return err
	}

	for _, index := range indexes {
		indexNode := &models.ASTNode{
			FilePath:     virtualPath,
			PackageName:  schemaName,
			TypeName:     tableName,
			MethodName:   index.IndexName,
			NodeType:     models.NodeTypeMethod, // Use base method type
			StartLine:    -1,
			LastModified: time.Now(),
			Summary:      models.StringPtr("Database index"),
		}

		// Set parent relationship
		indexNode.Parent = parentTable

		result.AddNode(indexNode)
	}
	return nil
}

// extractForeignKeys extracts foreign keys for a table using dbtpl
func (e *SQLASTExtractor) extractForeignKeys(ctx context.Context, result *types.ASTResult, schemaName, virtualPath, tableName string, parentTable *models.ASTNode) error {
	foreignKeys, err := loader.TableForeignKeys(ctx, tableName)
	if err != nil {
		return err
	}

	for _, fk := range foreignKeys {
		// Find source column node
		sourceColumn := e.findColumnNode(result.Nodes, schemaName, tableName, fk.ColumnName)
		if sourceColumn == nil {
			continue // Skip if source column not found
		}

		// Find target column node (may be nil if target table not in current extraction)
		targetColumn := e.findColumnNode(result.Nodes, schemaName, fk.RefTableName, fk.RefColumnName)

		// Create foreign key relationship
		relationship := &models.ASTRelationship{
			FromASTID:        sourceColumn.ID,
			RelationshipType: models.RelationshipTypeForeignKey,
			Comments:         fmt.Sprintf("Foreign key constraint: %s", fk.ForeignKeyName),
			Text:             fmt.Sprintf("%s.%s -> %s.%s", tableName, fk.ColumnName, fk.RefTableName, fk.RefColumnName),
		}

		// Set target if found in current result set
		if targetColumn != nil {
			relationship.ToASTID = &targetColumn.ID
		}

		// Store the relationship reference for later ID assignment
		relationship.FromAST = sourceColumn
		if targetColumn != nil {
			relationship.ToAST = targetColumn
		}

		result.AddRelationship(relationship)
	}
	return nil
}

// findColumnNode finds a column node by schema, table, and column name
func (e *SQLASTExtractor) findColumnNode(nodes []*models.ASTNode, schemaName, tableName, columnName string) *models.ASTNode {
	for _, node := range nodes {
		if node.NodeType == models.NodeTypeFieldColumn &&
			node.PackageName == schemaName &&
			node.TypeName == tableName &&
			node.FieldName == columnName {
			return node
		}
	}
	return nil
}

// extractProcs extracts stored procedures and functions using dbtpl
func (e *SQLASTExtractor) extractProcs(ctx context.Context, result *types.ASTResult, schemaName, virtualPath string) error {
	procs, err := loader.Procs(ctx)
	if err != nil {
		// Some databases don't support procedures, so we can ignore this error
		return nil
	}

	for _, proc := range procs {
		nodeType := models.NodeTypeMethodStoredProc
		if proc.ProcType == "function" {
			nodeType = models.NodeTypeMethodFunction
		}

		// Get procedure parameters
		params, err := loader.ProcParams(ctx, proc.ProcID)
		if err != nil {
			params = nil // Continue without parameters if extraction fails
		}

		// Convert parameters
		parameters := make([]models.Parameter, len(params))
		for i, param := range params {
			parameters[i] = models.Parameter{
				Name:       param.ParamName,
				Type:       param.ParamType,
				NameLength: len(param.ParamName),
			}
		}

		procNode := &models.ASTNode{
			FilePath:       virtualPath,
			PackageName:    schemaName,
			MethodName:     proc.ProcName,
			NodeType:       nodeType,
			StartLine:      -1,
			Parameters:     parameters,
			ParameterCount: len(parameters),
			LastModified:   time.Now(),
			Summary:        models.StringPtr(fmt.Sprintf("SQL %s with %d parameters", proc.ProcType, len(parameters))),
		}
		result.AddNode(procNode)
	}
	return nil
}

