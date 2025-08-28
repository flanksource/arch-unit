package models

import (
	"fmt"
	"path/filepath"
	"strings"
)

type RuleType string

const (
	RuleTypeAllow          RuleType = "allow"
	RuleTypeDeny           RuleType = "deny"
	RuleTypeOverride       RuleType = "override"
	RuleTypeMaxFileLength  RuleType = "max_file_length"
	RuleTypeMaxNameLength  RuleType = "max_name_length"
	RuleTypeDisallowedName RuleType = "disallowed_name"
	RuleTypeCommentQuality RuleType = "comment_quality"
)

type Rule struct {
	Type         RuleType
	Pattern      string
	Package      string
	Method       string
	SourceFile   string
	LineNumber   int
	Scope        string // Directory where this rule applies
	OriginalLine string
	FilePattern  string // File-specific pattern (e.g., "*_test.go", "cmd/*/main.go")

	// Quality rule parameters
	MaxFileLines        int      `yaml:"max_file_lines,omitempty"`
	MaxNameLength       int      `yaml:"max_name_length,omitempty"`
	DisallowedPatterns  []string `yaml:"disallowed_patterns,omitempty"`
	CommentWordLimit    int      `yaml:"comment_word_limit,omitempty"`
	CommentAIModel      string   `yaml:"comment_ai_model,omitempty"`
	MinDescriptiveScore float64  `yaml:"min_descriptive_score,omitempty"`
}

func (r Rule) String() string {
	prefix := ""
	switch r.Type {
	case RuleTypeDeny:
		prefix = "!"
	case RuleTypeOverride:
		prefix = "+"
	}

	if r.Method != "" {
		return fmt.Sprintf("%s%s:%s", prefix, r.Package, r.Method)
	}
	return fmt.Sprintf("%s%s", prefix, r.Pattern)
}

func (r Rule) Matches(pkg, method string) bool {
	if r.Package != "" {
		if r.Package == "*" || matchesPattern(pkg, r.Package) {
			if r.Method == "" {
				return true
			}
			return matchesPattern(method, r.Method)
		}
		return false
	}

	return matchesPattern(pkg, r.Pattern) || matchesPattern(filepath.ToSlash(pkg), r.Pattern)
}

func (r Rule) AppliesToFile(filePath string) bool {
	if r.FilePattern == "" {
		return true
	}

	// Clean the file path for consistent matching
	cleanPath := filepath.Clean(filePath)

	// Try matching against just the filename
	matched, err := filepath.Match(r.FilePattern, filepath.Base(cleanPath))
	if err == nil && matched {
		return true
	}

	// Try matching against the full path
	matched, err = filepath.Match(r.FilePattern, cleanPath)
	if err == nil && matched {
		return true
	}

	// For patterns with path separators, try different path formats
	if strings.Contains(r.FilePattern, "/") || strings.Contains(r.FilePattern, string(filepath.Separator)) {
		pattern := filepath.ToSlash(r.FilePattern)
		path := filepath.ToSlash(cleanPath)

		// Try direct match
		matched, err = filepath.Match(pattern, path)
		if err == nil && matched {
			return true
		}

		// Try matching against relative path from various common roots
		// This handles cases where the file path might be absolute or have different prefixes
		pathParts := strings.Split(path, "/")
		patternParts := strings.Split(pattern, "/")

		// If pattern is shorter, try to match it against the end of the path
		if len(patternParts) <= len(pathParts) {
			for i := 0; i <= len(pathParts)-len(patternParts); i++ {
				subPath := strings.Join(pathParts[i:], "/")
				matched, err = filepath.Match(pattern, subPath)
				if err == nil && matched {
					return true
				}
			}
		}
	}

	return false
}

func matchesPattern(text, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "!") {
		return !matchesPattern(text, pattern[1:])
	}

	// Handle trailing slash pattern (e.g., "internal/")
	if strings.HasSuffix(pattern, "/") {
		prefix := pattern[:len(pattern)-1]
		return text == prefix || strings.HasPrefix(text, pattern)
	}

	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			hasPrefix := parts[0] == "" || strings.HasPrefix(text, parts[0])
			hasSuffix := parts[1] == "" || strings.HasSuffix(text, parts[1])
			if parts[0] != "" && parts[1] != "" {
				return hasPrefix && hasSuffix && strings.Contains(text, parts[0])
			}
			return hasPrefix && hasSuffix
		}
	}

	return text == pattern || strings.HasPrefix(text, pattern+"/") || strings.HasPrefix(text, pattern+".")
}

type RuleSet struct {
	Rules []Rule
	Path  string
}

func (rs *RuleSet) IsAllowed(pkg, method string) (bool, *Rule) {
	var lastMatchingRule *Rule
	allowed := true

	for i := range rs.Rules {
		rule := &rs.Rules[i]
		if rule.Matches(pkg, method) {
			lastMatchingRule = rule
			switch rule.Type {
			case RuleTypeDeny:
				allowed = false
			case RuleTypeAllow, RuleTypeOverride:
				allowed = true
			}
		}
	}

	if !allowed && lastMatchingRule != nil {
		return false, lastMatchingRule
	}

	return true, nil
}

func (rs *RuleSet) IsAllowedForFile(pkg, method, filePath string) (bool, *Rule) {
	var lastMatchingRule *Rule
	allowed := true

	for i := range rs.Rules {
		rule := &rs.Rules[i]
		// Check if rule applies to this specific file
		if !rule.AppliesToFile(filePath) {
			continue
		}

		if rule.Matches(pkg, method) {
			lastMatchingRule = rule
			switch rule.Type {
			case RuleTypeDeny:
				allowed = false
			case RuleTypeAllow, RuleTypeOverride:
				allowed = true
			}
		}
	}

	if !allowed && lastMatchingRule != nil {
		return false, lastMatchingRule
	}

	return true, nil
}

// QualityRule represents a quality-specific rule with validation methods
type QualityRule struct {
	Rule
}

// NewQualityRule creates a new quality rule with defaults
func NewQualityRule(ruleType RuleType) *QualityRule {
	rule := &QualityRule{
		Rule: Rule{
			Type: ruleType,
		},
	}

	// Set defaults based on rule type
	switch ruleType {
	case RuleTypeMaxFileLength:
		rule.MaxFileLines = 400
	case RuleTypeMaxNameLength:
		rule.MaxNameLength = 50
	case RuleTypeCommentQuality:
		rule.CommentWordLimit = 10
		rule.CommentAIModel = "claude-3-haiku-20240307"
		rule.MinDescriptiveScore = 0.7
	}

	return rule
}

// ValidateFileLength checks if a file exceeds the maximum allowed lines
func (qr *QualityRule) ValidateFileLength(lineCount int) bool {
	if qr.Type != RuleTypeMaxFileLength || qr.MaxFileLines <= 0 {
		return true
	}
	return lineCount <= qr.MaxFileLines
}

// ValidateNameLength checks if a name exceeds the maximum allowed length
func (qr *QualityRule) ValidateNameLength(name string) bool {
	if qr.Type != RuleTypeMaxNameLength || qr.MaxNameLength <= 0 {
		return true
	}
	return len(name) <= qr.MaxNameLength
}

// ValidateDisallowedName checks if a name matches any disallowed patterns
func (qr *QualityRule) ValidateDisallowedName(name string) bool {
	if qr.Type != RuleTypeDisallowedName {
		return true
	}

	for _, pattern := range qr.DisallowedPatterns {
		if matchesPattern(name, pattern) {
			return false
		}
	}
	return true
}

// GetCommentWordLimit returns the word limit for comment analysis
func (qr *QualityRule) GetCommentWordLimit() int {
	if qr.CommentWordLimit <= 0 {
		return 10 // Default
	}
	return qr.CommentWordLimit
}

// GetCommentAIModel returns the AI model to use for comment analysis
func (qr *QualityRule) GetCommentAIModel() string {
	if qr.CommentAIModel == "" {
		return "claude-3-haiku-20240307" // Default low-cost model
	}
	return qr.CommentAIModel
}

// GetMinDescriptiveScore returns the minimum score for descriptive comments
func (qr *QualityRule) GetMinDescriptiveScore() float64 {
	if qr.MinDescriptiveScore <= 0 {
		return 0.7 // Default
	}
	return qr.MinDescriptiveScore
}

// QualityRuleSet extends RuleSet with quality-specific functionality
type QualityRuleSet struct {
	RuleSet
	QualityRules []*QualityRule
}

// NewQualityRuleSet creates a new quality rule set
func NewQualityRuleSet(path string) *QualityRuleSet {
	return &QualityRuleSet{
		RuleSet: RuleSet{
			Path: path,
		},
		QualityRules: []*QualityRule{},
	}
}

// AddQualityRule adds a quality rule to the set
func (qrs *QualityRuleSet) AddQualityRule(rule *QualityRule) {
	qrs.QualityRules = append(qrs.QualityRules, rule)
	qrs.Rules = append(qrs.Rules, rule.Rule)
}

// GetQualityRules returns quality rules of a specific type
func (qrs *QualityRuleSet) GetQualityRules(ruleType RuleType) []*QualityRule {
	var rules []*QualityRule
	for _, rule := range qrs.QualityRules {
		if rule.Type == ruleType {
			rules = append(rules, rule)
		}
	}
	return rules
}

// GetMaxFileLength returns the maximum file length allowed
func (qrs *QualityRuleSet) GetMaxFileLength() int {
	rules := qrs.GetQualityRules(RuleTypeMaxFileLength)
	if len(rules) > 0 {
		return rules[0].MaxFileLines
	}
	return 0 // No limit
}

// GetMaxNameLength returns the maximum name length allowed
func (qrs *QualityRuleSet) GetMaxNameLength() int {
	rules := qrs.GetQualityRules(RuleTypeMaxNameLength)
	if len(rules) > 0 {
		return rules[0].MaxNameLength
	}
	return 0 // No limit
}

// GetCommentQualityRule returns the comment quality rule if configured
func (qrs *QualityRuleSet) GetCommentQualityRule() *QualityRule {
	rules := qrs.GetQualityRules(RuleTypeCommentQuality)
	if len(rules) > 0 {
		return rules[0]
	}
	return nil
}
