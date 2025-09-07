package models_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("NewQualityRule", func() {
	DescribeTable("creating quality rules with defaults",
		func(ruleType models.RuleType, expectDefaults bool) {
			rule := models.NewQualityRule(ruleType)
			Expect(rule.Type).To(Equal(ruleType))

			switch ruleType {
			case models.RuleTypeMaxFileLength:
				Expect(rule.MaxFileLines).To(Equal(400))
			case models.RuleTypeMaxNameLength:
				Expect(rule.MaxNameLength).To(Equal(50))
			case models.RuleTypeCommentQuality:
				Expect(rule.CommentWordLimit).To(Equal(10))
				Expect(rule.CommentAIModel).To(Equal("claude-3-haiku-20240307"))
				Expect(rule.MinDescriptiveScore).To(BeNumerically("==", 0.7))
			}
		},
		Entry("max file length rule", models.RuleTypeMaxFileLength, true),
		Entry("max name length rule", models.RuleTypeMaxNameLength, true),
		Entry("comment quality rule", models.RuleTypeCommentQuality, true),
		Entry("disallowed name rule", models.RuleTypeDisallowedName, false),
	)
})

var _ = Describe("QualityRule.ValidateFileLength", func() {
	var rule *models.QualityRule

	BeforeEach(func() {
		rule = models.NewQualityRule(models.RuleTypeMaxFileLength)
		rule.MaxFileLines = 100
	})

	DescribeTable("validating file lengths",
		func(lineCount int, expected bool) {
			result := rule.ValidateFileLength(lineCount)
			Expect(result).To(Equal(expected))
		},
		Entry("file within limit", 50, true),
		Entry("file at limit", 100, true),
		Entry("file exceeds limit", 150, false),
	)
})

var _ = Describe("QualityRule.ValidateNameLength", func() {
	var rule *models.QualityRule

	BeforeEach(func() {
		rule = models.NewQualityRule(models.RuleTypeMaxNameLength)
		rule.MaxNameLength = 20
	})

	DescribeTable("validating name lengths",
		func(input string, expected bool) {
			result := rule.ValidateNameLength(input)
			Expect(result).To(Equal(expected))
		},
		Entry("name within limit", "shortName", true),
		Entry("name at limit", "exactlyTwentyCharact", true),
		Entry("name exceeds limit", "thisNameIsDefinitelyTooLongForTheLimit", false),
	)
})

var _ = Describe("QualityRule.ValidateDisallowedName", func() {
	var rule *models.QualityRule

	BeforeEach(func() {
		rule = models.NewQualityRule(models.RuleTypeDisallowedName)
		rule.DisallowedPatterns = []string{"temp*", "*Manager", "test*"}
	})

	DescribeTable("validating disallowed names",
		func(input string, expected bool) {
			result := rule.ValidateDisallowedName(input)
			Expect(result).To(Equal(expected))
		},
		Entry("allowed name", "goodName", true),
		Entry("temp prefix disallowed", "tempVariable", false),
		Entry("Manager suffix disallowed", "UserManager", false),
		Entry("test prefix disallowed", "testFunc", false),
		Entry("case sensitive match", "TempVariable", true),
	)
})

var _ = Describe("QualityRule getter methods", func() {
	Context("with comment quality rule", func() {
		var rule *models.QualityRule

		BeforeEach(func() {
			rule = models.NewQualityRule(models.RuleTypeCommentQuality)
		})

		It("should return default values initially", func() {
			Expect(rule.GetCommentWordLimit()).To(Equal(10))
			Expect(rule.GetCommentAIModel()).To(Equal("claude-3-haiku-20240307"))
			Expect(rule.GetMinDescriptiveScore()).To(BeNumerically("==", 0.7))
		})

		It("should return custom values when set", func() {
			rule.CommentWordLimit = 15
			rule.CommentAIModel = "custom-model"
			rule.MinDescriptiveScore = 0.8

			Expect(rule.GetCommentWordLimit()).To(Equal(15))
			Expect(rule.GetCommentAIModel()).To(Equal("custom-model"))
			Expect(rule.GetMinDescriptiveScore()).To(BeNumerically("==", 0.8))
		})
	})

	Context("with zero-value rule", func() {
		It("should return defaults for zero/empty values", func() {
			rule := &models.QualityRule{}

			Expect(rule.GetCommentWordLimit()).To(Equal(10))
			Expect(rule.GetCommentAIModel()).To(Equal("claude-3-haiku-20240307"))
			Expect(rule.GetMinDescriptiveScore()).To(BeNumerically("==", 0.7))
		})
	})
})

var _ = Describe("NewQualityRuleSet", func() {
	It("should create a new rule set with given path", func() {
		path := "/test/path"
		ruleSet := models.NewQualityRuleSet(path)

		Expect(ruleSet.Path).To(Equal(path))
		Expect(ruleSet.QualityRules).To(BeEmpty())
		Expect(ruleSet.Rules).To(BeEmpty())
	})
})

var _ = Describe("QualityRuleSet", func() {
	Describe("AddQualityRule and GetQualityRules", func() {
		var ruleSet *models.QualityRuleSet

		BeforeEach(func() {
			ruleSet = models.NewQualityRuleSet("/test")
		})

		It("should add and retrieve rules correctly", func() {
			fileRule := models.NewQualityRule(models.RuleTypeMaxFileLength)
			nameRule := models.NewQualityRule(models.RuleTypeMaxNameLength)
			commentRule := models.NewQualityRule(models.RuleTypeCommentQuality)

			ruleSet.AddQualityRule(fileRule)
			ruleSet.AddQualityRule(nameRule)
			ruleSet.AddQualityRule(commentRule)

			Expect(ruleSet.QualityRules).To(HaveLen(3))
			Expect(ruleSet.Rules).To(HaveLen(3))

			fileRules := ruleSet.GetQualityRules(models.RuleTypeMaxFileLength)
			Expect(fileRules).To(HaveLen(1))

			nameRules := ruleSet.GetQualityRules(models.RuleTypeMaxNameLength)
			Expect(nameRules).To(HaveLen(1))

			disallowedRules := ruleSet.GetQualityRules(models.RuleTypeDisallowedName)
			Expect(disallowedRules).To(BeEmpty())
		})
	})

	Describe("GetMaxValues", func() {
		var ruleSet *models.QualityRuleSet

		BeforeEach(func() {
			ruleSet = models.NewQualityRuleSet("/test")
		})

		It("should return 0 when no rules exist", func() {
			Expect(ruleSet.GetMaxFileLength()).To(Equal(0))
			Expect(ruleSet.GetMaxNameLength()).To(Equal(0))
		})

		It("should return max values when rules exist", func() {
			fileRule := models.NewQualityRule(models.RuleTypeMaxFileLength)
			fileRule.MaxFileLines = 200

			nameRule := models.NewQualityRule(models.RuleTypeMaxNameLength)
			nameRule.MaxNameLength = 30

			ruleSet.AddQualityRule(fileRule)
			ruleSet.AddQualityRule(nameRule)

			Expect(ruleSet.GetMaxFileLength()).To(Equal(200))
			Expect(ruleSet.GetMaxNameLength()).To(Equal(30))
		})
	})

	Describe("GetCommentQualityRule", func() {
		var ruleSet *models.QualityRuleSet

		BeforeEach(func() {
			ruleSet = models.NewQualityRuleSet("/test")
		})

		It("should return nil when no comment quality rule exists", func() {
			Expect(ruleSet.GetCommentQualityRule()).To(BeNil())
		})

		It("should return the comment quality rule when it exists", func() {
			commentRule := models.NewQualityRule(models.RuleTypeCommentQuality)
			commentRule.CommentWordLimit = 15

			ruleSet.AddQualityRule(commentRule)

			retrievedRule := ruleSet.GetCommentQualityRule()
			Expect(retrievedRule).NotTo(BeNil())
			Expect(retrievedRule.CommentWordLimit).To(Equal(15))
		})
	})
})

var _ = Describe("Performance tests", func() {
	It("should validate name length efficiently", func() {
		rule := models.NewQualityRule(models.RuleTypeMaxNameLength)
		name := "averageLengthFunctionName"

		result := rule.ValidateNameLength(name)
		Expect(result).To(BeTrue())
	})

	It("should validate disallowed names efficiently", func() {
		rule := models.NewQualityRule(models.RuleTypeDisallowedName)
		rule.DisallowedPatterns = []string{"temp*", "*Manager", "test*", "data*", "*Util"}
		name := "averageFunctionName"

		result := rule.ValidateDisallowedName(name)
		Expect(result).To(BeTrue())
	})
})
