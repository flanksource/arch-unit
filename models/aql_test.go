package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected *AQLPattern
		wantErr  bool
	}{
		{
			name:    "Package with slash",
			pattern: "internal/controllers",
			expected: &AQLPattern{
				Package:    "internal/controllers",
				Original:   "internal/controllers",
				IsWildcard: false,
			},
		},
		{
			name:    "Single method name",
			pattern: "GetUser",
			expected: &AQLPattern{
				Package:    "*",
				Type:       "*",
				Method:     "GetUser",
				Original:   "GetUser",
				IsWildcard: false,
			},
		},
		{
			name:    "Single method with lowercase",
			pattern: "processData",
			expected: &AQLPattern{
				Package:    "*",
				Type:       "*",
				Method:     "processData",
				Original:   "processData",
				IsWildcard: false,
			},
		},
		{
			name:    "Single type name",
			pattern: "UserController",
			expected: &AQLPattern{
				Package:    "*",
				Type:       "UserController",
				Original:   "UserController",
				IsWildcard: false,
			},
		},
		{
			name:    "Type:Method shorthand",
			pattern: "UserController:GetUser",
			expected: &AQLPattern{
				Package:    "*",
				Type:       "UserController",
				Method:     "GetUser",
				Original:   "UserController:GetUser",
				IsWildcard: false,
			},
		},
		{
			name:    "Dot notation for package.Type",
			pattern: "widgets.Table",
			expected: &AQLPattern{
				Package:    "widgets",
				Type:       "Table",
				Original:   "widgets.Table",
				IsWildcard: false,
			},
		},
		{
			name:    "Colon notation for package:Type",
			pattern: "widgets:Table",
			expected: &AQLPattern{
				Package:    "widgets",
				Type:       "Table",
				Original:   "widgets:Table",
				IsWildcard: false,
			},
		},
		{
			name:    "Metric pattern with dot",
			pattern: "*.cyclomatic",
			expected: &AQLPattern{
				Package:    "*",
				Metric:     "cyclomatic",
				Original:   "*.cyclomatic",
				IsWildcard: true,
			},
		},
		{
			name:    "Complex pattern with metric",
			pattern: "controllers:UserController:Get*.lines",
			expected: &AQLPattern{
				Package:    "controllers",
				Type:       "UserController",
				Method:     "Get*",
				Metric:     "lines",
				Original:   "controllers:UserController:Get*.lines",
				IsWildcard: true,
			},
		},
		{
			name:    "Dot notation with method",
			pattern: "widgets.Table.draw",
			expected: &AQLPattern{
				Package:    "widgets",
				Type:       "Table",
				Method:     "draw",
				Original:   "widgets.Table.draw",
				IsWildcard: false,
			},
		},
		{
			name:    "Package:Type:Method:Field",
			pattern: "models:User:GetName:id",
			expected: &AQLPattern{
				Package:    "models",
				Type:       "User",
				Method:     "GetName",
				Field:      "id",
				Original:   "models:User:GetName:id",
				IsWildcard: false,
			},
		},
		{
			name:    "Type with wildcard suffix",
			pattern: "*Service",
			expected: &AQLPattern{
				Package:    "*",
				Type:       "*Service",
				Original:   "*Service",
				IsWildcard: true,
			},
		},
		{
			name:    "Method with wildcard prefix",
			pattern: "Get*",
			expected: &AQLPattern{
				Package:    "*",
				Type:       "*",
				Method:     "Get*",
				Original:   "Get*",
				IsWildcard: true,
			},
		},
		{
			name:    "Traditional package:type",
			pattern: "controllers:UserController",
			expected: &AQLPattern{
				Package:    "controllers",
				Type:       "UserController",
				Original:   "controllers:UserController",
				IsWildcard: false,
			},
		},
		{
			name:    "Full pattern with all parts",
			pattern: "controllers:UserController:GetUser:id",
			expected: &AQLPattern{
				Package:    "controllers",
				Type:       "UserController",
				Method:     "GetUser",
				Field:      "id",
				Original:   "controllers:UserController:GetUser:id",
				IsWildcard: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePattern(tt.pattern)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParsePattern_MetricDetection(t *testing.T) {
	// Test that known metrics are detected with wildcard pattern
	knownMetrics := []string{"cyclomatic", "parameters", "returns", "lines"}
	for _, metric := range knownMetrics {
		// Use wildcard pattern with metric
		pattern := "*." + metric
		result, err := ParsePattern(pattern)
		require.NoError(t, err)
		assert.Equal(t, metric, result.Metric)
		assert.Equal(t, "*", result.Package)
	}

	// Test metrics with package/path patterns
	packagePatterns := []string{"internal/service", "controllers", "models"}
	for _, pkg := range packagePatterns {
		pattern := pkg + ".lines"
		result, err := ParsePattern(pattern)
		require.NoError(t, err)
		assert.Equal(t, "lines", result.Metric)
		assert.Equal(t, pkg, result.Package)
	}

	// Test that unknown words after dot are treated as type names
	unknownWords := []string{"Table", "Widget", "Controller", "Service"}
	for _, word := range unknownWords {
		pattern := "mypackage." + word
		result, err := ParsePattern(pattern)
		require.NoError(t, err)
		assert.Empty(t, result.Metric)
		assert.Equal(t, "mypackage", result.Package)
		assert.Equal(t, word, result.Type)
	}
}

func TestParsePattern_Heuristics(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		expectType   string
		expectMethod string
	}{
		// Method-like patterns
		{"Get prefix", "GetUser", "*", "GetUser"},
		{"Create prefix", "CreateProduct", "*", "CreateProduct"},
		{"lowercase start", "processData", "*", "processData"},
		{"Test prefix", "TestFunction", "*", "TestFunction"},

		// Type-like patterns
		{"Controller suffix", "UserController", "UserController", ""},
		{"Service suffix", "EmailService", "EmailService", ""},
		{"Repository suffix", "UserRepository", "UserRepository", ""},

		// Wildcards
		{"Method wildcard", "Get*", "*", "Get*"},
		{"Type wildcard", "*Controller", "*Controller", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePattern(tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.expectType, result.Type)
			assert.Equal(t, tt.expectMethod, result.Method)
			assert.Equal(t, "*", result.Package) // Should default to wildcard
		})
	}
}
