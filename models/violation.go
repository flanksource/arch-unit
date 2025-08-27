package models

import (
	"fmt"
	"time"
)

type Violation struct {
	File              string    `json:"file,omitempty"`
	Line              int       `json:"line,omitempty"`
	Column            int       `json:"column,omitempty"`
	CallerPackage     string    `json:"caller_package,omitempty"`
	CallerMethod      string    `json:"caller_method,omitempty"`
	CalledPackage     string    `json:"called_package,omitempty"`
	CalledMethod      string    `json:"called_method,omitempty"`
	Rule              *Rule     `json:"rule,omitempty"`
	Message           string    `json:"message,omitempty"`
	Source            string    `json:"source,omitempty"` // Source tool that reported the violation (e.g., arch-unit, golangci-lint)
	Fixable           bool      `json:"fixable,omitempty"`
	FixApplicability  string    `json:"fix_applicability,omitempty"`
	CreatedAt         time.Time `json:"created_at,omitempty"`
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
