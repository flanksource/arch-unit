#!/usr/bin/env python3
"""
Python AST Extractor for arch-unit
Extracts AST information from Python source files and outputs JSON
"""

import ast
import json
import sys
from typing import List, Dict, Any, Optional


class PythonASTNode:
    """Represents a node in the Python AST"""
    def __init__(self, node_type: str, name: str, start_line: int, end_line: int):
        self.type = node_type
        self.name = name
        self.start_line = start_line
        self.end_line = end_line
        self.parameter_count = 0
        self.return_count = 0
        self.parameters = []
        self.return_values = []
        self.cyclomatic_complexity = 1
        self.parent = ""
        self.decorators = []
        self.base_classes = []


class CyclomaticComplexityCalculator(ast.NodeVisitor):
    """Calculates cyclomatic complexity for a function/method"""

    def __init__(self):
        self.complexity = 1  # Base complexity

    def visit_If(self, node):
        self.complexity += 1
        # Count elif clauses
        if hasattr(node, 'orelse') and node.orelse:
            if isinstance(node.orelse[0], ast.If):
                # This is handled by the elif visit
                pass
            else:
                # This is an else clause
                pass
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

    def visit_AsyncWith(self, node):
        self.complexity += 1
        self.generic_visit(node)

    def visit_AsyncFor(self, node):
        self.complexity += 1
        self.generic_visit(node)

    def visit_BoolOp(self, node):
        if isinstance(node.op, (ast.And, ast.Or)):
            self.complexity += len(node.values) - 1
        self.generic_visit(node)

    def visit_ListComp(self, node):
        for generator in node.generators:
            if generator.ifs:
                self.complexity += len(generator.ifs)
        self.generic_visit(node)

    def visit_SetComp(self, node):
        for generator in node.generators:
            if generator.ifs:
                self.complexity += len(generator.ifs)
        self.generic_visit(node)

    def visit_DictComp(self, node):
        for generator in node.generators:
            if generator.ifs:
                self.complexity += len(generator.ifs)
        self.generic_visit(node)

    def visit_GeneratorExp(self, node):
        for generator in node.generators:
            if generator.ifs:
                self.complexity += len(generator.ifs)
        self.generic_visit(node)


class PythonASTExtractor(ast.NodeVisitor):
    """Main AST extractor class"""

    def __init__(self, source_code: str, file_path: str):
        self.source_code = source_code
        self.file_path = file_path
        self.nodes = []
        self.imports = []
        self.relationships = []
        self.current_class = None
        self.source_lines = source_code.splitlines()

    def extract(self) -> Dict[str, Any]:
        """Extract AST information and return as dictionary"""
        try:
            tree = ast.parse(self.source_code, filename=self.file_path)
            self.visit(tree)

            return {
                "module": self._get_module_name(),
                "nodes": [self._node_to_dict(node) for node in self.nodes],
                "imports": self.imports,
                "relationships": self.relationships
            }
        except SyntaxError as e:
            # Return empty result for syntax errors
            return {
                "module": self._get_module_name(),
                "nodes": [],
                "imports": [],
                "relationships": []
            }

    def _get_module_name(self) -> str:
        """Extract module name from file path"""
        import os
        return os.path.splitext(os.path.basename(self.file_path))[0]

    def _node_to_dict(self, node: PythonASTNode) -> Dict[str, Any]:
        """Convert PythonASTNode to dictionary"""
        return {
            "type": node.type,
            "name": node.name,
            "start_line": node.start_line,
            "end_line": node.end_line,
            "parameter_count": node.parameter_count,
            "return_count": node.return_count,
            "parameters": node.parameters,
            "return_values": node.return_values,
            "cyclomatic_complexity": node.cyclomatic_complexity,
            "parent": node.parent,
            "decorators": node.decorators,
            "base_classes": node.base_classes
        }

    def visit_ClassDef(self, node):
        """Visit class definition"""
        # Create class node
        class_node = PythonASTNode(
            node_type="class",
            name=node.name,
            start_line=node.lineno,
            end_line=self._get_end_line(node)
        )

        # Extract base classes
        class_node.base_classes = [self._get_name_from_node(base) for base in node.bases]

        # Extract decorators
        class_node.decorators = [self._get_name_from_node(dec) for dec in node.decorator_list]

        self.nodes.append(class_node)

        # Set current class context for methods
        previous_class = self.current_class
        self.current_class = node.name

        # Visit class body
        self.generic_visit(node)

        # Restore previous class context
        self.current_class = previous_class

    def visit_FunctionDef(self, node):
        """Visit function/method definition"""
        # Determine if this is a method or function
        if self.current_class:
            node_type = "method"
            parent = self.current_class
        else:
            node_type = "function"
            parent = ""

        # Create function/method node
        func_node = PythonASTNode(
            node_type=node_type,
            name=node.name,
            start_line=node.lineno,
            end_line=self._get_end_line(node)
        )
        func_node.parent = parent

        # Extract parameters
        parameters = []
        args = node.args

        # Regular arguments
        for arg in args.args:
            param_type = self._get_annotation(arg.annotation) if arg.annotation else ""
            parameters.append({
                "name": arg.arg,
                "type": param_type
            })

        # *args
        if args.vararg:
            param_type = self._get_annotation(args.vararg.annotation) if args.vararg.annotation else ""
            parameters.append({
                "name": f"*{args.vararg.arg}",
                "type": param_type
            })

        # **kwargs
        if args.kwarg:
            param_type = self._get_annotation(args.kwarg.annotation) if args.kwarg.annotation else ""
            parameters.append({
                "name": f"**{args.kwarg.arg}",
                "type": param_type
            })

        func_node.parameters = parameters
        func_node.parameter_count = len(parameters)

        # Extract return type
        if node.returns:
            return_type = self._get_annotation(node.returns)
            func_node.return_values = [{"name": "", "type": return_type}]
            func_node.return_count = 1

        # Calculate cyclomatic complexity
        complexity_calc = CyclomaticComplexityCalculator()
        complexity_calc.visit(node)
        func_node.cyclomatic_complexity = complexity_calc.complexity

        # Extract decorators
        func_node.decorators = [self._get_name_from_node(dec) for dec in node.decorator_list]

        self.nodes.append(func_node)

        # Don't visit function body to avoid nested function detection
        # self.generic_visit(node)

    def visit_AsyncFunctionDef(self, node):
        """Visit async function definition"""
        # Treat async functions the same as regular functions
        self.visit_FunctionDef(node)

    def visit_Import(self, node):
        """Visit import statement"""
        for alias in node.names:
            import_info = {
                "module": alias.name,
                "name": alias.asname if alias.asname else alias.name,
                "alias": alias.asname if alias.asname else "",
                "line": node.lineno
            }
            self.imports.append(import_info)

    def visit_ImportFrom(self, node):
        """Visit from...import statement"""
        module = node.module if node.module else ""
        for alias in node.names:
            import_info = {
                "module": f"{module}.{alias.name}" if module else alias.name,
                "name": alias.asname if alias.asname else alias.name,
                "alias": alias.asname if alias.asname else "",
                "line": node.lineno
            }
            self.imports.append(import_info)

    def _get_end_line(self, node) -> int:
        """Get the end line of a node"""
        if hasattr(node, 'end_lineno') and node.end_lineno:
            return node.end_lineno

        # Fallback: find the last line with content
        start_line = node.lineno
        max_line = start_line

        for child in ast.walk(node):
            if hasattr(child, 'lineno') and child.lineno:
                max_line = max(max_line, child.lineno)

        return max_line

    def _get_annotation(self, annotation) -> str:
        """Extract type annotation as string"""
        if annotation is None:
            return ""

        try:
            if isinstance(annotation, ast.Name):
                return annotation.id
            elif isinstance(annotation, ast.Constant):
                return str(annotation.value)
            elif isinstance(annotation, ast.Attribute):
                return f"{self._get_annotation(annotation.value)}.{annotation.attr}"
            else:
                # For complex annotations, return a simplified representation
                return ast.unparse(annotation) if hasattr(ast, 'unparse') else ""
        except:
            return ""

    def _get_name_from_node(self, node) -> str:
        """Extract name from various node types"""
        try:
            if isinstance(node, ast.Name):
                return node.id
            elif isinstance(node, ast.Attribute):
                return f"{self._get_name_from_node(node.value)}.{node.attr}"
            elif isinstance(node, ast.Constant):
                return str(node.value)
            else:
                return ast.unparse(node) if hasattr(ast, 'unparse') else ""
        except:
            return ""


def main():
    """Main entry point"""
    if len(sys.argv) != 2:
        print(json.dumps({
            "module": "",
            "nodes": [],
            "imports": [],
            "relationships": []
        }))
        sys.exit(1)

    file_path = sys.argv[1]

    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            source_code = f.read()

        extractor = PythonASTExtractor(source_code, file_path)
        result = extractor.extract()

        print(json.dumps(result, indent=None, separators=(',', ':')))

    except Exception as e:
        # Return empty result on any error
        print(json.dumps({
            "module": "",
            "nodes": [],
            "imports": [],
            "relationships": []
        }))
        sys.exit(0)


if __name__ == "__main__":
    main()