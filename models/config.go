package models

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/flanksource/commons/logger"
)

// Config represents the arch-unit.yaml configuration structure
type Config struct {
	Version        string                       `yaml:"version"`
	GeneratedFrom  string                       `yaml:"generated_from,omitempty"` // Style guide or template used
	Debounce       string                       `yaml:"debounce,omitempty"`
	Variables      map[string]interface{}       `yaml:"variables,omitempty"`     // Variable definitions for interpolation
	BuiltinRules   map[string]BuiltinRuleConfig `yaml:"builtin_rules,omitempty"` // Built-in rule configurations
	Rules          map[string]RuleConfig        `yaml:"rules"`
	Linters        map[string]LinterConfig      `yaml:"linters,omitempty"`
	GlobalExcludes []string                     `yaml:"global_excludes,omitempty"`
	Languages      map[string]LanguageConfig    `yaml:"languages,omitempty"`
	AQLRules       []AQLRuleConfig              `yaml:"aql_rules,omitempty"` // AQL architecture rules
}

// RuleConfig represents configuration for a specific path pattern
type RuleConfig struct {
	Imports  []string                `yaml:"imports,omitempty"`
	Debounce string                  `yaml:"debounce,omitempty"`
	Linters  map[string]LinterConfig `yaml:"linters,omitempty"`
	Quality  *QualityConfig          `yaml:"quality,omitempty"`
}

// QualityConfig represents quality analysis configuration
type QualityConfig struct {
	MaxFileLength       int                     `yaml:"max_file_length,omitempty"`
	MaxFunctionNameLen  int                     `yaml:"max_function_name_length,omitempty"`
	MaxVariableNameLen  int                     `yaml:"max_variable_name_length,omitempty"`
	MaxParameterNameLen int                     `yaml:"max_parameter_name_length,omitempty"`
	DisallowedNames     []DisallowedNamePattern `yaml:"disallowed_names,omitempty"`
	CommentAnalysis     CommentAnalysisConfig   `yaml:"comment_analysis,omitempty"`
}

// DisallowedNamePattern represents a pattern for disallowed names
type DisallowedNamePattern struct {
	Pattern string `yaml:"pattern"`
	Reason  string `yaml:"reason,omitempty"`
}

// CommentAnalysisConfig represents comment analysis configuration
type CommentAnalysisConfig struct {
	Enabled             bool    `yaml:"enabled"`
	WordLimit           int     `yaml:"word_limit"`
	AIModel             string  `yaml:"ai_model"`
	MinDescriptiveScore float64 `yaml:"min_descriptive_score"`
	CheckVerbosity      bool    `yaml:"check_verbosity"`
}

// LanguageConfig represents configuration for a specific language
type LanguageConfig struct {
	Includes []string `yaml:"includes,omitempty"`
	Excludes []string `yaml:"excludes,omitempty"`
}

// LinterConfig represents configuration for a specific linter
type LinterConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Debounce     string   `yaml:"debounce,omitempty"`
	Args         []string `yaml:"args,omitempty"`
	OutputFormat string   `yaml:"output_format,omitempty"`
}

// AQLRuleConfig represents configuration for AQL rules
type AQLRuleConfig struct {
	File    string `yaml:"file,omitempty"`   // Path to AQL rule file
	Inline  string `yaml:"inline,omitempty"` // Inline AQL rule text
	Enabled bool   `yaml:"enabled"`          // Whether this rule is enabled
}

// BuiltinRuleConfig represents configuration for a built-in rule
type BuiltinRuleConfig struct {
	Enabled bool                   `yaml:"enabled"`
	Config  map[string]interface{} `yaml:"config,omitempty"`
}

// GetDebounceDuration returns the parsed debounce duration for a config
func (c *Config) GetDebounceDuration() (time.Duration, error) {
	if c.Debounce == "" {
		return 0, nil
	}
	return time.ParseDuration(c.Debounce)
}

// GetDebounceDuration returns the parsed debounce duration for a rule config
func (r *RuleConfig) GetDebounceDuration() (time.Duration, error) {
	if r.Debounce == "" {
		return 0, nil
	}
	return time.ParseDuration(r.Debounce)
}

// GetDebounceDuration returns the parsed debounce duration for a linter config
func (l *LinterConfig) GetDebounceDuration() (time.Duration, error) {
	if l.Debounce == "" {
		return 0, nil
	}
	return time.ParseDuration(l.Debounce)
}

// GetRulesForFile returns the applicable rules for a given file path
func (c *Config) GetRulesForFile(filePath string) (*RuleSet, error) {
	var rules []Rule

	// Convert to absolute path for consistent matching
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	// Collect patterns that match this file, with their specificity
	type patternMatch struct {
		pattern     string
		ruleConfig  RuleConfig
		specificity int
	}
	var matches []patternMatch

	for pattern, ruleConfig := range c.Rules {
		if c.patternMatches(pattern, absPath, filePath) {
			// Calculate specificity: more specific patterns should be processed last
			specificity := 0
			if pattern == "**" {
				specificity = 0 // Most general
			} else if strings.Contains(pattern, "**") {
				specificity = 1 // Glob patterns
			} else if strings.Contains(pattern, "*") {
				specificity = 2 // Wildcard patterns
			} else {
				specificity = 3 // Exact file paths are most specific
			}

			matches = append(matches, patternMatch{
				pattern:     pattern,
				ruleConfig:  ruleConfig,
				specificity: specificity,
			})
		}
	}

	// Sort by specificity (least specific first, so overrides work correctly)
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].specificity != matches[j].specificity {
			return matches[i].specificity < matches[j].specificity
		}
		// For same specificity, sort alphabetically for consistency
		return matches[i].pattern < matches[j].pattern
	})

	// Process patterns in order
	for _, match := range matches {
		logger.Debugf("Pattern '%s' matched file '%s'", match.pattern, filePath)
		// Convert import rules to Rule structs
		for i, importRule := range match.ruleConfig.Imports {
			rule, err := c.parseImportRule(importRule, match.pattern, i)
			if err != nil {
				return nil, fmt.Errorf("invalid import rule '%s' in pattern '%s': %w", importRule, match.pattern, err)
			}
			rules = append(rules, *rule)
			logger.Debugf("Added rule from pattern '%s': %s", match.pattern, importRule)
		}
	}

	return &RuleSet{
		Rules: rules,
		Path:  filepath.Dir(filePath),
	}, nil
}

// patternMatches checks if a file path matches a given pattern
func (c *Config) patternMatches(pattern, absPath, relPath string) bool {
	// Handle special "**" pattern (matches everything)
	if pattern == "**" {
		return true
	}

	// Try exact string match for file-specific patterns (no wildcards)
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") && !strings.Contains(pattern, "[") {
		// Check if pattern matches the end of the absolute or relative path
		if strings.HasSuffix(absPath, pattern) || strings.HasSuffix(relPath, pattern) {
			logger.Debugf("Pattern '%s' matched file path via suffix match", pattern)
			return true
		}
		// Also check exact match
		if pattern == relPath || pattern == absPath {
			logger.Debugf("Pattern '%s' matched file path via exact match", pattern)
			return true
		}
	}

	// Try matching against relative path first
	if matched, err := filepath.Match(pattern, relPath); err == nil && matched {
		logger.Debugf("Pattern '%s' matched relPath '%s' via filepath.Match", pattern, relPath)
		return true
	}

	// Try matching against absolute path
	if matched, err := filepath.Match(pattern, absPath); err == nil && matched {
		return true
	}

	// Handle glob patterns with **
	if strings.Contains(pattern, "**") {
		// For patterns like "**/*_test.go", also try matching just the suffix
		if strings.HasPrefix(pattern, "**/") {
			suffix := strings.TrimPrefix(pattern, "**/")
			if matched, err := filepath.Match(suffix, filepath.Base(relPath)); err == nil && matched {
				return true
			}
			// Also try against the full relative path
			if matched, err := filepath.Match(suffix, relPath); err == nil && matched {
				return true
			}
		}

		// Convert ** pattern to standard glob for directory-based matching
		globPattern := strings.ReplaceAll(pattern, "**", "*")

		// Try different parts of the path
		pathParts := strings.Split(filepath.ToSlash(relPath), "/")
		for i := 0; i < len(pathParts); i++ {
			subPath := strings.Join(pathParts[i:], "/")
			if matched, err := filepath.Match(globPattern, subPath); err == nil && matched {
				return true
			}
		}
	}

	// Handle directory patterns
	if strings.HasSuffix(pattern, "/**") {
		dirPattern := strings.TrimSuffix(pattern, "/**")
		relDir := filepath.Dir(relPath)
		if strings.HasPrefix(relDir, dirPattern) {
			return true
		}
	}

	return false
}

// parseImportRule converts an import rule string to a Rule struct
func (c *Config) parseImportRule(importRule, sourcePattern string, lineNum int) (*Rule, error) {
	originalRule := importRule
	rule := &Rule{
		SourceFile:   "arch-unit.yaml:" + sourcePattern,
		LineNumber:   lineNum + 1,
		Scope:        ".",
		OriginalLine: originalRule,
		Type:         RuleTypeAllow,
	}

	// Handle rule type prefixes
	if strings.HasPrefix(importRule, "+") {
		rule.Type = RuleTypeOverride
		importRule = importRule[1:]
	} else if strings.HasPrefix(importRule, "!") {
		rule.Type = RuleTypeDeny
		importRule = importRule[1:]
	}

	// Handle method-specific rules (package:method)
	if strings.Contains(importRule, ":") {
		parts := strings.SplitN(importRule, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid method rule format: %s", originalRule)
		}

		rule.Package = strings.TrimSpace(parts[0])
		methodPart := strings.TrimSpace(parts[1])

		// Handle method negation
		if strings.HasPrefix(methodPart, "!") {
			rule.Method = methodPart[1:]
			if rule.Type == RuleTypeAllow {
				rule.Type = RuleTypeDeny
			}
		} else {
			rule.Method = methodPart
		}
	} else {
		// It's a package/pattern rule
		rule.Pattern = importRule
	}

	return rule, nil
}

// GetLinterConfig returns the linter configuration for a specific linter and file
func (c *Config) GetLinterConfig(linterName, filePath string) *LinterConfig {
	// Start with global linter config
	var config LinterConfig
	if globalConfig, exists := c.Linters[linterName]; exists {
		config = globalConfig
	}

	// Override with path-specific config
	for pattern, ruleConfig := range c.Rules {
		if c.patternMatches(pattern, filePath, filePath) {
			if linterConfig, exists := ruleConfig.Linters[linterName]; exists {
				// Merge configurations (path-specific takes precedence)
				if linterConfig.Enabled {
					config.Enabled = linterConfig.Enabled
				}
				if linterConfig.Debounce != "" {
					config.Debounce = linterConfig.Debounce
				}
				if len(linterConfig.Args) > 0 {
					config.Args = linterConfig.Args
				}
				if linterConfig.OutputFormat != "" {
					config.OutputFormat = linterConfig.OutputFormat
				}
			}
		}
	}

	return &config
}

// GetEnabledLinters returns all enabled linters for the configuration
func (c *Config) GetEnabledLinters() []string {
	var enabled []string
	for name, config := range c.Linters {
		if config.Enabled {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// GetQualityConfig returns the quality configuration for a specific file path
func (c *Config) GetQualityConfig(filePath string) *QualityConfig {
	var config *QualityConfig

	// Find the most specific pattern match
	for pattern, ruleConfig := range c.Rules {
		if c.patternMatches(pattern, filePath, filePath) && ruleConfig.Quality != nil {
			if config == nil {
				// First match - create a copy
				configCopy := *ruleConfig.Quality
				config = &configCopy
			} else {
				// Merge with more specific pattern (override non-zero values)
				if ruleConfig.Quality.MaxFileLength != 0 {
					config.MaxFileLength = ruleConfig.Quality.MaxFileLength
				}
				if ruleConfig.Quality.MaxFunctionNameLen != 0 {
					config.MaxFunctionNameLen = ruleConfig.Quality.MaxFunctionNameLen
				}
				if ruleConfig.Quality.MaxVariableNameLen != 0 {
					config.MaxVariableNameLen = ruleConfig.Quality.MaxVariableNameLen
				}
				if ruleConfig.Quality.MaxParameterNameLen != 0 {
					config.MaxParameterNameLen = ruleConfig.Quality.MaxParameterNameLen
				}
				if len(ruleConfig.Quality.DisallowedNames) > 0 {
					config.DisallowedNames = append(config.DisallowedNames, ruleConfig.Quality.DisallowedNames...)
				}
				// Override comment analysis config
				if ruleConfig.Quality.CommentAnalysis.Enabled {
					config.CommentAnalysis = ruleConfig.Quality.CommentAnalysis
				}
			}
		}
	}

	// Apply defaults if config exists
	if config != nil {
		config.ApplyDefaults()
	}

	return config
}

// ApplyDefaults applies default values to the quality configuration
func (qc *QualityConfig) ApplyDefaults() {
	if qc.MaxFileLength == 0 {
		qc.MaxFileLength = 400
	}
	if qc.MaxFunctionNameLen == 0 {
		qc.MaxFunctionNameLen = 50
	}
	if qc.MaxVariableNameLen == 0 {
		qc.MaxVariableNameLen = 30
	}
	if qc.MaxParameterNameLen == 0 {
		qc.MaxParameterNameLen = 25
	}

	// Apply comment analysis defaults
	if qc.CommentAnalysis.WordLimit == 0 {
		qc.CommentAnalysis.WordLimit = 10
	}
	if qc.CommentAnalysis.AIModel == "" {
		qc.CommentAnalysis.AIModel = "claude-3-haiku-20240307"
	}
	if qc.CommentAnalysis.MinDescriptiveScore == 0 {
		qc.CommentAnalysis.MinDescriptiveScore = 0.7
	}
}

// IsQualityEnabled returns true if any quality rules are configured
func (c *Config) IsQualityEnabled(filePath string) bool {
	return c.GetQualityConfig(filePath) != nil
}

// GetDisallowedNamePatterns returns all disallowed name patterns for a file
func (qc *QualityConfig) GetDisallowedNamePatterns() []string {
	if qc == nil {
		return nil
	}

	var patterns []string
	for _, pattern := range qc.DisallowedNames {
		patterns = append(patterns, pattern.Pattern)
	}
	return patterns
}

// IsNameDisallowed checks if a name matches any disallowed patterns
func (qc *QualityConfig) IsNameDisallowed(name string) (bool, string) {
	if qc == nil {
		return false, ""
	}

	for _, pattern := range qc.DisallowedNames {
		if matched, err := filepath.Match(pattern.Pattern, name); err == nil && matched {
			reason := pattern.Reason
			if reason == "" {
				reason = fmt.Sprintf("matches disallowed pattern: %s", pattern.Pattern)
			}
			return true, reason
		}
	}
	return false, ""
}

// ValidateFileLength validates if a file exceeds the maximum allowed lines
func (qc *QualityConfig) ValidateFileLength(lineCount int) (bool, string) {
	if qc == nil || qc.MaxFileLength == 0 {
		return true, ""
	}

	if lineCount > qc.MaxFileLength {
		return false, fmt.Sprintf("file has %d lines, exceeds maximum of %d", lineCount, qc.MaxFileLength)
	}
	return true, ""
}

// ValidateFunctionNameLength validates function name length
func (qc *QualityConfig) ValidateFunctionNameLength(name string) (bool, string) {
	if qc == nil || qc.MaxFunctionNameLen == 0 {
		return true, ""
	}

	if len(name) > qc.MaxFunctionNameLen {
		return false, fmt.Sprintf("function name '%s' has %d characters, exceeds maximum of %d", name, len(name), qc.MaxFunctionNameLen)
	}
	return true, ""
}

// ValidateVariableNameLength validates variable name length
func (qc *QualityConfig) ValidateVariableNameLength(name string) (bool, string) {
	if qc == nil || qc.MaxVariableNameLen == 0 {
		return true, ""
	}

	if len(name) > qc.MaxVariableNameLen {
		return false, fmt.Sprintf("variable name '%s' has %d characters, exceeds maximum of %d", name, len(name), qc.MaxVariableNameLen)
	}
	return true, ""
}

// ValidateParameterNameLength validates parameter name length
func (qc *QualityConfig) ValidateParameterNameLength(name string) (bool, string) {
	if qc == nil || qc.MaxParameterNameLen == 0 {
		return true, ""
	}

	if len(name) > qc.MaxParameterNameLen {
		return false, fmt.Sprintf("parameter name '%s' has %d characters, exceeds maximum of %d", name, len(name), qc.MaxParameterNameLen)
	}
	return true, ""
}

// GetAllLanguageExcludes returns the combined exclusion patterns from all sources
// This is the "all_language_excludes" macro that combines:
// 1. Built-in system excludes (examples/**, hack/**, .git/**, etc.)
// 2. User-defined global excludes
// 3. User-defined language-specific excludes
// 4. Linter-specific excludes
func (c *Config) GetAllLanguageExcludes(language string, linterSpecificExcludes []string) []string {
	var allExcludes []string

	// 1. Add built-in system excludes
	allExcludes = append(allExcludes, GetBuiltinExcludePatterns()...)

	// 2. Add user-defined global excludes
	if c.GlobalExcludes != nil {
		allExcludes = append(allExcludes, c.GlobalExcludes...)
	}

	// 3. Add language-specific excludes
	if language != "" && c.Languages != nil {
		if langConfig, exists := c.Languages[language]; exists {
			allExcludes = append(allExcludes, langConfig.Excludes...)
		}
	}

	// 4. Add linter-specific excludes
	if linterSpecificExcludes != nil {
		allExcludes = append(allExcludes, linterSpecificExcludes...)
	}

	// Remove duplicates
	return removeDuplicateStrings(allExcludes)
}

// GetAllLanguageIncludes returns the combined inclusion patterns for a language
func (c *Config) GetAllLanguageIncludes(language string, linterSpecificIncludes []string) []string {
	var allIncludes []string

	// Add language-specific includes
	if language != "" && c.Languages != nil {
		if langConfig, exists := c.Languages[language]; exists {
			allIncludes = append(allIncludes, langConfig.Includes...)
		}
	}

	// Add linter-specific includes
	if linterSpecificIncludes != nil {
		allIncludes = append(allIncludes, linterSpecificIncludes...)
	}

	// Remove duplicates
	return removeDuplicateStrings(allIncludes)
}

// GetBuiltinExcludePatterns returns the built-in exclusion patterns used across the system
func GetBuiltinExcludePatterns() []string {
	return []string{
		".git/**",         // Git metadata
		".svn/**",         // SVN metadata
		".hg/**",          // Mercurial metadata
		"examples/**",     // Example code directories
		"hack/**",         // Development/build scripts
		"node_modules/**", // Node.js dependencies
		"vendor/**",       // Go vendor dependencies
		"build/**",        // Build output directories
		"dist/**",         // Distribution files
		"coverage/**",     // Test coverage files
		".next/**",        // Next.js build files
		".nuxt/**",        // Nuxt.js build files
		"__pycache__/**",  // Python cache directories
		".venv/**",        // Python virtual environments
		".env/**",         // Environment directories
		"*.min.*",         // Minified files
		"target/**",       // Rust/Java build output
		".cargo/**",       // Cargo cache
		".gradle/**",      // Gradle cache
		".idea/**",        // IDE files
		".vscode/**",      // VSCode files
	}
}

// GetTestExcludePatterns returns patterns to exclude when running tests
// This is used by Taskfile.yml and test scripts
func GetTestExcludePatterns() []string {
	builtins := GetBuiltinExcludePatterns()

	// Add test-specific exclusions
	testSpecific := []string{
		"**/testdata/**", // Go test data directories
	}

	return append(builtins, testSpecific...)
}

// removeDuplicateStrings removes duplicate strings from a slice
func removeDuplicateStrings(strs []string) []string {
	if len(strs) <= 1 {
		return strs
	}

	seen := make(map[string]bool, len(strs))
	result := make([]string, 0, len(strs))

	for _, str := range strs {
		if !seen[str] {
			seen[str] = true
			result = append(result, str)
		}
	}

	return result
}
