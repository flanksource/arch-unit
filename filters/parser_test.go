package filters

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("Filter Parser", func() {
	var parser *Parser

	BeforeEach(func() {
		parser = NewParser("/test")
	})

	Describe("parsing individual lines", func() {
		DescribeTable("parsing different line formats",
			func(line string, expectedType models.RuleType, expectedPkg, expectedMethod, expectedPattern string) {
				// Skip comment and empty line tests for actual parsing
				if line == "" || (len(line) > 0 && line[0] == '#') {
					Skip("Skipping comment/empty line test")
					return
				}

				rule, err := parser.parseLine(line, "test.ARCHUNIT", 1, "/test")

				Expect(err).NotTo(HaveOccurred())
				Expect(rule).NotTo(BeNil())
				Expect(rule.Type).To(Equal(expectedType))
				Expect(rule.Package).To(Equal(expectedPkg))
				Expect(rule.Method).To(Equal(expectedMethod))
				Expect(rule.Pattern).To(Equal(expectedPattern))
			},
			Entry("deny pattern", "!internal/", models.RuleTypeDeny, "", "", "internal/"),
			Entry("override pattern", "+internal/api", models.RuleTypeOverride, "", "", "internal/api"),
			Entry("allow pattern", "utils/", models.RuleTypeAllow, "", "", "utils/"),
			Entry("method deny", "fmt:!Println", models.RuleTypeDeny, "fmt", "Println", ""),
			Entry("wildcard method", "*:!Test*", models.RuleTypeDeny, "*", "Test*", ""),
			Entry("comment line", "# This is a comment", models.RuleTypeAllow, "", "", ""),
			Entry("empty line", "", models.RuleTypeAllow, "", "", ""),
		)
	})

	Describe("parsing rule files", func() {
		It("should parse .ARCHUNIT files correctly", func() {
			// Create a temporary .ARCHUNIT file
			tmpDir := GinkgoT().TempDir()
			archUnitPath := filepath.Join(tmpDir, ".ARCHUNIT")

			content := `# Test rules
!internal/
!testing

# Method rules
fmt:!Println
*:!Test*

# Override
+internal/api
`

			err := os.WriteFile(archUnitPath, []byte(content), 0644)
			Expect(err).NotTo(HaveOccurred())

			parser := NewParser(tmpDir)
			ruleSet, err := parser.parseRuleFile(archUnitPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(5))

			// Check specific rules
			expectedRules := []struct {
				Type    models.RuleType
				Pattern string
				Package string
				Method  string
			}{
				{models.RuleTypeDeny, "internal/", "", ""},
				{models.RuleTypeDeny, "testing", "", ""},
				{models.RuleTypeDeny, "", "fmt", "Println"},
				{models.RuleTypeDeny, "", "*", "Test*"},
				{models.RuleTypeOverride, "internal/api", "", ""},
			}

			for i, expected := range expectedRules {
				rule := ruleSet.Rules[i]
				Expect(rule.Type).To(Equal(expected.Type), "Rule %d type mismatch", i)
				Expect(rule.Pattern).To(Equal(expected.Pattern), "Rule %d pattern mismatch", i)
				Expect(rule.Package).To(Equal(expected.Package), "Rule %d package mismatch", i)
				Expect(rule.Method).To(Equal(expected.Method), "Rule %d method mismatch", i)
			}
		})
	})
})
