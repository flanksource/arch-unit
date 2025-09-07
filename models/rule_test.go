package models_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("Rule.Matches", func() {
	DescribeTable("matching rules against packages and methods",
		func(rule models.Rule, pkg string, method string, expected bool) {
			result := rule.Matches(pkg, method)
			Expect(result).To(Equal(expected))
		},
		Entry("exact package match", models.Rule{Pattern: "internal"}, "internal", "", true),
		Entry("package with slash match", models.Rule{Pattern: "internal/"}, "internal/utils", "", true),
		Entry("wildcard suffix match", models.Rule{Pattern: "*_test"}, "utils_test", "", true),
		Entry("wildcard prefix match", models.Rule{Pattern: "test*"}, "testing", "", true),
		Entry("method specific match", models.Rule{Package: "fmt", Method: "Println"}, "fmt", "Println", true),
		Entry("method wildcard match", models.Rule{Package: "*", Method: "Test*"}, "anything", "TestSomething", true),
		Entry("no match different package", models.Rule{Pattern: "internal"}, "external", "", false),
		Entry("no match different method", models.Rule{Package: "fmt", Method: "Println"}, "fmt", "Printf", false),
	)
})

var _ = Describe("RuleSet.IsAllowed", func() {
	DescribeTable("checking access permissions with different rule sets",
		func(ruleSet models.RuleSet, pkg string, method string, allowed bool, hasViolation bool) {
			allowedResult, rule := ruleSet.IsAllowed(pkg, method)
			Expect(allowedResult).To(Equal(allowed))
			Expect(rule != nil).To(Equal(hasViolation))
		},
		Entry("deny rule blocks access",
			models.RuleSet{
				Rules: []models.Rule{
					{Type: models.RuleTypeDeny, Pattern: "internal"},
				},
			},
			"internal", "", false, true),
		Entry("override rule allows previously denied",
			models.RuleSet{
				Rules: []models.Rule{
					{Type: models.RuleTypeDeny, Pattern: "internal"},
					{Type: models.RuleTypeOverride, Pattern: "internal/api"},
				},
			},
			"internal/api", "", true, false),
		Entry("no matching rules allows access",
			models.RuleSet{
				Rules: []models.Rule{
					{Type: models.RuleTypeDeny, Pattern: "internal"},
				},
			},
			"external", "", true, false),
		Entry("method specific deny",
			models.RuleSet{
				Rules: []models.Rule{
					{Type: models.RuleTypeDeny, Package: "fmt", Method: "Println"},
				},
			},
			"fmt", "Println", false, true),
	)
})
