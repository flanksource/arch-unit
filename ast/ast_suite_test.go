package ast_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/internal/cache"
)

var (
	sharedASTCache *cache.ASTCache
	sharedAnalyzer *ast.Analyzer
	sharedTmpDir   string
)

var _ = BeforeSuite(func() {
	// Initialize shared cache and analyzer for all tests
	var err error
	sharedTmpDir = GinkgoT().TempDir()

	// Reset cache to ensure clean state
	// cache.ResetASTCache()
	sharedASTCache = cache.MustGetASTCache()

	// Clear any existing data
	err = sharedASTCache.ClearAllData()
	Expect(err).NotTo(HaveOccurred())

	sharedAnalyzer = ast.NewAnalyzer(sharedASTCache, sharedTmpDir)

	// Set up initial test data by copying example files
	exampleDir := filepath.Join("..", "examples", "go-project")
	err = copyExampleFiles(exampleDir, sharedTmpDir)
	if err == nil {
		// Only analyze if example files exist
		err = sharedAnalyzer.AnalyzeFiles()
		if err != nil {
			GinkgoWriter.Printf("Warning: Failed to analyze example files: %v\n", err)
		}
	} else {
		GinkgoWriter.Printf("Warning: Example files not found at %s: %v\n", exampleDir, err)
	}
})

var _ = AfterSuite(func() {
	if sharedASTCache != nil {
		_ = sharedASTCache.Close()
	}
	if sharedTmpDir != "" {
		_ = os.RemoveAll(sharedTmpDir)
	}
	// cache.ResetASTCache()
})

// copyExampleFiles recursively copies files from source to destination directory
func copyExampleFiles(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

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

		sourceFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = sourceFile.Close() }()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer func() { _ = destFile.Close() }()

		_, err = io.Copy(destFile, sourceFile)
		return err
	})
}

func XTestAst(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ast Suite")
}
