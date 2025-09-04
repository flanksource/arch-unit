package database_test_suite

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

const (
	testPackageName = "github.com/gin-gonic/gin"
	testPackageType = "go"
	testGitURL      = "https://github.com/gin-gonic/gin.git"
)

var _ = Describe("GORM Integration", func() {
	Describe("Database Connection", func() {
		It("should create all expected tables", func() {
			migrator := testDB.DB().Migrator()

			expectedTables := []interface{}{
				&models.ASTNode{},
				&models.ASTRelationship{},
				&models.LibraryNode{},
				&models.LibraryRelationship{},
				&models.FileMetadata{},
				&models.FileScan{},
				&models.DependencyAlias{},
				&models.Violation{},
			}

			for _, table := range expectedTables {
				Expect(migrator.HasTable(table)).To(BeTrue(), "Table for model %T was not created", table)
			}
		})
	})

	Describe("Model Operations", func() {
		Context("AST Node", func() {
			It("should create and retrieve AST nodes", func() {
				astNode := testDB.CreateTestASTNode()
				Expect(astNode.ID).ToNot(BeZero(), "AST node ID should be set after creation")

				var retrievedNode models.ASTNode
				result := testDB.DB().First(&retrievedNode, astNode.ID)
				Expect(result.Error).ToNot(HaveOccurred())

				Expect(retrievedNode.FilePath).To(Equal(astNode.FilePath))
				Expect(retrievedNode.PackageName).To(Equal(astNode.PackageName))
				Expect(retrievedNode.TypeName).To(Equal(astNode.TypeName))
				Expect(retrievedNode.NodeType).To(Equal(astNode.NodeType))
			})

			It("should handle custom AST node properties", func() {
				astNode := testDB.CreateTestASTNode(func(node *models.ASTNode) {
					node.FilePath = "/custom/path.go"
					node.PackageName = "custom"
					node.MethodName = "CustomMethod"
					node.NodeType = models.NodeTypeMethod
					node.CyclomaticComplexity = 5
				})

				var retrieved models.ASTNode
				result := testDB.DB().First(&retrieved, astNode.ID)
				Expect(result.Error).ToNot(HaveOccurred())

				Expect(retrieved.FilePath).To(Equal("/custom/path.go"))
				Expect(retrieved.PackageName).To(Equal("custom"))
				Expect(retrieved.MethodName).To(Equal("CustomMethod"))
				Expect(retrieved.NodeType).To(Equal(models.NodeTypeMethod))
				Expect(retrieved.CyclomaticComplexity).To(Equal(5))
			})
		})

		Context("Violation", func() {
			It("should create and retrieve violations", func() {
				violation := testDB.CreateTestViolation()
				Expect(violation.ID).ToNot(BeZero(), "Violation ID should be set after creation")

				var violations []models.Violation
				result := testDB.DB().Where("file_path = ?", violation.File).Find(&violations)
				Expect(result.Error).ToNot(HaveOccurred())
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Message).To(Equal(violation.Message))
				Expect(violations[0].Source).To(Equal(violation.Source))
			})

			It("should handle custom violation properties", func() {
				violation := testDB.CreateTestViolation(func(v *models.Violation) {
					v.File = "/custom/violation.go"
					v.Line = 100
					v.Column = 25
					v.Source = "custom-linter"
					v.Message = "Custom violation message"
					v.CallerPackage = "custom.package"
					v.CallerMethod = "CustomMethod"
				})

				var retrieved models.Violation
				result := testDB.DB().First(&retrieved, violation.ID)
				Expect(result.Error).ToNot(HaveOccurred())

				Expect(retrieved.File).To(Equal("/custom/violation.go"))
				Expect(retrieved.Line).To(Equal(100))
				Expect(retrieved.Column).To(Equal(25))
				Expect(retrieved.Source).To(Equal("custom-linter"))
				Expect(retrieved.Message).To(Equal("Custom violation message"))
			})
		})

		Context("Library Node", func() {
			It("should create and retrieve library nodes", func() {
				libNode := testDB.CreateTestLibraryNode()
				Expect(libNode.ID).ToNot(BeZero())

				var retrieved models.LibraryNode
				result := testDB.DB().First(&retrieved, libNode.ID)
				Expect(result.Error).ToNot(HaveOccurred())

				Expect(retrieved.Package).To(Equal(libNode.Package))
				Expect(retrieved.Class).To(Equal(libNode.Class))
				Expect(retrieved.Method).To(Equal(libNode.Method))
				Expect(retrieved.Language).To(Equal(libNode.Language))
			})
		})
	})

	Describe("Dependency Alias", func() {
		It("should create and retrieve dependency aliases", func() {
			alias := testDB.CreateTestDependencyAlias()
			Expect(alias.ID).ToNot(BeZero())

			var retrievedAlias models.DependencyAlias
			result := testDB.DB().Where("package_name = ? AND package_type = ?",
				alias.PackageName, alias.PackageType).First(&retrievedAlias)
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(retrievedAlias.GitURL).To(Equal(alias.GitURL))
		})

		It("should handle custom dependency alias properties", func() {
			alias := testDB.CreateTestDependencyAlias(func(a *models.DependencyAlias) {
				a.PackageName = "custom/package"
				a.PackageType = "npm"
				a.GitURL = "https://github.com/custom/package.git"
			})

			var retrieved models.DependencyAlias
			result := testDB.DB().First(&retrieved, alias.ID)
			Expect(result.Error).ToNot(HaveOccurred())

			Expect(retrieved.PackageName).To(Equal("custom/package"))
			Expect(retrieved.PackageType).To(Equal("npm"))
			Expect(retrieved.GitURL).To(Equal("https://github.com/custom/package.git"))
		})

		It("should allow duplicate package names with different types", func() {
			// Create first alias
			testDB.CreateTestDependencyAlias()

			// Create another alias with same package but different type - should succeed
			duplicate := &models.DependencyAlias{
				PackageName: testPackageName,
				PackageType: "npm", // Different type
				GitURL:      "https://different.url.git",
				LastChecked: time.Now().Unix(),
				CreatedAt:   time.Now().Unix(),
			}

			result := testDB.DB().Create(duplicate)
			Expect(result.Error).ToNot(HaveOccurred(), "Different package types should be allowed")
		})
	})

	Describe("Data Isolation", func() {
		It("should start with clean database", func() {
			var count int64

			testDB.DB().Model(&models.ASTNode{}).Count(&count)
			Expect(count).To(BeZero(), "AST nodes should be cleared between tests")

			testDB.DB().Model(&models.Violation{}).Count(&count)
			Expect(count).To(BeZero(), "Violations should be cleared between tests")

			testDB.DB().Model(&models.DependencyAlias{}).Count(&count)
			Expect(count).To(BeZero(), "Dependency aliases should be cleared between tests")
		})
	})
})
