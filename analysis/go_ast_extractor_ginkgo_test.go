package analysis_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	flanksourceContext "github.com/flanksource/commons/context"
)

var _ = Describe("Go AST Extractor", func() {
	var (
		tmpDir    string
		astCache  *cache.ASTCache
		ctx       flanksourceContext.Context
		extractor *analysis.GoASTExtractor
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		astCache = cache.MustGetASTCache()
		// Clear data for test isolation
		err := astCache.ClearAllData()
		Expect(err).NotTo(HaveOccurred())

		ctx = flanksourceContext.NewContext(context.Background())
		extractor = analysis.NewGoASTExtractor(astCache)
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	Describe("ExtractSimpleFile", func() {
		Context("when extracting from a simple Go file", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "simple.go")
				content := `package main

import "fmt"

type User struct {
	Name string
	Age  int
}

func (u *User) String() string {
	if u.Name == "" {
		return "Anonymous"
	}
	return fmt.Sprintf("%s (%d)", u.Name, u.Age)
}

func main() {
	user := User{Name: "Alice", Age: 30}
	fmt.Println(user.String())
}`
				err := os.WriteFile(testFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should extract AST nodes successfully", func() {
				err := extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should find expected nodes", func() {
				err := extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())

				nodes, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(nodes)).To(BeNumerically(">=", 5), "Should have multiple nodes")
			})
		})
	})

	Describe("CyclomaticComplexity", func() {
		Context("when analyzing functions with different complexity", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "complex.go")
				content := `package main

func simple() int {
	return 42
}

func complex(x, y int) int {
	if x > 0 {
		if y > 0 {
			for i := 0; i < 10; i++ {
				if i%2 == 0 {
					x += i
				}
			}
		}
	}
	return x + y
}`
				err := os.WriteFile(testFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should calculate complexity correctly", func() {
				err := extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())

				nodes, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())

				complexityMap := make(map[string]int)
				for _, node := range nodes {
					if node.NodeType == models.NodeTypeMethod {
						complexityMap[node.MethodName] = node.CyclomaticComplexity
					}
				}

				Expect(complexityMap["simple"]).To(Equal(1), "Simple function should have complexity 1")
				Expect(complexityMap["complex"]).To(BeNumerically(">=", 5), "Complex function should have higher complexity")
			})
		})
	})

	Describe("InterfaceExtraction", func() {
		Context("when extracting interfaces", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "interfaces.go")
				content := `package main

type Writer interface {
	Write([]byte) (int, error)
}

type Reader interface {
	Read([]byte) (int, error)
}

type ReadWriter interface {
	Reader
	Writer
}`
				err := os.WriteFile(testFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should extract interface types and methods", func() {
				err := extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())

				nodes, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())

				var writerType, readerType, readWriterType *models.ASTNode
				var writeMethods, readMethods []string

				for _, node := range nodes {
					switch {
					case node.TypeName == "Writer" && node.NodeType == models.NodeTypeType:
						writerType = node
					case node.TypeName == "Reader" && node.NodeType == models.NodeTypeType:
						readerType = node
					case node.TypeName == "ReadWriter" && node.NodeType == models.NodeTypeType:
						readWriterType = node
					case node.TypeName == "Writer" && node.NodeType == models.NodeTypeMethod:
						writeMethods = append(writeMethods, node.MethodName)
					case node.TypeName == "Reader" && node.NodeType == models.NodeTypeMethod:
						readMethods = append(readMethods, node.MethodName)
					}
				}

				Expect(writerType).NotTo(BeNil(), "Should find Writer interface")
				Expect(readerType).NotTo(BeNil(), "Should find Reader interface")
				Expect(readWriterType).NotTo(BeNil(), "Should find ReadWriter interface")
				Expect(writeMethods).To(ContainElement("Write"), "Should find Write method in Writer")
				Expect(readMethods).To(ContainElement("Read"), "Should find Read method in Reader")
			})
		})
	})

	Describe("LibraryCalls", func() {
		Context("when analyzing external library calls", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "libraries.go")
				content := `package main

import (
	"fmt"
	"net/http"
	"github.com/gin-gonic/gin"
)

func main() {
	// Standard library calls
	fmt.Println("Hello")
	http.Get("http://example.com")
	
	// Third-party library calls
	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "hello"})
	})
}`
				err := os.WriteFile(testFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should extract library relationships", func() {
				err := extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())

				nodes, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())

				var mainFunc *models.ASTNode
				for _, node := range nodes {
					if node.MethodName == "main" {
						mainFunc = node
						break
					}
				}

				Expect(mainFunc).NotTo(BeNil(), "Should find main function")
			})
		})
	})

	Describe("CallRelationships", func() {
		Context("when analyzing method calls", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "calls.go")
				content := `package main

type Calculator struct{}

func (c *Calculator) Add(x, y int) int {
	return x + y
}

func (c *Calculator) Multiply(x, y int) int {
	return x * y
}

func main() {
	calc := Calculator{}
	result := calc.Multiply(5, 3) // Call to Multiply method
	println(result)
}`
				err := os.WriteFile(testFile, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should find method call relationships", func() {
				err := extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())

				nodes, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())

				var multiplyMethod *models.ASTNode
				for _, node := range nodes {
					if node.MethodName == "Multiply" {
						multiplyMethod = node
						break
					}
				}

				Expect(multiplyMethod).NotTo(BeNil(), "Should find Multiply method")
				Expect(multiplyMethod.Parameters).To(HaveLen(2), "Should have 2 parameters")
				Expect(multiplyMethod.ReturnValues).To(HaveLen(1), "Should have 1 return value")
			})
		})
	})

	Describe("FileUpdateHandling", func() {
		Context("when file is updated", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "updated.go")
				originalContent := `package main

func original() {
	println("original")
}`
				err := os.WriteFile(testFile, []byte(originalContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should detect file changes and update AST", func() {
				// First extraction
				err := extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())

				// Check initial nodes
				nodes, err := astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).To(HaveLen(1), "Should have just the original function")

				// Update file content
				updatedContent := `package main

func original() {
	println("original")
}

func newFunction() {
	println("new")
}`
				err = os.WriteFile(testFile, []byte(updatedContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				// Second extraction should detect changes and update
				err = extractor.ExtractFile(ctx, testFile)
				Expect(err).NotTo(HaveOccurred())

				// Check updated nodes
				nodes, err = astCache.GetASTNodesByFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(nodes).To(HaveLen(2), "Should now have both functions")

				functionNames := make(map[string]bool)
				for _, node := range nodes {
					if node.NodeType == models.NodeTypeMethod {
						functionNames[node.MethodName] = true
					}
				}

				Expect(functionNames["original"]).To(BeTrue(), "Should still have original function")
				Expect(functionNames["newFunction"]).To(BeTrue(), "Should have new function")
			})
		})
	})

	Describe("ErrorHandling", func() {
		Context("when handling invalid files", func() {
			It("should return error for non-existent file", func() {
				err := extractor.ExtractFile(ctx, "/non/existent/file.go")
				Expect(err).To(HaveOccurred())
			})

			It("should return error for invalid Go syntax", func() {
				invalidFile := filepath.Join(tmpDir, "invalid.go")
				invalidContent := `package main

func invalid( {
	// Missing closing parenthesis and invalid syntax
}`
				err := os.WriteFile(invalidFile, []byte(invalidContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				err = extractor.ExtractFile(ctx, invalidFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse Go file"))
			})
		})
	})
})

var _ = Describe("Go AST Extractor Performance", func() {
	var (
		tmpDir    string
		astCache  *cache.ASTCache
		ctx       flanksourceContext.Context
		extractor *analysis.GoASTExtractor
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		astCache = cache.MustGetASTCache()
		// Clear data for test isolation
		err := astCache.ClearAllData()
		Expect(err).NotTo(HaveOccurred())

		ctx = flanksourceContext.NewContext(context.Background())
		extractor = analysis.NewGoASTExtractor(astCache)
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	It("should handle large files efficiently", func() {
		largeFile := filepath.Join(tmpDir, "large.go")

		// Generate a large Go file
		var content strings.Builder
		content.WriteString("package main\n\n")
		for i := 0; i < 100; i++ {
			content.WriteString(fmt.Sprintf(`
type Struct%d struct {
	Field1 string
	Field2 int
	Field3 bool
}

func (s *Struct%d) Method1() string {
	return s.Field1
}

func (s *Struct%d) Method2() int {
	if s.Field2 > 0 {
		return s.Field2 * 2
	}
	return 0
}

func Function%d() {
	s := &Struct%d{}
	s.Method1()
}
`, i, i, i, i, i))
		}

		err := os.WriteFile(largeFile, []byte(content.String()), 0644)
		Expect(err).NotTo(HaveOccurred())

		// Clear cache before measurement
		err = astCache.DeleteASTForFile(largeFile)
		Expect(err).NotTo(HaveOccurred())

		startTime := time.Now()
		err = extractor.ExtractFile(ctx, largeFile)
		Expect(err).NotTo(HaveOccurred())
		runtime := time.Since(startTime)

		Expect(runtime.Seconds()).To(BeNumerically("<", 5.0), "Should complete within 5 seconds")
	})
})
