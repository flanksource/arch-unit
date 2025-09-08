package parser_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
)

var _ = Describe("AQL Parser", func() {
	Describe("parsing simple rules", func() {
		It("should parse a simple LIMIT rule correctly", func() {
			aql := `RULE "Simple Rule" {
				LIMIT(*.cyclomatic > 10)
			}`

			ruleSet, err := parser.ParseAQL(aql)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			Expect(rule.Name).To(Equal("Simple Rule"))
			Expect(rule.Statements).To(HaveLen(1))

			stmt := rule.Statements[0]
			Expect(stmt.Type).To(Equal(models.AQLStatementLimit))
			Expect(stmt.Condition).NotTo(BeNil())

			Expect(stmt.Condition.Operator).To(Equal(models.OpGreaterThan))
			Expect(stmt.Condition.Property).To(Equal("cyclomatic"))
			Expect(stmt.Condition.Value).To(Equal(10.0))
		})
	})

	Describe("parsing complex rules", func() {
		It("should parse a multi-statement architecture rule", func() {
			aql := `RULE "Architecture Rule" {
				LIMIT(Controller*.cyclomatic > 15)
				FORBID(Controller* -> Repository*)
				REQUIRE(Controller* -> Service*)
				ALLOW(Service* -> Repository*)
			}`

			ruleSet, err := parser.ParseAQL(aql)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			Expect(rule.Name).To(Equal("Architecture Rule"))
			Expect(rule.Statements).To(HaveLen(4))

			// Verify LIMIT statement
			limitStmt := rule.Statements[0]
			Expect(limitStmt.Type).To(Equal(models.AQLStatementLimit))
			Expect(limitStmt.Pattern.String()).To(Equal("Controller*"))
			Expect(limitStmt.Condition.Operator).To(Equal(models.OpGreaterThan))
			Expect(limitStmt.Condition.Property).To(Equal("cyclomatic"))
			Expect(limitStmt.Condition.Value).To(Equal(15.0))

			// Verify FORBID statement
			forbidStmt := rule.Statements[1]
			Expect(forbidStmt.Type).To(Equal(models.AQLStatementForbid))
			Expect(forbidStmt.FromPattern.String()).To(Equal("Controller*"))
			Expect(forbidStmt.ToPattern.String()).To(Equal("Repository*"))

			// Verify REQUIRE statement
			requireStmt := rule.Statements[2]
			Expect(requireStmt.Type).To(Equal(models.AQLStatementRequire))
			Expect(requireStmt.FromPattern.String()).To(Equal("Controller*"))
			Expect(requireStmt.ToPattern.String()).To(Equal("Service*"))

			// Verify ALLOW statement
			allowStmt := rule.Statements[3]
			Expect(allowStmt.Type).To(Equal(models.AQLStatementAllow))
			Expect(allowStmt.FromPattern.String()).To(Equal("Service*"))
			Expect(allowStmt.ToPattern.String()).To(Equal("Repository*"))
		})
	})

	Describe("parsing multiple rules", func() {
		It("should parse multiple rules in a single file", func() {
			aql := `
			RULE "Complexity Rule" {
				LIMIT(*.cyclomatic > 10)
			}

			RULE "Layer Rule" {
				FORBID(Model* -> Controller*)
				REQUIRE(Controller* -> Service*)
			}`

			ruleSet, err := parser.ParseAQL(aql)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(2))

			Expect(ruleSet.Rules[0].Name).To(Equal("Complexity Rule"))
			Expect(ruleSet.Rules[1].Name).To(Equal("Layer Rule"))

			Expect(ruleSet.Rules[0].Statements).To(HaveLen(1))
			Expect(ruleSet.Rules[1].Statements).To(HaveLen(2))
		})
	})

	Describe("parsing pattern variants", func() {
		XDescribeTable("correctly parsing different pattern formats",
			func(patternStr string, expected models.AQLPattern) {
				aql := `RULE "Test" {
					LIMIT(` + patternStr + `.cyclomatic > 5)
				}`

				ruleSet, err := parser.ParseAQL(aql)
				Expect(err).NotTo(HaveOccurred())
				Expect(ruleSet.Rules).To(HaveLen(1))
				Expect(ruleSet.Rules[0].Statements).To(HaveLen(1))

				stmt := ruleSet.Rules[0].Statements[0]
				pattern := stmt.Pattern

				Expect(pattern.Package).To(Equal(expected.Package))
				Expect(pattern.Type).To(Equal(expected.Type))
				Expect(pattern.Method).To(Equal(expected.Method))
			},
			Entry("wildcard pattern", "*", models.AQLPattern{Package: "*", Type: "*", Method: "*"}),
			Entry("package wildcard", "pkg.*", models.AQLPattern{Package: "pkg", Type: "*", Method: "*"}),
			Entry("type pattern", "*.UserService", models.AQLPattern{Package: "*", Type: "UserService", Method: "*"}),
			Entry("method pattern", "*.UserService:Create*", models.AQLPattern{Package: "*", Type: "UserService", Method: "Create*"}),
			Entry("nested package", "api.controller.*", models.AQLPattern{Package: "api.controller", Type: "*", Method: "*"}),
			Entry("specific method", "main.Calculator:Add", models.AQLPattern{Package: "main", Type: "Calculator", Method: "Add"}),
		)
	})

	Describe("parsing condition operators", func() {
		DescribeTable("correctly parsing different comparison operators",
			func(conditionStr string, expectedOperator models.ComparisonOperator, expectedProperty string, expectedValue interface{}) {
				aql := `RULE "Test" {
					LIMIT(` + conditionStr + `)
				}`

				ruleSet, err := parser.ParseAQL(aql)
				Expect(err).NotTo(HaveOccurred())
				Expect(ruleSet.Rules).To(HaveLen(1))
				Expect(ruleSet.Rules[0].Statements).To(HaveLen(1))

				stmt := ruleSet.Rules[0].Statements[0]
				condition := stmt.Condition

				Expect(condition.Operator).To(Equal(expectedOperator))
				Expect(condition.Property).To(Equal(expectedProperty))
				Expect(condition.Value).To(Equal(expectedValue))
			},
			Entry("greater than", "*.cyclomatic > 10", models.OpGreaterThan, "cyclomatic", 10.0),
			Entry("greater than equal", "*.cyclomatic >= 5", models.OpGreaterThanEqual, "cyclomatic", 5.0),
			Entry("less than", "*.cyclomatic < 20", models.OpLessThan, "cyclomatic", 20.0),
			Entry("less than equal", "*.cyclomatic <= 15", models.OpLessThanEqual, "cyclomatic", 15.0),
			Entry("equal", "*.cyclomatic == 1", models.OpEqual, "cyclomatic", 1.0),
			Entry("not equal", "*.cyclomatic != 0", models.OpNotEqual, "cyclomatic", 0.0),
			Entry("lines metric", "*.lines > 100", models.OpGreaterThan, "lines", 100.0),
			Entry("params metric", "*.params < 5", models.OpLessThan, "params", 5.0),
		)
	})

	Describe("error handling", func() {
		DescribeTable("handling various syntax errors",
			func(aql string, expectedError string) {
				_, err := parser.ParseAQL(aql)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedError))
			},
			Entry("missing rule name", `RULE {
				LIMIT(*.cyclomatic > 10)
			}`, "expected rule name"),
			Entry("missing opening brace", `RULE "Test"
				LIMIT(*.cyclomatic > 10)
			}`, "expected '{' after rule name"),
			Entry("missing closing brace", `RULE "Test" {
				LIMIT(*.cyclomatic > 10)`, "expected '}' to close rule"),
			Entry("invalid pattern", `RULE "Test" {
				LIMIT(invalid..pattern.cyclomatic > 10)
			}`, "expected identifier"),
			Entry("missing condition in LIMIT", `RULE "Test" {
				LIMIT(*.cyclomatic)
			}`, "expected operator"),
			Entry("invalid operator", `RULE "Test" {
				LIMIT(*.cyclomatic ?? 10)
			}`, "expected operator"),
			Entry("missing value", `RULE "Test" {
				LIMIT(*.cyclomatic >)
			}`, "expected value"),
			Entry("missing arrow in relationship", `RULE "Test" {
				FORBID(Controller* Repository*)
			}`, "expected ')' after pattern"),
			Entry("invalid statement type", `RULE "Test" {
				INVALID(*.cyclomatic > 10)
			}`, "unexpected token"),
		)
	})

	Describe("whitespace handling", func() {
		It("should handle various whitespace patterns correctly", func() {
			aql := `


			RULE    "Spaced Rule"    {
				LIMIT   (   Controller*.cyclomatic   >   10   )
				FORBID  (  Controller*   ->   Repository*  )
			}


			`

			ruleSet, err := parser.ParseAQL(aql)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			Expect(rule.Name).To(Equal("Spaced Rule"))
			Expect(rule.Statements).To(HaveLen(2))

			Expect(rule.Statements[0].Type).To(Equal(models.AQLStatementLimit))
			Expect(rule.Statements[1].Type).To(Equal(models.AQLStatementForbid))
		})
	})

	Describe("comment handling", func() {
		It("should ignore comments and parse rules correctly", func() {
			aql := `// This is a comment
			RULE "Commented Rule" { // Another comment
				// This rule limits complexity
				LIMIT(*.cyclomatic > 10) // Max complexity is 10
				// And forbids direct access
				FORBID(Controller* -> Model*) // Controllers can't access models directly
			}
			// End of rules`

			ruleSet, err := parser.ParseAQL(aql)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			Expect(rule.Name).To(Equal("Commented Rule"))
			Expect(rule.Statements).To(HaveLen(2))
		})
	})

	Describe("performance testing", func() {
		It("should handle large rule sets efficiently", func() {
			var aqlBuilder strings.Builder

			// Generate a large rule set
			for i := 0; i < 100; i++ {
				aqlBuilder.WriteString(fmt.Sprintf(`
				RULE "Rule %d" {
					LIMIT(*.cyclomatic > %d)
					FORBID(Controller%d* -> Model%d*)
					REQUIRE(Controller%d* -> Service%d*)
				}
				`, i, i%20+1, i, i, i, i))
			}

			ruleSet, err := parser.ParseAQL(aqlBuilder.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(100))

			// Verify some rules
			for i := 0; i < 10; i++ {
				rule := ruleSet.Rules[i]
				Expect(rule.Name).To(Equal(fmt.Sprintf("Rule %d", i)))
				Expect(rule.Statements).To(HaveLen(3))
			}
		})
	})

	Describe("pattern string representation", func() {
		DescribeTable("correctly converting patterns to strings",
			func(pattern models.AQLPattern, expected string) {
				result := pattern.String()
				Expect(result).To(Equal(expected))
			},
			Entry("full wildcard", models.AQLPattern{Package: "*", Type: "*", Method: "*"}, "*"),
			Entry("package with wildcard", models.AQLPattern{Package: "pkg", Type: "*", Method: "*"}, "pkg.*"),
			Entry("type specific", models.AQLPattern{Package: "*", Type: "User", Method: "*"}, "*.User"),
			Entry("method specific", models.AQLPattern{Package: "*", Type: "User", Method: "Create"}, "*.User:Create"),
			Entry("package and type", models.AQLPattern{Package: "api", Type: "Controller", Method: "*"}, "api.Controller"),
		)
	})
})
