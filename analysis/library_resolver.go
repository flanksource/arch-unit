package analysis

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

// LibraryResolver identifies and classifies external libraries and frameworks
type LibraryResolver struct {
	cache             *cache.ASTCache
	knownLibraries    map[string]*LibraryInfo
	standardLibraries map[string]bool
}

// LibraryInfo contains metadata about a library
type LibraryInfo struct {
	Name          string
	Framework     string
	Language      string
	Version       string
	Category      string // "web", "database", "testing", "utility", etc.
	CommonTypes   []string
	CommonMethods []string
}

// NewLibraryResolver creates a new library resolver
func NewLibraryResolver(astCache *cache.ASTCache) *LibraryResolver {
	resolver := &LibraryResolver{
		cache:             astCache,
		knownLibraries:    make(map[string]*LibraryInfo),
		standardLibraries: make(map[string]bool),
	}

	resolver.initializeKnownLibraries()
	resolver.initializeGoStandardLibraries()

	return resolver
}

// initializeKnownLibraries populates the known libraries database
func (r *LibraryResolver) initializeKnownLibraries() {
	libraries := []*LibraryInfo{
		// Web Frameworks
		{
			Name: "github.com/gin-gonic/gin", Framework: "gin", Language: "go",
			Category: "web", CommonTypes: []string{"Context", "Engine", "RouterGroup"},
			CommonMethods: []string{"GET", "POST", "PUT", "DELETE", "JSON", "Bind"},
		},
		{
			Name: "github.com/labstack/echo", Framework: "echo", Language: "go",
			Category: "web", CommonTypes: []string{"Context", "Echo", "Group"},
			CommonMethods: []string{"GET", "POST", "PUT", "DELETE", "JSON", "Bind"},
		},
		{
			Name: "github.com/gorilla/mux", Framework: "gorilla", Language: "go",
			Category: "web", CommonTypes: []string{"Router", "Route"},
			CommonMethods: []string{"HandleFunc", "PathPrefix", "Subrouter"},
		},
		{
			Name: "net/http", Framework: "stdlib", Language: "go",
			Category: "web", CommonTypes: []string{"Request", "Response", "Handler"},
			CommonMethods: []string{"Get", "Post", "ListenAndServe", "HandleFunc"},
		},

		// Database
		{
			Name: "gorm.io/gorm", Framework: "gorm", Language: "go",
			Category: "database", CommonTypes: []string{"DB", "Model"},
			CommonMethods: []string{"Create", "Find", "Update", "Delete", "Where", "First"},
		},
		{
			Name: "database/sql", Framework: "stdlib", Language: "go",
			Category: "database", CommonTypes: []string{"DB", "Tx", "Row", "Rows"},
			CommonMethods: []string{"Query", "Exec", "Prepare", "Begin", "Commit"},
		},
		{
			Name: "github.com/lib/pq", Framework: "postgres", Language: "go",
			Category: "database", CommonTypes: []string{"Driver"},
			CommonMethods: []string{},
		},

		// Testing
		{
			Name: "testing", Framework: "stdlib", Language: "go",
			Category: "testing", CommonTypes: []string{"T", "B", "TB"},
			CommonMethods: []string{"Run", "Error", "Fatal", "Log", "Parallel"},
		},
		{
			Name: "github.com/stretchr/testify", Framework: "testify", Language: "go",
			Category: "testing", CommonTypes: []string{"Suite"},
			CommonMethods: []string{"Equal", "NotEqual", "True", "False", "Nil", "NotNil"},
		},

		// Logging
		{
			Name: "github.com/sirupsen/logrus", Framework: "logrus", Language: "go",
			Category: "logging", CommonTypes: []string{"Logger", "Entry"},
			CommonMethods: []string{"Info", "Debug", "Warn", "Error", "Fatal", "WithField"},
		},
		{
			Name: "go.uber.org/zap", Framework: "zap", Language: "go",
			Category: "logging", CommonTypes: []string{"Logger", "SugaredLogger"},
			CommonMethods: []string{"Info", "Debug", "Warn", "Error", "Fatal", "With"},
		},

		// JSON/Serialization
		{
			Name: "encoding/json", Framework: "stdlib", Language: "go",
			Category: "serialization", CommonTypes: []string{"Encoder", "Decoder"},
			CommonMethods: []string{"Marshal", "Unmarshal", "NewEncoder", "NewDecoder"},
		},

		// Context
		{
			Name: "context", Framework: "stdlib", Language: "go",
			Category: "concurrency", CommonTypes: []string{"Context"},
			CommonMethods: []string{"Background", "TODO", "WithCancel", "WithTimeout", "WithValue"},
		},

		// Time
		{
			Name: "time", Framework: "stdlib", Language: "go",
			Category: "utility", CommonTypes: []string{"Time", "Duration", "Timer", "Ticker"},
			CommonMethods: []string{"Now", "Since", "Parse", "Format", "Sleep"},
		},

		// Python Libraries
		{
			Name: "django", Framework: "django", Language: "python",
			Category: "web", CommonTypes: []string{"Model", "View", "HttpRequest", "HttpResponse"},
			CommonMethods: []string{"get", "post", "save", "delete", "filter"},
		},
		{
			Name: "flask", Framework: "flask", Language: "python",
			Category: "web", CommonTypes: []string{"Flask", "Blueprint", "Request", "Response"},
			CommonMethods: []string{"route", "before_request", "after_request", "jsonify"},
		},
		{
			Name: "fastapi", Framework: "fastapi", Language: "python",
			Category: "web", CommonTypes: []string{"FastAPI", "APIRouter", "Request", "Response"},
			CommonMethods: []string{"get", "post", "put", "delete"},
		},
		{
			Name: "numpy", Framework: "numpy", Language: "python",
			Category: "scientific", CommonTypes: []string{"ndarray", "dtype"},
			CommonMethods: []string{"array", "zeros", "ones", "arange", "linspace"},
		},
		{
			Name: "pandas", Framework: "pandas", Language: "python",
			Category: "data", CommonTypes: []string{"DataFrame", "Series", "Index"},
			CommonMethods: []string{"read_csv", "to_csv", "merge", "groupby", "pivot"},
		},
		{
			Name: "pytest", Framework: "pytest", Language: "python",
			Category: "testing", CommonTypes: []string{"fixture", "mark"},
			CommonMethods: []string{"raises", "approx", "skip", "parametrize"},
		},
		{
			Name: "sqlalchemy", Framework: "sqlalchemy", Language: "python",
			Category: "database", CommonTypes: []string{"Session", "Model", "Engine"},
			CommonMethods: []string{"query", "add", "commit", "rollback", "filter"},
		},
		{
			Name: "requests", Framework: "requests", Language: "python",
			Category: "http", CommonTypes: []string{"Response", "Session"},
			CommonMethods: []string{"get", "post", "put", "delete", "head"},
		},

		// JavaScript/TypeScript Libraries
		{
			Name: "react", Framework: "react", Language: "javascript",
			Category: "frontend", CommonTypes: []string{"Component", "PureComponent"},
			CommonMethods: []string{"useState", "useEffect", "useContext", "useMemo", "useCallback"},
		},
		{
			Name: "vue", Framework: "vue", Language: "javascript",
			Category: "frontend", CommonTypes: []string{"Vue", "Component"},
			CommonMethods: []string{"data", "computed", "methods", "watch", "mounted"},
		},
		{
			Name: "angular", Framework: "angular", Language: "typescript",
			Category: "frontend", CommonTypes: []string{"Component", "Service", "Module"},
			CommonMethods: []string{"ngOnInit", "ngOnDestroy", "subscribe", "pipe"},
		},
		{
			Name: "express", Framework: "express", Language: "javascript",
			Category: "web", CommonTypes: []string{"Application", "Request", "Response", "Router"},
			CommonMethods: []string{"get", "post", "put", "delete", "use", "listen"},
		},
		{
			Name: "axios", Framework: "axios", Language: "javascript",
			Category: "http", CommonTypes: []string{"AxiosInstance", "AxiosResponse"},
			CommonMethods: []string{"get", "post", "put", "delete", "request"},
		},
		{
			Name: "lodash", Framework: "lodash", Language: "javascript",
			Category: "utility", CommonTypes: []string{},
			CommonMethods: []string{"map", "filter", "reduce", "debounce", "throttle", "merge"},
		},
		{
			Name: "jest", Framework: "jest", Language: "javascript",
			Category: "testing", CommonTypes: []string{},
			CommonMethods: []string{"describe", "it", "test", "expect", "beforeEach", "afterEach"},
		},
		{
			Name: "mocha", Framework: "mocha", Language: "javascript",
			Category: "testing", CommonTypes: []string{},
			CommonMethods: []string{"describe", "it", "before", "after", "beforeEach", "afterEach"},
		},
		{
			Name: "mongodb", Framework: "mongodb", Language: "javascript",
			Category: "database", CommonTypes: []string{"MongoClient", "Db", "Collection"},
			CommonMethods: []string{"connect", "find", "insert", "update", "delete"},
		},
		{
			Name: "mongoose", Framework: "mongoose", Language: "javascript",
			Category: "database", CommonTypes: []string{"Schema", "Model", "Document"},
			CommonMethods: []string{"find", "findOne", "save", "update", "delete"},
		},
		{
			Name: "@angular/core", Framework: "angular", Language: "typescript",
			Category: "frontend", CommonTypes: []string{"Component", "Injectable", "NgModule"},
			CommonMethods: []string{"ngOnInit", "ngOnDestroy", "ngOnChanges"},
		},
		{
			Name: "@types/node", Framework: "types", Language: "typescript",
			Category: "types", CommonTypes: []string{},
			CommonMethods: []string{},
		},
		{
			Name: "rxjs", Framework: "rxjs", Language: "typescript",
			Category: "reactive", CommonTypes: []string{"Observable", "Subject", "BehaviorSubject"},
			CommonMethods: []string{"subscribe", "pipe", "map", "filter", "switchMap"},
		},
	}

	for _, lib := range libraries {
		r.knownLibraries[lib.Name] = lib
	}
}

// initializeGoStandardLibraries populates Go standard library packages
func (r *LibraryResolver) initializeGoStandardLibraries() {
	stdLibPackages := []string{
		"bufio", "bytes", "context", "crypto", "database/sql", "encoding",
		"errors", "fmt", "go", "hash", "html", "image", "io", "log", "math",
		"mime", "net", "os", "path", "reflect", "regexp", "runtime", "sort",
		"strconv", "strings", "sync", "syscall", "testing", "text", "time",
		"unicode", "unsafe",
	}

	for _, pkg := range stdLibPackages {
		r.standardLibraries[pkg] = true
	}
}

// ResolveLibrary resolves a package path to library information
func (r *LibraryResolver) ResolveLibrary(packagePath string) *LibraryInfo {
	// Direct match
	if lib, found := r.knownLibraries[packagePath]; found {
		return lib
	}

	// Check for standard library
	if r.isStandardLibrary(packagePath) {
		return &LibraryInfo{
			Name:      packagePath,
			Framework: "stdlib",
			Language:  "go",
			Category:  r.categorizeStandardLibrary(packagePath),
		}
	}

	// Try to match by prefix for versioned packages
	for knownPath, lib := range r.knownLibraries {
		if strings.HasPrefix(packagePath, knownPath) {
			// Create a copy with the actual path
			resolved := *lib
			resolved.Name = packagePath
			return &resolved
		}
	}

	// Unknown third-party library
	return &LibraryInfo{
		Name:      packagePath,
		Framework: "third-party",
		Language:  "go",
		Category:  r.guessCategory(packagePath),
	}
}

// isStandardLibrary checks if a package is part of Go standard library
func (r *LibraryResolver) isStandardLibrary(packagePath string) bool {
	// Standard library packages don't contain dots (except for some special cases)
	if !strings.Contains(packagePath, "/") {
		return true
	}

	// Some standard library packages have slashes
	parts := strings.Split(packagePath, "/")
	if len(parts) >= 1 && r.standardLibraries[parts[0]] {
		return true
	}

	return r.standardLibraries[packagePath]
}

// categorizeStandardLibrary categorizes standard library packages
func (r *LibraryResolver) categorizeStandardLibrary(packagePath string) string {
	switch {
	case strings.HasPrefix(packagePath, "net"):
		return "networking"
	case strings.HasPrefix(packagePath, "crypto"):
		return "crypto"
	case strings.HasPrefix(packagePath, "database"):
		return "database"
	case strings.HasPrefix(packagePath, "encoding"):
		return "serialization"
	case strings.HasPrefix(packagePath, "text") || strings.HasPrefix(packagePath, "html"):
		return "text"
	case packagePath == "testing":
		return "testing"
	case packagePath == "log":
		return "logging"
	case packagePath == "time":
		return "time"
	case packagePath == "context":
		return "concurrency"
	case strings.HasPrefix(packagePath, "sync"):
		return "concurrency"
	case strings.HasPrefix(packagePath, "os") || strings.HasPrefix(packagePath, "path"):
		return "filesystem"
	case strings.HasPrefix(packagePath, "io"):
		return "io"
	default:
		return "utility"
	}
}

// guessCategory attempts to guess the category of an unknown library
func (r *LibraryResolver) guessCategory(packagePath string) string {
	lowered := strings.ToLower(packagePath)

	switch {
	case strings.Contains(lowered, "gin") || strings.Contains(lowered, "echo") ||
		strings.Contains(lowered, "mux") || strings.Contains(lowered, "http") ||
		strings.Contains(lowered, "web") || strings.Contains(lowered, "rest") ||
		strings.Contains(lowered, "api"):
		return "web"
	case strings.Contains(lowered, "sql") || strings.Contains(lowered, "db") ||
		strings.Contains(lowered, "database") || strings.Contains(lowered, "orm") ||
		strings.Contains(lowered, "postgres") || strings.Contains(lowered, "mysql") ||
		strings.Contains(lowered, "mongo") || strings.Contains(lowered, "redis"):
		return "database"
	case strings.Contains(lowered, "test") || strings.Contains(lowered, "mock") ||
		strings.Contains(lowered, "assert") || strings.Contains(lowered, "spec"):
		return "testing"
	case strings.Contains(lowered, "log") || strings.Contains(lowered, "zap") ||
		strings.Contains(lowered, "logrus"):
		return "logging"
	case strings.Contains(lowered, "json") || strings.Contains(lowered, "xml") ||
		strings.Contains(lowered, "yaml") || strings.Contains(lowered, "proto"):
		return "serialization"
	case strings.Contains(lowered, "crypto") || strings.Contains(lowered, "hash") ||
		strings.Contains(lowered, "security"):
		return "crypto"
	case strings.Contains(lowered, "config") || strings.Contains(lowered, "env"):
		return "config"
	default:
		return "utility"
	}
}

// StoreLibraryNodes pre-populates the database with known libraries
func (r *LibraryResolver) StoreLibraryNodes() error {
	for _, lib := range r.knownLibraries {
		// Store main library node
		_, err := r.cache.StoreLibraryNode(lib.Name, "", "", "", models.NodeTypePackage, lib.Language, lib.Framework)
		if err != nil {
			return fmt.Errorf("failed to store library node for %s: %w", lib.Name, err)
		}

		// Store common types
		for _, typeName := range lib.CommonTypes {
			_, err := r.cache.StoreLibraryNode(lib.Name, typeName, "", "", "class", lib.Language, lib.Framework)
			if err != nil {
				return fmt.Errorf("failed to store library type %s.%s: %w", lib.Name, typeName, err)
			}
		}

		// Store common methods for each type
		for _, typeName := range lib.CommonTypes {
			for _, methodName := range lib.CommonMethods {
				_, err := r.cache.StoreLibraryNode(lib.Name, typeName, methodName, "", models.NodeTypeMethod, lib.Language, lib.Framework)
				if err != nil {
					return fmt.Errorf("failed to store library method %s.%s.%s: %w", lib.Name, typeName, methodName, err)
				}
			}
		}
	}

	return nil
}

// ResolvePythonLibrary resolves Python library and returns its ID
func (r *LibraryResolver) ResolvePythonLibrary(importPath string) (int64, error) {
	// Check if it's a standard library
	if r.isPythonStandardLibrary(importPath) {
		// Try to get or create the library node
		return r.cache.StoreLibraryNode(importPath, "", "", "", models.NodeTypePackage, "python", "stdlib")
	}

	// Check known Python libraries
	for _, lib := range r.knownLibraries {
		if lib.Language == "python" && lib.Name == importPath {
			return r.cache.StoreLibraryNode(lib.Name, "", "", "", models.NodeTypePackage, lib.Language, lib.Framework)
		}
	}

	// Unknown library - store as external
	return r.cache.StoreLibraryNode(importPath, "", "", "", models.NodeTypePackage, "python", "external")
}

// ResolveJavaScriptLibrary resolves JavaScript library and returns its ID
func (r *LibraryResolver) ResolveJavaScriptLibrary(importPath string) (int64, error) {
	// Check if it's a Node.js built-in module
	if r.isNodeBuiltinModule(importPath) {
		return r.cache.StoreLibraryNode(importPath, "", "", "", models.NodeTypePackage, "javascript", "nodejs")
	}

	// Check known JavaScript libraries
	for _, lib := range r.knownLibraries {
		if lib.Language == "javascript" && (lib.Name == importPath || strings.HasPrefix(importPath, lib.Name+"/")) {
			return r.cache.StoreLibraryNode(lib.Name, "", "", "", models.NodeTypePackage, lib.Language, lib.Framework)
		}
	}

	// Unknown library - store as npm package
	return r.cache.StoreLibraryNode(importPath, "", "", "", models.NodeTypePackage, "javascript", "npm")
}

// ResolveTypeScriptLibrary resolves TypeScript library and returns its ID
func (r *LibraryResolver) ResolveTypeScriptLibrary(importPath string) (int64, error) {
	// TypeScript includes all JavaScript libraries
	if r.isNodeBuiltinModule(importPath) {
		return r.cache.StoreLibraryNode(importPath, "", "", "", models.NodeTypePackage, "typescript", "nodejs")
	}

	// Check for @types packages
	if strings.HasPrefix(importPath, "@types/") {
		libName := strings.TrimPrefix(importPath, "@types/")
		return r.cache.StoreLibraryNode(libName, "", "", "", models.NodeTypePackage, "typescript", "types")
	}

	// Check known TypeScript/JavaScript libraries
	for _, lib := range r.knownLibraries {
		if (lib.Language == "typescript" || lib.Language == "javascript") &&
			(lib.Name == importPath || strings.HasPrefix(importPath, lib.Name+"/")) {
			return r.cache.StoreLibraryNode(lib.Name, "", "", "", models.NodeTypePackage, lib.Language, lib.Framework)
		}
	}

	// Unknown library - store as npm package
	return r.cache.StoreLibraryNode(importPath, "", "", "", models.NodeTypePackage, "typescript", "npm")
}

// isPythonStandardLibrary checks if the import is from Python standard library
func (r *LibraryResolver) isPythonStandardLibrary(importPath string) bool {
	// Get the base module name
	parts := strings.Split(importPath, ".")
	if len(parts) == 0 {
		return false
	}
	baseName := parts[0]

	// Common Python standard library modules
	pythonStdlib := []string{
		"abc", "argparse", "array", "ast", "asyncio", "base64", "builtins",
		"collections", "contextlib", "copy", "csv", "datetime", "decimal",
		"email", "enum", "functools", "hashlib", "heapq", "html", "http",
		"importlib", "inspect", "io", "itertools", "json", "logging", "math",
		"multiprocessing", "os", "pathlib", "pickle", "platform", "pprint",
		"queue", "random", "re", "shutil", "signal", "socket", "sqlite3",
		"string", "struct", "subprocess", "sys", "tempfile", "threading",
		"time", "traceback", "types", "typing", "unittest", "urllib", "uuid",
		"warnings", "weakref", "xml", "zipfile",
	}

	for _, lib := range pythonStdlib {
		if baseName == lib {
			return true
		}
	}
	return false
}

// isNodeBuiltinModule checks if the import is a Node.js built-in module
func (r *LibraryResolver) isNodeBuiltinModule(importPath string) bool {
	// Node.js built-in modules
	nodeBuiltins := []string{
		"assert", "async_hooks", "buffer", "child_process", "cluster", "console",
		"constants", "crypto", "dgram", "diagnostics_channel", "dns", "domain",
		"events", "fs", "http", "http2", "https", "inspector", "module", "net",
		"os", "path", "perf_hooks", "process", "punycode", "querystring",
		"readline", "repl", "stream", "string_decoder", "sys", "timers", "tls",
		"trace_events", "tty", "url", "util", "v8", "vm", "wasi", "worker_threads",
		"zlib",
	}

	// Check with and without "node:" prefix
	cleanPath := strings.TrimPrefix(importPath, "node:")
	for _, builtin := range nodeBuiltins {
		if cleanPath == builtin || importPath == "node:"+builtin {
			return true
		}
	}
	return false
}

// AnalyzeGoMod analyzes go.mod file to discover project dependencies
func (r *LibraryResolver) AnalyzeGoMod(projectRoot string) ([]string, error) {
	goModPath := filepath.Join(projectRoot, "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open go.mod: %w", err)
	}
	defer func() { _ = file.Close() }()

	var dependencies []string
	scanner := bufio.NewScanner(file)
	inRequireBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "require (") {
			inRequireBlock = true
			continue
		}

		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}

		if strings.HasPrefix(line, "require ") {
			// Single line require
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dep := parts[1]
				dependencies = append(dependencies, dep)
			}
		} else if inRequireBlock && line != "" {
			// Multi-line require block
			parts := strings.Fields(line)
			if len(parts) >= 1 && !strings.HasPrefix(parts[0], "//") {
				dep := parts[0]
				dependencies = append(dependencies, dep)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading go.mod: %w", err)
	}

	return dependencies, nil
}

// GetLibraryInfo returns information about a library
func (r *LibraryResolver) GetLibraryInfo(packagePath string) *LibraryInfo {
	return r.ResolveLibrary(packagePath)
}

// IsWebFramework checks if a package is a web framework
func (r *LibraryResolver) IsWebFramework(packagePath string) bool {
	lib := r.ResolveLibrary(packagePath)
	return lib.Category == "web"
}

// IsDatabaseLibrary checks if a package is database-related
func (r *LibraryResolver) IsDatabaseLibrary(packagePath string) bool {
	lib := r.ResolveLibrary(packagePath)
	return lib.Category == "database"
}

// IsTestingLibrary checks if a package is testing-related
func (r *LibraryResolver) IsTestingLibrary(packagePath string) bool {
	lib := r.ResolveLibrary(packagePath)
	return lib.Category == "testing"
}
