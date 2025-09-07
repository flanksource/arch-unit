package fixtures

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/gomplate/v3"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// CELEvaluator evaluates CEL expressions against test results using gomplate
type CELEvaluator struct {
	// No longer needs CEL environment as gomplate handles CEL evaluation
}

// NewCELEvaluator creates a new CEL evaluator that uses gomplate for expression evaluation
func NewCELEvaluator() (*CELEvaluator, error) {
	// Gomplate handles CEL evaluation with its full function library
	return &CELEvaluator{}, nil
}

// EvaluateNodes evaluates a CEL expression against AST nodes
func (e *CELEvaluator) EvaluateNodes(expression string, nodes []*models.ASTNode) (bool, error) {
	if expression == "" || expression == "true" {
		return true, nil
	}

	// Convert nodes to CEL-compatible format
	nodeList := make([]map[string]interface{}, len(nodes))
	for i, node := range nodes {
		nodeList[i] = node.AsMap()
	}

	// Prepare template data for gomplate
	templateData := map[string]interface{}{
		"nodes": nodeList,
	}

	// Create our own CEL environment to avoid conflicts with gomplate's default AnyType declarations
	env, err := cel.NewEnv(
		cel.Variable("nodes", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
		cel.StdLib(),
		ext.Strings(),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Compile the expression
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
	}

	// Create program
	prg, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL program: %w", err)
	}

	// Evaluate
	out, _, err := prg.Eval(templateData)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	// Convert CEL result to Go boolean
	result := out.Value()
	if boolResult, ok := result.(bool); ok {
		return boolResult, nil
	}

	// Try to convert string results
	if stringResult := fmt.Sprintf("%v", result); stringResult != "" {
		if strings.ToLower(strings.TrimSpace(stringResult)) == "true" {
			return true, nil
		} else if strings.ToLower(strings.TrimSpace(stringResult)) == "false" {
			return false, nil
		}
	}

	return false, fmt.Errorf("CEL expression did not return a boolean: got %T(%v)", result, result)
}

// EvaluateOutput evaluates a CEL expression against command output
func (e *CELEvaluator) EvaluateOutput(expression string, output string) (bool, error) {
	if expression == "" || expression == "true" {
		return true, nil
	}

	// Prepare template data for gomplate
	templateData := map[string]interface{}{
		"output": output,
	}

	// Create our own CEL environment to avoid conflicts with gomplate's default AnyType declarations
	env, err := cel.NewEnv(
		cel.Variable("output", cel.StringType),
		cel.StdLib(),
		ext.Strings(),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Compile the expression
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
	}

	// Create program
	prg, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("failed to create CEL program: %w", err)
	}

	// Evaluate
	out, _, err := prg.Eval(templateData)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
	}

	// Convert CEL result to Go boolean
	result := out.Value()
	if boolResult, ok := result.(bool); ok {
		return boolResult, nil
	}

	// Try to convert string results
	if stringResult := fmt.Sprintf("%v", result); stringResult != "" {
		if strings.ToLower(strings.TrimSpace(stringResult)) == "true" {
			return true, nil
		} else if strings.ToLower(strings.TrimSpace(stringResult)) == "false" {
			return false, nil
		}
	}

	return false, fmt.Errorf("CEL expression did not return a boolean: got %T(%v)", result, result)
}

// EvaluateResult evaluates a CEL expression against a generic result map
func (e *CELEvaluator) EvaluateResult(expression string, result map[string]interface{}) (bool, error) {
	if expression == "" || expression == "true" {
		return true, nil
	}

	// Prepare template data - expose individual fields directly without the result wrapper
	// This matches what gomplate expects and avoids overlapping identifier errors
	templateData := make(map[string]interface{})
	for key, value := range result {
		templateData[key] = value
	}

	// Use RunExpression without explicit CelEnvs - let gomplate auto-detect variables
	// This avoids the overlapping identifier issue since gomplate handles variable declarations internally
	tmpl := gomplate.Template{
		Expression: expression,
		// Don't specify CelEnvs - let gomplate infer variables from templateData
	}

	// Use RunExpression for CEL expressions, not RunTemplate
	output, err := gomplate.RunExpression(templateData, tmpl)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate CEL expression with gomplate: %w", err)
	}

	// Convert result to boolean
	if boolResult, ok := output.(bool); ok {
		return boolResult, nil
	}

	// Try to convert string results
	if stringResult := fmt.Sprintf("%v", output); stringResult != "" {
		if strings.ToLower(strings.TrimSpace(stringResult)) == "true" {
			return true, nil
		} else if strings.ToLower(strings.TrimSpace(stringResult)) == "false" {
			return false, nil
		}
	}

	return false, fmt.Errorf("CEL expression did not return a boolean: got %T(%v)", output, output)
}

// ValidateCELExpression validates a CEL expression without evaluating it
func (e *CELEvaluator) ValidateCELExpression(expression string) error {
	if expression == "" || expression == "true" {
		return nil
	}

	// Create minimal template data for validation
	templateData := map[string]interface{}{
		"nodes":  []map[string]interface{}{},
		"output": "",
		"result": map[string]interface{}{},
	}

	// Use gomplate's CEL evaluation with proper type information for validation
	tmpl := gomplate.Template{
		Expression: expression,
		CelEnvs: []cel.EnvOption{
			cel.Variable("nodes", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
			cel.Variable("output", cel.StringType),
			cel.Variable("result", cel.MapType(cel.StringType, cel.DynType)),
		},
	}

	// Use RunExpression for CEL validation
	_, err := gomplate.RunExpression(templateData, tmpl)
	if err != nil {
		return fmt.Errorf("invalid CEL expression: %w", err)
	}

	return nil
}

// GetAvailableVariables returns a list of available variables for CEL expressions
func (e *CELEvaluator) GetAvailableVariables() []string {
	return []string{
		"nodes - List of AST nodes",
		"node - Single AST node",
		"output - String output from command",
		"result - Generic result map",
		"stdout - Command stdout",
		"stderr - Command stderr",
		"rawStdout - Raw command stdout",
		"rawStderr - Raw command stderr",
		"exitCode - Command exit code",
		"isHelpError - Whether help text was detected",
		"json - Parsed JSON data (when available)",
		"temp - Temporary file data",
	}
}

// GetAvailableFunctions returns a list of available functions for CEL expressions
func (e *CELEvaluator) GetAvailableFunctions() []string {
	return []string{
		"CEL Functions:",
		"  string.endsWith(suffix) - Check if string ends with suffix",
		"  string.contains(substring) - Check if string contains substring",
		"  string.startsWith(prefix) - Check if string starts with prefix",
		"  nodes.all(n, predicate) - Check if all nodes match predicate",
		"  nodes.exists(n, predicate) - Check if any node matches predicate",
		"  nodes.filter(n, predicate) - Filter nodes by predicate",
		"  list.unique() - Get unique values from list",
		"  size(list) - Get list size",
		"",
		"Gomplate Functions (via Template.Expr):",
		"  String: strings.Contains, strings.HasPrefix, strings.HasSuffix, etc.",
		"  Math: math.Abs, math.Max, math.Min, math.Round, etc.",
		"  Collections: coll.Has, coll.Keys, coll.Values, etc.",
		"  Conversion: conv.ToString, conv.ToInt, conv.ToBool, etc.",
		"  Crypto: crypto.SHA1, crypto.SHA256, crypto.MD5, etc.",
		"  Data: data.JSON, data.YAML, data.CSV, etc.",
		"  File: file.Exists, file.IsDir, file.Read, etc.",
		"  Network: net.LookupIP, net.LookupCNAME, etc.",
		"  Regex: regexp.Match, regexp.FindAll, regexp.Replace, etc.",
		"  Time: time.Now, time.Parse, time.Format, etc.",
		"  UUID: uuid.V1, uuid.V4, etc.",
		"",
		"See gomplate documentation for complete function reference.",
	}
}

// Helper function to check if a value is truthy for simple comparisons
func isTruthy(v interface{}) bool {
	if v == nil {
		return false
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String() != ""
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() > 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	default:
		return true
	}
}
