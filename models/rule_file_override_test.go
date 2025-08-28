package models

import (
	"testing"
)

func TestRule_AppliesToFile(t *testing.T) {
	tests := []struct {
		name        string
		filePattern string
		filePath    string
		shouldMatch bool
	}{
		{
			name:        "empty pattern matches all files",
			filePattern: "",
			filePath:    "any/file.go",
			shouldMatch: true,
		},
		{
			name:        "exact filename match",
			filePattern: "main.go",
			filePath:    "cmd/app/main.go",
			shouldMatch: true,
		},
		{
			name:        "glob pattern with asterisk",
			filePattern: "*_test.go",
			filePath:    "service/user_test.go",
			shouldMatch: true,
		},
		{
			name:        "glob pattern doesn't match",
			filePattern: "*_test.go",
			filePath:    "service/user.go",
			shouldMatch: false,
		},
		{
			name:        "path pattern with directory",
			filePattern: "cmd/*/main.go",
			filePath:    "cmd/app/main.go",
			shouldMatch: true,
		},
		{
			name:        "path pattern with multiple directories",
			filePattern: "internal/*/service/*.go",
			filePath:    "internal/user/service/handler.go",
			shouldMatch: true,
		},
		{
			name:        "path pattern doesn't match different structure",
			filePattern: "cmd/*/main.go",
			filePath:    "pkg/app/main.go",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Rule{
				FilePattern: tt.filePattern,
			}
			if got := rule.AppliesToFile(tt.filePath); got != tt.shouldMatch {
				t.Errorf("AppliesToFile() = %v, want %v", got, tt.shouldMatch)
			}
		})
	}
}

func TestRuleSet_IsAllowedForFile(t *testing.T) {
	tests := []struct {
		name        string
		rules       []Rule
		pkg         string
		method      string
		filePath    string
		wantAllowed bool
		wantRule    bool
	}{
		{
			name: "file-specific deny rule blocks in matching files",
			rules: []Rule{
				{
					Type:        RuleTypeDeny,
					Package:     "testing",
					FilePattern: "*_service.go",
				},
			},
			pkg:         "testing",
			method:      "T",
			filePath:    "user_service.go",
			wantAllowed: false,
			wantRule:    true,
		},
		{
			name: "file-specific deny rule allows in non-matching files",
			rules: []Rule{
				{
					Type:        RuleTypeDeny,
					Package:     "testing",
					FilePattern: "*_service.go",
				},
			},
			pkg:         "testing",
			method:      "T",
			filePath:    "user_test.go",
			wantAllowed: true,
			wantRule:    false,
		},
		{
			name: "file-specific override allows previously denied",
			rules: []Rule{
				{
					Type:    RuleTypeDeny,
					Package: "fmt",
				},
				{
					Type:        RuleTypeOverride,
					Package:     "fmt",
					FilePattern: "*_test.go",
				},
			},
			pkg:         "fmt",
			method:      "Println",
			filePath:    "user_test.go",
			wantAllowed: true,
			wantRule:    false,
		},
		{
			name: "file-specific override doesn't affect other files",
			rules: []Rule{
				{
					Type:    RuleTypeDeny,
					Package: "fmt",
				},
				{
					Type:        RuleTypeOverride,
					Package:     "fmt",
					FilePattern: "*_test.go",
				},
			},
			pkg:         "fmt",
			method:      "Println",
			filePath:    "user_service.go",
			wantAllowed: false,
			wantRule:    true,
		},
		{
			name: "multiple file patterns with different rules",
			rules: []Rule{
				{
					Type:        RuleTypeDeny,
					Package:     "os",
					FilePattern: "cmd/*/main.go",
				},
				{
					Type:        RuleTypeAllow,
					Package:     "os",
					FilePattern: "cmd/admin/main.go",
				},
			},
			pkg:         "os",
			method:      "Exit",
			filePath:    "cmd/admin/main.go",
			wantAllowed: true,
			wantRule:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := &RuleSet{
				Rules: tt.rules,
			}
			gotAllowed, gotRule := rs.IsAllowedForFile(tt.pkg, tt.method, tt.filePath)
			if gotAllowed != tt.wantAllowed {
				t.Errorf("IsAllowedForFile() allowed = %v, want %v", gotAllowed, tt.wantAllowed)
			}
			if (gotRule != nil) != tt.wantRule {
				t.Errorf("IsAllowedForFile() returned rule = %v, want rule = %v", gotRule != nil, tt.wantRule)
			}
		})
	}
}
