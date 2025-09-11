package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/flanksource/clicky/api"
)

type Violation struct {
	ID     uint     `json:"id" gorm:"primaryKey;autoIncrement"`
	File   string   `json:"file,omitempty" gorm:"column:file_path;not null;index"`
	Line   int      `json:"line,omitempty" gorm:"column:line;not null"`
	Column int      `json:"column,omitempty" gorm:"column:column;not null"`
	
	// Foreign keys to ASTNode
	CallerID *int64   `json:"-" gorm:"column:caller_id;index"`
	Caller   *ASTNode `json:"caller,omitempty" gorm:"foreignKey:CallerID;references:ID"`
	
	CalledID *int64   `json:"-" gorm:"column:called_id;index"`
	Called   *ASTNode `json:"called,omitempty" gorm:"foreignKey:CalledID;references:ID"`
	
	// The line of code the violation was found on.
	Code    string `json:"code,omitempty" gorm:"column:code"`
	Rule    *Rule  `json:"rule,omitempty" gorm:"column:rule_json;serializer:json"`
	Message string `json:"message,omitempty" gorm:"column:message"`
	// Source tool that reported the violation (e.g., arch-unit, golangci-lint)
	Source           string    `json:"source,omitempty" gorm:"column:source;not null;index"`
	Fixable          bool      `json:"fixable,omitempty" gorm:"column:fixable;default:false"`
	FixApplicability string    `json:"fix_applicability,omitempty" gorm:"column:fix_applicability;default:''"`
	CreatedAt        time.Time `json:"created_at,omitempty" gorm:"column:stored_at;index"`
}

// TableName specifies the table name for Violation
func (Violation) TableName() string {
	return "violations"
}

func (v Violation) String() string {
	return v.Pretty().String()
}

// Pretty returns a formatted text representation of the violation with styling
func (v Violation) Pretty() api.Text {
	// Use only filename for display since directory structure is shown in tree
	// Format: Type.method():L123 → fmt.Println, ⇥ fmt.Println("hello world")

	var t api.Text
	if v.Caller != nil {
		t = v.Caller.PrettyShort().Append(":", "text-gray-500").Append(strconv.Itoa(v.Line))
	} else {
		t = api.Text{}.Append("unknown", "text-gray-500").Append(":", "text-gray-500").Append(strconv.Itoa(v.Line))
	}

	if v.Called != nil {
		t = t.Append("→", "text-red-600").Add(v.Called.PrettyShort())
	}

	// Add code snippet if available
	if v.Code != "" {
		t = t.Append(", ⇥ ", "text-gray-400").Append(strings.TrimSpace(v.Code), "text-blue-500")
	}

	return t.Append(" (").Add(v.Rule.Pretty()).Append(")")
}

// ViolationNode represents an individual violation as a tree node
type ViolationNode struct {
	violation Violation
}

// Tree returns a tree node representation of the violation
func (v Violation) Tree() api.TreeNode {
	return &ViolationTreeNode{violation: v}
}

func (vn *ViolationNode) Tree() api.TreeNode {
	return &ViolationTreeNode{violation: vn.violation}
}

// ViolationTreeNode is a TreeNode wrapper for individual violations
type ViolationTreeNode struct {
	violation Violation
}

func (vt *ViolationTreeNode) Pretty() api.Text {
	return vt.violation.Pretty()
}

func (vt *ViolationTreeNode) GetChildren() []api.TreeNode {
	return nil // Leaf node
}

// MarshalJSON implements custom JSON marshaling for Violation to use relative file paths
func (v Violation) MarshalJSON() ([]byte, error) {
	// Convert file path to relative path for JSON output
	displayFile := v.File
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, v.File); err == nil && !strings.HasPrefix(rel, "../") {
			displayFile = rel
		}
	}

	// Create alias type to avoid infinite recursion
	type ViolationAlias Violation
	return json.Marshal(&struct {
		File string `json:"file,omitempty"`
		*ViolationAlias
	}{
		File:           displayFile,
		ViolationAlias: (*ViolationAlias)(&v),
	})
}

type AnalysisResult struct {
	Violations []Violation `json:"violations,omitempty"`
	FileCount  int         `json:"file_count,omitempty"`
	RuleCount  int         `json:"rule_count,omitempty"`
}
