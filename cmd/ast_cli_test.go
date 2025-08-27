package cmd

import (
	"bytes"
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AST Command", func() {
	var output *bytes.Buffer
	var originalDir string
	var tempDir string

	BeforeEach(func() {
		output = &bytes.Buffer{}
		astCmd.SetOut(output)
		astCmd.SetErr(output)

		var err error
		originalDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
			tempDir = ""
		}
		os.Chdir(originalDir)
	})

	Context("Help", func() {
		It("should display help message", func() {
			astCmd.SetArgs([]string{"--help"})
			_ = astCmd.Execute() // Ignore error from help command

			helpOutput := output.String()
			Expect(helpOutput).To(ContainSubstring("Analyze and inspect AST"))
			Expect(helpOutput).To(ContainSubstring("--format"))
			Expect(helpOutput).To(ContainSubstring("--complexity"))
			Expect(helpOutput).To(ContainSubstring("--calls"))
			Expect(helpOutput).To(ContainSubstring("--libraries"))
			Expect(helpOutput).To(ContainSubstring("--query"))
		})
	})

	Context("with an empty directory", func() {
		It("should report no Go files found", func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())

			Expect(output.String()).To(ContainSubstring("No Go files found"))
		})
	})

	Context("with a simple Go file", func() {
		It("should produce a tree output", func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

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
	user := &User{Name: "Alice", Age: 30}
	fmt.Println(user.String())
}`
			err = os.WriteFile("simple.go", []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			origFormat := astFormat
			origCachedOnly := astCachedOnly
			defer func() {
				astFormat = origFormat
				astCachedOnly = origCachedOnly
			}()
			astFormat = "tree"
			astCachedOnly = false

			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err = runAST(astCmd, []string{})
			Expect(err).NotTo(HaveOccurred())

			w.Close()
			os.Stdout = oldStdout

			out, _ := io.ReadAll(r)
			outputStr := string(out)

			Expect(outputStr).To(ContainSubstring("User"))
			Expect(outputStr).To(ContainSubstring("String"))
			Expect(outputStr).To(ContainSubstring("main"))
		})
	})

	Context("Pattern Matching", func() {
		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			controllerContent := `package main

type UserController struct{}

func (c *UserController) GetUser(id string) string { return "user-" + id }

func (c *UserController) CreateUser(name, email string) string { return name + ":" + email }`
			err = os.WriteFile("controller.go", []byte(controllerContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			serviceContent := `package main

type UserService struct{}

func (s *UserService) ProcessUser(data string) string { return "processed-" + data }`
			err = os.WriteFile("service.go", []byte(serviceContent), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("should match patterns",
			func(pattern string, expected []string, format string) {
				output.Reset()
				args := []string{pattern}
				if format != "" {
					args = append(args, "--format", format)
				}
				astCmd.SetArgs(args)

				err := astCmd.Execute()
				Expect(err).NotTo(HaveOccurred())

				outputStr := output.String()
				for _, exp := range expected {
					Expect(outputStr).To(ContainSubstring(exp))
				}
			},
			Entry("Controller pattern", "*Controller*", []string{"UserController", "GetUser", "CreateUser"}, "tree"),
			Entry("Service pattern", "*Service*", []string{"UserService", "ProcessUser"}, "tree"),
			Entry("All pattern", "*", []string{"UserController", "UserService", "GetUser", "CreateUser", "ProcessUser"}, "table"),
		)
	})

	Context("Complexity Analysis", func() {
		It("should filter by complexity threshold", func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			content := `package main

func simple() { println("hello") }

func complex(x, y, z int) int {
	result := 0
	if x > 0 {
		for i := 0; i < x; i++ {
			if i%2 == 0 {
				switch y {
				case 1: result += i
				case 2: result += i * 2
				case 3:
					if z > 0 { result += i * z } else { result -= i }
				default: result += i * 3
				}
			} else { result += i }
		}
	} else if x < 0 { result = -1 }
	return result
}`
			err = os.WriteFile("complex.go", []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{"*", "--complexity", "--threshold", "5"})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())

			outputStr := output.String()
			Expect(outputStr).To(ContainSubstring("complex"))
			Expect(outputStr).To(ContainSubstring("(c:"))
			Expect(outputStr).NotTo(ContainSubstring("simple"))
		})
	})

	Context("JSON Output", func() {
		It("should produce valid JSON", func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			content := `package main

type TestStruct struct {
	Field1 string
	Field2 int
}

func (t *TestStruct) Method1() string { return t.Field1 }

func SimpleFunction() int { return 42 }`
			err = os.WriteFile("test.go", []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{"*", "--format", "json"})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())

			outputStr := output.String()
			Expect(outputStr).To(ContainSubstring(`"file_path"`))
			Expect(outputStr).To(ContainSubstring(`"package_name"`))
			Expect(outputStr).To(ContainSubstring(`"type_name"`))
			Expect(outputStr).To(ContainSubstring(`"method_name"`))
			Expect(outputStr).To(ContainSubstring(`"node_type"`))
			Expect(outputStr).To(ContainSubstring(`"cyclomatic_complexity"`))
			Expect(outputStr).To(ContainSubstring("TestStruct"))
			Expect(outputStr).To(ContainSubstring("Method1"))
			Expect(outputStr).To(ContainSubstring("SimpleFunction"))
		})
	})

	Context("AQL Query", func() {
		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			content := `package main

func simple() { println("simple") }

func complex(a, b, c, d int) int {
	result := 0
	for i := 0; i < a; i++ {
		if i%2 == 0 {
			switch b {
			case 1: result += i
			case 2:
				if c > 0 { result += i * c }
			default: result += i * d
			}
		}
	}
	return result
}`
			err = os.WriteFile("querytest.go", []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable("should execute AQL queries",
			func(query string, expected []string) {
				output.Reset()
				astCmd.SetArgs([]string{"--query", query})
				err := astCmd.Execute()
				Expect(err).NotTo(HaveOccurred())

				outputStr := output.String()
				Expect(outputStr).To(ContainSubstring("AQL Query: " + query))
				for _, exp := range expected {
					Expect(outputStr).To(ContainSubstring(exp))
				}
			},
			Entry("high complexity", "*.cyclomatic > 5", []string{"complex"}),
			Entry("many parameters", "*.params > 3", []string{"complex"}),
			Entry("exact complexity", "*.cyclomatic == 1", []string{"simple"}),
		)
	})

	Context("Cached Only", func() {
		It("should use cache and not re-analyze", func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			content := `package main
func testFunction() { println("test") }`
			err = os.WriteFile("cached.go", []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			// First, run normal analysis to populate cache
			astCmd.SetArgs([]string{})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).To(ContainSubstring("Analyzing"))
			Expect(output.String()).To(ContainSubstring("method"))

			// Now run with cached-only flag
			output.Reset()
			astCmd.SetArgs([]string{"--cached-only"})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).NotTo(ContainSubstring("Analyzing"))
			Expect(output.String()).To(ContainSubstring("AST Overview"))
		})
	})

	Context("Table Format", func() {
		It("should display results in a table", func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			content := `package main
type Calculator struct { value int }
func (c *Calculator) Add(x int) { c.value += x }
func (c *Calculator) Multiply(x, y int) int { return x * y }
func SimpleAdd(a, b int) int { return a + b }`
			err = os.WriteFile("table.go", []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{"*", "--format", "table"})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())

			outputStr := output.String()
			Expect(outputStr).To(ContainSubstring("File"))
			Expect(outputStr).To(ContainSubstring("Package"))
			Expect(outputStr).To(ContainSubstring("Type"))
			Expect(outputStr).To(ContainSubstring("Method"))
			Expect(outputStr).To(ContainSubstring("Complexity"))
			Expect(outputStr).To(ContainSubstring("Lines"))
			Expect(outputStr).To(ContainSubstring("Calculator"))
			Expect(outputStr).To(ContainSubstring("Add"))
			Expect(outputStr).To(ContainSubstring("Multiply"))
			Expect(outputStr).To(ContainSubstring("SimpleAdd"))
			Expect(outputStr).To(ContainSubstring("table.go"))
			Expect(outputStr).NotTo(ContainSubstring(tempDir))
		})
	})

	Context("Tree Format Grouping", func() {
		It("should group results by file", func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			controllerContent := `package controller
type UserController struct{}
func (c *UserController) GetUser(id string) string { return "user" }
func (c *UserController) DeleteUser(id string) error { return nil }`
			err = os.WriteFile("user_controller.go", []byte(controllerContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			modelContent := `package model
type User struct { ID string; Name string; Age int }
func (u *User) String() string { return u.Name }`
			err = os.WriteFile("user_model.go", []byte(modelContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{"*", "--format", "tree"})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())

			outputStr := output.String()
			Expect(outputStr).To(ContainSubstring("📁 user_controller.go"))
			Expect(outputStr).To(ContainSubstring("📁 user_model.go"))
			Expect(outputStr).To(ContainSubstring("🏗️  UserController"))
			Expect(outputStr).To(ContainSubstring("🏗️  User"))
			Expect(outputStr).To(ContainSubstring("⚡ method:"))
			Expect(outputStr).To(ContainSubstring("📊 field:"))
			Expect(outputStr).To(ContainSubstring("GetUser"))
			Expect(outputStr).To(ContainSubstring("DeleteUser"))
			Expect(outputStr).To(ContainSubstring("String"))
			Expect(outputStr).To(ContainSubstring("ID"))
			Expect(outputStr).To(ContainSubstring("Name"))
			Expect(outputStr).To(ContainSubstring("Age"))
		})
	})

	Context("Template Format", func() {
		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should format output using a simple template", func() {
			goContent := `package main
import "fmt"
func main() { fmt.Println("Hello World") }
func calculate(a, b int) int { return a + b }`
			err := os.WriteFile("main.go", []byte(goContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{"*", "--format", "template", "--template", "{{.Package}}.{{.Method}} ({{.Lines}} SLOC)"})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())

			result := output.String()
			Expect(result).To(ContainSubstring("main.main"))
			Expect(result).To(ContainSubstring("main.calculate"))
			Expect(result).To(ContainSubstring("SLOC"))
		})

		It("should format output using a complex template", func() {
			goContent := `package service
type UserService struct { id int }
func (u *UserService) GetUser(id int) string { return "user" }`
			err := os.WriteFile("service.go", []byte(goContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{"*", "--format", "template", "--template", "{{.NodeType}}: {{.Package}}.{{.Class}}.{{.Method}} [{{.Complexity}} complexity]"})
			err = astCmd.Execute()
			Expect(err).NotTo(HaveOccurred())

			result := output.String()
			Expect(result).To(ContainSubstring("service.UserService"))
			Expect(result).To(ContainSubstring("complexity"))
			Expect(result).To(ContainSubstring("method:"))
		})

		It("should return an error for invalid template syntax", func() {
			goContent := `package main
func main() {}`
			err := os.WriteFile("main.go", []byte(goContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			astCmd.SetArgs([]string{"*", "--format", "template", "--template", "{{.Invalid}}"})
			err = astCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template execution error"))
		})
	})

	Context("Template Validation", func() {
		DescribeTable("should validate template flags",
			func(args []string, errorMsg string) {
				astCmd.SetArgs(args)
				err := astCmd.Execute()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(errorMsg))
			},
			Entry("template format without template flag", []string{"*", "--format", "template"}, "--template flag is required when using --format template"),
			Entry("template flag without template format", []string{"*", "--format", "json", "--template", "{{.Package}}"}, "--template flag can only be used with --format template"),
		)
	})

	Context("Template Help", func() {
		It("should include template information in help output", func() {
			astCmd.SetArgs([]string{"--help"})
			_ = astCmd.Execute()

			helpOutput := output.String()
			Expect(helpOutput).To(ContainSubstring("template"))
			Expect(helpOutput).To(ContainSubstring("TEMPLATE VARIABLES"))
			Expect(helpOutput).To(ContainSubstring("{{.Package}}"))
			Expect(helpOutput).To(ContainSubstring("{{.Lines}}"))
			Expect(helpOutput).To(ContainSubstring("SLOC"))
		})
	})
})
