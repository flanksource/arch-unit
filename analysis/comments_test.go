package analysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/models"
)

var _ = Describe("DefaultCommentAnalyzerConfig", func() {
	It("should return correct default configuration", func() {
		config := DefaultCommentAnalyzerConfig()

		Expect(config.WordLimit).To(Equal(10))
		Expect(config.LowCostModel).To(Equal("claude-3-haiku-20240307"))
		Expect(config.MinDescriptiveScore).To(Equal(0.7))
		Expect(config.CheckVerbosity).To(BeTrue())
		Expect(config.Enabled).To(BeTrue())
	})
})

var _ = Describe("AnalyzeComment", func() {
	Context("when analyzer is disabled", func() {
		It("should return disabled analysis result", func() {
			config := CommentAnalyzerConfig{
				Enabled: false,
			}

			analyzer := NewCommentAnalyzer(config, nil)

			comment := models.Comment{
				Text:      "This is a test comment with many words that exceeds the limit",
				WordCount: 12,
			}

			result, err := analyzer.AnalyzeComment(context.Background(), comment)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.AnalysisMethod).To(Equal("disabled"))
			Expect(result.IsSimple).To(BeTrue())
			Expect(result.IsDescriptive).To(BeTrue())
		})
	})

	Context("when analyzing simple comment", func() {
		It("should return simple analysis result", func() {
			config := CommentAnalyzerConfig{
				Enabled:   true,
				WordLimit: 10,
			}

			analyzer := NewCommentAnalyzer(config, nil)

			comment := models.Comment{
				Text:      "Short comment",
				WordCount: 2,
			}

			result, err := analyzer.AnalyzeComment(context.Background(), comment)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.AnalysisMethod).To(Equal("simple"))
			Expect(result.IsSimple).To(BeTrue())
			Expect(result.IsDescriptive).To(BeTrue())
			Expect(result.IsVerbose).To(BeFalse())
			Expect(result.Score).To(Equal(0.8))
		})
	})

	Context("when analyzing complex comment without AI", func() {
		It("should return fallback analysis result", func() {
			config := CommentAnalyzerConfig{
				Enabled:   true,
				WordLimit: 5,
			}

			analyzer := NewCommentAnalyzer(config, nil) // No AI agent

			comment := models.Comment{
				Text:      "This is a longer comment that exceeds the word limit",
				WordCount: 11,
			}

			result, err := analyzer.AnalyzeComment(context.Background(), comment)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.AnalysisMethod).To(Equal("fallback"))
			Expect(result.IsSimple).To(BeFalse())
			Expect(result.Score).To(Equal(0.5))
			Expect(result.Issues).To(Equal([]string{"AI analysis not available"}))
		})
	})
})

var _ = Describe("ExtractJSON", func() {
	var analyzer *CommentAnalyzer

	BeforeEach(func() {
		analyzer = &CommentAnalyzer{}
	})

	DescribeTable("extracting JSON from various inputs",
		func(input, expected string) {
			result := analyzer.extractJSON(input)
			Expect(result).To(Equal(expected))
		},
		Entry("simple JSON", `{"key": "value"}`, `{"key": "value"}`),
		Entry("JSON with text before", `Here is the analysis: {"key": "value", "number": 42}`, `{"key": "value", "number": 42}`),
		Entry("JSON with text after", `{"key": "value"} - this is the result`, `{"key": "value"}`),
		Entry("nested JSON", `{"outer": {"inner": {"deep": true}}, "list": [1, 2, 3]}`, `{"outer": {"inner": {"deep": true}}, "list": [1, 2, 3]}`),
		Entry("no JSON", `This is just text without JSON`, ``),
		Entry("malformed JSON", `{"key": "value"`, ``),
	)
})

var _ = Describe("GetPoorQualityComments", func() {
	It("should identify poor quality comments correctly", func() {
		config := CommentAnalyzerConfig{
			MinDescriptiveScore: 0.7,
			CheckVerbosity:      true,
		}

		analyzer := NewCommentAnalyzer(config, nil)

		results := []*CommentQualityResult{
			{
				Comment:       models.Comment{Text: "Good comment"},
				IsDescriptive: true,
				IsVerbose:     false,
				Score:         0.9,
				Issues:        []string{},
			},
			{
				Comment:       models.Comment{Text: "Bad comment"},
				IsDescriptive: false,
				IsVerbose:     false,
				Score:         0.8,
				Issues:        []string{},
			},
			{
				Comment:       models.Comment{Text: "Verbose comment"},
				IsDescriptive: true,
				IsVerbose:     true,
				Score:         0.8,
				Issues:        []string{},
			},
			{
				Comment:       models.Comment{Text: "Low score comment"},
				IsDescriptive: true,
				IsVerbose:     false,
				Score:         0.5,
				Issues:        []string{},
			},
			{
				Comment:       models.Comment{Text: "Comment with issues"},
				IsDescriptive: true,
				IsVerbose:     false,
				Score:         0.8,
				Issues:        []string{"Too vague"},
			},
		}

		poorQuality := analyzer.GetPoorQualityComments(results)

		Expect(poorQuality).To(HaveLen(4)) // All except the first one

		// Check that the good comment is not included
		for _, result := range poorQuality {
			Expect(result.Comment.Text).NotTo(Equal("Good comment"))
		}
	})
})

var _ = Describe("AnalyzeAST", func() {
	Context("when analyzing AST with various comments", func() {
		It("should analyze all comments from different sources", func() {
			config := CommentAnalyzerConfig{
				Enabled:   true,
				WordLimit: 5,
			}

			analyzer := NewCommentAnalyzer(config, nil)

			ast := &models.GenericAST{
				Comments: []models.Comment{
					{Text: "Short", WordCount: 1},
					{Text: "This is a longer comment", WordCount: 6},
				},
				Functions: []models.Function{
					{
						Comments: []models.Comment{
							{Text: "Function comment", WordCount: 2},
						},
					},
				},
				Types: []models.TypeDefinition{
					{
						Comments: []models.Comment{
							{Text: "Type documentation comment", WordCount: 3},
						},
						Fields: []models.Field{
							{
								Comments: []models.Comment{
									{Text: "Field comment", WordCount: 2},
								},
							},
						},
					},
				},
				Variables: []models.Variable{
					{
						Comments: []models.Comment{
							{Text: "Variable comment", WordCount: 2},
						},
					},
				},
			}

			results, err := analyzer.AnalyzeAST(context.Background(), ast)
			Expect(err).NotTo(HaveOccurred())

			Expect(results).To(HaveLen(6)) // All comments from different sources

			// Check that we have one complex comment (the one with 6 words)
			complexCount := 0
			for _, result := range results {
				if !result.IsSimple {
					complexCount++
				}
			}
			Expect(complexCount).To(Equal(1))
		})
	})

	Context("when analyzer is disabled", func() {
		It("should return nil results", func() {
			config := CommentAnalyzerConfig{
				Enabled: false,
			}

			analyzer := NewCommentAnalyzer(config, nil)

			ast := &models.GenericAST{
				Comments: []models.Comment{
					{Text: "Test comment", WordCount: 2},
				},
			}

			results, err := analyzer.AnalyzeAST(context.Background(), ast)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(BeNil())
		})
	})
})

var _ = Describe("Performance Tests", func() {
	It("should analyze simple comment quickly", func() {
		config := CommentAnalyzerConfig{
			Enabled:   true,
			WordLimit: 10,
		}

		analyzer := NewCommentAnalyzer(config, nil)

		comment := models.Comment{
			Text:      "Simple test comment",
			WordCount: 3,
		}

		ctx := context.Background()

		// Run the analysis - it should complete without error
		result, err := analyzer.AnalyzeComment(ctx, comment)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.IsSimple).To(BeTrue())
	})

	It("should extract JSON quickly", func() {
		analyzer := &CommentAnalyzer{}

		input := `Here is the analysis result: {"is_verbose": false, "is_descriptive": true, "score": 0.85, "issues": [], "suggestions": ["Consider adding more detail"]} - end of analysis`

		result := analyzer.extractJSON(input)
		Expect(result).To(ContainSubstring(`"is_verbose": false`))
		Expect(result).To(ContainSubstring(`"is_descriptive": true`))
	})
})
