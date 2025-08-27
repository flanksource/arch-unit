package filters

import (
	"testing"

	"github.com/flanksource/arch-unit/models"
)

func TestParser_parseLine_FileSpecific(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantErr      bool
		wantFile     string
		wantPackage  string
		wantMethod   string
		wantType     models.RuleType
	}{
		{
			name:        "file-specific deny rule",
			line:        "[*_test.go] !fmt:Println",
			wantErr:     false,
			wantFile:    "*_test.go",
			wantPackage: "fmt",
			wantMethod:  "Println",
			wantType:    models.RuleTypeDeny,
		},
		{
			name:        "file-specific allow rule",
			line:        "[cmd/*/main.go] os:Exit",
			wantErr:     false,
			wantFile:    "cmd/*/main.go",
			wantPackage: "os",
			wantMethod:  "Exit",
			wantType:    models.RuleTypeAllow,
		},
		{
			name:        "file-specific override rule",
			line:        "[*_test.go] +testing",
			wantErr:     false,
			wantFile:    "*_test.go",
			wantPackage: "",
			wantMethod:  "",
			wantType:    models.RuleTypeOverride,
		},
		{
			name:        "file-specific with complex pattern",
			line:        "[internal/*/service/*.go] !database/sql",
			wantErr:     false,
			wantFile:    "internal/*/service/*.go",
			wantPackage: "",
			wantMethod:  "",
			wantType:    models.RuleTypeDeny,
		},
		{
			name:    "missing closing bracket",
			line:    "[*_test.go fmt:Println",
			wantErr: true,
		},
		{
			name:    "empty rule after pattern",
			line:    "[*_test.go]",
			wantErr: true,
		},
		{
			name:    "empty pattern",
			line:    "[] fmt:Println",
			wantErr: true,
		},
		{
			name:        "file-specific with spaces",
			line:        "[ *_test.go ] fmt:Println",
			wantErr:     false,
			wantFile:    "*_test.go",
			wantPackage: "fmt",
			wantMethod:  "Println",
			wantType:    models.RuleTypeAllow,
		},
	}

	p := NewParser(".")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := p.parseLine(tt.line, "test.ARCHUNIT", 1, ".")
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if rule == nil {
				t.Fatal("parseLine() returned nil rule")
			}
			if rule.FilePattern != tt.wantFile {
				t.Errorf("FilePattern = %v, want %v", rule.FilePattern, tt.wantFile)
			}
			if rule.Package != tt.wantPackage {
				t.Errorf("Package = %v, want %v", rule.Package, tt.wantPackage)
			}
			if rule.Method != tt.wantMethod {
				t.Errorf("Method = %v, want %v", rule.Method, tt.wantMethod)
			}
			if rule.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", rule.Type, tt.wantType)
			}
		})
	}
}