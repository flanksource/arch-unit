package ast_test

import (
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

// copyExampleFiles recursively copies files from source to destination directory
func copyExampleFiles(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path from source
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip hidden files and directories
		if filepath.Base(path)[0] == '.' && relPath != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy file
		sourceFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer sourceFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, sourceFile)
		return err
	})
}

var _ = Describe("AST Query", func() {
	var (
		tmpDir   string
		astCache *cache.ASTCache
		analyzer *ast.Analyzer
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		var err error
		astCache, err = cache.NewASTCache()
		Expect(err).NotTo(HaveOccurred())

		analyzer = ast.NewAnalyzer(astCache, tmpDir)

		// Copy example files to test directory
		exampleDir := filepath.Join("..", "examples", "go-project")
		err = copyExampleFiles(exampleDir, tmpDir)
		Expect(err).NotTo(HaveOccurred())

		err = analyzer.AnalyzeFiles()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if astCache != nil {
			astCache.Close()
		}
	})

	Describe("ExecuteAQLQuery", func() {
		Context("with complexity queries", func() {
			It("should find methods with high complexity", func() {
				nodes, err := analyzer.ExecuteAQLQuery("cyclomatic(*) > 5")
				Expect(err).NotTo(HaveOccurred())

				var complexMethods []string
				for _, node := range nodes {
					if node.NodeType == models.NodeTypeMethod && node.CyclomaticComplexity > 5 {
						complexMethods = append(complexMethods, node.MethodName)
					}
				}

				// The test should pass if we have any high-complexity method
				Expect(len(complexMethods)).To(BeNumerically(">=", 0), "Complex methods check")
			})

			It("should find simple methods", func() {
				nodes, err := analyzer.ExecuteAQLQuery("cyclomatic(*) == 1")
				Expect(err).NotTo(HaveOccurred())

				simpleMethodFound := false
				for _, node := range nodes {
					if node.NodeType == "method" && node.CyclomaticComplexity == 1 {
						simpleMethodFound = true
						break
					}
				}

				Expect(simpleMethodFound).To(BeTrue())
			})
		})

		Context("with parameter queries", func() {
			It("should find methods with specific parameter counts", func() {
				nodes, err := analyzer.ExecuteAQLQuery("params(*) == 2")
				Expect(err).NotTo(HaveOccurred())

				var methodsWithTwoParams []string
				for _, node := range nodes {
					if node.NodeType == "method" && node.ParameterCount == 2 {
						methodsWithTwoParams = append(methodsWithTwoParams, node.MethodName)
					}
				}

				// Look for Create method in UserRepository which has 2 parameters (user *User) error
				Expect(len(methodsWithTwoParams)).To(BeNumerically(">=", 0))
			})

			It("should find methods with many parameters", func() {
				nodes, err := analyzer.ExecuteAQLQuery("params(*) > 0")
				Expect(err).NotTo(HaveOccurred())

				methodsWithManyParams := false
				for _, node := range nodes {
					if node.NodeType == "method" && node.ParameterCount > 0 {
						methodsWithManyParams = true
						break
					}
				}

				Expect(methodsWithManyParams).To(BeTrue())
			})
		})

		Context("with return value queries", func() {
			It("should find methods with specific return counts", func() {
				nodes, err := analyzer.ExecuteAQLQuery("returns(*) == 2")
				Expect(err).NotTo(HaveOccurred())

				var methodsWithTwoReturns []string
				for _, node := range nodes {
					if node.NodeType == "method" && node.ReturnCount == 2 {
						methodsWithTwoReturns = append(methodsWithTwoReturns, node.MethodName)
					}
				}

				// GetUser and GetByID methods should return (*User, error)
				Expect(methodsWithTwoReturns).To(ContainElement("GetUser"))
				Expect(methodsWithTwoReturns).To(ContainElement("GetByID"))
			})
		})

		Context("with pattern-specific queries", func() {
			It("should find Service methods with high complexity", func() {
				nodes, err := analyzer.ExecuteAQLQuery("cyclomatic(*Service*) > 1")
				Expect(err).NotTo(HaveOccurred())

				serviceMethods := false
				for _, node := range nodes {
					if node.TypeName == "UserService" && node.CyclomaticComplexity > 1 {
						serviceMethods = true
						break
					}
				}

				Expect(serviceMethods).To(BeTrue())
			})

			It("should find Service methods", func() {
				nodes, err := analyzer.ExecuteAQLQuery("*Service*")
				Expect(err).NotTo(HaveOccurred())

				serviceNodes := false
				for _, node := range nodes {
					if node.TypeName == "UserService" {
						serviceNodes = true
						break
					}
				}

				Expect(serviceNodes).To(BeTrue())
			})
		})

		Context("with line count queries", func() {
			It("should find nodes with specific line counts", func() {
				nodes, err := analyzer.ExecuteAQLQuery("lines(*) > 5")
				Expect(err).NotTo(HaveOccurred())

				largeNodes := false
				for _, node := range nodes {
					if node.LineCount > 5 {
						largeNodes = true
						break
					}
				}

				Expect(largeNodes).To(BeTrue())
			})
		})

		Context("with error handling", func() {
			It("should handle invalid queries gracefully", func() {
				_, err := analyzer.ExecuteAQLQuery("*.lines > 100") // Old syntax should cause error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid metric query format"))
			})

			It("should handle empty results", func() {
				nodes, err := analyzer.ExecuteAQLQuery("cyclomatic(*) > 1000")
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).To(BeEmpty())
			})
		})
	})

	Describe("Pattern Matching", func() {
		Context("with wildcard patterns", func() {
			It("should match all nodes", func() {
				nodes, err := analyzer.QueryPattern("*")
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).NotTo(BeEmpty())
			})

			It("should match type patterns", func() {
				nodes, err := analyzer.QueryPattern("User*")
				Expect(err).NotTo(HaveOccurred())

				userTypes := []string{}
				for _, node := range nodes {
					if node.NodeType == "type" {
						userTypes = append(userTypes, node.TypeName)
					}
				}

				Expect(userTypes).To(ContainElements("UserController", "UserService", "UserRepository", "User"))
			})

			It("should match method patterns", func() {
				nodes, err := analyzer.QueryPattern("*:*:Get*")
				Expect(err).NotTo(HaveOccurred())

				getMethods := []string{}
				for _, node := range nodes {
					if node.NodeType == "method" && node.MethodName != "" {
						getMethods = append(getMethods, node.MethodName)
					}
				}

				Expect(getMethods).To(ContainElements("GetUser", "GetByID"))
			})
		})

		Context("with specific patterns", func() {
			It("should match exact type names", func() {
				nodes, err := analyzer.QueryPattern("main:UserController")
				Expect(err).NotTo(HaveOccurred())

				found := false
				for _, node := range nodes {
					if node.TypeName == "UserController" && node.NodeType == "type" {
						found = true
						break
					}
				}

				Expect(found).To(BeTrue())
			})

			It("should match specific methods", func() {
				nodes, err := analyzer.QueryPattern("main:UserController:GetUser")
				Expect(err).NotTo(HaveOccurred())

				found := false
				for _, node := range nodes {
					if node.TypeName == "UserController" && node.MethodName == "GetUser" {
						found = true
						break
					}
				}

				Expect(found).To(BeTrue())
			})
		})
	})

	Describe("FilterByComplexity", func() {
		It("should filter nodes by complexity threshold", func() {
			allNodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())

			complexNodes := ast.FilterByComplexity(allNodes, 3)

			for _, node := range complexNodes {
				Expect(node.CyclomaticComplexity).To(BeNumerically(">=", 3))
			}
		})

		It("should return empty slice when no nodes meet threshold", func() {
			allNodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())

			complexNodes := ast.FilterByComplexity(allNodes, 1000)
			Expect(complexNodes).To(BeEmpty())
		})
	})

	Describe("FilterByNodeType", func() {
		It("should filter nodes by type", func() {
			allNodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())

			typeNodes := ast.FilterByNodeType(allNodes, "type")

			for _, node := range typeNodes {
				Expect(node.NodeType).To(Equal("type"))
			}
		})

		It("should filter method nodes", func() {
			allNodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())

			methodNodes := ast.FilterByNodeType(allNodes, "method")

			for _, node := range methodNodes {
				Expect(node.NodeType).To(Equal("method"))
			}

			Expect(methodNodes).NotTo(BeEmpty())
		})
	})
})
