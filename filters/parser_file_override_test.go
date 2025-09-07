package filters

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("Parser File-Specific Rules", func() {
	var parser *Parser

	BeforeEach(func() {
		parser = NewParser(".")
	})

	Describe("parsing file-specific lines", func() {
		Context("when parsing valid file-specific rules", func() {
			DescribeTable("correctly parsing different file-specific patterns",
				func(line, expectedFile, expectedPackage, expectedMethod string, expectedType models.RuleType) {
					rule, err := parser.parseLine(line, "test.ARCHUNIT", 1, ".")
					
					Expect(err).NotTo(HaveOccurred())
					Expect(rule).NotTo(BeNil())
					Expect(rule.FilePattern).To(Equal(expectedFile))
					Expect(rule.Package).To(Equal(expectedPackage))
					Expect(rule.Method).To(Equal(expectedMethod))
					Expect(rule.Type).To(Equal(expectedType))
				},
				Entry("file-specific deny rule", "[*_test.go] !fmt:Println", "*_test.go", "fmt", "Println", models.RuleTypeDeny),
				Entry("file-specific allow rule", "[cmd/*/main.go] os:Exit", "cmd/*/main.go", "os", "Exit", models.RuleTypeAllow),
				Entry("file-specific override rule", "[*_test.go] +testing", "*_test.go", "", "", models.RuleTypeOverride),
				Entry("file-specific with complex pattern", "[internal/*/service/*.go] !database/sql", "internal/*/service/*.go", "", "", models.RuleTypeDeny),
				Entry("file-specific with spaces", "[ *_test.go ] fmt:Println", "*_test.go", "fmt", "Println", models.RuleTypeAllow),
			)
		})

		Context("when parsing invalid file-specific rules", func() {
			DescribeTable("handling various error cases",
				func(line string) {
					_, err := parser.parseLine(line, "test.ARCHUNIT", 1, ".")
					Expect(err).To(HaveOccurred())
				},
				Entry("missing closing bracket", "[*_test.go fmt:Println"),
				Entry("empty rule after pattern", "[*_test.go]"),
				Entry("empty pattern", "[] fmt:Println"),
			)
		})
	})
})
