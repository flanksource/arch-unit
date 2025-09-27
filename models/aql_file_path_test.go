package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAQLPatternFilePath(t *testing.T) {
	testCases := []struct {
		name           string
		pattern        string
		expectedFilePath string
		expectedAST      string
		expectedError    bool
	}{
		{
			name:             "Simple file path with @ prefix",
			pattern:          "@src/**/*.go",
			expectedFilePath: "src/**/*.go",
			expectedAST:      "*",
		},
		{
			name:             "File path with AST pattern using @ prefix",
			pattern:          "@src/**/*.go:Service*",
			expectedFilePath: "src/**/*.go",
			expectedAST:      "Service*",
		},
		{
			name:             "Path function with no AST pattern",
			pattern:          "path(internal/**/*.go)",
			expectedFilePath: "internal/**/*.go",
			expectedAST:      "*",
		},
		{
			name:             "Path function with AST pattern",
			pattern:          "path(src/**/*.go) AND Service*",
			expectedFilePath: "src/**/*.go",
			expectedAST:      "Service*",
		},
		{
			name:             "Complex pattern with file path and package:type:method",
			pattern:          "@controllers/**/*.go:api:UserService:GetUser",
			expectedFilePath: "controllers/**/*.go",
			expectedAST:      "api:UserService:GetUser",
		},
		{
			name:             "File path with metric",
			pattern:          "@src/**/*.go:Service*.cyclomatic",
			expectedFilePath: "src/**/*.go",
			expectedAST:      "Service*.cyclomatic",
		},
		{
			name:          "Invalid path function syntax",
			pattern:       "path(missing_close_paren",
			expectedError: true,
		},
		{
			name:          "Invalid path function with bad AND syntax",
			pattern:       "path(src/**/*.go) INVALID Service*",
			expectedError: true,
		},
		{
			name:        "No file path pattern",
			pattern:     "Service*:GetUser",
			expectedAST: "Service*:GetUser",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := ParsePattern(tc.pattern)

			if tc.expectedError {
				assert.Error(t, err, "Expected error for pattern: %s", tc.pattern)
				return
			}

			require.NoError(t, err, "Failed to parse pattern: %s", tc.pattern)
			require.NotNil(t, pattern, "Pattern should not be nil")

			assert.Equal(t, tc.expectedFilePath, pattern.FilePath, "FilePath mismatch for pattern: %s", tc.pattern)

			// Verify the AST pattern was parsed correctly by checking the resulting String representation
			if tc.expectedAST != "" {
				// Create a pattern with just the AST part to compare
				astOnlyPattern, err := ParsePattern(tc.expectedAST)
				require.NoError(t, err)

				// Compare the AST components
				assert.Equal(t, astOnlyPattern.Package, pattern.Package, "Package mismatch")
				assert.Equal(t, astOnlyPattern.Type, pattern.Type, "Type mismatch")
				assert.Equal(t, astOnlyPattern.Method, pattern.Method, "Method mismatch")
				assert.Equal(t, astOnlyPattern.Field, pattern.Field, "Field mismatch")
				assert.Equal(t, astOnlyPattern.Metric, pattern.Metric, "Metric mismatch")
			}
		})
	}
}

func TestMatchesFilePath(t *testing.T) {
	testCases := []struct {
		name     string
		filePath string
		pattern  string
		expected bool
	}{
		{
			name:     "Exact match",
			filePath: "/project/src/main.go",
			pattern:  "/project/src/main.go",
			expected: true,
		},
		{
			name:     "Simple wildcard match",
			filePath: "/project/src/main.go",
			pattern:  "*",
			expected: true,
		},
		{
			name:     "Doublestar match with directory",
			filePath: "/project/src/controllers/user.go",
			pattern:  "**/controllers/*.go",
			expected: true,
		},
		{
			name:     "Doublestar match at start",
			filePath: "/project/deep/nested/path/file.go",
			pattern:  "**/file.go",
			expected: true,
		},
		{
			name:     "Extension match",
			filePath: "/project/src/main.go",
			pattern:  "**/*.go",
			expected: true,
		},
		{
			name:     "No match different extension",
			filePath: "/project/src/main.py",
			pattern:  "**/*.go",
			expected: false,
		},
		{
			name:     "Directory specific match",
			filePath: "/project/src/controllers/user.go",
			pattern:  "src/**/*.go",
			expected: false, // Should fail because absolute path doesn't start with "src"
		},
		{
			name:     "Basename match fallback",
			filePath: "/project/src/controllers/user.go",
			pattern:  "user.go",
			expected: true, // Should match basename
		},
		{
			name:     "Complex pattern with directory structure",
			filePath: "/project/internal/services/user_service.go",
			pattern:  "internal/**/*_service.go",
			expected: false, // Should fail because absolute path doesn't start with "internal"
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := matchesFilePath(tc.filePath, tc.pattern)
			assert.Equal(t, tc.expected, result, "Unexpected result for filePath=%s pattern=%s", tc.filePath, tc.pattern)
		})
	}
}

func TestAQLPatternMatches(t *testing.T) {
	testCases := []struct {
		name     string
		pattern  string
		node     *ASTNode
		expected bool
	}{
		{
			name:    "File path and AST pattern both match",
			pattern: "@**/*.go:Service*",
			node: &ASTNode{
				FilePath: "/project/src/user_service.go",
				TypeName: "ServiceImpl",
			},
			expected: true,
		},
		{
			name:    "File path matches but AST pattern doesn't",
			pattern: "@**/*.go:Controller*",
			node: &ASTNode{
				FilePath: "/project/src/user_service.go",
				TypeName: "ServiceImpl",
			},
			expected: false,
		},
		{
			name:    "File path doesn't match but AST pattern does",
			pattern: "@**/*.py:Service*",
			node: &ASTNode{
				FilePath: "/project/src/user_service.go",
				TypeName: "ServiceImpl",
			},
			expected: false,
		},
		{
			name:    "Only AST pattern specified - should match",
			pattern: "Service*",
			node: &ASTNode{
				FilePath: "/project/src/user_service.go",
				TypeName: "ServiceImpl",
			},
			expected: true,
		},
		{
			name:    "Only file path pattern specified - should match",
			pattern: "@**/*.go",
			node: &ASTNode{
				FilePath: "/project/src/user_service.go",
				TypeName: "ServiceImpl",
			},
			expected: true,
		},
		{
			name:    "Complex pattern with package, type, method and file path",
			pattern: "@**/*.go:api:UserController:GetUser",
			node: &ASTNode{
				FilePath:    "/project/controllers/api/user.go",
				PackageName: "api",
				TypeName:    "UserController",
				MethodName:  "GetUser",
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := ParsePattern(tc.pattern)
			require.NoError(t, err, "Failed to parse pattern: %s", tc.pattern)

			result := pattern.Matches(tc.node)
			assert.Equal(t, tc.expected, result, "Unexpected match result for pattern=%s", tc.pattern)
		})
	}
}

func TestAQLPatternString(t *testing.T) {
	testCases := []struct {
		name     string
		pattern  *AQLPattern
		expected string
	}{
		{
			name: "File path with AST pattern",
			pattern: &AQLPattern{
				FilePath: "src/**/*.go",
				Package:  "api",
				Type:     "UserService",
			},
			expected: "@src/**/*.go:api:UserService",
		},
		{
			name: "Only file path",
			pattern: &AQLPattern{
				FilePath: "**/*.go",
				Package:  "*",
				Type:     "*",
				Method:   "*",
			},
			expected: "@**/*.go:*",
		},
		{
			name: "File path with metric",
			pattern: &AQLPattern{
				FilePath: "src/**/*.go",
				Package:  "api",
				Type:     "UserService",
				Metric:   "cyclomatic",
			},
			expected: "@src/**/*.go:api:UserService.cyclomatic",
		},
		{
			name: "Original pattern takes precedence",
			pattern: &AQLPattern{
				Original: "@original/pattern",
				FilePath: "different/path",
				Package:  "pkg",
			},
			expected: "@original/pattern",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.pattern.String()
			assert.Equal(t, tc.expected, result, "Unexpected string representation")
		})
	}
}