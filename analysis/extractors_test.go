package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonASTExtractor(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.py")

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
	require.NoError(t, err)

	// Create AST cache
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Create Python extractor
	extractor := NewPythonASTExtractor(astCache)

	// Extract AST
	err = extractor.ExtractFile(testFile)
	assert.NoError(t, err)

	// Verify nodes were extracted
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	// Check we found the expected elements
	var foundClass, foundInit, foundAdd, foundMultiply, foundMain bool
	for _, node := range nodes {
		switch {
		case node.TypeName == "Calculator" && node.NodeType == "type":
			foundClass = true
		case node.MethodName == "__init__" && node.TypeName == "Calculator":
			foundInit = true
		case node.MethodName == "add" && node.TypeName == "Calculator":
			foundAdd = true
			// Check complexity was calculated
			assert.Greater(t, node.CyclomaticComplexity, 1, "add method should have complexity > 1")
		case node.MethodName == "multiply" && node.TypeName == "Calculator":
			foundMultiply = true
			// Check complexity was calculated
			assert.Greater(t, node.CyclomaticComplexity, 2, "multiply method should have complexity > 2")
		case node.MethodName == "main":
			foundMain = true
		}
	}

	assert.True(t, foundClass, "Should find Calculator class")
	assert.True(t, foundInit, "Should find __init__ method")
	assert.True(t, foundAdd, "Should find add method")
	assert.True(t, foundMultiply, "Should find multiply method")
	assert.True(t, foundMain, "Should find main function")
}

func TestJavaScriptASTExtractor(t *testing.T) {
	// Skip if node is not installed
	if _, err := os.Stat("/usr/bin/node"); os.IsNotExist(err) {
		if _, err := os.Stat("/usr/local/bin/node"); os.IsNotExist(err) {
			t.Skip("Node.js not installed")
		}
	}

	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.js")

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
	require.NoError(t, err)

	// Create AST cache
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Create JavaScript extractor
	extractor := NewJavaScriptASTExtractor(astCache)

	// Extract AST
	err = extractor.ExtractFile(testFile)
	// Note: This will fail if acorn is not installed globally
	// For actual testing, we'd need to ensure dependencies are available
	if err != nil {
		t.Skipf("JavaScript extraction failed (likely missing acorn): %v", err)
	}

	// Verify nodes were extracted
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	assert.NotEmpty(t, nodes, "Should extract JavaScript nodes")
}

func TestTypeScriptASTExtractor(t *testing.T) {
	// Skip if node or typescript is not installed
	if _, err := os.Stat("/usr/bin/node"); os.IsNotExist(err) {
		if _, err := os.Stat("/usr/local/bin/node"); os.IsNotExist(err) {
			t.Skip("Node.js not installed")
		}
	}

	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ts")

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
	require.NoError(t, err)

	// Create AST cache
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Create TypeScript extractor
	extractor := NewTypeScriptASTExtractor(astCache)

	// Extract AST
	err = extractor.ExtractFile(testFile)
	// Note: This will fail if typescript is not installed globally
	if err != nil {
		t.Skipf("TypeScript extraction failed (likely missing typescript): %v", err)
	}

	// Verify nodes were extracted
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	assert.NotEmpty(t, nodes, "Should extract TypeScript nodes")
}

func TestMarkdownASTExtractor(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "README.md")

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
	require.NoError(t, err)

	// Create AST cache
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Create Markdown extractor
	extractor := NewMarkdownASTExtractor(astCache)

	// Extract AST
	err = extractor.ExtractFile(testFile)
	assert.NoError(t, err)

	// Verify nodes were extracted
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	// Check we found the expected structure
	var foundPackage, foundInstallation, foundUsage, foundAPI bool
	var codeBlockCount int

	for _, node := range nodes {
		t.Logf("Node: Type=%s, TypeName=%s, MethodName=%s", node.NodeType, node.TypeName, node.MethodName)

		// Check code blocks first (before checking sections)
		if node.NodeType == "method" && strings.HasPrefix(node.MethodName, "code_") {
			codeBlockCount++
			t.Logf("Found code block: %s, TypeName=%s (lines %d-%d)", node.MethodName, node.TypeName, node.StartLine, node.EndLine)
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

	assert.True(t, foundPackage, "Should find document package node")
	assert.True(t, foundInstallation, "Should find Installation section")
	assert.True(t, foundUsage, "Should find Usage section")
	assert.True(t, foundAPI, "Should find API Reference section")
	assert.Equal(t, 3, codeBlockCount, "Should find 3 code blocks")
}

func TestGoASTExtractor(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

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
	require.NoError(t, err)

	// Create AST cache
	cacheDir := t.TempDir()
	astCache, err := cache.NewASTCacheWithPath(cacheDir)
	require.NoError(t, err)
	defer astCache.Close()

	// Create Go extractor
	extractor := NewGoASTExtractor(astCache)

	// Extract AST
	err = extractor.ExtractFile(testFile)
	assert.NoError(t, err)

	// Verify nodes were extracted
	nodes, err := astCache.GetASTNodesByFile(testFile)
	require.NoError(t, err)

	// Check we found the expected elements
	var foundStruct, foundAdd, foundMultiply, foundMain bool
	for _, node := range nodes {
		switch {
		case node.TypeName == "Calculator" && node.NodeType == "type":
			foundStruct = true
		case node.MethodName == "Add" && node.TypeName == "Calculator":
			foundAdd = true
			assert.Greater(t, node.CyclomaticComplexity, 1, "Add method should have complexity > 1")
		case node.MethodName == "Multiply" && node.TypeName == "Calculator":
			foundMultiply = true
			assert.Greater(t, node.CyclomaticComplexity, 2, "Multiply method should have complexity > 2")
		case node.MethodName == "main":
			foundMain = true
		}
	}

	assert.True(t, foundStruct, "Should find Calculator struct")
	assert.True(t, foundAdd, "Should find Add method")
	assert.True(t, foundMultiply, "Should find Multiply method")
	assert.True(t, foundMain, "Should find main function")
}
