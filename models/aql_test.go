package models_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("ParsePattern", func() {
	DescribeTable("parsing various AQL patterns",
		func(pattern string, expected *models.AQLPattern, wantErr bool) {
			result, err := models.ParsePattern(pattern)
			if wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("Package with slash", "internal/controllers", &models.AQLPattern{
			Package:    "internal/controllers",
			Original:   "internal/controllers",
			IsWildcard: false,
		}, false),
		Entry("Single method name (now treated as type)", "GetUser", &models.AQLPattern{
			Package:    "*",
			Type:       "GetUser",
			Method:     "",
			Original:   "GetUser",
			IsWildcard: false,
		}, false),
		Entry("Single method with lowercase (now treated as package)", "processData", &models.AQLPattern{
			Package:    "processData",
			Type:       "",
			Method:     "",
			Original:   "processData",
			IsWildcard: false,
		}, false),
		Entry("Single type name", "UserController", &models.AQLPattern{
			Package:    "*",
			Type:       "UserController",
			Method:     "",
			Original:   "UserController",
			IsWildcard: false,
		}, false),
		Entry("Type:Method shorthand", "UserController:GetUser", &models.AQLPattern{
			Package:    "UserController",
			Type:       "GetUser",
			Method:     "",
			Original:   "UserController:GetUser",
			IsWildcard: false,
		}, false),
		Entry("Dot notation for package.Type", "widgets.Table", &models.AQLPattern{
			Package:    "widgets",
			Type:       "Table",
			Method:     "",
			Original:   "widgets.Table",
			IsWildcard: false,
		}, false),
		Entry("Colon notation for package:Type", "widgets:Table", &models.AQLPattern{
			Package:    "widgets",
			Type:       "Table",
			Method:     "",
			Original:   "widgets:Table",
			IsWildcard: false,
		}, false),
		Entry("Metric pattern with dot", "*.cyclomatic", &models.AQLPattern{
			Package:    "*",
			Type:       "",
			Method:     "",
			Metric:     "cyclomatic",
			Original:   "*.cyclomatic",
			IsWildcard: true,
		}, false),
		Entry("Complex pattern with metric", "controllers:UserController:Get*.lines", &models.AQLPattern{
			Package:    "controllers",
			Type:       "UserController",
			Method:     "Get*",
			Metric:     "lines",
			Original:   "controllers:UserController:Get*.lines",
			IsWildcard: true,
		}, false),
		Entry("Dot notation with method", "widgets.Table.draw", &models.AQLPattern{
			Package:    "widgets",
			Type:       "Table",
			Method:     "draw",
			Original:   "widgets.Table.draw",
			IsWildcard: false,
		}, false),
		Entry("Package:Type:Method:Field", "models:User:GetName:id", &models.AQLPattern{
			Package:    "models",
			Type:       "User",
			Method:     "GetName",
			Field:      "id",
			Original:   "models:User:GetName:id",
			IsWildcard: false,
		}, false),
		Entry("Type with wildcard suffix", "*Service", &models.AQLPattern{
			Package:    "*",
			Type:       "*Service",
			Method:     "",
			Original:   "*Service",
			IsWildcard: true,
		}, false),
		Entry("Method with wildcard prefix (now treated as type)", "Get*", &models.AQLPattern{
			Package:    "*",
			Type:       "Get*",
			Method:     "",
			Original:   "Get*",
			IsWildcard: true,
		}, false),
		Entry("Traditional package:type", "controllers:UserController", &models.AQLPattern{
			Package:    "controllers",
			Type:       "UserController",
			Method:     "",
			Original:   "controllers:UserController",
			IsWildcard: false,
		}, false),
		Entry("Full pattern with all parts", "controllers:UserController:GetUser:id", &models.AQLPattern{
			Package:    "controllers",
			Type:       "UserController",
			Method:     "GetUser",
			Field:      "id",
			Original:   "controllers:UserController:GetUser:id",
			IsWildcard: false,
		}, false),
	)
})

var _ = Describe("ParsePattern metric detection", func() {
	Context("when using known metrics", func() {
		DescribeTable("detecting metrics with wildcard pattern",
			func(metric string) {
				pattern := "*." + metric
				result, err := models.ParsePattern(pattern)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Metric).To(Equal(metric))
				Expect(result.Package).To(Equal("*"))
			},
			Entry("cyclomatic metric", "cyclomatic"),
			Entry("parameters metric", "parameters"),
			Entry("returns metric", "returns"),
			Entry("lines metric", "lines"),
		)

		DescribeTable("detecting metrics with package patterns",
			func(pkg string) {
				pattern := pkg + ".lines"
				result, err := models.ParsePattern(pattern)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Metric).To(Equal("lines"))
				Expect(result.Package).To(Equal(pkg))
			},
			Entry("internal/service package", "internal/service"),
			Entry("controllers package", "controllers"),
			Entry("models package", "models"),
		)
	})

	Context("when using unknown words after dot", func() {
		DescribeTable("treating unknown words as type names",
			func(word string) {
				pattern := "mypackage." + word
				result, err := models.ParsePattern(pattern)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Metric).To(BeEmpty())
				Expect(result.Package).To(Equal("mypackage"))
				Expect(result.Type).To(Equal(word))
			},
			Entry("Table type", "Table"),
			Entry("Widget type", "Widget"),
			Entry("Controller type", "Controller"),
			Entry("Service type", "Service"),
		)
	})
})

