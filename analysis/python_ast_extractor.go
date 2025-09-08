package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

// PythonASTExtractor extracts AST information from Python source files
type PythonASTExtractor struct {
	filePath    string
	packageName string
}

// NewPythonASTExtractor creates a new Python AST extractor
func NewPythonASTExtractor() *PythonASTExtractor {
	return &PythonASTExtractor{}
}

// PythonASTNode represents a node in Python AST
type PythonASTNode struct {
	Type                 string               `json:"type"`
	Name                 string               `json:"name"`
	StartLine            int                  `json:"start_line"`
	EndLine              int                  `json:"end_line"`
	ParameterCount       int                  `json:"parameter_count"`
	ReturnCount          int                  `json:"return_count"`
	Parameters           []models.Parameter   `json:"parameters,omitempty"`
	ReturnValues         []models.ReturnValue `json:"return_values,omitempty"`
	CyclomaticComplexity int                  `json:"cyclomatic_complexity"`
	Parent               string               `json:"parent"`
	Decorators           []string             `json:"decorators"`
	BaseClasses          []string             `json:"base_classes"`
}

// PythonImport represents an import in Python
type PythonImport struct {
	Module string `json:"module"`
	Name   string `json:"name"`
	Alias  string `json:"alias"`
	Line   int    `json:"line"`
}

// PythonRelationship represents a relationship between Python entities
type PythonRelationship struct {
	FromEntity string `json:"from_entity"`
	ToEntity   string `json:"to_entity"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
}

// PythonASTResult contains the complete AST analysis result
type PythonASTResult struct {
	Module        string               `json:"module"`
	Nodes         []PythonASTNode      `json:"nodes"`
	Imports       []PythonImport       `json:"imports"`
	Relationships []PythonRelationship `json:"relationships"`
}

// ExtractFile extracts AST information from a Python file
func (e *PythonASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*ASTResult, error) {
	// Create result container
	result := NewASTResult(filePath, "python")

	e.filePath = filePath

	// Extract package name from file path
	e.packageName = e.extractPackageName(filePath)
	result.PackageName = e.packageName

	// Run Python AST extraction
	pythonResult, err := e.runPythonASTExtraction(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract Python AST: %w", err)
	}

	// Convert Python AST results to generic AST nodes
	for _, node := range pythonResult.Nodes {
		astNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             "",
			MethodName:           "",
			FieldName:            "",
			NodeType:             e.mapPythonNodeType(node.Type),
			StartLine:            node.StartLine,
			EndLine:              node.EndLine,
			LineCount:            node.EndLine - node.StartLine + 1,
			CyclomaticComplexity: node.CyclomaticComplexity,
			ParameterCount:       node.ParameterCount,
			ReturnCount:          node.ReturnCount,
			Parameters:           node.Parameters,
			ReturnValues:         node.ReturnValues,
			LastModified:         time.Now(),
		}

		// Set appropriate fields based on node type
		switch node.Type {
		case "class":
			astNode.TypeName = node.Name
		case "method":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
				astNode.MethodName = node.Name
			} else {
				astNode.MethodName = node.Name
			}
		case "function":
			astNode.MethodName = node.Name
		case "variable", "attribute":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
			}
			astNode.FieldName = node.Name
		}

		result.AddNode(astNode)
	}

	// Convert relationships
	for _, rel := range pythonResult.Relationships {
		astRel := &models.ASTRelationship{
			FromASTID:        0, // Will be filled by analyzer
			ToASTID:          nil, // Will be resolved by analyzer if possible
			LineNo:           rel.Line,
			RelationshipType: models.RelationshipType(e.mapRelationshipType(rel.Type)),
			Text:             rel.Text,
		}
		result.AddRelationship(astRel)
	}

	// Convert imports to library relationships
	for _, imp := range pythonResult.Imports {
		libRel := &models.LibraryRelationship{
			ASTID:            0, // Will be filled by analyzer
			LibraryID:        0, // Will be resolved by analyzer
			LineNo:           imp.Line,
			RelationshipType: string(models.RelationshipImport),
			Text:             fmt.Sprintf("import %s (module=%s;alias=%s;framework=python)", imp.Module, imp.Module, imp.Alias),
		}
		result.Libraries = append(result.Libraries, libRel)
	}

	return result, nil
}

// extractPackageName extracts package name from file path
func (e *PythonASTExtractor) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)
	parts := strings.Split(dir, string(filepath.Separator))

	// Look for common Python package indicators
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "src" || parts[i] == "lib" || parts[i] == "app" {
			if i < len(parts)-1 {
				return strings.Join(parts[i+1:], ".")
			}
		}
		// Check for __init__.py in directory
		initPath := filepath.Join(strings.Join(parts[:i+1], string(filepath.Separator)), "__init__.py")
		if _, err := os.Stat(initPath); err == nil {
			return strings.Join(parts[i:], ".")
		}
	}

	// Default to last directory name
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "main"
}

// runPythonASTExtraction runs the Python AST extraction script
func (e *PythonASTExtractor) runPythonASTExtraction(filePath string) (*PythonASTResult, error) {
	// Create temp file with Python script
	tmpFile, err := os.CreateTemp("", "python_ast_*.py")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(pythonASTExtractorScript); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}

	// Execute the script
	cmd := exec.Command("python3", tmpFile.Name(), filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try with python if python3 fails
		cmd = exec.Command("python", tmpFile.Name(), filePath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("python AST extraction failed: %w - output: %s", err, string(output))
		}
	}

	// Parse JSON output
	var result PythonASTResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Python AST JSON: %w", err)
	}

	return &result, nil
}

// mapPythonNodeType maps Python node types to generic AST node types
func (e *PythonASTExtractor) mapPythonNodeType(pythonType string) string {
	switch pythonType {
	case "class":
		return models.NodeTypeType
	case "function", "method":
		return models.NodeTypeMethod
	case "variable", "attribute":
		return models.NodeTypeField
	default:
		return models.NodeTypeVariable
	}
}

// mapRelationshipType maps Python relationship types to generic relationship types
func (e *PythonASTExtractor) mapRelationshipType(pythonRelType string) string {
	switch pythonRelType {
	case "inherits":
		return models.RelationshipInheritance
	case "calls":
		return models.RelationshipCall
	case "imports":
		return models.RelationshipImport
	case "uses":
		return models.RelationshipReference
	default:
		return models.RelationshipReference
	}
}

// getNodeFullName returns the full qualified name of a Python node
func (e *PythonASTExtractor) getNodeFullName(node PythonASTNode) string {
	parts := []string{e.packageName}

	if node.Parent != "" {
		parts = append(parts, node.Parent)
	}

	parts = append(parts, node.Name)
	return strings.Join(parts, ".")
}

// Python AST extractor script
const pythonASTExtractorScript = `
import ast
import json
import sys
import os

class CyclomaticComplexityVisitor(ast.NodeVisitor):
    """Calculate cyclomatic complexity of a function/method"""
    
    def __init__(self):
        self.complexity = 1
    
    def visit_If(self, node):
        self.complexity += 1
        self.generic_visit(node)
    
    def visit_While(self, node):
        self.complexity += 1
        self.generic_visit(node)
    
    def visit_For(self, node):
        self.complexity += 1
        self.generic_visit(node)
    
    def visit_ExceptHandler(self, node):
        self.complexity += 1
        self.generic_visit(node)
    
    def visit_With(self, node):
        self.complexity += 1
        self.generic_visit(node)
    
    def visit_Assert(self, node):
        self.complexity += 1
        self.generic_visit(node)
    
    def visit_BoolOp(self, node):
        # Each 'and' or 'or' adds to complexity
        self.complexity += len(node.values) - 1
        self.generic_visit(node)
    
    def visit_Lambda(self, node):
        self.complexity += 1
        self.generic_visit(node)

class PythonASTExtractor(ast.NodeVisitor):
    def __init__(self, filename):
        self.filename = filename
        self.module = os.path.splitext(os.path.basename(filename))[0]
        self.nodes = []
        self.imports = []
        self.relationships = []
        self.current_class = None
        self.import_map = {}
    
    def extract(self, tree):
        self.visit(tree)
        return {
            "module": self.module,
            "nodes": self.nodes,
            "imports": self.imports,
            "relationships": self.relationships
        }
    
    def calculate_complexity(self, node):
        visitor = CyclomaticComplexityVisitor()
        visitor.visit(node)
        return visitor.complexity
    
    def visit_Import(self, node):
        for alias in node.names:
            import_info = {
                "module": alias.name,
                "name": alias.name.split('.')[-1],
                "alias": alias.asname if alias.asname else alias.name,
                "line": node.lineno
            }
            self.imports.append(import_info)
            self.import_map[import_info["alias"]] = alias.name
        self.generic_visit(node)
    
    def visit_ImportFrom(self, node):
        module = node.module or ''
        for alias in node.names:
            full_name = f"{module}.{alias.name}" if module else alias.name
            import_info = {
                "module": module,
                "name": alias.name,
                "alias": alias.asname if alias.asname else alias.name,
                "line": node.lineno
            }
            self.imports.append(import_info)
            self.import_map[import_info["alias"]] = full_name
        self.generic_visit(node)
    
    def visit_ClassDef(self, node):
        # Extract base classes
        base_classes = []
        for base in node.bases:
            if isinstance(base, ast.Name):
                base_classes.append(base.id)
            elif isinstance(base, ast.Attribute):
                base_classes.append(self.get_full_name(base))
        
        # Extract decorators
        decorators = [self.get_decorator_name(d) for d in node.decorator_list]
        
        class_info = {
            "type": "class",
            "name": node.name,
            "start_line": node.lineno,
            "end_line": node.end_lineno if hasattr(node, 'end_lineno') else node.lineno,
            "parameter_count": 0,
            "return_count": 0,
            "cyclomatic_complexity": 1,
            "parent": "",
            "decorators": decorators,
            "base_classes": base_classes
        }
        self.nodes.append(class_info)
        
        # Add inheritance relationships
        for base in base_classes:
            self.relationships.append({
                "from_entity": node.name,
                "to_entity": base,
                "type": "inherits",
                "line": node.lineno,
                "text": f"inherits from {base}"
            })
        
        # Process class body
        old_class = self.current_class
        self.current_class = node.name
        self.generic_visit(node)
        self.current_class = old_class
    
    def visit_FunctionDef(self, node):
        self.process_function(node, "function")
    
    def visit_AsyncFunctionDef(self, node):
        self.process_function(node, "function")
    
    def process_function(self, node, func_type):
        # Determine if it's a method or function
        is_method = self.current_class is not None
        node_type = "method" if is_method else "function"
        
        # Extract detailed parameter information
        parameters = self.extract_parameters(node.args, is_method)
        param_count = len(parameters)
        
        # Extract return type information
        return_values = self.extract_return_values(node)
        return_count = len(return_values)
        
        # Calculate cyclomatic complexity
        complexity = self.calculate_complexity(node)
        
        # Extract decorators
        decorators = [self.get_decorator_name(d) for d in node.decorator_list]
        
        func_info = {
            "type": node_type,
            "name": node.name,
            "start_line": node.lineno,
            "end_line": node.end_lineno if hasattr(node, 'end_lineno') else node.lineno,
            "parameter_count": param_count,
            "return_count": return_count,
            "parameters": parameters,
            "return_values": return_values,
            "cyclomatic_complexity": complexity,
            "parent": self.current_class or "",
            "decorators": decorators,
            "base_classes": []
        }
        self.nodes.append(func_info)
        
        # Extract function calls
        self.extract_calls(node)
    
    def extract_calls(self, node):
        """Extract function/method calls from a node"""
        for child in ast.walk(node):
            if isinstance(child, ast.Call):
                called_name = self.get_call_name(child)
                if called_name:
                    caller = self.current_class + "." + node.name if self.current_class else node.name
                    self.relationships.append({
                        "from_entity": caller,
                        "to_entity": called_name,
                        "type": "calls",
                        "line": child.lineno if hasattr(child, 'lineno') else 0,
                        "text": f"calls {called_name}"
                    })
    
    def get_call_name(self, node):
        """Get the name of a called function/method"""
        if isinstance(node.func, ast.Name):
            return node.func.id
        elif isinstance(node.func, ast.Attribute):
            return self.get_full_name(node.func)
        return None
    
    def get_full_name(self, node):
        """Get full qualified name from an attribute node"""
        parts = []
        while isinstance(node, ast.Attribute):
            parts.insert(0, node.attr)
            node = node.value
        if isinstance(node, ast.Name):
            parts.insert(0, node.id)
        return '.'.join(parts)
    
    def get_decorator_name(self, decorator):
        """Get decorator name as string"""
        if isinstance(decorator, ast.Name):
            return decorator.id
        elif isinstance(decorator, ast.Attribute):
            return self.get_full_name(decorator)
        elif isinstance(decorator, ast.Call):
            if isinstance(decorator.func, ast.Name):
                return decorator.func.id
            elif isinstance(decorator.func, ast.Attribute):
                return self.get_full_name(decorator.func)
        return "unknown"
    
    def extract_parameters(self, args, is_method):
        """Extract detailed parameter information"""
        parameters = []
        
        # Regular arguments
        for i, arg in enumerate(args.args):
            # Skip 'self' parameter for methods
            if is_method and i == 0 and arg.arg == 'self':
                continue
                
            param_type = "Any"
            if arg.annotation:
                param_type = self.get_type_string(arg.annotation)
                
            parameters.append({
                "name": arg.arg,
                "type": param_type,
                "name_length": len(arg.arg)
            })
        
        # *args parameter
        if args.vararg:
            param_type = "Any"
            if args.vararg.annotation:
                param_type = self.get_type_string(args.vararg.annotation)
            parameters.append({
                "name": "*" + args.vararg.arg,
                "type": param_type,
                "name_length": len(args.vararg.arg) + 1
            })
        
        # **kwargs parameter
        if args.kwarg:
            param_type = "Any"
            if args.kwarg.annotation:
                param_type = self.get_type_string(args.kwarg.annotation)
            parameters.append({
                "name": "**" + args.kwarg.arg,
                "type": param_type,
                "name_length": len(args.kwarg.arg) + 2
            })
            
        return parameters
    
    def extract_return_values(self, node):
        """Extract return type information"""
        return_values = []
        
        # Check for return annotation
        if hasattr(node, 'returns') and node.returns:
            return_type = self.get_type_string(node.returns)
            return_values.append({
                "name": "",  # Python return values are typically unnamed
                "type": return_type
            })
        else:
            # No annotation, check for actual return statements
            return_count = sum(1 for n in ast.walk(node) if isinstance(n, ast.Return))
            if return_count > 0:
                return_values.append({
                    "name": "",
                    "type": "Any"
                })
                
        return return_values
    
    def get_type_string(self, annotation):
        """Convert a type annotation to string"""
        if isinstance(annotation, ast.Name):
            return annotation.id
        elif isinstance(annotation, ast.Attribute):
            return self.get_full_name(annotation)
        elif isinstance(annotation, ast.Subscript):
            # Handle List[int], Dict[str, int], etc.
            value = self.get_type_string(annotation.value)
            if isinstance(annotation.slice, ast.Tuple):
                # Multiple type args like Dict[str, int]
                slice_types = [self.get_type_string(elt) for elt in annotation.slice.elts]
                return f"{value}[{', '.join(slice_types)}]"
            else:
                # Single type arg like List[int]
                slice_type = self.get_type_string(annotation.slice)
                return f"{value}[{slice_type}]"
        elif isinstance(annotation, ast.Constant):
            return str(annotation.value)
        else:
            return "Any"
    
    def visit_Assign(self, node):
        """Visit assignments to extract class/module level variables"""
        if self.current_class or isinstance(node.value, (ast.Constant, ast.List, ast.Dict, ast.Tuple)):
            for target in node.targets:
                if isinstance(target, ast.Name):
                    var_info = {
                        "type": "variable" if not self.current_class else "attribute",
                        "name": target.id,
                        "start_line": node.lineno,
                        "end_line": node.lineno,
                        "parameter_count": 0,
                        "return_count": 0,
                        "cyclomatic_complexity": 0,
                        "parent": self.current_class or "",
                        "decorators": [],
                        "base_classes": []
                    }
                    self.nodes.append(var_info)
        self.generic_visit(node)
    
    def visit_AnnAssign(self, node):
        """Visit annotated assignments"""
        if isinstance(node.target, ast.Name):
            var_info = {
                "type": "variable" if not self.current_class else "attribute",
                "name": node.target.id,
                "start_line": node.lineno,
                "end_line": node.lineno,
                "parameter_count": 0,
                "return_count": 0,
                "cyclomatic_complexity": 0,
                "parent": self.current_class or "",
                "decorators": [],
                "base_classes": []
            }
            self.nodes.append(var_info)
        self.generic_visit(node)

def main():
    if len(sys.argv) != 2:
        print(json.dumps({"error": "Usage: python script.py <file>"}))
        sys.exit(1)
    
    filepath = sys.argv[1]
    
    try:
        with open(filepath, 'r', encoding='utf-8') as f:
            source = f.read()
        
        tree = ast.parse(source, filepath)
        extractor = PythonASTExtractor(filepath)
        result = extractor.extract(tree)
        
        print(json.dumps(result, indent=2))
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)

if __name__ == "__main__":
    main()
`
