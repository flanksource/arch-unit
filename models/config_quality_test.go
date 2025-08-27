package models

import (
	"testing"
)

func TestQualityConfigApplyDefaults(t *testing.T) {
	config := &QualityConfig{}
	config.ApplyDefaults()
	
	// Test that all defaults are applied
	if config.MaxFileLength != 400 {
		t.Errorf("Expected default max file length 400, got %d", config.MaxFileLength)
	}
	
	if config.MaxFunctionNameLen != 50 {
		t.Errorf("Expected default max function name length 50, got %d", config.MaxFunctionNameLen)
	}
	
	if config.MaxVariableNameLen != 30 {
		t.Errorf("Expected default max variable name length 30, got %d", config.MaxVariableNameLen)
	}
	
	if config.MaxParameterNameLen != 25 {
		t.Errorf("Expected default max parameter name length 25, got %d", config.MaxParameterNameLen)
	}
	
	if config.CommentAnalysis.WordLimit != 10 {
		t.Errorf("Expected default comment word limit 10, got %d", config.CommentAnalysis.WordLimit)
	}
	
	if config.CommentAnalysis.AIModel != "claude-3-haiku-20240307" {
		t.Errorf("Expected default AI model 'claude-3-haiku-20240307', got %q", config.CommentAnalysis.AIModel)
	}
	
	if config.CommentAnalysis.MinDescriptiveScore != 0.7 {
		t.Errorf("Expected default min descriptive score 0.7, got %f", config.CommentAnalysis.MinDescriptiveScore)
	}
}

func TestQualityConfigApplyDefaultsWithExistingValues(t *testing.T) {
	config := &QualityConfig{
		MaxFileLength:       500,
		MaxFunctionNameLen:  60,
		MaxVariableNameLen:  40,
		MaxParameterNameLen: 35,
		CommentAnalysis: CommentAnalysisConfig{
			WordLimit:           15,
			AIModel:             "custom-model",
			MinDescriptiveScore: 0.8,
		},
	}
	
	config.ApplyDefaults()
	
	// Test that existing values are preserved
	if config.MaxFileLength != 500 {
		t.Errorf("Expected preserved max file length 500, got %d", config.MaxFileLength)
	}
	
	if config.MaxFunctionNameLen != 60 {
		t.Errorf("Expected preserved max function name length 60, got %d", config.MaxFunctionNameLen)
	}
	
	if config.MaxVariableNameLen != 40 {
		t.Errorf("Expected preserved max variable name length 40, got %d", config.MaxVariableNameLen)
	}
	
	if config.MaxParameterNameLen != 35 {
		t.Errorf("Expected preserved max parameter name length 35, got %d", config.MaxParameterNameLen)
	}
	
	if config.CommentAnalysis.WordLimit != 15 {
		t.Errorf("Expected preserved comment word limit 15, got %d", config.CommentAnalysis.WordLimit)
	}
	
	if config.CommentAnalysis.AIModel != "custom-model" {
		t.Errorf("Expected preserved AI model 'custom-model', got %q", config.CommentAnalysis.AIModel)
	}
	
	if config.CommentAnalysis.MinDescriptiveScore != 0.8 {
		t.Errorf("Expected preserved min descriptive score 0.8, got %f", config.CommentAnalysis.MinDescriptiveScore)
	}
}

func TestQualityConfigGetDisallowedNamePatterns(t *testing.T) {
	// Test nil config
	var nilConfig *QualityConfig
	patterns := nilConfig.GetDisallowedNamePatterns()
	if patterns != nil {
		t.Errorf("Expected nil patterns for nil config, got %v", patterns)
	}
	
	// Test config with patterns
	config := &QualityConfig{
		DisallowedNames: []DisallowedNamePattern{
			{Pattern: "temp*", Reason: "Temporary names are not descriptive"},
			{Pattern: "*Manager", Reason: "Manager suffix is overused"},
			{Pattern: "data*"},
		},
	}
	
	patterns = config.GetDisallowedNamePatterns()
	expected := []string{"temp*", "*Manager", "data*"}
	
	if len(patterns) != len(expected) {
		t.Errorf("Expected %d patterns, got %d", len(expected), len(patterns))
	}
	
	for i, pattern := range patterns {
		if pattern != expected[i] {
			t.Errorf("Expected pattern %q at index %d, got %q", expected[i], i, pattern)
		}
	}
}

func TestQualityConfigIsNameDisallowed(t *testing.T) {
	// Test nil config
	var nilConfig *QualityConfig
	disallowed, reason := nilConfig.IsNameDisallowed("anyName")
	if disallowed {
		t.Errorf("Expected name not to be disallowed for nil config")
	}
	if reason != "" {
		t.Errorf("Expected empty reason for nil config, got %q", reason)
	}
	
	// Test config with patterns
	config := &QualityConfig{
		DisallowedNames: []DisallowedNamePattern{
			{Pattern: "temp*", Reason: "Temporary names are not descriptive"},
			{Pattern: "*Manager", Reason: "Manager suffix is overused"},
			{Pattern: "data*"}, // No reason provided
		},
	}
	
	testCases := []struct {
		name           string
		input          string
		expectedBanned bool
		expectedReason string
	}{
		{
			name:           "allowed name",
			input:          "goodFunctionName",
			expectedBanned: false,
			expectedReason: "",
		},
		{
			name:           "temp prefix disallowed with reason",
			input:          "tempVariable",
			expectedBanned: true,
			expectedReason: "Temporary names are not descriptive",
		},
		{
			name:           "Manager suffix disallowed with reason",
			input:          "UserManager",
			expectedBanned: true,
			expectedReason: "Manager suffix is overused",
		},
		{
			name:           "data prefix disallowed without reason",
			input:          "dataProcessor",
			expectedBanned: true,
			expectedReason: "matches disallowed pattern: data*",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			disallowed, reason := config.IsNameDisallowed(tc.input)
			
			if disallowed != tc.expectedBanned {
				t.Errorf("IsNameDisallowed(%q) = %v, expected %v", tc.input, disallowed, tc.expectedBanned)
			}
			
			if reason != tc.expectedReason {
				t.Errorf("IsNameDisallowed(%q) reason = %q, expected %q", tc.input, reason, tc.expectedReason)
			}
		})
	}
}

func TestQualityConfigValidateFileLength(t *testing.T) {
	// Test nil config
	var nilConfig *QualityConfig
	valid, message := nilConfig.ValidateFileLength(1000)
	if !valid {
		t.Errorf("Expected nil config to allow any file length")
	}
	if message != "" {
		t.Errorf("Expected empty message for nil config, got %q", message)
	}
	
	// Test config with zero limit (disabled)
	config := &QualityConfig{MaxFileLength: 0}
	valid, message = config.ValidateFileLength(1000)
	if !valid {
		t.Errorf("Expected zero limit to allow any file length")
	}
	if message != "" {
		t.Errorf("Expected empty message for zero limit, got %q", message)
	}
	
	// Test config with limit
	config = &QualityConfig{MaxFileLength: 100}
	
	testCases := []struct {
		name        string
		lineCount   int
		expectValid bool
	}{
		{
			name:        "within limit",
			lineCount:   50,
			expectValid: true,
		},
		{
			name:        "at limit",
			lineCount:   100,
			expectValid: true,
		},
		{
			name:        "exceeds limit",
			lineCount:   150,
			expectValid: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid, message := config.ValidateFileLength(tc.lineCount)
			
			if valid != tc.expectValid {
				t.Errorf("ValidateFileLength(%d) = %v, expected %v", tc.lineCount, valid, tc.expectValid)
			}
			
			if !tc.expectValid && message == "" {
				t.Errorf("Expected error message for invalid file length")
			}
			
			if tc.expectValid && message != "" {
				t.Errorf("Expected no error message for valid file length, got %q", message)
			}
		})
	}
}

func TestQualityConfigValidateNameLengths(t *testing.T) {
	config := &QualityConfig{
		MaxFunctionNameLen:  20,
		MaxVariableNameLen:  15,
		MaxParameterNameLen: 10,
	}
	
	testCases := []struct {
		name        string
		validator   func(string) (bool, string)
		input       string
		expectValid bool
	}{
		{
			name:        "valid function name",
			validator:   config.ValidateFunctionNameLength,
			input:       "shortFunc",
			expectValid: true,
		},
		{
			name:        "function name at limit",
			validator:   config.ValidateFunctionNameLength,
			input:       "exactlyTwentyCharact", // 20 chars
			expectValid: true,
		},
		{
			name:        "function name too long",
			validator:   config.ValidateFunctionNameLength,
			input:       "thisIsAVeryLongFunctionNameThatExceedsTheLimit",
			expectValid: false,
		},
		{
			name:        "valid variable name",
			validator:   config.ValidateVariableNameLength,
			input:       "shortVar",
			expectValid: true,
		},
		{
			name:        "variable name at limit",
			validator:   config.ValidateVariableNameLength,
			input:       "fifteenCharName", // 15 chars
			expectValid: true,
		},
		{
			name:        "variable name too long",
			validator:   config.ValidateVariableNameLength,
			input:       "thisIsAVeryLongVariableNameThatExceedsTheLimit",
			expectValid: false,
		},
		{
			name:        "valid parameter name",
			validator:   config.ValidateParameterNameLength,
			input:       "shortParam",
			expectValid: true,
		},
		{
			name:        "parameter name at limit",
			validator:   config.ValidateParameterNameLength,
			input:       "tenCharPar", // 10 chars
			expectValid: true,
		},
		{
			name:        "parameter name too long",
			validator:   config.ValidateParameterNameLength,
			input:       "veryLongParameterName",
			expectValid: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid, message := tc.validator(tc.input)
			
			if valid != tc.expectValid {
				t.Errorf("Validator(%q) = %v, expected %v", tc.input, valid, tc.expectValid)
			}
			
			if !tc.expectValid && message == "" {
				t.Errorf("Expected error message for invalid name")
			}
			
			if tc.expectValid && message != "" {
				t.Errorf("Expected no error message for valid name, got %q", message)
			}
		})
	}
}

func TestConfigGetQualityConfig(t *testing.T) {
	config := &Config{
		Rules: map[string]RuleConfig{
			"**/*.go": {
				Quality: &QualityConfig{
					MaxFileLength:      300,
					MaxFunctionNameLen: 40,
				},
			},
			"**/test*.go": {
				Quality: &QualityConfig{
					MaxFileLength:      500, // More lenient for tests
					MaxFunctionNameLen: 60,
				},
			},
		},
	}
	
	// Test getting config for regular Go file
	qualityConfig := config.GetQualityConfig("src/main.go")
	if qualityConfig == nil {
		t.Fatalf("Expected quality config for Go file, got nil")
	}
	
	if qualityConfig.MaxFileLength != 300 {
		t.Errorf("Expected max file length 300, got %d", qualityConfig.MaxFileLength)
	}
	
	if qualityConfig.MaxFunctionNameLen != 40 {
		t.Errorf("Expected max function name length 40, got %d", qualityConfig.MaxFunctionNameLen)
	}
	
	// Defaults should be applied
	if qualityConfig.MaxVariableNameLen != 30 {
		t.Errorf("Expected default max variable name length 30, got %d", qualityConfig.MaxVariableNameLen)
	}
	
	// Test getting config for test file (should override with more lenient rules)
	testConfig := config.GetQualityConfig("src/test_main.go")
	if testConfig == nil {
		t.Fatalf("Expected quality config for test file, got nil")
	}
	
	if testConfig.MaxFileLength != 500 {
		t.Errorf("Expected test max file length 500, got %d", testConfig.MaxFileLength)
	}
	
	if testConfig.MaxFunctionNameLen != 60 {
		t.Errorf("Expected test max function name length 60, got %d", testConfig.MaxFunctionNameLen)
	}
}

func TestConfigIsQualityEnabled(t *testing.T) {
	config := &Config{
		Rules: map[string]RuleConfig{
			"**/*.go": {
				Quality: &QualityConfig{
					MaxFileLength: 400,
				},
			},
		},
	}
	
	// Test for file with quality config
	if !config.IsQualityEnabled("src/main.go") {
		t.Errorf("Expected quality to be enabled for Go file")
	}
	
	// Test for file without quality config
	if config.IsQualityEnabled("README.md") {
		t.Errorf("Expected quality to be disabled for non-Go file")
	}
}

func BenchmarkIsNameDisallowed(b *testing.B) {
	config := &QualityConfig{
		DisallowedNames: []DisallowedNamePattern{
			{Pattern: "temp*"},
			{Pattern: "*Manager"},
			{Pattern: "test*"},
			{Pattern: "data*"},
			{Pattern: "*Util"},
		},
	}
	
	name := "averageFunctionName"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.IsNameDisallowed(name)
	}
}

func BenchmarkValidateFileLength(b *testing.B) {
	config := &QualityConfig{
		MaxFileLength: 400,
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.ValidateFileLength(350)
	}
}