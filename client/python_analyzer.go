package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

type PythonAnalyzer struct {
	rootDir string
}

func NewPythonAnalyzer(rootDir string) *PythonAnalyzer {
	return &PythonAnalyzer{rootDir: rootDir}
}

type PythonCall struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Module   string `json:"module"`
	Function string `json:"function"`
	Caller   string `json:"caller"`
}

const pythonAnalyzerScript = `
import ast
import json
import sys
import os

class CallVisitor(ast.NodeVisitor):
    def __init__(self, filename):
        self.filename = filename
        self.calls = []
        self.current_function = None
        self.imports = {}
        
    def visit_Import(self, node):
        for alias in node.names:
            name = alias.asname if alias.asname else alias.name
            self.imports[name] = alias.name
        self.generic_visit(node)
        
    def visit_ImportFrom(self, node):
        module = node.module or ''
        for alias in node.names:
            name = alias.asname if alias.asname else alias.name
            if module:
                self.imports[name] = f"{module}.{alias.name}"
            else:
                self.imports[name] = alias.name
        self.generic_visit(node)
        
    def visit_FunctionDef(self, node):
        old_function = self.current_function
        self.current_function = node.name
        self.generic_visit(node)
        self.current_function = old_function
        
    def visit_ClassDef(self, node):
        old_function = self.current_function
        self.current_function = f"{node.name}.__init__"
        self.generic_visit(node)
        self.current_function = old_function
        
    def visit_Call(self, node):
        module = ""
        function = ""
        
        if isinstance(node.func, ast.Name):
            function = node.func.id
            if function in self.imports:
                parts = self.imports[function].rsplit('.', 1)
                if len(parts) == 2:
                    module, function = parts
                else:
                    module = self.imports[function]
                    
        elif isinstance(node.func, ast.Attribute):
            function = node.func.attr
            
            if isinstance(node.func.value, ast.Name):
                obj_name = node.func.value.id
                if obj_name in self.imports:
                    module = self.imports[obj_name]
                else:
                    module = obj_name
                    
            elif isinstance(node.func.value, ast.Attribute):
                parts = []
                curr = node.func.value
                while isinstance(curr, ast.Attribute):
                    parts.insert(0, curr.attr)
                    curr = curr.value
                if isinstance(curr, ast.Name):
                    parts.insert(0, curr.id)
                module = '.'.join(parts)
                
        if module or function:
            self.calls.append({
                'file': self.filename,
                'line': node.lineno,
                'column': node.col_offset,
                'module': module,
                'function': function,
                'caller': self.current_function or 'module'
            })
            
        self.generic_visit(node)

def analyze_file(filepath):
    try:
        with open(filepath, 'r', encoding='utf-8') as f:
            tree = ast.parse(f.read(), filepath)
            
        visitor = CallVisitor(filepath)
        visitor.visit(tree)
        return visitor.calls
    except Exception as e:
        return []

if __name__ == '__main__':
    if len(sys.argv) < 2:
        sys.exit(1)
        
    filepath = sys.argv[1]
    calls = analyze_file(filepath)
    print(json.dumps(calls))
`

func (a *PythonAnalyzer) AnalyzeFile(filePath string, rules *models.RuleSet) ([]models.Violation, error) {
	// Create temporary Python script
	tmpFile, err := os.CreateTemp("", "arch_unit_*.py")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(pythonAnalyzerScript); err != nil {
		return nil, err
	}
	tmpFile.Close()

	// Run Python analyzer
	cmd := exec.Command("python3", tmpFile.Name(), filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to analyze Python file: %w", err)
	}

	var calls []PythonCall
	if err := json.Unmarshal(output, &calls); err != nil {
		return nil, fmt.Errorf("failed to parse Python analyzer output: %w", err)
	}

	// Check for violations
	var violations []models.Violation
	for _, call := range calls {
		if rules != nil {
			allowed, rule := rules.IsAllowedForFile(call.Module, call.Function, filePath)
			if !allowed {
				violationMsg := fmt.Sprintf("Call to %s.%s violates architecture rule", call.Module, call.Function)
				if rule.FilePattern != "" {
					violationMsg = fmt.Sprintf("Call to %s.%s violates file-specific rule [%s]", call.Module, call.Function, rule.FilePattern)
				}
				violations = append(violations, models.Violation{
					File:          filePath,
					Line:          call.Line,
					Column:        call.Column,
					CallerPackage: filepath.Dir(filePath),
					CallerMethod:  call.Caller,
					CalledPackage: call.Module,
					CalledMethod:  call.Function,
					Rule:          rule,
					Message:       violationMsg,
				})
			}
		}
	}

	return violations, nil
}

func AnalyzePythonFiles(rootDir string, files []string, ruleSets []models.RuleSet) (*models.AnalysisResult, error) {
	analyzer := NewPythonAnalyzer(rootDir)
	parser := NewParser(rootDir)
	result := &models.AnalysisResult{
		FileCount: len(files),
	}

	for _, file := range files {
		rules := parser.GetRulesForFile(file, ruleSets)
		if rules != nil {
			result.RuleCount += len(rules.Rules)
		}

		violations, err := analyzer.AnalyzeFile(file, rules)
		if err != nil {
			// Skip files with errors
			continue
		}

		result.Violations = append(result.Violations, violations...)
	}

	return result, nil
}

func IsPythonFile(path string) bool {
	return strings.HasSuffix(path, ".py")
}

func IsGoFile(path string) bool {
	return strings.HasSuffix(path, ".go")
}

// ExportAST implements the ASTExporter interface to convert Python AST to GenericAST
func (a *PythonAnalyzer) ExportAST(filePath string) (*models.GenericAST, error) {
	// Count lines in file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	lineCount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineCount++
	}

	// Create and run the Python AST extraction script
	script := pythonASTExtractionScript

	// Create temp script file
	tmpFile, err := os.CreateTemp("", "python_ast_*.py")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(script); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}

	// Execute the script
	cmd := exec.Command("python3", tmpFile.Name(), filePath)
	cmd.Dir = a.rootDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("python analysis failed: %w - output: %s", err, string(output))
	}

	// Parse the JSON output
	var pythonAST PythonASTData
	if err := json.Unmarshal(output, &pythonAST); err != nil {
		return nil, fmt.Errorf("failed to parse python AST JSON: %w", err)
	}

	// Convert to GenericAST
	genericAST := &models.GenericAST{
		Language:    "python",
		FilePath:    filePath,
		LineCount:   lineCount,
		PackageName: pythonAST.Module,
		Functions:   []models.Function{},
		Types:       []models.TypeDefinition{},
		Variables:   []models.Variable{},
		Comments:    []models.Comment{},
		Imports:     []models.Import{},
	}

	// Convert imports
	for _, imp := range pythonAST.Imports {
		genericImport := models.Import{
			Path:  imp.Module,
			Alias: imp.Alias,
			Line:  imp.Line,
		}
		genericAST.Imports = append(genericAST.Imports, genericImport)
	}

	// Convert functions
	for _, fn := range pythonAST.Functions {
		function := models.Function{
			Name:       fn.Name,
			NameLength: len(fn.Name),
			StartLine:  fn.StartLine,
			EndLine:    fn.EndLine,
			LineCount:  fn.EndLine - fn.StartLine + 1,
			Parameters: []models.Parameter{},
			Comments:   []models.Comment{},
			IsExported: !strings.HasPrefix(fn.Name, "_"), // Python convention
		}

		// Convert parameters
		for _, param := range fn.Parameters {
			parameter := models.Parameter{
				Name:       param.Name,
				Type:       param.Type,
				NameLength: len(param.Name),
			}
			function.Parameters = append(function.Parameters, parameter)
		}

		// Convert function comments
		for _, comment := range fn.Comments {
			genericComment := models.NewComment(
				comment.Text,
				comment.StartLine,
				comment.EndLine,
				models.CommentType(comment.Type),
				fmt.Sprintf("function:%s", fn.Name),
			)
			function.Comments = append(function.Comments, genericComment)
		}

		genericAST.Functions = append(genericAST.Functions, function)
	}

	// Convert classes (as types)
	for _, cls := range pythonAST.Classes {
		typeDef := models.TypeDefinition{
			Name:       cls.Name,
			NameLength: len(cls.Name),
			Kind:       "class",
			StartLine:  cls.StartLine,
			EndLine:    cls.EndLine,
			LineCount:  cls.EndLine - cls.StartLine + 1,
			Fields:     []models.Field{},
			Methods:    []models.Function{},
			Comments:   []models.Comment{},
			IsExported: !strings.HasPrefix(cls.Name, "_"),
		}

		// Convert methods as functions
		for _, method := range cls.Methods {
			methodFunc := models.Function{
				Name:       method.Name,
				NameLength: len(method.Name),
				StartLine:  method.StartLine,
				EndLine:    method.EndLine,
				LineCount:  method.EndLine - method.StartLine + 1,
				Parameters: []models.Parameter{},
				Comments:   []models.Comment{},
				IsExported: !strings.HasPrefix(method.Name, "_"),
			}

			for _, param := range method.Parameters {
				parameter := models.Parameter{
					Name:       param.Name,
					Type:       param.Type,
					NameLength: len(param.Name),
				}
				methodFunc.Parameters = append(methodFunc.Parameters, parameter)
			}

			typeDef.Methods = append(typeDef.Methods, methodFunc)
		}

		// Convert class comments
		for _, comment := range cls.Comments {
			genericComment := models.NewComment(
				comment.Text,
				comment.StartLine,
				comment.EndLine,
				models.CommentType(comment.Type),
				fmt.Sprintf("class:%s", cls.Name),
			)
			typeDef.Comments = append(typeDef.Comments, genericComment)
		}

		genericAST.Types = append(genericAST.Types, typeDef)
	}

	// Convert variables
	for _, variable := range pythonAST.Variables {
		genericVar := models.Variable{
			Name:       variable.Name,
			Type:       variable.Type,
			NameLength: len(variable.Name),
			Line:       variable.Line,
			IsConstant: strings.ToUpper(variable.Name) == variable.Name, // Python convention
			IsExported: !strings.HasPrefix(variable.Name, "_"),
			Comments:   []models.Comment{},
		}
		genericAST.Variables = append(genericAST.Variables, genericVar)
	}

	// Convert comments
	for _, comment := range pythonAST.Comments {
		genericComment := models.NewComment(
			comment.Text,
			comment.StartLine,
			comment.EndLine,
			models.CommentType(comment.Type),
			"file-level",
		)
		genericAST.Comments = append(genericAST.Comments, genericComment)
	}

	return genericAST, nil
}

// PythonASTData represents the structure of Python AST data
type PythonASTData struct {
	Module    string           `json:"module"`
	Imports   []PythonImport   `json:"imports"`
	Functions []PythonFunction `json:"functions"`
	Classes   []PythonClass    `json:"classes"`
	Variables []PythonVariable `json:"variables"`
	Comments  []PythonComment  `json:"comments"`
}

type PythonImport struct {
	Module string `json:"module"`
	Alias  string `json:"alias"`
	Line   int    `json:"line"`
}

type PythonFunction struct {
	Name       string            `json:"name"`
	StartLine  int               `json:"start_line"`
	EndLine    int               `json:"end_line"`
	Parameters []PythonParameter `json:"parameters"`
	Comments   []PythonComment   `json:"comments"`
}

type PythonClass struct {
	Name      string           `json:"name"`
	StartLine int              `json:"start_line"`
	EndLine   int              `json:"end_line"`
	Methods   []PythonFunction `json:"methods"`
	Comments  []PythonComment  `json:"comments"`
}

type PythonVariable struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Line int    `json:"line"`
}

type PythonParameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type PythonComment struct {
	Text      string `json:"text"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Type      string `json:"type"`
}

// Python script for AST extraction
const pythonASTExtractionScript = `
import ast
import json
import sys
import os

class ASTExtractor(ast.NodeVisitor):
    def __init__(self, filename):
        self.filename = filename
        self.module = os.path.splitext(os.path.basename(filename))[0]
        self.imports = []
        self.functions = []
        self.classes = []
        self.variables = []
        self.comments = []
        self.current_class = None
        
    def visit_Import(self, node):
        for alias in node.names:
            import_data = {
                "module": alias.name,
                "alias": alias.asname if alias.asname else "",
                "line": node.lineno
            }
            self.imports.append(import_data)
        self.generic_visit(node)
        
    def visit_ImportFrom(self, node):
        module = node.module or ''
        for alias in node.names:
            import_data = {
                "module": f"{module}.{alias.name}" if module else alias.name,
                "alias": alias.asname if alias.asname else "",
                "line": node.lineno
            }
            self.imports.append(import_data)
        self.generic_visit(node)
        
    def visit_FunctionDef(self, node):
        parameters = []
        for arg in node.args.args:
            param = {
                "name": arg.arg,
                "type": self.get_annotation(arg.annotation) if arg.annotation else "Any"
            }
            parameters.append(param)
            
        comments = self.extract_docstring(node)
        
        function_data = {
            "name": node.name,
            "start_line": node.lineno,
            "end_line": self.get_end_line(node),
            "parameters": parameters,
            "comments": comments
        }
        
        if self.current_class:
            self.current_class["methods"].append(function_data)
        else:
            self.functions.append(function_data)
            
        self.generic_visit(node)
        
    def visit_AsyncFunctionDef(self, node):
        # Handle async functions same as regular functions
        self.visit_FunctionDef(node)
        
    def visit_ClassDef(self, node):
        old_class = self.current_class
        
        comments = self.extract_docstring(node)
        
        class_data = {
            "name": node.name,
            "start_line": node.lineno,
            "end_line": self.get_end_line(node),
            "methods": [],
            "comments": comments
        }
        
        self.current_class = class_data
        self.generic_visit(node)
        self.current_class = old_class
        
        self.classes.append(class_data)
        
    def visit_Assign(self, node):
        # Extract variable assignments at module level
        if self.current_class is None:
            for target in node.targets:
                if isinstance(target, ast.Name):
                    var_data = {
                        "name": target.id,
                        "type": "Any",  # Type inference would be complex
                        "line": node.lineno
                    }
                    self.variables.append(var_data)
        self.generic_visit(node)
        
    def visit_AnnAssign(self, node):
        # Handle annotated assignments
        if self.current_class is None and isinstance(node.target, ast.Name):
            var_data = {
                "name": node.target.id,
                "type": self.get_annotation(node.annotation) if node.annotation else "Any",
                "line": node.lineno
            }
            self.variables.append(var_data)
        self.generic_visit(node)
        
    def extract_docstring(self, node):
        """Extract docstring from a node"""
        comments = []
        if (node.body and isinstance(node.body[0], ast.Expr) and 
            isinstance(node.body[0].value, ast.Str)):
            docstring = node.body[0].value.s
            comment = {
                "text": docstring,
                "start_line": node.body[0].lineno,
                "end_line": node.body[0].lineno + docstring.count('\n'),
                "type": "documentation"
            }
            comments.append(comment)
        return comments
        
    def get_annotation(self, annotation):
        """Convert annotation to string"""
        if isinstance(annotation, ast.Name):
            return annotation.id
        elif isinstance(annotation, ast.Attribute):
            return f"{self.get_annotation(annotation.value)}.{annotation.attr}"
        elif isinstance(annotation, ast.Subscript):
            return f"{self.get_annotation(annotation.value)}[{self.get_annotation(annotation.slice)}]"
        else:
            return "Any"
            
    def get_end_line(self, node):
        """Get the end line number of a node"""
        if hasattr(node, 'end_lineno') and node.end_lineno:
            return node.end_lineno
        # Fallback: estimate based on body
        if hasattr(node, 'body') and node.body:
            return max(self.get_end_line(child) for child in node.body)
        return node.lineno

def extract_ast_data(filepath):
    try:
        with open(filepath, 'r', encoding='utf-8') as f:
            content = f.read()
            
        tree = ast.parse(content, filepath)
        extractor = ASTExtractor(filepath)
        extractor.visit(tree)
        
        # Extract standalone comments (this is limited in Python AST)
        # For now, we'll just include docstrings found in functions/classes
        all_comments = []
        
        result = {
            "module": extractor.module,
            "imports": extractor.imports,
            "functions": extractor.functions,
            "classes": extractor.classes,
            "variables": extractor.variables,
            "comments": all_comments
        }
        
        return json.dumps(result, indent=2)
        
    except Exception as e:
        error_result = {
            "error": str(e),
            "module": "",
            "imports": [],
            "functions": [],
            "classes": [],
            "variables": [],
            "comments": []
        }
        return json.dumps(error_result, indent=2)

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python ast_extractor.py <file_path>")
        sys.exit(1)
        
    filepath = sys.argv[1]
    result = extract_ast_data(filepath)
    print(result)
`

func FindSourceFiles(rootDir string) ([]string, []string, error) {
	var goFiles, pythonFiles []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			// Skip vendor and hidden directories
			if info.Name() == "vendor" || info.Name() == ".git" || strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Store relative path
		relPath, err := filepath.Rel(".", path)
		if err != nil || strings.HasPrefix(relPath, "..") {
			relPath = path
		}

		if IsGoFile(path) {
			goFiles = append(goFiles, relPath)
		} else if IsPythonFile(path) {
			pythonFiles = append(pythonFiles, relPath)
		}

		return nil
	})

	return goFiles, pythonFiles, err
}
