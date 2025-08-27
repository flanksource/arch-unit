package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebounceFlag(t *testing.T) {
	// Build arch-unit binary
	buildCmd := exec.Command("go", "build", "-o", "test-arch-unit", ".")
	buildCmd.Dir = filepath.Join("..")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build arch-unit: %v\nOutput: %s", err, output)
	}
	defer os.Remove("../test-arch-unit")

	// Create a temporary test directory
	tempDir := t.TempDir()

	// Create a simple test file and .ARCHUNIT
	testFiles := map[string]string{
		"main.go": `package main
import "fmt"
func main() {
	fmt.Println("test")
}`,
		".ARCHUNIT": `!fmt:Println`,
	}

	for name, content := range testFiles {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", name, err)
		}
	}

	tests := []struct {
		name            string
		debounceFlag    string
		expectError     bool
		expectSkip      bool
		runTwice        bool
		delayBetweenRuns time.Duration
	}{
		{
			name:         "no_debounce_flag",
			debounceFlag: "",
			expectError:  false,
			expectSkip:   false,
		},
		{
			name:         "invalid_debounce_duration",
			debounceFlag: "invalid",
			expectError:  true,
		},
		{
			name:         "valid_debounce_first_run",
			debounceFlag: "30s",
			expectError:  false,
			expectSkip:   false,
		},
		{
			name:             "valid_debounce_second_run_within_period",
			debounceFlag:     "30s",
			expectError:      false,
			expectSkip:       true,
			runTwice:         true,
			delayBetweenRuns: 100 * time.Millisecond,
		},
		{
			name:             "valid_debounce_second_run_outside_period",
			debounceFlag:     "100ms",
			expectError:      false,
			expectSkip:       false,
			runTwice:         true,
			delayBetweenRuns: 200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run first command
			args := []string{"check", tempDir}
			if tt.debounceFlag != "" {
				args = append(args, "--debounce="+tt.debounceFlag)
			}

			cmd := exec.Command("../test-arch-unit", args...)
			output1, err1 := cmd.CombinedOutput()
			output1Str := string(output1)

			if tt.expectError {
				if err1 == nil {
					t.Errorf("Expected command to fail but it succeeded. Output: %s", output1Str)
				}
				return // Don't continue with second run test if first should fail
			}

			if err1 == nil && strings.Contains(output1Str, "1 architecture violation") {
				// This is expected - the test file violates the rule
			} else if err1 != nil && !strings.Contains(output1Str, "violation") {
				t.Errorf("Unexpected error on first run: %v\nOutput: %s", err1, output1Str)
			}

			// Check for skip message on first run (should not be skipped)
			if strings.Contains(output1Str, "Skipping check") && !tt.expectSkip {
				t.Errorf("First run unexpectedly showed skip message: %s", output1Str)
			}

			// If we're testing the second run
			if tt.runTwice {
				time.Sleep(tt.delayBetweenRuns)

				cmd2 := exec.Command("../test-arch-unit", args...)
				output2, err2 := cmd2.CombinedOutput()
				output2Str := string(output2)

				if tt.expectSkip {
					// Should be skipped
					if !strings.Contains(output2Str, "Skipping check") {
						t.Errorf("Expected second run to be skipped but it wasn't. Output: %s", output2Str)
					}
					if err2 != nil {
						t.Errorf("Skipped run should not return error: %v\nOutput: %s", err2, output2Str)
					}
				} else {
					// Should not be skipped
					if strings.Contains(output2Str, "Skipping check") {
						t.Errorf("Expected second run to proceed but it was skipped. Output: %s", output2Str)
					}
					// Should have similar results to first run
					if err2 == nil && strings.Contains(output2Str, "1 architecture violation") {
						// Expected
					} else if err2 != nil && !strings.Contains(output2Str, "violation") {
						t.Errorf("Unexpected error on second run: %v\nOutput: %s", err2, output2Str)
					}
				}
			}
		})
	}
}

func TestDebounceWithDifferentDirectories(t *testing.T) {
	// Build arch-unit binary
	buildCmd := exec.Command("go", "build", "-o", "test-arch-unit", ".")
	buildCmd.Dir = filepath.Join("..")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build arch-unit: %v\nOutput: %s", err, output)
	}
	defer os.Remove("../test-arch-unit")

	// Create two temporary test directories
	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()

	testFiles := map[string]string{
		"main.go":   `package main; import "fmt"; func main() { fmt.Println("test") }`,
		".ARCHUNIT": `!fmt:Println`,
	}

	// Create files in both directories
	for _, dir := range []string{tempDir1, tempDir2} {
		for name, content := range testFiles {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to create test file %s in %s: %v", name, dir, err)
			}
		}
	}

	// Run on first directory
	cmd1 := exec.Command("../test-arch-unit", "check", tempDir1, "--debounce=30s")
	output1, err1 := cmd1.CombinedOutput()
	if err1 == nil && !strings.Contains(string(output1), "violation") {
		t.Errorf("Expected violations in first directory run: %s", string(output1))
	}

	// Immediately run on second directory (should not be debounced)
	cmd2 := exec.Command("../test-arch-unit", "check", tempDir2, "--debounce=30s")
	output2, err2 := cmd2.CombinedOutput()
	output2Str := string(output2)

	if strings.Contains(output2Str, "Skipping check") {
		t.Errorf("Different directory should not be debounced: %s", output2Str)
	}

	if err2 == nil && !strings.Contains(output2Str, "violation") {
		t.Errorf("Expected violations in second directory run: %s", output2Str)
	}

	// Run on first directory again (should be debounced)
	cmd3 := exec.Command("../test-arch-unit", "check", tempDir1, "--debounce=30s")
	output3, _ := cmd3.CombinedOutput()
	output3Str := string(output3)

	if !strings.Contains(output3Str, "Skipping check") {
		t.Errorf("Same directory should be debounced on second run: %s", output3Str)
	}
}