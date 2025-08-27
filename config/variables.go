package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

var variableRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

// InterpolateVariables replaces ${var} placeholders with actual values throughout the config
func InterpolateVariables(config *models.Config) error {
	if config.Variables == nil || len(config.Variables) == 0 {
		return nil
	}

	// Interpolate in Rules
	for pattern, ruleConfig := range config.Rules {
		if err := interpolateRuleConfig(&ruleConfig, config.Variables); err != nil {
			return fmt.Errorf("error interpolating rule %s: %w", pattern, err)
		}
		config.Rules[pattern] = ruleConfig
	}

	// Interpolate in Quality configs
	for pattern, ruleConfig := range config.Rules {
		if ruleConfig.Quality != nil {
			if err := interpolateQualityConfig(ruleConfig.Quality, config.Variables); err != nil {
				return fmt.Errorf("error interpolating quality config for %s: %w", pattern, err)
			}
		}
	}

	// Interpolate in Linter configs
	for name, linterConfig := range config.Linters {
		if err := interpolateLinterConfig(&linterConfig, config.Variables); err != nil {
			return fmt.Errorf("error interpolating linter %s: %w", name, err)
		}
		config.Linters[name] = linterConfig
	}

	return nil
}

func interpolateRuleConfig(rule *models.RuleConfig, variables map[string]interface{}) error {
	// Interpolate imports
	for i, imp := range rule.Imports {
		rule.Imports[i] = interpolateString(imp, variables)
	}

	// Interpolate debounce
	rule.Debounce = interpolateString(rule.Debounce, variables)

	return nil
}

func interpolateQualityConfig(quality *models.QualityConfig, variables map[string]interface{}) error {
	// Interpolate integer fields
	if val := getVariableInt("max_file_length", variables); val > 0 && quality.MaxFileLength == 0 {
		quality.MaxFileLength = val
	}
	if val := getVariableInt("max_function_length", variables); val > 0 && quality.MaxFunctionNameLen == 0 {
		quality.MaxFunctionNameLen = val
	}
	if val := getVariableInt("max_variable_name_length", variables); val > 0 && quality.MaxVariableNameLen == 0 {
		quality.MaxVariableNameLen = val
	}
	if val := getVariableInt("max_parameter_name_length", variables); val > 0 && quality.MaxParameterNameLen == 0 {
		quality.MaxParameterNameLen = val
	}

	// Interpolate disallowed names
	for i, pattern := range quality.DisallowedNames {
		pattern.Pattern = interpolateString(pattern.Pattern, variables)
		pattern.Reason = interpolateString(pattern.Reason, variables)
		quality.DisallowedNames[i] = pattern
	}

	return nil
}

func interpolateLinterConfig(linter *models.LinterConfig, variables map[string]interface{}) error {
	// Interpolate args
	for i, arg := range linter.Args {
		linter.Args[i] = interpolateString(arg, variables)
	}

	// Interpolate debounce
	linter.Debounce = interpolateString(linter.Debounce, variables)

	return nil
}

func interpolateString(s string, variables map[string]interface{}) string {
	return variableRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := variableRegex.FindStringSubmatch(match)[1]
		if val, ok := variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match // Keep original if variable not found
	})
}

func getVariableInt(name string, variables map[string]interface{}) int {
	if val, ok := variables[name]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
	}
	return 0
}

// ExpandVariablesInString expands ${var} references in a string using the provided variables
func ExpandVariablesInString(s string, variables map[string]interface{}) string {
	if variables == nil || len(variables) == 0 {
		return s
	}
	
	return variableRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")
		if val, ok := variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})
}