package models

import (
	"testing"
)

func TestNewQualityRule(t *testing.T) {
	testCases := []struct {
		name           string
		ruleType       RuleType
		expectDefaults bool
	}{
		{
			name:           "max file length rule",
			ruleType:       RuleTypeMaxFileLength,
			expectDefaults: true,
		},
		{
			name:           "max name length rule",
			ruleType:       RuleTypeMaxNameLength,
			expectDefaults: true,
		},
		{
			name:           "comment quality rule",
			ruleType:       RuleTypeCommentQuality,
			expectDefaults: true,
		},
		{
			name:           "disallowed name rule",
			ruleType:       RuleTypeDisallowedName,
			expectDefaults: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := NewQualityRule(tc.ruleType)

			if rule.Type != tc.ruleType {
				t.Errorf("Expected rule type %s, got %s", tc.ruleType, rule.Type)
			}

			switch tc.ruleType {
			case RuleTypeMaxFileLength:
				if rule.MaxFileLines != 400 {
					t.Errorf("Expected default max file lines 400, got %d", rule.MaxFileLines)
				}
			case RuleTypeMaxNameLength:
				if rule.MaxNameLength != 50 {
					t.Errorf("Expected default max name length 50, got %d", rule.MaxNameLength)
				}
			case RuleTypeCommentQuality:
				if rule.CommentWordLimit != 10 {
					t.Errorf("Expected default comment word limit 10, got %d", rule.CommentWordLimit)
				}
				if rule.CommentAIModel != "claude-3-haiku-20240307" {
					t.Errorf("Expected default AI model 'claude-3-haiku-20240307', got %q", rule.CommentAIModel)
				}
				if rule.MinDescriptiveScore != 0.7 {
					t.Errorf("Expected default min descriptive score 0.7, got %f", rule.MinDescriptiveScore)
				}
			}
		})
	}
}

func TestQualityRuleValidateFileLength(t *testing.T) {
	rule := NewQualityRule(RuleTypeMaxFileLength)
	rule.MaxFileLines = 100

	testCases := []struct {
		name      string
		lineCount int
		expected  bool
	}{
		{
			name:      "file within limit",
			lineCount: 50,
			expected:  true,
		},
		{
			name:      "file at limit",
			lineCount: 100,
			expected:  true,
		},
		{
			name:      "file exceeds limit",
			lineCount: 150,
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rule.ValidateFileLength(tc.lineCount)
			if result != tc.expected {
				t.Errorf("ValidateFileLength(%d) = %v, expected %v", tc.lineCount, result, tc.expected)
			}
		})
	}
}

func TestQualityRuleValidateNameLength(t *testing.T) {
	rule := NewQualityRule(RuleTypeMaxNameLength)
	rule.MaxNameLength = 20

	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "name within limit",
			input:    "shortName",
			expected: true,
		},
		{
			name:     "name at limit",
			input:    "exactlyTwentyCharact", // 20 chars
			expected: true,
		},
		{
			name:     "name exceeds limit",
			input:    "thisNameIsDefinitelyTooLongForTheLimit", // >20 chars
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rule.ValidateNameLength(tc.input)
			if result != tc.expected {
				t.Errorf("ValidateNameLength(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestQualityRuleValidateDisallowedName(t *testing.T) {
	rule := NewQualityRule(RuleTypeDisallowedName)
	rule.DisallowedPatterns = []string{"temp*", "*Manager", "test*"}

	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "allowed name",
			input:    "goodName",
			expected: true,
		},
		{
			name:     "temp prefix disallowed",
			input:    "tempVariable",
			expected: false,
		},
		{
			name:     "Manager suffix disallowed",
			input:    "UserManager",
			expected: false,
		},
		{
			name:     "test prefix disallowed",
			input:    "testFunc",
			expected: false,
		},
		{
			name:     "case sensitive match",
			input:    "TempVariable", // Capital T
			expected: true,           // Should not match "temp*"
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := rule.ValidateDisallowedName(tc.input)
			if result != tc.expected {
				t.Errorf("ValidateDisallowedName(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestQualityRuleGetMethods(t *testing.T) {
	rule := NewQualityRule(RuleTypeCommentQuality)

	// Test default values
	if rule.GetCommentWordLimit() != 10 {
		t.Errorf("Expected default word limit 10, got %d", rule.GetCommentWordLimit())
	}

	if rule.GetCommentAIModel() != "claude-3-haiku-20240307" {
		t.Errorf("Expected default AI model 'claude-3-haiku-20240307', got %q", rule.GetCommentAIModel())
	}

	if rule.GetMinDescriptiveScore() != 0.7 {
		t.Errorf("Expected default min descriptive score 0.7, got %f", rule.GetMinDescriptiveScore())
	}

	// Test custom values
	rule.CommentWordLimit = 15
	rule.CommentAIModel = "custom-model"
	rule.MinDescriptiveScore = 0.8

	if rule.GetCommentWordLimit() != 15 {
		t.Errorf("Expected custom word limit 15, got %d", rule.GetCommentWordLimit())
	}

	if rule.GetCommentAIModel() != "custom-model" {
		t.Errorf("Expected custom AI model 'custom-model', got %q", rule.GetCommentAIModel())
	}

	if rule.GetMinDescriptiveScore() != 0.8 {
		t.Errorf("Expected custom min descriptive score 0.8, got %f", rule.GetMinDescriptiveScore())
	}
}

func TestQualityRuleZeroValues(t *testing.T) {
	rule := &QualityRule{}

	// Test that zero/empty values return defaults
	if rule.GetCommentWordLimit() != 10 {
		t.Errorf("Expected default word limit for zero value, got %d", rule.GetCommentWordLimit())
	}

	if rule.GetCommentAIModel() != "claude-3-haiku-20240307" {
		t.Errorf("Expected default AI model for empty value, got %q", rule.GetCommentAIModel())
	}

	if rule.GetMinDescriptiveScore() != 0.7 {
		t.Errorf("Expected default min descriptive score for zero value, got %f", rule.GetMinDescriptiveScore())
	}
}

func TestNewQualityRuleSet(t *testing.T) {
	path := "/test/path"
	ruleSet := NewQualityRuleSet(path)

	if ruleSet.Path != path {
		t.Errorf("Expected path %q, got %q", path, ruleSet.Path)
	}

	if len(ruleSet.QualityRules) != 0 {
		t.Errorf("Expected empty quality rules, got %d", len(ruleSet.QualityRules))
	}

	if len(ruleSet.Rules) != 0 {
		t.Errorf("Expected empty base rules, got %d", len(ruleSet.Rules))
	}
}

func TestQualityRuleSetAddAndGet(t *testing.T) {
	ruleSet := NewQualityRuleSet("/test")

	// Add different types of rules
	fileRule := NewQualityRule(RuleTypeMaxFileLength)
	nameRule := NewQualityRule(RuleTypeMaxNameLength)
	commentRule := NewQualityRule(RuleTypeCommentQuality)

	ruleSet.AddQualityRule(fileRule)
	ruleSet.AddQualityRule(nameRule)
	ruleSet.AddQualityRule(commentRule)

	// Test total count
	if len(ruleSet.QualityRules) != 3 {
		t.Errorf("Expected 3 quality rules, got %d", len(ruleSet.QualityRules))
	}

	if len(ruleSet.Rules) != 3 {
		t.Errorf("Expected 3 base rules, got %d", len(ruleSet.Rules))
	}

	// Test getting specific types
	fileRules := ruleSet.GetQualityRules(RuleTypeMaxFileLength)
	if len(fileRules) != 1 {
		t.Errorf("Expected 1 file length rule, got %d", len(fileRules))
	}

	nameRules := ruleSet.GetQualityRules(RuleTypeMaxNameLength)
	if len(nameRules) != 1 {
		t.Errorf("Expected 1 name length rule, got %d", len(nameRules))
	}

	// Test getting non-existent type
	disallowedRules := ruleSet.GetQualityRules(RuleTypeDisallowedName)
	if len(disallowedRules) != 0 {
		t.Errorf("Expected 0 disallowed name rules, got %d", len(disallowedRules))
	}
}

func TestQualityRuleSetGetMaxValues(t *testing.T) {
	ruleSet := NewQualityRuleSet("/test")

	// Test with no rules
	if ruleSet.GetMaxFileLength() != 0 {
		t.Errorf("Expected 0 for no file length rules, got %d", ruleSet.GetMaxFileLength())
	}

	if ruleSet.GetMaxNameLength() != 0 {
		t.Errorf("Expected 0 for no name length rules, got %d", ruleSet.GetMaxNameLength())
	}

	// Add rules with custom values
	fileRule := NewQualityRule(RuleTypeMaxFileLength)
	fileRule.MaxFileLines = 200

	nameRule := NewQualityRule(RuleTypeMaxNameLength)
	nameRule.MaxNameLength = 30

	ruleSet.AddQualityRule(fileRule)
	ruleSet.AddQualityRule(nameRule)

	// Test with rules
	if ruleSet.GetMaxFileLength() != 200 {
		t.Errorf("Expected max file length 200, got %d", ruleSet.GetMaxFileLength())
	}

	if ruleSet.GetMaxNameLength() != 30 {
		t.Errorf("Expected max name length 30, got %d", ruleSet.GetMaxNameLength())
	}
}

func TestQualityRuleSetGetCommentQualityRule(t *testing.T) {
	ruleSet := NewQualityRuleSet("/test")

	// Test with no comment quality rule
	if ruleSet.GetCommentQualityRule() != nil {
		t.Errorf("Expected nil for no comment quality rules")
	}

	// Add comment quality rule
	commentRule := NewQualityRule(RuleTypeCommentQuality)
	commentRule.CommentWordLimit = 15

	ruleSet.AddQualityRule(commentRule)

	// Test with rule
	retrievedRule := ruleSet.GetCommentQualityRule()
	if retrievedRule == nil {
		t.Fatalf("Expected comment quality rule, got nil")
	}

	if retrievedRule.CommentWordLimit != 15 {
		t.Errorf("Expected word limit 15, got %d", retrievedRule.CommentWordLimit)
	}
}

func BenchmarkValidateNameLength(b *testing.B) {
	rule := NewQualityRule(RuleTypeMaxNameLength)
	name := "averageLengthFunctionName"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rule.ValidateNameLength(name)
	}
}

func BenchmarkValidateDisallowedName(b *testing.B) {
	rule := NewQualityRule(RuleTypeDisallowedName)
	rule.DisallowedPatterns = []string{"temp*", "*Manager", "test*", "data*", "*Util"}
	name := "averageFunctionName"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rule.ValidateDisallowedName(name)
	}
}
