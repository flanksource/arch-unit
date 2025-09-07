package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("Config Parser", func() {
	Describe("loading configuration", func() {
		It("should load and parse YAML config files correctly", func() {
			// Create a temporary directory and config file
			tempDir := GinkgoT().TempDir()

			configContent := `
version: "1.0"
debounce: "30s"
rules:
  "**":
    imports:
      - "!internal/"
      - "!fmt:Println"
    debounce: "30s"
  "**/*_test.go":
    imports:
      - "+testing"
      - "+fmt:Println"
    debounce: "10s"
linters:
  golangci-lint:
    enabled: true
`

			configPath := filepath.Join(tempDir, ConfigFileName)
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Test loading config
			parser := NewParser(tempDir)
			config, err := parser.LoadConfig()
			Expect(err).NotTo(HaveOccurred())

			// Validate config
			Expect(config.Version).To(Equal("1.0"))
			Expect(config.Debounce).To(Equal("30s"))

			// Check rules
			Expect(config.Rules).To(HaveLen(2))

			globalRules, exists := config.Rules["**"]
			Expect(exists).To(BeTrue())
			Expect(globalRules.Imports).To(HaveLen(2))

			// Check linters
			Expect(config.Linters).To(HaveLen(1))

			golangciLint, exists := config.Linters["golangci-lint"]
			Expect(exists).To(BeTrue())
			Expect(golangciLint.Enabled).To(BeTrue())
		})
	})

	Describe("getting rules for files", func() {
		var config *models.Config

		BeforeEach(func() {
			config = &models.Config{
				Rules: map[string]models.RuleConfig{
					"**": {
						Imports: []string{"!internal/", "!fmt:Println"},
					},
					"**/*_test.go": {
						Imports: []string{"+testing", "+fmt:Println"},
					},
					"**/main.go": {
						Imports: []string{"+os:Exit"},
					},
				},
			}
		})

		DescribeTable("applying pattern-based rules to different file types",
			func(filePath string, expectedRules int, shouldHaveFmt, shouldHaveTest bool) {
				ruleSet, err := config.GetRulesForFile(filePath)
				Expect(err).NotTo(HaveOccurred())
				Expect(ruleSet.Rules).To(HaveLen(expectedRules))

				hasFmtRule := false
				hasTestRule := false

				for _, rule := range ruleSet.Rules {
					if rule.Package == "fmt" && rule.Method == "Println" {
						hasFmtRule = true
					}
					if rule.Pattern == "testing" {
						hasTestRule = true
					}
				}

				Expect(hasFmtRule).To(Equal(shouldHaveFmt), "fmt rule presence for %s", filePath)
				Expect(hasTestRule).To(Equal(shouldHaveTest), "test rule presence for %s", filePath)
			},
			Entry("regular Go file", "service.go", 2, true, false),      // global rules
			Entry("test Go file", "service_test.go", 4, true, true),    // global + test rules
			Entry("main Go file", "cmd/main.go", 3, true, false),       // global + main rules
		)
	})
})
