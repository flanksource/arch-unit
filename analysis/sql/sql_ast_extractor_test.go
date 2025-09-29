package sql_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sqlextractor "github.com/flanksource/arch-unit/analysis/sql"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"

	_ "github.com/mattn/go-sqlite3"
)

func TestSQL(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQL Suite")
}

var _ = Describe("SQL AST Extractor", func() {
	var (
		extractor *sqlextractor.SQLASTExtractor
		astCache  *cache.ASTCache
		testDBPath string
	)

	BeforeEach(func() {
		extractor = sqlextractor.NewSQLASTExtractor()
		astCache = cache.MustGetASTCache()

		// Create a temporary test database
		tmpDir, err := os.MkdirTemp("", "arch-unit-sql-test")
		Expect(err).NotTo(HaveOccurred())
		testDBPath = filepath.Join(tmpDir, "test.db")
	})

	AfterEach(func() {
		if testDBPath != "" {
			os.RemoveAll(filepath.Dir(testDBPath))
		}
	})

	Context("when creating a test database with known schema", func() {
		var testDB *sql.DB

		BeforeEach(func() {
			var err error
			testDB, err = sql.Open("sqlite3", testDBPath)
			Expect(err).NotTo(HaveOccurred())

			// Create test schema
			_, err = testDB.Exec(`
				CREATE TABLE users (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					email TEXT NOT NULL UNIQUE,
					name TEXT NOT NULL,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE orders (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id INTEGER NOT NULL,
					total REAL NOT NULL,
					status TEXT DEFAULT 'pending',
					FOREIGN KEY (user_id) REFERENCES users(id)
				);

				CREATE VIEW user_orders AS
				SELECT u.name, u.email, COUNT(o.id) as order_count
				FROM users u
				LEFT JOIN orders o ON u.id = o.user_id
				GROUP BY u.id, u.name, u.email;

				CREATE INDEX idx_orders_user_id ON orders(user_id);
				CREATE INDEX idx_users_email ON users(email);
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if testDB != nil {
				testDB.Close()
			}
		})

		It("should extract tables correctly", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Check tables
			var tableNodes []*models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeTypeTable {
					tableNodes = append(tableNodes, node)
				}
			}

			Expect(tableNodes).To(HaveLen(2))

			tableNames := make([]string, len(tableNodes))
			for i, table := range tableNodes {
				tableNames[i] = table.TypeName
			}
			Expect(tableNames).To(ConsistOf("users", "orders"))
		})

		It("should extract views correctly", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Check views
			var viewNodes []*models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeTypeView {
					viewNodes = append(viewNodes, node)
				}
			}

			Expect(viewNodes).To(HaveLen(1))
			Expect(viewNodes[0].TypeName).To(Equal("user_orders"))
		})

		It("should extract columns correctly", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Check columns for users table
			var userColumns []*models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeFieldColumn && node.TypeName == "users" {
					userColumns = append(userColumns, node)
				}
			}

			Expect(userColumns).To(HaveLen(4))

			columnNames := make([]string, len(userColumns))
			for i, col := range userColumns {
				columnNames[i] = col.FieldName
			}
			Expect(columnNames).To(ConsistOf("id", "email", "name", "created_at"))
		})

		It("should have correct parent-child relationships for columns", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Find table nodes
			var usersTable, ordersTable *models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeTypeTable {
					if node.TypeName == "users" {
						usersTable = node
					} else if node.TypeName == "orders" {
						ordersTable = node
					}
				}
			}

			Expect(usersTable).NotTo(BeNil())
			Expect(ordersTable).NotTo(BeNil())

			// Check that columns have correct parent relationships
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeFieldColumn {
					if node.TypeName == "users" {
						Expect(node.ParentID).NotTo(BeNil())
						Expect(*node.ParentID).To(Equal(usersTable.ID))
						Expect(node.Parent).To(Equal(usersTable))
					} else if node.TypeName == "orders" {
						Expect(node.ParentID).NotTo(BeNil())
						Expect(*node.ParentID).To(Equal(ordersTable.ID))
						Expect(node.Parent).To(Equal(ordersTable))
					}
				}
			}
		})

		It("should extract indexes correctly", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Check indexes
			var indexNodes []*models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeMethod && node.Summary == "Database index" {
					indexNodes = append(indexNodes, node)
				}
			}

			// Should have at least our created indexes
			Expect(len(indexNodes)).To(BeNumerically(">=", 2))

			// Check for specific indexes
			indexNames := make([]string, len(indexNodes))
			for i, idx := range indexNodes {
				indexNames[i] = idx.MethodName
			}

			// Our explicitly created indexes should be there
			Expect(indexNames).To(ContainElement("idx_orders_user_id"))
			Expect(indexNames).To(ContainElement("idx_users_email"))
		})

		It("should have correct parent-child relationships for indexes", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Find table nodes
			var usersTable, ordersTable *models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeTypeTable {
					if node.TypeName == "users" {
						usersTable = node
					} else if node.TypeName == "orders" {
						ordersTable = node
					}
				}
			}

			Expect(usersTable).NotTo(BeNil())
			Expect(ordersTable).NotTo(BeNil())

			// Check that indexes have correct parent relationships
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeMethod && node.Summary == "Database index" {
					if node.TypeName == "users" {
						Expect(node.ParentID).NotTo(BeNil())
						Expect(*node.ParentID).To(Equal(usersTable.ID))
						Expect(node.Parent).To(Equal(usersTable))
					} else if node.TypeName == "orders" {
						Expect(node.ParentID).NotTo(BeNil())
						Expect(*node.ParentID).To(Equal(ordersTable.ID))
						Expect(node.Parent).To(Equal(ordersTable))
					}
				}
			}
		})

		It("should display correct hierarchy in tree format", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Build hierarchical tree
			config := models.DisplayConfig{
				ShowDirs:     false,
				ShowFiles:    true,
				ShowPackages: false,
				ShowTypes:    true,
				ShowMethods:  true,
				ShowFields:   true,
				ShowLineNo:   false,
				ShowFileStats: false,
				ShowComplexity: false,
			}

			tree := models.BuildHierarchicalASTTree(result.Nodes, config, filepath.Dir(testDBPath))

			// Format tree as text
			output, err := clicky.Format(tree, clicky.FormatOptions{
				Format:  "tree",
				NoColor: true,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify tree structure contains expected elements
			Expect(output).To(ContainSubstring("sqlite3"))  // File level
			Expect(output).To(ContainSubstring("users"))    // Table level
			Expect(output).To(ContainSubstring("orders"))   // Table level
			Expect(output).To(ContainSubstring("email"))    // Column under users
			Expect(output).To(ContainSubstring("user_id"))  // Column under orders
			Expect(output).To(ContainSubstring("idx_users_email"))    // Index under users
			Expect(output).To(ContainSubstring("idx_orders_user_id")) // Index under orders

			// Verify hierarchy structure: columns and indexes should be indented under tables
			lines := strings.Split(output, "\n")
			var usersLineIdx, ordersLineIdx int
			for i, line := range lines {
				if strings.Contains(line, "users") && strings.Contains(line, "🏷️") {
					usersLineIdx = i
				}
				if strings.Contains(line, "orders") && strings.Contains(line, "🏷️") {
					ordersLineIdx = i
				}
			}

			// Check that columns and indexes appear after their respective table and are more indented
			for i := usersLineIdx + 1; i < len(lines) && i < ordersLineIdx; i++ {
				line := lines[i]
				if strings.Contains(line, "email") || strings.Contains(line, "idx_users_email") {
					// Should be more indented than the table line
					Expect(len(line) - len(strings.TrimLeft(line, " │├└─"))).To(BeNumerically(">",
						len(lines[usersLineIdx]) - len(strings.TrimLeft(lines[usersLineIdx], " │├└─"))))
				}
			}
		})

		It("should work with both ShowDirs true and false configurations", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Test with ShowDirs: true (default behavior)
			configWithDirs := models.DefaultDisplayConfig()
			treeWithDirs := models.BuildHierarchicalASTTree(result.Nodes, configWithDirs, filepath.Dir(testDBPath))

			outputWithDirs, err := clicky.Format(treeWithDirs, clicky.FormatOptions{
				Format:  "tree",
				NoColor: true,
			})
			Expect(err).NotTo(HaveOccurred())

			// Test with ShowDirs: false
			configNoDirs := models.DisplayConfig{
				ShowDirs:     false,
				ShowFiles:    true,
				ShowPackages: false,
				ShowTypes:    true,
				ShowMethods:  true,
				ShowFields:   true,
				ShowLineNo:   false,
				ShowFileStats: false,
				ShowComplexity: false,
			}
			treeNoDirs := models.BuildHierarchicalASTTree(result.Nodes, configNoDirs, filepath.Dir(testDBPath))

			outputNoDirs, err := clicky.Format(treeNoDirs, clicky.FormatOptions{
				Format:  "tree",
				NoColor: true,
			})
			Expect(err).NotTo(HaveOccurred())

			// Both outputs should contain the same essential structure
			expectedElements := []string{"sqlite3", "users", "orders", "email", "user_id", "idx_users_email", "idx_orders_user_id"}

			for _, element := range expectedElements {
				Expect(outputWithDirs).To(ContainSubstring(element), "ShowDirs: true should contain %s", element)
				Expect(outputNoDirs).To(ContainSubstring(element), "ShowDirs: false should contain %s", element)
			}

			// Both should be non-empty
			Expect(strings.TrimSpace(outputWithDirs)).NotTo(BeEmpty(), "ShowDirs: true should produce output")
			Expect(strings.TrimSpace(outputNoDirs)).NotTo(BeEmpty(), "ShowDirs: false should produce output")
		})

		It("should extract foreign keys correctly", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())

			// Check foreign keys
			var fkNodes []*models.ASTNode
			for _, node := range result.Nodes {
				if node.NodeType == models.NodeTypeMethod && strings.Contains(node.Summary, "Foreign key") {
					fkNodes = append(fkNodes, node)
				}
			}

			// Should have the foreign key from orders to users
			Expect(len(fkNodes)).To(BeNumerically(">=", 1))

			// Check that at least one foreign key points to users
			var hasUsersFK bool
			for _, fk := range fkNodes {
				if fk.TypeName == "orders" && fk.Summary == "Foreign key to users" {
					hasUsersFK = true
					break
				}
			}
			Expect(hasUsersFK).To(BeTrue())
		})
	})

	Context("when testing cache integration", func() {
		var testDB *sql.DB

		BeforeEach(func() {
			var err error
			testDB, err = sql.Open("sqlite3", testDBPath)
			Expect(err).NotTo(HaveOccurred())

			// Create simple test schema
			_, err = testDB.Exec(`
				CREATE TABLE test_table (
					id INTEGER PRIMARY KEY,
					name TEXT NOT NULL
				);
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if testDB != nil {
				testDB.Close()
			}
		})

		It("should extract schema and store in cache successfully", func() {
			connectionString := "sqlite3://" + testDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Try to store the nodes in cache
			for _, node := range result.Nodes {
				nodeID, err := astCache.StoreASTNode(node)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodeID).To(BeNumerically(">", 0))
			}

			// Verify we can retrieve the nodes
			retrievedNodes, err := astCache.GetASTNodesByFile(result.FilePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(retrievedNodes)).To(BeNumerically(">=", 1))
		})

		It("should handle empty database gracefully", func() {
			// Create empty database
			emptyDBPath := filepath.Join(filepath.Dir(testDBPath), "empty.db")
			emptyDB, err := sql.Open("sqlite3", emptyDBPath)
			Expect(err).NotTo(HaveOccurred())
			defer emptyDB.Close()

			connectionString := "sqlite3://" + emptyDBPath

			result, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Should have no nodes for empty database
			Expect(result.Nodes).To(BeEmpty())
		})
	})

	Context("when handling connection errors", func() {
		It("should handle invalid connection strings", func() {
			invalidConnString := "invalid://connection/string"

			_, err := extractor.ExtractFromConnection(invalidConnString)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse connection string"))
		})

		It("should handle non-existent database files", func() {
			nonExistentPath := "/path/that/does/not/exist/db.sqlite"
			connectionString := "sqlite3://" + nonExistentPath

			_, err := extractor.ExtractFromConnection(connectionString)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ping database"))
		})
	})
})