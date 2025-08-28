package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EFileOverrides(t *testing.T) {
	// Build arch-unit binary
	buildCmd := exec.Command("go", "build", "-o", "test-arch-unit", ".")
	buildCmd.Dir = filepath.Join("..")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build arch-unit: %v\nOutput: %s", err, output)
	}
	defer os.Remove("../test-arch-unit")

	testCases := []struct {
		name          string
		files         map[string]string
		archunitRules string
		expectFail    bool
		expectOutput  []string
		notExpect     []string
	}{
		{
			name: "basic_file_override",
			files: map[string]string{
				"service.go": `package main
import "fmt"
func Service() { fmt.Println("no") }`,
				"service_test.go": `package main
import "fmt"
import "testing"
func TestService(t *testing.T) { fmt.Println("ok") }`,
			},
			archunitRules: `!fmt:Println
[*_test.go] +fmt:Println`,
			expectFail:   true,
			expectOutput: []string{"service.go"},
			notExpect:    []string{"service_test.go"},
		},
		{
			name: "complex_path_patterns",
			files: map[string]string{
				"cmd/app/main.go": `package main
import "os"
func main() { os.Exit(0) }`,
				"pkg/service/handler.go": `package service
import "os"
func Handle() { os.Exit(1) }`,
			},
			archunitRules: `!os:Exit
[cmd/*/main.go] +os:Exit`,
			expectFail:   true,
			expectOutput: []string{"handler.go"},
			notExpect:    []string{"main.go"},
		},
		{
			name: "multiple_override_rules",
			files: map[string]string{
				"user_repository.go": `package main
import "database/sql"
func GetUser() { var db *sql.DB; _ = db }`,
				"user_service.go": `package main
import "database/sql"
func ProcessUser() { var db *sql.DB; _ = db }`,
				"order_repository.go": `package main
import "database/sql"  
func GetOrder() { var db *sql.DB; _ = db }`,
			},
			archunitRules: `!database/sql
[*_repository.go] +database/sql`,
			expectFail:   true,
			expectOutput: []string{"user_service.go"},
			notExpect:    []string{"user_repository.go", "order_repository.go"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp directory
			tempDir := t.TempDir()

			// Write test files
			for path, content := range tc.files {
				fullPath := filepath.Join(tempDir, path)
				dir := filepath.Dir(fullPath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory %s: %v", dir, err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to write file %s: %v", path, err)
				}
			}

			// Write .ARCHUNIT file
			archunitPath := filepath.Join(tempDir, ".ARCHUNIT")
			if err := os.WriteFile(archunitPath, []byte(tc.archunitRules), 0644); err != nil {
				t.Fatalf("Failed to write .ARCHUNIT: %v", err)
			}

			// Run arch-unit check
			cmd := exec.Command("../test-arch-unit", "check", tempDir)
			output, err := cmd.CombinedOutput()
			outputStr := string(output)

			// Check exit code
			if tc.expectFail && err == nil {
				t.Errorf("Expected command to fail but it succeeded")
			}
			if !tc.expectFail && err != nil {
				t.Errorf("Expected command to succeed but it failed: %v\nOutput: %s", err, outputStr)
			}

			// Check expected output
			for _, expected := range tc.expectOutput {
				if !strings.Contains(outputStr, expected) {
					t.Errorf("Expected output to contain %q but it didn't.\nOutput: %s", expected, outputStr)
				}
			}

			// Check not expected output
			for _, notExpected := range tc.notExpect {
				if strings.Contains(outputStr, notExpected) {
					t.Errorf("Expected output NOT to contain %q but it did.\nOutput: %s", notExpected, outputStr)
				}
			}
		})
	}
}
