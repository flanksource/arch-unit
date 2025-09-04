package models

import (
	"fmt"
	"time"
)

type Violation struct {
	ID               uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	File             string    `json:"file,omitempty" gorm:"column:file_path;not null;index"`
	Line             int       `json:"line,omitempty" gorm:"column:line;not null"`
	Column           int       `json:"column,omitempty" gorm:"column:column;not null"`
	CallerPackage    string    `json:"caller_package,omitempty" gorm:"column:caller_package"`
	CallerMethod     string    `json:"caller_method,omitempty" gorm:"column:caller_method"`
	CalledPackage    string    `json:"called_package,omitempty" gorm:"column:called_package"`
	CalledMethod     string    `json:"called_method,omitempty" gorm:"column:called_method"`
	Rule             *Rule     `json:"rule,omitempty" gorm:"serializer:json"`
	Message          string    `json:"message,omitempty" gorm:"column:message"`
	Source           string    `json:"source,omitempty" gorm:"column:source;not null;index"` // Source tool that reported the violation (e.g., arch-unit, golangci-lint)
	Fixable          bool      `json:"fixable,omitempty" gorm:"column:fixable;default:false"`
	FixApplicability string    `json:"fix_applicability,omitempty" gorm:"column:fix_applicability;default:''"`
	CreatedAt        time.Time `json:"created_at,omitempty" gorm:"column:stored_at;index"`
}

// TableName specifies the table name for Violation
func (Violation) TableName() string {
	return "violations"
}

func (v Violation) String() string {
	location := fmt.Sprintf("%s:%d:%d", v.File, v.Line, v.Column)
	call := fmt.Sprintf("%s.%s", v.CalledPackage, v.CalledMethod)
	if v.CalledMethod == "" {
		call = v.CalledPackage
	}

	ruleInfo := ""
	if v.Rule != nil {
		ruleInfo = fmt.Sprintf(" (rule: %s in %s:%d)", v.Rule.OriginalLine, v.Rule.SourceFile, v.Rule.LineNumber)
	}

	return fmt.Sprintf("%s: %s calls forbidden %s%s", location, v.CallerMethod, call, ruleInfo)
}

type AnalysisResult struct {
	Violations []Violation `json:"violations,omitempty"`
	FileCount  int         `json:"file_count,omitempty"`
	RuleCount  int         `json:"rule_count,omitempty"`
}
