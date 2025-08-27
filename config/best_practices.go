package config

import (
	"github.com/flanksource/arch-unit/languages"
	"github.com/flanksource/arch-unit/models"
)

// StrictnessLevel represents how strict the rules should be
type StrictnessLevel string

const (
	StrictnessStrict   StrictnessLevel = "strict"
	StrictnessModerate StrictnessLevel = "moderate"
	StrictnessLenient  StrictnessLevel = "lenient"
)

// StyleGuide represents a predefined style guide configuration
type StyleGuide struct {
	Name        string
	Description string
	Languages   []string
	Variables   map[string]interface{}
	BuiltinRules map[string]models.BuiltinRuleConfig
	Rules       map[string]models.RuleConfig
	Linters     map[string]models.LinterConfig
}

// StyleGuides contains all predefined style guides
var StyleGuides = map[string]StyleGuide{
	"google-go": {
		Name:        "Google Go Style Guide",
		Description: "Based on Effective Go and Google's Go Style Guide",
		Languages:   []string{"go"},
		Variables: map[string]interface{}{
			"max_file_length":           400,
			"max_function_length":       50,
			"max_params":                5,
			"max_cyclomatic_complexity": 10,
			"max_variable_name_length":  30,
			"max_package_name_length":   15,
		},
		BuiltinRules: map[string]models.BuiltinRuleConfig{
			"no_fmt_print":           {Enabled: true},
			"proper_error_handling":  {Enabled: true},
			"secure_imports":         {Enabled: true},
			"test_naming_convention": {Enabled: true},
		},
		Rules: map[string]models.RuleConfig{
			"**/*.go": {
				Quality: &models.QualityConfig{
					MaxFileLength:       400,
					MaxFunctionNameLen:  50,
					MaxVariableNameLen:  30,
					MaxParameterNameLen: 25,
					DisallowedNames: []models.DisallowedNamePattern{
						{Pattern: "temp*", Reason: "Use descriptive names"},
						{Pattern: "*Manager", Reason: "Manager is too generic"},
						{Pattern: "*Util", Reason: "Util indicates poor design"},
					},
				},
				Imports: []string{
					"!fmt:Print*",
					"!log:Print*",
				},
			},
			"**/*_test.go": {
				Quality: &models.QualityConfig{
					MaxFileLength:       600,
					MaxFunctionNameLen:  80,
				},
				Imports: []string{
					"+fmt:Print*",
					"+testing",
				},
			},
		},
		Linters: map[string]models.LinterConfig{
			"golangci-lint": {
				Enabled: true,
				Args:    []string{"--enable=errcheck", "--enable=ineffassign", "--enable=gosec"},
			},
		},
	},

	"google-python": {
		Name:        "Google Python Style Guide",
		Description: "Based on Google's Python Style Guide",
		Languages:   []string{"python"},
		Variables: map[string]interface{}{
			"max_line_length":          100,
			"max_file_length":          400,
			"max_function_length":      50,
			"max_params":               5,
			"max_local_variables":      15,
			"max_variable_name_length": 30,
		},
		BuiltinRules: map[string]models.BuiltinRuleConfig{
			"no_print_statements":    {Enabled: true},
			"secure_imports":         {Enabled: true},
			"test_naming_convention": {Enabled: true},
		},
		Rules: map[string]models.RuleConfig{
			"**/*.py": {
				Quality: &models.QualityConfig{
					MaxFileLength:       400,
					MaxFunctionNameLen:  50,
					MaxVariableNameLen:  30,
					MaxParameterNameLen: 25,
					DisallowedNames: []models.DisallowedNamePattern{
						{Pattern: "temp*", Reason: "Use descriptive names"},
						{Pattern: "*mgr", Reason: "Avoid abbreviations"},
						{Pattern: "data", Reason: "Too generic"},
					},
				},
				Imports: []string{
					"!print",
					"!eval",
					"!exec",
				},
			},
		},
		Linters: map[string]models.LinterConfig{
			"ruff": {
				Enabled: true,
				Args:    []string{"--select=E,W,F,C90,N,UP,YTT,S,BLE,B,A,COM,C4,DTZ,EM,ISC,G,INP,PIE,PT,RSE,RET,SIM,TID,ARG,PTH,PD,PL,TRY,NPY,RUF"},
			},
		},
	},

	"pep8": {
		Name:        "PEP 8 Python Style Guide",
		Description: "Python's official PEP 8 style guide",
		Languages:   []string{"python"},
		Variables: map[string]interface{}{
			"max_line_length":     79,
			"max_file_length":     400,
			"max_function_length": 50,
			"max_params":          5,
		},
		BuiltinRules: map[string]models.BuiltinRuleConfig{
			"no_print_statements": {Enabled: true},
			"secure_imports":      {Enabled: true},
		},
		Rules: map[string]models.RuleConfig{
			"**/*.py": {
				Quality: &models.QualityConfig{
					MaxFileLength:       400,
					MaxFunctionNameLen:  50,
					MaxVariableNameLen:  30,
				},
			},
		},
		Linters: map[string]models.LinterConfig{
			"ruff": {
				Enabled: true,
				Args:    []string{"--select=E,W,F", "--line-length=79"},
			},
		},
	},

	"google-java": {
		Name:        "Google Java Style Guide",
		Description: "Based on Google's Java Style Guide",
		Languages:   []string{"java"},
		Variables: map[string]interface{}{
			"max_line_length":          100,
			"max_file_length":          2000,
			"max_method_length":        40,
			"max_params":               7,
			"max_class_dependencies":   20,
			"max_cyclomatic_complexity": 10,
		},
		BuiltinRules: map[string]models.BuiltinRuleConfig{
			"no_system_out_print":    {Enabled: true},
			"secure_imports":         {Enabled: true},
			"test_naming_convention": {Enabled: true},
		},
		Rules: map[string]models.RuleConfig{
			"**/*.java": {
				Quality: &models.QualityConfig{
					MaxFileLength:       2000,
					MaxFunctionNameLen:  40,
					MaxVariableNameLen:  30,
					MaxParameterNameLen: 25,
					DisallowedNames: []models.DisallowedNamePattern{
						{Pattern: "temp*", Reason: "Use descriptive names"},
						{Pattern: "*Mgr", Reason: "Avoid abbreviations"},
						{Pattern: "*Impl", Reason: "Implementation suffix is redundant"},
					},
				},
				Imports: []string{
					"!java.util.Date",     // Use java.time instead
					"!System.out:print*",
					"!System.err:print*",
				},
			},
		},
		Linters: map[string]models.LinterConfig{
			"checkstyle": {
				Enabled: true,
			},
		},
	},

	"airbnb-javascript": {
		Name:        "Airbnb JavaScript Style Guide",
		Description: "Popular JavaScript style guide by Airbnb",
		Languages:   []string{"javascript", "typescript"},
		Variables: map[string]interface{}{
			"max_line_length":          100,
			"max_file_length":          300,
			"max_function_length":      50,
			"max_params":               3,
			"max_nested_callbacks":     3,
			"max_cyclomatic_complexity": 10,
		},
		BuiltinRules: map[string]models.BuiltinRuleConfig{
			"no_console_log":        {Enabled: true},
			"secure_imports":         {Enabled: true},
			"test_naming_convention": {Enabled: true},
		},
		Rules: map[string]models.RuleConfig{
			"**/*.{js,jsx,ts,tsx}": {
				Quality: &models.QualityConfig{
					MaxFileLength:       300,
					MaxFunctionNameLen:  50,
					MaxVariableNameLen:  30,
					MaxParameterNameLen: 25,
					DisallowedNames: []models.DisallowedNamePattern{
						{Pattern: "temp*", Reason: "Use descriptive names"},
						{Pattern: "data", Reason: "Too generic"},
						{Pattern: "*Manager", Reason: "Manager is too generic"},
					},
				},
			},
		},
		Linters: map[string]models.LinterConfig{
			"eslint": {
				Enabled: true,
				Args:    []string{"--config=airbnb"},
			},
		},
	},

	"rust-official": {
		Name:        "Rust Official Style Guide",
		Description: "Rust's official style guide and API guidelines",
		Languages:   []string{"rust"},
		Variables: map[string]interface{}{
			"max_line_length":     100,
			"max_file_length":     400,
			"max_function_length": 50,
			"max_params":          5,
		},
		Rules: map[string]models.RuleConfig{
			"**/*.rs": {
				Quality: &models.QualityConfig{
					MaxFileLength:       400,
					MaxFunctionNameLen:  50,
					MaxVariableNameLen:  30,
				},
			},
		},
		Linters: map[string]models.LinterConfig{
			"rustfmt": {
				Enabled: true,
			},
			"clippy": {
				Enabled: true,
				Args:    []string{"--", "-W", "clippy::all"},
			},
		},
	},
}

// GetBestPracticesForLanguage returns best practices configuration for a language and strictness level
func GetBestPracticesForLanguage(language string, strictness StrictnessLevel) map[string]interface{} {
	// Convert StrictnessLevel to string for registry
	strictnessStr := "normal"
	switch strictness {
	case StrictnessStrict:
		strictnessStr = "strict"
	case StrictnessLenient:
		strictnessStr = "relaxed"
	}
	
	// Try to get from registry first
	if practices := languages.DefaultRegistry.GetBestPractices(language, strictnessStr); len(practices) > 0 {
		return practices
	}
	
	// Fallback for languages not yet in registry
	practices := make(map[string]interface{})
	
	switch language {
	case "java":
		practices["max_file_length"] = getValueByStrictness(strictness, 1500, 2000, 3000)
		practices["max_method_length"] = getValueByStrictness(strictness, 30, 40, 60)
		practices["max_params"] = getValueByStrictness(strictness, 5, 7, 10)
		practices["max_class_dependencies"] = getValueByStrictness(strictness, 15, 20, 30)

	case "javascript", "typescript":
		practices["max_file_length"] = getValueByStrictness(strictness, 250, 300, 400)
		practices["max_function_length"] = getValueByStrictness(strictness, 40, 50, 70)
		practices["max_params"] = getValueByStrictness(strictness, 3, 4, 5)
		practices["max_nested_callbacks"] = getValueByStrictness(strictness, 2, 3, 4)

	case "rust":
		practices["max_file_length"] = getValueByStrictness(strictness, 300, 400, 500)
		practices["max_function_length"] = getValueByStrictness(strictness, 40, 50, 70)
		practices["max_params"] = getValueByStrictness(strictness, 4, 5, 7)

	case "ruby":
		practices["max_file_length"] = getValueByStrictness(strictness, 250, 300, 400)
		practices["max_method_length"] = getValueByStrictness(strictness, 15, 20, 30)
		practices["max_params"] = getValueByStrictness(strictness, 3, 4, 5)
	}

	return practices
}

func getValueByStrictness(level StrictnessLevel, strict, moderate, lenient int) int {
	switch level {
	case StrictnessStrict:
		return strict
	case StrictnessModerate:
		return moderate
	case StrictnessLenient:
		return lenient
	default:
		return moderate
	}
}

// GetStyleGuide returns a style guide configuration by name
func GetStyleGuide(name string) (StyleGuide, bool) {
	guide, exists := StyleGuides[name]
	return guide, exists
}

// ApplyStyleGuide applies a style guide to the configuration
func ApplyStyleGuide(config *models.Config, guideName string) error {
	guide, exists := StyleGuides[guideName]
	if !exists {
		return nil
	}

	// Set generated from
	config.GeneratedFrom = guide.Name

	// Merge variables
	if config.Variables == nil {
		config.Variables = make(map[string]interface{})
	}
	for k, v := range guide.Variables {
		if _, exists := config.Variables[k]; !exists {
			config.Variables[k] = v
		}
	}

	// Merge built-in rules
	if config.BuiltinRules == nil {
		config.BuiltinRules = make(map[string]models.BuiltinRuleConfig)
	}
	for k, v := range guide.BuiltinRules {
		if _, exists := config.BuiltinRules[k]; !exists {
			config.BuiltinRules[k] = v
		}
	}

	// Merge rules
	if config.Rules == nil {
		config.Rules = make(map[string]models.RuleConfig)
	}
	for k, v := range guide.Rules {
		if _, exists := config.Rules[k]; !exists {
			config.Rules[k] = v
		}
	}

	// Merge linters
	if config.Linters == nil {
		config.Linters = make(map[string]models.LinterConfig)
	}
	for k, v := range guide.Linters {
		if _, exists := config.Linters[k]; !exists {
			config.Linters[k] = v
		}
	}

	return nil
}