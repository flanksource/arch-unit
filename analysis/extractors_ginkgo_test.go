package analysis_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("AST Extractors", func() {
	var (
		tmpDir   string
		astCache *cache.ASTCache
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()

		astCache = cache.MustGetASTCache()
	})

	AfterEach(func() {
		// AST cache is now a singleton, no need to close
	})

	Describe("Python AST Extractor", func() {
		var extractor *analysis.PythonASTExtractor

		BeforeEach(func() {
			extractor = analysis.NewPythonASTExtractor()
		})

		Context("when extracting from a Python file with classes and methods", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "test.py")
				pythonCode := `
class Calculator:
    def __init__(self):
        self.value = 0
    
    def add(self, x):
        if x > 0:
            self.value += x
        else:
            self.value -= abs(x)
        return self.value
    
    def multiply(self, x, y):
        result = x * y
        for i in range(y):
            if i % 2 == 0:
                result += i
        return result

def main():
    calc = Calculator()
    calc.add(10)
    print(calc.multiply(5, 3))

if __name__ == "__main__":
    main()
`
				err := os.WriteFile(testFile, []byte(pythonCode), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should successfully extract AST nodes", func() {
				content, err := os.ReadFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				
				result, err := extractor.ExtractFile(astCache, testFile, content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Nodes).NotTo(BeEmpty())
			})

			It("should find expected classes, methods, and functions", func() {
				content, err := os.ReadFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				
				result, err := extractor.ExtractFile(astCache, testFile, content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Nodes).NotTo(BeEmpty())

				nodes := result.Nodes

				var foundClass, foundInit, foundAdd, foundMultiply, foundMain bool
				for _, node := range nodes {
					switch {
					case node.TypeName == "Calculator" && node.NodeType == "type":
						foundClass = true
					case node.MethodName == "__init__" && node.TypeName == "Calculator":
						foundInit = true
					case node.MethodName == "add" && node.TypeName == "Calculator":
						foundAdd = true
						Expect(node.CyclomaticComplexity).To(BeNumerically(">", 1), "add method should have complexity > 1")
					case node.MethodName == "multiply" && node.TypeName == "Calculator":
						foundMultiply = true
						Expect(node.CyclomaticComplexity).To(BeNumerically(">", 2), "multiply method should have complexity > 2")
					case node.MethodName == "main":
						foundMain = true
					}
				}

				Expect(foundClass).To(BeTrue(), "Should find Calculator class")
				Expect(foundInit).To(BeTrue(), "Should find __init__ method")
				Expect(foundAdd).To(BeTrue(), "Should find add method")
				Expect(foundMultiply).To(BeTrue(), "Should find multiply method")
				Expect(foundMain).To(BeTrue(), "Should find main function")
			})
		})
	})

	Describe("JavaScript AST Extractor", func() {
		var extractor *analysis.JavaScriptASTExtractor

		BeforeEach(func() {
			extractor = analysis.NewJavaScriptASTExtractor()
		})

		Context("when node is available", func() {
			BeforeEach(func() {
				// Skip if node is not installed
				if _, err := os.Stat("/usr/bin/node"); os.IsNotExist(err) {
					if _, err := os.Stat("/usr/local/bin/node"); os.IsNotExist(err) {
						Skip("Node.js not installed")
					}
				}
			})

			Context("when extracting from a JavaScript file", func() {
				var testFile string

				BeforeEach(func() {
					testFile = filepath.Join(tmpDir, "test.js")
					jsCode := `
class UserService {
    constructor(database) {
        this.db = database;
        this.cache = new Map();
    }
    
    async getUser(id) {
        if (this.cache.has(id)) {
            return this.cache.get(id);
        }
        
        const user = await this.db.findUser(id);
        if (user) {
            this.cache.set(id, user);
        }
        return user;
    }
    
    createUser(name, email) {
        if (!name || !email) {
            throw new Error("Name and email required");
        }
        
        const user = {
            id: Date.now(),
            name: name,
            email: email
        };
        
        this.db.saveUser(user);
        return user;
    }
}

function validateEmail(email) {
    const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return re.test(email);
}

const service = new UserService();
export default service;
`
					err := os.WriteFile(testFile, []byte(jsCode), 0644)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should successfully extract JavaScript nodes or skip gracefully", func() {
					content, err := os.ReadFile(testFile)
					Expect(err).NotTo(HaveOccurred())
					
					result, err := extractor.ExtractFile(astCache, testFile, content)
					if err != nil {
						// If acorn is not installed globally, skip
						if strings.Contains(err.Error(), "acorn") {
							Skip("JavaScript extraction failed (likely missing acorn): " + err.Error())
						}
						Fail("Unexpected error: " + err.Error())
					}

					Expect(result).NotTo(BeNil())
					Expect(result.Nodes).NotTo(BeEmpty(), "Should extract JavaScript nodes")
				})
			})
		})
	})

	Describe("TypeScript AST Extractor", func() {
		var extractor *analysis.TypeScriptASTExtractor

		BeforeEach(func() {
			extractor = analysis.NewTypeScriptASTExtractor()
		})

		Context("when node and typescript are available", func() {
			BeforeEach(func() {
				// Skip if node or typescript is not installed
				if _, err := os.Stat("/usr/bin/node"); os.IsNotExist(err) {
					if _, err := os.Stat("/usr/local/bin/node"); os.IsNotExist(err) {
						Skip("Node.js not installed")
					}
				}
			})

			Context("when extracting from a TypeScript file", func() {
				var testFile string

				BeforeEach(func() {
					testFile = filepath.Join(tmpDir, "test.ts")
					tsCode := `
interface User {
    id: number;
    name: string;
    email: string;
    roles: string[];
}

class UserRepository {
    private users: Map<number, User>;
    
    constructor() {
        this.users = new Map();
    }
    
    async findById(id: number): Promise<User | null> {
        const user = this.users.get(id);
        return user || null;
    }
    
    save(user: User): void {
        if (!user.id) {
            throw new Error("User must have an ID");
        }
        this.users.set(user.id, user);
    }
    
    findByRole(role: string): User[] {
        const results: User[] = [];
        for (const user of this.users.values()) {
            if (user.roles.includes(role)) {
                results.push(user);
            }
        }
        return results;
    }
}

enum UserRole {
    Admin = "ADMIN",
    User = "USER",
    Guest = "GUEST"
}

type UserWithTimestamp = User & {
    createdAt: Date;
    updatedAt: Date;
};

export { UserRepository, UserRole, UserWithTimestamp };
`
					err := os.WriteFile(testFile, []byte(tsCode), 0644)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should successfully extract TypeScript nodes or skip gracefully", func() {
					content, err := os.ReadFile(testFile)
					Expect(err).NotTo(HaveOccurred())
					
					result, err := extractor.ExtractFile(astCache, testFile, content)
					if err != nil {
						// If typescript is not installed globally, skip
						if strings.Contains(err.Error(), "typescript") {
							Skip("TypeScript extraction failed (likely missing typescript): " + err.Error())
						}
						Fail("Unexpected error: " + err.Error())
					}

					Expect(result).NotTo(BeNil())
					Expect(result.Nodes).NotTo(BeEmpty(), "Should extract TypeScript nodes")
				})
			})
		})
	})

	Describe("Markdown AST Extractor", func() {
		var extractor *analysis.MarkdownASTExtractor

		BeforeEach(func() {
			extractor = analysis.NewMarkdownASTExtractor()
		})

		Context("when extracting from a Markdown file", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "README.md")
				mdContent := `# Project Title

This is a test project for AST extraction.

## Installation

Install the dependencies:

` + "```bash" + `
npm install
pip install -r requirements.txt
` + "```" + `

## Usage

### Python Example

Here's how to use the Python module:

` + "```python" + `
import calculator

calc = calculator.Calculator()
result = calc.add(10, 20)
print(result)
` + "```" + `

### JavaScript Example

And here's the JavaScript version:

` + "```javascript" + `
const Calculator = require('./calculator');

const calc = new Calculator();
const result = calc.add(10, 20);
console.log(result);
` + "```" + `

## API Reference

See [API Documentation](https://example.com/api) for details.

### Methods

- ` + "`add(a, b)`" + ` - Adds two numbers
- ` + "`multiply(a, b)`" + ` - Multiplies two numbers

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.
`
				err := os.WriteFile(testFile, []byte(mdContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should successfully extract AST nodes", func() {
				content, err := os.ReadFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				
				result, err := extractor.ExtractFile(astCache, testFile, content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Nodes).NotTo(BeEmpty())
			})

			It("should find expected document structure and code blocks", func() {
				content, err := os.ReadFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				
				result, err := extractor.ExtractFile(astCache, testFile, content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Nodes).NotTo(BeEmpty())

				nodes := result.Nodes

				var foundPackage, foundInstallation, foundUsage, foundAPI bool
				var codeBlockCount int

				for _, node := range nodes {
					// Check code blocks first (before checking sections)
					if node.NodeType == "method" && strings.HasPrefix(node.MethodName, "code_") {
						codeBlockCount++
					}

					switch {
					case node.NodeType == "package":
						foundPackage = true
					case node.TypeName == "Installation":
						foundInstallation = true
					case node.TypeName == "Usage":
						foundUsage = true
					case node.TypeName == "API Reference":
						foundAPI = true
					}
				}

				Expect(foundPackage).To(BeTrue(), "Should find document package node")
				Expect(foundInstallation).To(BeTrue(), "Should find Installation section")
				Expect(foundUsage).To(BeTrue(), "Should find Usage section")
				Expect(foundAPI).To(BeTrue(), "Should find API Reference section")
				Expect(codeBlockCount).To(Equal(3), "Should find 3 code blocks")
			})
		})
	})

	Describe("Go AST Extractor", func() {
		var extractor *analysis.GoASTExtractor

		BeforeEach(func() {
			extractor = analysis.NewGoASTExtractor()
		})

		Context("when extracting from a Go file", func() {
			var testFile string

			BeforeEach(func() {
				testFile = filepath.Join(tmpDir, "test.go")
				goCode := `package main

import (
	"fmt"
	"errors"
)

type Calculator struct {
	value int
}

func (c *Calculator) Add(x int) error {
	if x < 0 {
		return errors.New("negative values not allowed")
	}
	c.value += x
	return nil
}

func (c *Calculator) Multiply(x, y int) int {
	result := x * y
	for i := 0; i < y; i++ {
		if i%2 == 0 {
			result += i
		}
	}
	return result
}

func main() {
	calc := &Calculator{}
	err := calc.Add(10)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	
	result := calc.Multiply(5, 3)
	fmt.Printf("Result: %d\n", result)
}
`
				err := os.WriteFile(testFile, []byte(goCode), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should successfully extract AST nodes", func() {
				content, err := os.ReadFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				
				result, err := extractor.ExtractFile(astCache, testFile, content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Nodes).NotTo(BeEmpty())
			})

			It("should find expected structs, methods, and functions", func() {
				content, err := os.ReadFile(testFile)
				Expect(err).NotTo(HaveOccurred())
				
				result, err := extractor.ExtractFile(astCache, testFile, content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Nodes).NotTo(BeEmpty())

				nodes := result.Nodes

				var foundStruct, foundAdd, foundMultiply, foundMain bool
				for _, node := range nodes {
					switch {
					case node.TypeName == "Calculator" && node.NodeType == "type":
						foundStruct = true
					case node.MethodName == "Add" && node.TypeName == "Calculator":
						foundAdd = true
						Expect(node.CyclomaticComplexity).To(BeNumerically(">", 1), "Add method should have complexity > 1")
					case node.MethodName == "Multiply" && node.TypeName == "Calculator":
						foundMultiply = true
						Expect(node.CyclomaticComplexity).To(BeNumerically(">", 2), "Multiply method should have complexity > 2")
					case node.MethodName == "main":
						foundMain = true
					}
				}

				Expect(foundStruct).To(BeTrue(), "Should find Calculator struct")
				Expect(foundAdd).To(BeTrue(), "Should find Add method")
				Expect(foundMultiply).To(BeTrue(), "Should find Multiply method")
				Expect(foundMain).To(BeTrue(), "Should find main function")
			})
		})
	})
})
