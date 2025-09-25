package tests

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/filters"
	"github.com/flanksource/arch-unit/internal/files"
)

var _ = Describe("File Specific Overrides", func() {
	var tempDir string

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()

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
			err := os.WriteFile(path, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred(), "Failed to create test file %s: %v", name, err)
		}

		// Write .ARCHUNIT file
		archunitPath := filepath.Join(tempDir, ".ARCHUNIT")
		err := os.WriteFile(archunitPath, []byte(archunitContent), 0644)
		Expect(err).NotTo(HaveOccurred(), "Failed to create .ARCHUNIT file: %v", err)
	})

	It("should apply file-specific overrides correctly", func() {
		// Load rules
		parser := filters.NewParser(tempDir)
		ruleSets, err := parser.LoadRules()
		Expect(err).NotTo(HaveOccurred())
		Expect(ruleSets).To(HaveLen(1))

		// Find and analyze Go files
		goFiles, _, err := files.FindSourceFiles(tempDir)
		Expect(err).NotTo(HaveOccurred())

		// Use the new unified analyzer
		result, err := analysis.AnalyzeGoFiles(tempDir, goFiles, ruleSets)
		Expect(err).NotTo(HaveOccurred())

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
			if shouldViolate {
				Expect(hasViolation).To(BeTrue(), "Expected violation in %s but found none", file)
			} else {
				Expect(hasViolation).To(BeFalse(), "Unexpected violation in %s", file)
			}
		}

		// Verify specific violation details
		for _, v := range result.Violations {
			if filepath.Base(v.File) == "service.go" {
				if v.Called != nil {
					Expect(v.Called.PackageName).To(Equal("fmt"))
					Expect(v.Called.MethodName).To(Equal("Println"))
				}
			}
		}
	})
})
