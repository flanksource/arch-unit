package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Debounce Flag", func() {
	var tempDir string

	BeforeEach(func() {
		// Build arch-unit binary
		buildCmd := exec.Command("go", "build", "-o", "test-arch-unit", ".")
		buildCmd.Dir = filepath.Join("..")
		output, err := buildCmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to build arch-unit: %v\nOutput: %s", err, output)

		// Create a temporary test directory
		tempDir = GinkgoT().TempDir()

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
			err := os.WriteFile(path, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred(), "Failed to create test file %s: %v", name, err)
		}
	})

	AfterEach(func() {
		os.Remove("../test-arch-unit")
	})

	XDescribeTable("should handle different debounce scenarios",
		func(debounceFlag string, expectError, expectSkip, runTwice bool, delayBetweenRuns time.Duration) {
			// Run first command
			args := []string{"check", tempDir}
			if debounceFlag != "" {
				args = append(args, "--debounce="+debounceFlag)
			}

			cmd := exec.Command("../test-arch-unit", args...)
			output1, err1 := cmd.CombinedOutput()
			output1Str := string(output1)

			if expectError {
				Expect(err1).To(HaveOccurred(), "Expected command to fail but it succeeded. Output: %s", output1Str)
				return // Don't continue with second run test if first should fail
			}

			// Either should succeed with violations or fail with violation message
			if err1 == nil {
				Expect(output1Str).To(ContainSubstring("1 architecture violation"))
			} else {
				Expect(output1Str).To(ContainSubstring("violation"))
			}

			// Check for skip message on first run (should not be skipped)
			if !expectSkip {
				Expect(output1Str).NotTo(ContainSubstring("Skipping check"), "First run unexpectedly showed skip message: %s", output1Str)
			}

			// If we're testing the second run
			if runTwice {
				time.Sleep(delayBetweenRuns)

				cmd2 := exec.Command("../test-arch-unit", args...)
				output2, err2 := cmd2.CombinedOutput()
				output2Str := string(output2)

				if expectSkip {
					// Should be skipped
					Expect(output2Str).To(ContainSubstring("Skipping check"), "Expected second run to be skipped but it wasn't. Output: %s", output2Str)
					Expect(err2).NotTo(HaveOccurred(), "Skipped run should not return error: %v\nOutput: %s", err2, output2Str)
				} else {
					// Should not be skipped
					Expect(output2Str).NotTo(ContainSubstring("Skipping check"), "Expected second run to proceed but it was skipped. Output: %s", output2Str)

					// Should have similar results to first run
					if err2 == nil {
						Expect(output2Str).To(ContainSubstring("1 architecture violation"))
					} else {
						Expect(output2Str).To(ContainSubstring("violation"))
					}
				}
			}
		},
		Entry("no debounce flag", "", false, false, false, time.Duration(0)),
		Entry("invalid debounce duration", "invalid", true, false, false, time.Duration(0)),
		Entry("valid debounce first run", "30s", false, false, false, time.Duration(0)),
		Entry("valid debounce second run within period", "30s", false, true, true, 100*time.Millisecond),
		Entry("valid debounce second run outside period", "100ms", false, false, true, 200*time.Millisecond),
	)
})

var _ = Describe("Debounce With Different Directories", func() {
	var tempDir1, tempDir2 string

	BeforeEach(func() {
		// Build arch-unit binary
		buildCmd := exec.Command("go", "build", "-o", "test-arch-unit", ".")
		buildCmd.Dir = filepath.Join("..")
		output, err := buildCmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to build arch-unit: %v\nOutput: %s", err, output)

		// Create two temporary test directories
		tempDir1 = GinkgoT().TempDir()
		tempDir2 = GinkgoT().TempDir()

		testFiles := map[string]string{
			"main.go":   `package main; import "fmt"; func main() { fmt.Println("test") }`,
			".ARCHUNIT": `!fmt:Println`,
		}

		// Create files in both directories
		for _, dir := range []string{tempDir1, tempDir2} {
			for name, content := range testFiles {
				path := filepath.Join(dir, name)
				err := os.WriteFile(path, []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred(), "Failed to create test file %s in %s: %v", name, dir, err)
			}
		}
	})

	AfterEach(func() {
		os.Remove("../test-arch-unit")
	})

	XIt("should not debounce different directories", func() {
		// Run on first directory
		cmd1 := exec.Command("../test-arch-unit", "check", tempDir1, "--debounce=30s")
		output1, err1 := cmd1.CombinedOutput()
		output1Str := string(output1)

		if err1 == nil {
			Expect(output1Str).To(ContainSubstring("violation"))
		} else {
			Expect(output1Str).To(ContainSubstring("violation"))
		}

		// Immediately run on second directory (should not be debounced)
		cmd2 := exec.Command("../test-arch-unit", "check", tempDir2, "--debounce=30s")
		output2, err2 := cmd2.CombinedOutput()
		output2Str := string(output2)

		Expect(output2Str).NotTo(ContainSubstring("Skipping check"), "Different directory should not be debounced: %s", output2Str)

		if err2 == nil {
			Expect(output2Str).To(ContainSubstring("violation"))
		} else {
			Expect(output2Str).To(ContainSubstring("violation"))
		}

		// Run on first directory again (should be debounced)
		cmd3 := exec.Command("../test-arch-unit", "check", tempDir1, "--debounce=30s")
		output3, _ := cmd3.CombinedOutput()
		output3Str := string(output3)

		Expect(output3Str).To(ContainSubstring("Skipping check"), "Same directory should be debounced on second run: %s", output3Str)
	})
})
