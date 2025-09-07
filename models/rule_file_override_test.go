package models_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("Rule.AppliesToFile", func() {
	DescribeTable("checking file pattern matching",
		func(filePattern string, filePath string, shouldMatch bool) {
			rule := models.Rule{
				FilePattern: filePattern,
			}
			result := rule.AppliesToFile(filePath)
			Expect(result).To(Equal(shouldMatch))
		},
		Entry("empty pattern matches all files", "", "any/file.go", true),
		Entry("exact filename match", "main.go", "cmd/app/main.go", true),
		Entry("glob pattern with asterisk", "*_test.go", "service/user_test.go", true),
		Entry("glob pattern doesn't match", "*_test.go", "service/user.go", false),
		Entry("path pattern with directory", "cmd/*/main.go", "cmd/app/main.go", true),
		Entry("path pattern with multiple directories", "internal/*/service/*.go", "internal/user/service/handler.go", true),
		Entry("path pattern doesn't match different structure", "cmd/*/main.go", "pkg/app/main.go", false),
	)
})

var _ = Describe("RuleSet.IsAllowedForFile", func() {
	DescribeTable("checking file-specific rule enforcement",
		func(rules []models.Rule, pkg string, method string, filePath string, wantAllowed bool, wantRule bool) {
			rs := &models.RuleSet{
				Rules: rules,
			}
			gotAllowed, gotRule := rs.IsAllowedForFile(pkg, method, filePath)
			Expect(gotAllowed).To(Equal(wantAllowed))
			Expect(gotRule != nil).To(Equal(wantRule))
		},
		Entry("file-specific deny rule blocks in matching files",
			[]models.Rule{
				{
					Type:        models.RuleTypeDeny,
					Package:     "testing",
					FilePattern: "*_service.go",
				},
			},
			"testing", "T", "user_service.go", false, true),
		Entry("file-specific deny rule allows in non-matching files",
			[]models.Rule{
				{
					Type:        models.RuleTypeDeny,
					Package:     "testing",
					FilePattern: "*_service.go",
				},
			},
			"testing", "T", "user_test.go", true, false),
		Entry("file-specific override allows previously denied",
			[]models.Rule{
				{
					Type:    models.RuleTypeDeny,
					Package: "fmt",
				},
				{
					Type:        models.RuleTypeOverride,
					Package:     "fmt",
					FilePattern: "*_test.go",
				},
			},
			"fmt", "Println", "user_test.go", true, false),
		Entry("file-specific override doesn't affect other files",
			[]models.Rule{
				{
					Type:    models.RuleTypeDeny,
					Package: "fmt",
				},
				{
					Type:        models.RuleTypeOverride,
					Package:     "fmt",
					FilePattern: "*_test.go",
				},
			},
			"fmt", "Println", "user_service.go", false, true),
		Entry("multiple file patterns with different rules",
			[]models.Rule{
				{
					Type:        models.RuleTypeDeny,
					Package:     "os",
					FilePattern: "cmd/*/main.go",
				},
				{
					Type:        models.RuleTypeAllow,
					Package:     "os",
					FilePattern: "cmd/admin/main.go",
				},
			},
			"os", "Exit", "cmd/admin/main.go", true, false),
	)
})
