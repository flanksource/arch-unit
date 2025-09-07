package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
)

var _ = Describe("AQL YAML Loader", func() {
	Describe("loading simple rules", func() {
		It("should load a simple LIMIT rule from YAML", func() {
			yaml := `
rules:
  - name: "Simple Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "*"
            metric: "cyclomatic"
          operator: ">"
          value: 10
`

			ruleSet, err := parser.LoadAQLFromYAML(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			Expect(rule.Name).To(Equal("Simple Rule"))
			Expect(rule.Statements).To(HaveLen(1))

			stmt := rule.Statements[0]
			Expect(stmt.Type).To(Equal(models.AQLStatementLimit))
			Expect(stmt.Condition).NotTo(BeNil())

			Expect(stmt.Condition.Operator).To(Equal(models.AQLOperatorGT))
			Expect(stmt.Condition.Pattern.Metric).To(Equal("cyclomatic"))
			Expect(stmt.Condition.Pattern.Package).To(Equal("*"))
			Expect(stmt.Condition.Value).To(Equal(10))
		})
	})

	Describe("loading complex rules", func() {
		It("should load a multi-statement architecture rule from YAML", func() {
			yaml := `
rules:
  - name: "Architecture Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            type: "Controller*"
            metric: "cyclomatic"
          operator: ">"
          value: 15
      - type: FORBID
        from_pattern:
          type: "Controller*"
        to_pattern:
          type: "Repository*"
      - type: REQUIRE
        from_pattern:
          type: "Controller*"
        to_pattern:
          type: "Service*"
      - type: ALLOW
        from_pattern:
          type: "Service*"
        to_pattern:
          type: "Repository*"
`

			ruleSet, err := parser.LoadAQLFromYAML(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			Expect(rule.Name).To(Equal("Architecture Rule"))
			Expect(rule.Statements).To(HaveLen(4))

			// Verify LIMIT statement
			limitStmt := rule.Statements[0]
			Expect(limitStmt.Type).To(Equal(models.AQLStatementLimit))
			Expect(limitStmt.Condition.Pattern.Type).To(Equal("Controller*"))
			Expect(limitStmt.Condition.Operator).To(Equal(models.AQLOperatorGT))
			Expect(limitStmt.Condition.Pattern.Metric).To(Equal("cyclomatic"))
			Expect(limitStmt.Condition.Value).To(Equal(15))

			// Verify FORBID statement
			forbidStmt := rule.Statements[1]
			Expect(forbidStmt.Type).To(Equal(models.AQLStatementForbid))
			Expect(forbidStmt.FromPattern.Type).To(Equal("Controller*"))
			Expect(forbidStmt.ToPattern.Type).To(Equal("Repository*"))

			// Verify REQUIRE statement
			requireStmt := rule.Statements[2]
			Expect(requireStmt.Type).To(Equal(models.AQLStatementRequire))
			Expect(requireStmt.FromPattern.Type).To(Equal("Controller*"))
			Expect(requireStmt.ToPattern.Type).To(Equal("Service*"))

			// Verify ALLOW statement
			allowStmt := rule.Statements[3]
			Expect(allowStmt.Type).To(Equal(models.AQLStatementAllow))
			Expect(allowStmt.FromPattern.Type).To(Equal("Service*"))
			Expect(allowStmt.ToPattern.Type).To(Equal("Repository*"))
		})
	})

	Describe("loading multiple rules", func() {
		It("should load multiple rules from YAML", func() {
			yaml := `
rules:
  - name: "Complexity Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "*"
            metric: "cyclomatic"
          operator: ">"
          value: 20
  - name: "Layer Rule"
    statements:
      - type: FORBID
        from_pattern:
          type: "*Controller"
        to_pattern:
          type: "*Repository"
`

			ruleSet, err := parser.LoadAQLFromYAML(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(2))

			// Verify first rule
			rule1 := ruleSet.Rules[0]
			Expect(rule1.Name).To(Equal("Complexity Rule"))
			Expect(rule1.Statements).To(HaveLen(1))
			
			stmt1 := rule1.Statements[0]
			Expect(stmt1.Type).To(Equal(models.AQLStatementLimit))
			Expect(stmt1.Condition.Value).To(Equal(20))

			// Verify second rule
			rule2 := ruleSet.Rules[1]
			Expect(rule2.Name).To(Equal("Layer Rule"))
			Expect(rule2.Statements).To(HaveLen(1))
			
			stmt2 := rule2.Statements[0]
			Expect(stmt2.Type).To(Equal(models.AQLStatementForbid))
			Expect(stmt2.FromPattern.Type).To(Equal("*Controller"))
			Expect(stmt2.ToPattern.Type).To(Equal("*Repository"))
		})
	})

	Describe("validation errors", func() {
		DescribeTable("handling various validation errors",
			func(yaml string, expectedError string) {
				_, err := parser.LoadAQLFromYAML(yaml)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedError))
			},
			Entry("empty rules", `rules: []`, "rule set must contain at least one rule"),
			Entry("missing rule name", `
rules:
  - statements:
      - type: LIMIT
        condition:
          pattern:
            metric: "cyclomatic"
          operator: ">"
          value: 10
`, "rule name is required"),
			Entry("missing statements", `
rules:
  - name: "Test Rule"
    statements: []
`, "rule must contain at least one statement"),
			Entry("LIMIT without condition", `
rules:
  - name: "Test Rule"
    statements:
      - type: LIMIT
`, "LIMIT statement requires a condition"),
			Entry("invalid operator", `
rules:
  - name: "Test Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            metric: "cyclomatic"
          operator: "invalid"
          value: 10
`, "invalid operator: invalid"),
			Entry("missing value", `
rules:
  - name: "Test Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            metric: "cyclomatic"
          operator: ">"
`, "condition requires a value"),
		)
	})

	Describe("backward compatibility", func() {
		It("should support legacy property field instead of metric in pattern", func() {
			yaml := `
rules:
  - name: "Backward Compatibility Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "*"
          property: "cyclomatic"
          operator: ">"
          value: 10
`

			ruleSet, err := parser.LoadAQLFromYAML(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			stmt := rule.Statements[0]
			Expect(stmt.Condition.Property).To(Equal("cyclomatic"))
			Expect(stmt.Condition.Pattern.Metric).To(BeEmpty())
		})
	})

	Describe("format detection", func() {
		DescribeTable("detecting legacy AQL format",
			func(content string, expected bool) {
				result := parser.IsLegacyAQLFormat(content)
				Expect(result).To(Equal(expected))
			},
			Entry("legacy AQL format", `RULE "Test" { LIMIT(*.cyclomatic > 10) }`, true),
			Entry("legacy AQL with whitespace", ` RULE "Test" { LIMIT(*.cyclomatic > 10) }`, true),
			Entry("YAML format", `rules:
  - name: "Test"
    statements:
      - type: LIMIT`, false),
			Entry("JSON format", `{"rules": []}`, false),
			Entry("empty content", "", false),
		)
	})

	Describe("complete pattern support", func() {
		It("should load rules with complete pattern specifications", func() {
			yaml := `
rules:
  - name: "Complete Pattern Rule"
    statements:
      - type: LIMIT
        condition:
          pattern:
            package: "internal/service"
            type: "UserService"
            method: "CreateUser"
            field: "id"
            metric: "parameters"
          operator: "<="
          value: 3
`

			ruleSet, err := parser.LoadAQLFromYAML(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(ruleSet.Rules).To(HaveLen(1))

			rule := ruleSet.Rules[0]
			stmt := rule.Statements[0]
			pattern := stmt.Condition.Pattern

			Expect(pattern.Package).To(Equal("internal/service"))
			Expect(pattern.Type).To(Equal("UserService"))
			Expect(pattern.Method).To(Equal("CreateUser"))
			Expect(pattern.Field).To(Equal("id"))
			Expect(pattern.Metric).To(Equal("parameters"))
			Expect(stmt.Condition.Operator).To(Equal(models.AQLOperatorLTE))
			Expect(stmt.Condition.Value).To(Equal(3))
		})
	})
})