package ast_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("AST Analyzer", func() {
	var (
		tmpDir   string
		astCache *cache.ASTCache
		analyzer *ast.Analyzer
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		var err error
		cacheTmpDir := GinkgoT().TempDir()
		astCache, err = cache.NewASTCacheWithPath(cacheTmpDir)
		Expect(err).NotTo(HaveOccurred())
		
		analyzer = ast.NewAnalyzer(astCache, tmpDir)
	})

	AfterEach(func() {
		if astCache != nil {
			astCache.Close()
		}
	})

	Describe("AnalyzeFiles", func() {
		Context("when analyzing Go files", func() {
			BeforeEach(func() {
				// Create test Go files
				testFile1 := filepath.Join(tmpDir, "test1.go")
				content1 := `package main

type User struct {
	Name string
	Age  int
}

func main() {
	user := User{Name: "Alice", Age: 30}
	println(user.Name)
}`
				err := os.WriteFile(testFile1, []byte(content1), 0644)
				Expect(err).NotTo(HaveOccurred())

				testFile2 := filepath.Join(tmpDir, "test2.go")
				content2 := `package main

func calculate(x, y int) int {
	if x > y {
		return x * y
	}
	return x + y
}`
				err = os.WriteFile(testFile2, []byte(content2), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should analyze all Go files successfully", func() {
				err := analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should extract AST nodes from all files", func() {
				err := analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())

				// Check that nodes were created
				stats, err := analyzer.GetCacheStats()
				Expect(err).NotTo(HaveOccurred())
				Expect(stats.TotalNodes).To(BeNumerically(">", 0))
				Expect(stats.CachedFiles).To(BeNumerically(">=", 2))
			})
		})

		Context("when no Go files exist", func() {
			It("should complete without error", func() {
				err := analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("AnalyzeFilesWithFilter", func() {
		BeforeEach(func() {
			// Create various test files
			goFile := filepath.Join(tmpDir, "main.go")
			goContent := `package main
func main() { println("hello") }`
			err := os.WriteFile(goFile, []byte(goContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			testFile := filepath.Join(tmpDir, "main_test.go")
			testContent := `package main
import "testing"
func TestMain(t *testing.T) {}`
			err = os.WriteFile(testFile, []byte(testContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			pyFile := filepath.Join(tmpDir, "script.py")
			pyContent := "print('hello')"
			err = os.WriteFile(pyFile, []byte(pyContent), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("with include patterns", func() {
			It("should only analyze matching files", func() {
				err := analyzer.AnalyzeFilesWithFilter([]string{"*.go"}, nil)
				Expect(err).NotTo(HaveOccurred())

				stats, err := analyzer.GetCacheStats()
				Expect(err).NotTo(HaveOccurred())
				Expect(stats.CachedFiles).To(BeNumerically(">=", 1))
			})
		})

		Context("with exclude patterns", func() {
			It("should skip excluded files", func() {
				err := analyzer.AnalyzeFilesWithFilter(nil, []string{"*_test.go"})
				Expect(err).NotTo(HaveOccurred())

				stats, err := analyzer.GetCacheStats()
				Expect(err).NotTo(HaveOccurred())
				// Should analyze files but exclude test files
				Expect(stats.CachedFiles).To(BeNumerically(">=", 1))
			})
		})
	})

	Describe("QueryPattern", func() {
		BeforeEach(func() {
			// Create test file and analyze it
			testFile := filepath.Join(tmpDir, "query_test.go")
			content := `package main

type UserService struct{}

func (s *UserService) GetUser(id string) string {
	return "user-" + id
}

func (s *UserService) CreateUser(name string) string {
	return "created-" + name
}

type AdminService struct{}

func (s *AdminService) GetAdmin(id string) string {
	return "admin-" + id
}`
			err := os.WriteFile(testFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			err = analyzer.AnalyzeFiles()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should find all nodes with wildcard", func() {
			nodes, err := analyzer.QueryPattern("*")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodes).NotTo(BeEmpty())
		})

		It("should find specific type patterns", func() {
			nodes, err := analyzer.QueryPattern("*Service")
			Expect(err).NotTo(HaveOccurred())
			
			var serviceTypes []string
			for _, node := range nodes {
				if node.NodeType == "type" {
					serviceTypes = append(serviceTypes, node.TypeName)
				}
			}
			
			Expect(serviceTypes).To(ContainElements("UserService", "AdminService"))
		})

		It("should find method patterns", func() {
			nodes, err := analyzer.QueryPattern("*Service:Get*")
			Expect(err).NotTo(HaveOccurred())
			
			var methodNames []string
			for _, node := range nodes {
				if node.NodeType == "method" && node.MethodName != "" {
					methodNames = append(methodNames, node.MethodName)
				}
			}
			
			Expect(methodNames).To(ContainElements("GetUser", "GetAdmin"))
		})
	})

	Describe("RebuildCache", func() {
		BeforeEach(func() {
			// Create a test file
			testFile := filepath.Join(tmpDir, "rebuild_test.go")
			content := `package main
func test() { println("test") }`
			err := os.WriteFile(testFile, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Initial analysis
			err = analyzer.AnalyzeFiles()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should rebuild the cache successfully", func() {
			err := analyzer.RebuildCache()
			Expect(err).NotTo(HaveOccurred())

			stats, err := analyzer.GetCacheStats()
			Expect(err).NotTo(HaveOccurred())
			Expect(stats.TotalNodes).To(BeNumerically(">", 0))
		})
	})

	Describe("GetCacheStats", func() {
		Context("with cached data", func() {
			BeforeEach(func() {
				testFile := filepath.Join(tmpDir, "stats_test.go")
				content := `package main
type Test struct{}
func (t *Test) Method() {}
func main() {}`
				err := os.WriteFile(testFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())

				err = analyzer.AnalyzeFiles()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return accurate statistics", func() {
				stats, err := analyzer.GetCacheStats()
				Expect(err).NotTo(HaveOccurred())
				
				GinkgoWriter.Printf("TotalNodes: %d, CachedFiles: %d, TotalFiles: %d, LastUpdated: %s\n", 
					stats.TotalNodes, stats.CachedFiles, stats.TotalFiles, stats.LastUpdated)
				
				// Debug: Check what's actually in the database
				rows, err := astCache.QueryRaw("SELECT last_modified FROM ast_nodes LIMIT 3")
				Expect(err).NotTo(HaveOccurred())
				defer rows.Close()
				
				var timestamps []string
				for rows.Next() {
					var timestamp *time.Time
					err := rows.Scan(&timestamp)
					Expect(err).NotTo(HaveOccurred())
					if timestamp != nil {
						timestamps = append(timestamps, timestamp.Format("2006-01-02 15:04:05"))
					} else {
						timestamps = append(timestamps, "NULL")
					}
				}
				GinkgoWriter.Printf("Database timestamps: %v\n", timestamps)
				
				// Debug: Test the exact MAX query that GetCacheStats uses
				var maxTimestamp *time.Time
				err = astCache.QueryRow("SELECT MAX(last_modified) FROM ast_nodes").Scan(&maxTimestamp)
				GinkgoWriter.Printf("MAX query error: %v\n", err)
				if maxTimestamp != nil {
					GinkgoWriter.Printf("MAX timestamp: %s\n", maxTimestamp.Format("2006-01-02 15:04:05"))
				} else {
					GinkgoWriter.Printf("MAX timestamp: NULL\n")
				}
				
				Expect(stats.TotalNodes).To(BeNumerically(">", 0), "Should have AST nodes after analysis")
				Expect(stats.CachedFiles).To(BeNumerically(">", 0), "Should have cached files")
				Expect(stats.TotalFiles).To(BeNumerically(">=", stats.CachedFiles), "Total files should be >= cached files")
				Expect(stats.LastUpdated).NotTo(Equal("Never"), "Should have a valid last updated time")
			})
		})

		Context("with empty cache", func() {
			var emptyAnalyzer *ast.Analyzer
			var emptyCache *cache.ASTCache
			var emptyTmpDir string

			BeforeEach(func() {
				emptyTmpDir = GinkgoT().TempDir()
				var err error
				emptyCacheTmpDir := GinkgoT().TempDir()
				emptyCache, err = cache.NewASTCacheWithPath(emptyCacheTmpDir)
				Expect(err).NotTo(HaveOccurred())
				
				emptyAnalyzer = ast.NewAnalyzer(emptyCache, emptyTmpDir)
			})

			AfterEach(func() {
				if emptyCache != nil {
					emptyCache.Close()
				}
			})

			It("should return zero statistics", func() {
				stats, err := emptyAnalyzer.GetCacheStats()
				Expect(err).NotTo(HaveOccurred())
				
				Expect(stats.TotalNodes).To(Equal(0))
				Expect(stats.CachedFiles).To(Equal(0))
			})
		})
	})

	Describe("GetWorkingDirectory", func() {
		It("should return the correct working directory", func() {
			workingDir := analyzer.GetWorkingDirectory()
			Expect(workingDir).To(Equal(tmpDir))
		})
	})

	Describe("GetCache", func() {
		It("should return the underlying cache", func() {
			cache := analyzer.GetCache()
			Expect(cache).NotTo(BeNil())
			Expect(cache).To(Equal(astCache))
		})
	})
})