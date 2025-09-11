package files

import (
	"os"
	"path/filepath"
	"strings"
)

// FindSourceFiles walks a directory tree and finds Go and Python source files
func FindSourceFiles(rootDir string) ([]string, []string, error) {
	var goFiles, pythonFiles []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			// Skip vendor and hidden directories
			if info.Name() == "vendor" || info.Name() == ".git" || strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			// Skip test files
			if !strings.HasSuffix(path, "_test.go") {
				goFiles = append(goFiles, path)
			}
		case ".py":
			// Skip test files
			if !strings.HasPrefix(filepath.Base(path), "test_") && !strings.HasSuffix(path, "_test.py") {
				pythonFiles = append(pythonFiles, path)
			}
		}

		return nil
	})

	return goFiles, pythonFiles, err
}