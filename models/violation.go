package models

import "fmt"

type Violation struct {
	File          string
	Line          int
	Column        int
	CallerPackage string
	CallerMethod  string
	CalledPackage string
	CalledMethod  string
	Rule          *Rule
	Message       string
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
	Violations []Violation
	FileCount  int
	RuleCount  int
}