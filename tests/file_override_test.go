package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/client"
	"github.com/flanksource/arch-unit/filters"
)

func TestFileSpecificOverrides(t *testing.T) {
	// Create a temporary test directory
	tempDir := t.TempDir()

	// Create test Go files
	testFiles := map[string]string{
		"service.go": `package main
import "fmt"
func DoService() {
	fmt.Println("service")
}`,
		"service_test.go": `package main
import (
	"fmt"
	"testing"
)
func TestService(t *testing.T) {
	fmt.Println("test")
}`,
		"repository.go": `package main
import "database/sql"
func GetData() {
	var db *sql.DB
	_ = db
}`,
	}

	// Create .ARCHUNIT file with file-specific rules
	archunitContent := `# Test rules with file-specific overrides

# Deny fmt.Println everywhere
fmt:!Println

# But allow it in test files
[*_test.go] +fmt:Println

# Deny database/sql everywhere  
!database/sql

# But allow it in repository files
[*_repository.go] +database/sql
`

	// Write test files
	for name, content := range testFiles {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	// Write .ARCHUNIT file
	archunitPath := filepath.Join(tempDir, ".ARCHUNIT")
	if err := os.WriteFile(archunitPath, []byte(archunitContent), 0644); err != nil {
		t.Fatalf("Failed to create .ARCHUNIT file: %v", err)
	}

	// Load rules
	parser := filters.NewParser(tempDir)
	ruleSets, err := parser.LoadRules()
	if err != nil {
		t.Fatalf("Failed to load rules: %v", err)
	}

	if len(ruleSets) != 1 {
		t.Fatalf("Expected 1 ruleset, got %d", len(ruleSets))
	}

	// Find and analyze Go files
	goFiles, _, err := client.FindSourceFiles(tempDir)
	if err != nil {
		t.Fatalf("Failed to find source files: %v", err)
	}

	result, err := client.AnalyzeGoFiles(tempDir, goFiles, ruleSets)
	if err != nil {
		t.Fatalf("Failed to analyze files: %v", err)
	}

	// Check violations
	expectedViolations := map[string]bool{
		"service.go":      true,  // Should violate fmt.Println rule
		"service_test.go": false, // Should be allowed by file-specific override
		"repository.go":   false, // Should be allowed by file-specific override
	}

	violationsByFile := make(map[string]bool)
	for _, v := range result.Violations {
		violationsByFile[filepath.Base(v.File)] = true
	}

	for file, shouldViolate := range expectedViolations {
		hasViolation := violationsByFile[file]
		if shouldViolate && !hasViolation {
			t.Errorf("Expected violation in %s but found none", file)
		}
		if !shouldViolate && hasViolation {
			t.Errorf("Unexpected violation in %s", file)
		}
	}

	// Verify specific violation details
	for _, v := range result.Violations {
		if filepath.Base(v.File) == "service.go" {
			if v.CalledPackage != "fmt" || v.CalledMethod != "Println" {
				t.Errorf("Unexpected violation: %s.%s in %s", v.CalledPackage, v.CalledMethod, v.File)
			}
		}
	}
}
