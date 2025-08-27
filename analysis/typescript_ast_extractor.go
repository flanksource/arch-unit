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
	flanksourceContext "github.com/flanksource/commons/context"
)

// TypeScriptASTExtractor extracts AST information from TypeScript source files
type TypeScriptASTExtractor struct {
	cache       *cache.ASTCache
	filePath    string
	packageName string
	depsManager *NodeDependenciesManager
}

// NewTypeScriptASTExtractor creates a new TypeScript AST extractor
func NewTypeScriptASTExtractor(astCache *cache.ASTCache) *TypeScriptASTExtractor {
	return &TypeScriptASTExtractor{
		cache:       astCache,
		depsManager: NewNodeDependenciesManager(),
	}
}

// TypeScriptASTNode represents a node in TypeScript AST
type TypeScriptASTNode struct {
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
	IsAsync              bool                 `json:"is_async"`
	IsGeneric            bool                 `json:"is_generic"`
	IsAbstract           bool                 `json:"is_abstract"`
	IsReadonly           bool                 `json:"is_readonly"`
	TypeParams           []string             `json:"type_params"`
	Modifiers            []string             `json:"modifiers"`
}

// TypeScriptImport represents an import in TypeScript
type TypeScriptImport struct {
	Source    string   `json:"source"`
	Imported  []string `json:"imported"`
	TypesOnly bool     `json:"types_only"`
	Line      int      `json:"line"`
}

// TypeScriptRelationship represents a relationship between TypeScript entities
type TypeScriptRelationship struct {
	FromEntity string `json:"from_entity"`
	ToEntity   string `json:"to_entity"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
}

// TypeScriptASTResult contains the complete AST analysis result
type TypeScriptASTResult struct {
	Module        string                   `json:"module"`
	Nodes         []TypeScriptASTNode      `json:"nodes"`
	Imports       []TypeScriptImport       `json:"imports"`
	Relationships []TypeScriptRelationship `json:"relationships"`
}

// ExtractFile extracts AST information from a TypeScript file
func (e *TypeScriptASTExtractor) ExtractFile(ctx flanksourceContext.Context, filePath string) error {
	// Check if file needs re-analysis
	needsAnalysis, err := e.cache.NeedsReanalysis(filePath)
	if err != nil {
		return fmt.Errorf("failed to check if file needs analysis: %w", err)
	}

	if !needsAnalysis {
		return nil // File is up to date
	}

	e.filePath = filePath

	// Clear existing AST data for the file
	if err := e.cache.DeleteASTForFile(filePath); err != nil {
		return fmt.Errorf("failed to clear existing AST data: %w", err)
	}

	// Extract package name from file path or package.json
	e.packageName = e.extractPackageName(filePath)

	// Run TypeScript AST extraction
	result, err := e.runTypeScriptASTExtraction(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to extract TypeScript AST: %w", err)
	}

	// Store nodes in cache
	nodeMap := make(map[string]int64)
	for _, node := range result.Nodes {
		astNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             "",
			MethodName:           "",
			FieldName:            "",
			NodeType:             e.mapTypeScriptNodeType(node.Type),
			StartLine:            node.StartLine,
			EndLine:              node.EndLine,
			LineCount:            node.EndLine - node.StartLine + 1,
			CyclomaticComplexity: node.CyclomaticComplexity,
			Parameters:           node.Parameters,
			ReturnValues:         node.ReturnValues,
			LastModified:         time.Now(),
		}

		// Set appropriate fields based on node type
		switch node.Type {
		case "class", "interface", "enum", "type":
			astNode.TypeName = node.Name
		case "method", "constructor":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
				astNode.MethodName = node.Name
			} else {
				astNode.MethodName = node.Name
			}
		case "function":
			astNode.MethodName = node.Name
		case "variable", "property", "field":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
			}
			astNode.FieldName = node.Name
		}

		nodeID, err := e.cache.StoreASTNode(astNode)
		if err != nil {
			return fmt.Errorf("failed to store AST node: %w", err)
		}

		fullName := e.getNodeFullName(node)
		nodeMap[fullName] = nodeID
	}

	// Store relationships
	for _, rel := range result.Relationships {
		fromID, fromExists := nodeMap[rel.FromEntity]
		if !fromExists {
			continue
		}

		var toID *int64
		if toNodeID, toExists := nodeMap[rel.ToEntity]; toExists {
			toID = &toNodeID
		}

		relType := e.mapRelationshipType(rel.Type)
		err := e.cache.StoreASTRelationship(fromID, toID, rel.Line, relType, rel.Text)
		if err != nil {
			// Log but don't fail on relationship storage errors
			continue
		}
	}

	// Store imports as library relationships
	libResolver := NewLibraryResolver(e.cache)
	for _, imp := range result.Imports {
		// Try to resolve the import as a library
		libID, err := libResolver.ResolveTypeScriptLibrary(imp.Source)
		if err == nil && libID > 0 {
			// Find the node that contains this import (usually module level)
			for fullName, nodeID := range nodeMap {
				if strings.HasPrefix(fullName, e.packageName) {
					importText := "import from " + imp.Source
					if imp.TypesOnly {
						importText = "import type from " + imp.Source
					}
					e.cache.StoreLibraryRelationship(nodeID, libID, imp.Line,
						models.RelationshipImport, importText)
					break
				}
			}
		}
	}

	// Update file metadata
	if err := e.cache.UpdateFileMetadata(filePath); err != nil {
		return fmt.Errorf("failed to update file metadata: %w", err)
	}

	return nil
}

// extractPackageName extracts package name from file path or package.json
func (e *TypeScriptASTExtractor) extractPackageName(filePath string) string {
	dir := filepath.Dir(filePath)

	// Look for package.json
	for currentDir := dir; currentDir != "/" && currentDir != ""; currentDir = filepath.Dir(currentDir) {
		packageJSONPath := filepath.Join(currentDir, "package.json")
		if data, err := os.ReadFile(packageJSONPath); err == nil {
			var packageJSON map[string]interface{}
			if json.Unmarshal(data, &packageJSON) == nil {
				if name, ok := packageJSON["name"].(string); ok {
					return name
				}
			}
		}
	}

	// Default to directory structure
	parts := strings.Split(dir, string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "src" || parts[i] == "lib" || parts[i] == "app" {
			if i < len(parts)-1 {
				return strings.Join(parts[i+1:], "/")
			}
		}
	}

	// Default to last directory name
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "main"
}

// runTypeScriptASTExtraction runs the TypeScript AST extraction script
func (e *TypeScriptASTExtractor) runTypeScriptASTExtraction(ctx flanksourceContext.Context, filePath string) (*TypeScriptASTResult, error) {
	// Create parser script with proper module resolution
	scriptPath, err := e.depsManager.CreateParserScript(ctx, typescriptASTExtractorScript, "typescript")
	if err != nil {
		return nil, fmt.Errorf("failed to create parser script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Execute the script with Node.js
	cmd := exec.Command("node", scriptPath, filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("TypeScript AST extraction failed: %w - output: %s", err, string(output))
	}

	// Parse JSON output
	var result TypeScriptASTResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse TypeScript AST JSON: %w", err)
	}

	return &result, nil
}

// mapTypeScriptNodeType maps TypeScript node types to generic AST node types
func (e *TypeScriptASTExtractor) mapTypeScriptNodeType(tsType string) string {
	switch tsType {
	case "class", "interface", "enum", "type":
		return models.NodeTypeType
	case "function", "method", "constructor":
		return models.NodeTypeMethod
	case "variable", "property", "field":
		return models.NodeTypeField
	default:
		return models.NodeTypeVariable
	}
}

// mapRelationshipType maps TypeScript relationship types to generic relationship types
func (e *TypeScriptASTExtractor) mapRelationshipType(tsRelType string) string {
	switch tsRelType {
	case "extends":
		return models.RelationshipExtends
	case "implements":
		return models.RelationshipImplements
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

// getNodeFullName returns the full qualified name of a TypeScript node
func (e *TypeScriptASTExtractor) getNodeFullName(node TypeScriptASTNode) string {
	parts := []string{e.packageName}

	if node.Parent != "" {
		parts = append(parts, node.Parent)
	}

	parts = append(parts, node.Name)
	return strings.Join(parts, ".")
}

// TypeScript AST extractor script using TypeScript Compiler API
const typescriptASTExtractorScript = `
const ts = require('typescript');
const fs = require('fs');
const path = require('path');

class TypeScriptASTExtractor {
  constructor(filename) {
    this.filename = filename;
    this.module = path.basename(filename, path.extname(filename));
    this.nodes = [];
    this.imports = [];
    this.relationships = [];
    this.currentClass = null;
    this.currentNamespace = null;
  }

  extract(sourceFile) {
    this.visitNode(sourceFile);
    return {
      module: this.module,
      nodes: this.nodes,
      imports: this.imports,
      relationships: this.relationships
    };
  }

  calculateComplexity(node) {
    let complexity = 1;

    const visit = (node) => {
      switch (node.kind) {
        case ts.SyntaxKind.IfStatement:
        case ts.SyntaxKind.ConditionalExpression:
          complexity++;
          break;
        case ts.SyntaxKind.ForStatement:
        case ts.SyntaxKind.ForInStatement:
        case ts.SyntaxKind.ForOfStatement:
        case ts.SyntaxKind.WhileStatement:
        case ts.SyntaxKind.DoStatement:
          complexity++;
          break;
        case ts.SyntaxKind.CatchClause:
          complexity++;
          break;
        case ts.SyntaxKind.CaseClause:
          if (node.expression) complexity++;
          break;
        case ts.SyntaxKind.BinaryExpression:
          const op = node.operatorToken.kind;
          if (op === ts.SyntaxKind.AmpersandAmpersandToken ||
              op === ts.SyntaxKind.BarBarToken ||
              op === ts.SyntaxKind.QuestionQuestionToken) {
            complexity++;
          }
          break;
      }
      ts.forEachChild(node, visit);
    };

    visit(node);
    return complexity;
  }

  visitNode(node) {
    switch (node.kind) {
      case ts.SyntaxKind.ImportDeclaration:
        this.processImport(node);
        break;
      case ts.SyntaxKind.ClassDeclaration:
        this.processClass(node);
        break;
      case ts.SyntaxKind.InterfaceDeclaration:
        this.processInterface(node);
        break;
      case ts.SyntaxKind.EnumDeclaration:
        this.processEnum(node);
        break;
      case ts.SyntaxKind.TypeAliasDeclaration:
        this.processTypeAlias(node);
        break;
      case ts.SyntaxKind.FunctionDeclaration:
        this.processFunction(node);
        break;
      case ts.SyntaxKind.VariableStatement:
        this.processVariableStatement(node);
        break;
      case ts.SyntaxKind.ModuleDeclaration:
        this.processNamespace(node);
        break;
    }

    ts.forEachChild(node, child => this.visitNode(child));
  }

  processImport(node) {
    const moduleSpecifier = node.moduleSpecifier ? node.moduleSpecifier.text : '';
    const imported = [];
    const isTypeOnly = node.importClause && node.importClause.isTypeOnly;

    if (node.importClause) {
      if (node.importClause.name) {
        imported.push('default');
      }
      if (node.importClause.namedBindings) {
        if (node.importClause.namedBindings.kind === ts.SyntaxKind.NamespaceImport) {
          imported.push('*');
        } else {
          node.importClause.namedBindings.elements.forEach(element => {
            imported.push(element.name.text);
          });
        }
      }
    }

    this.imports.push({
      source: moduleSpecifier,
      imported: imported,
      types_only: isTypeOnly || false,
      line: this.getLineNumber(node)
    });
  }

  processClass(node) {
    const className = node.name ? node.name.text : 'AnonymousClass';
    const modifiers = this.getModifiers(node);
    const typeParams = this.getTypeParameters(node);

    this.nodes.push({
      type: 'class',
      name: className,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: 0,
      return_count: 0,
      cyclomatic_complexity: 1,
      parent: this.currentNamespace || '',
      is_async: false,
      is_generic: typeParams.length > 0,
      is_abstract: modifiers.includes('abstract'),
      is_readonly: false,
      type_params: typeParams,
      return_type: '',
      modifiers: modifiers
    });

    // Process heritage clauses (extends/implements)
    if (node.heritageClauses) {
      node.heritageClauses.forEach(clause => {
        clause.types.forEach(type => {
          const typeName = this.getTypeName(type.expression);
          const relType = clause.token === ts.SyntaxKind.ExtendsKeyword ? 'extends' : 'implements';

          this.relationships.push({
            from_entity: className,
            to_entity: typeName,
            type: relType,
            line: this.getLineNumber(type),
            text: relType + ' ' + typeName
          });
        });
      });
    }

    // Process class members
    const oldClass = this.currentClass;
    this.currentClass = className;

    if (node.members) {
      node.members.forEach(member => {
        this.processClassMember(member);
      });
    }

    this.currentClass = oldClass;
  }

  processInterface(node) {
    const interfaceName = node.name ? node.name.text : 'AnonymousInterface';
    const typeParams = this.getTypeParameters(node);

    this.nodes.push({
      type: 'interface',
      name: interfaceName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: 0,
      return_count: 0,
      cyclomatic_complexity: 0,
      parent: this.currentNamespace || '',
      is_async: false,
      is_generic: typeParams.length > 0,
      is_abstract: false,
      is_readonly: false,
      type_params: typeParams,
      return_type: '',
      modifiers: []
    });

    // Process extends clauses
    if (node.heritageClauses) {
      node.heritageClauses.forEach(clause => {
        clause.types.forEach(type => {
          const typeName = this.getTypeName(type.expression);

          this.relationships.push({
            from_entity: interfaceName,
            to_entity: typeName,
            type: 'extends',
            line: this.getLineNumber(type),
            text: 'extends ' + typeName
          });
        });
      });
    }

    // Process interface members
    const oldClass = this.currentClass;
    this.currentClass = interfaceName;

    if (node.members) {
      node.members.forEach(member => {
        this.processInterfaceMember(member);
      });
    }

    this.currentClass = oldClass;
  }

  processEnum(node) {
    const enumName = node.name ? node.name.text : 'AnonymousEnum';
    const modifiers = this.getModifiers(node);

    this.nodes.push({
      type: 'enum',
      name: enumName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: 0,
      return_count: 0,
      cyclomatic_complexity: 0,
      parent: this.currentNamespace || '',
      is_async: false,
      is_generic: false,
      is_abstract: false,
      is_readonly: modifiers.includes('const'),
      type_params: [],
      return_type: '',
      modifiers: modifiers
    });

    // Process enum members
    if (node.members) {
      node.members.forEach(member => {
        if (member.name) {
          this.nodes.push({
            type: 'field',
            name: this.getNodeText(member.name),
            start_line: this.getLineNumber(member),
            end_line: this.getEndLineNumber(member),
            parameter_count: 0,
            return_count: 0,
            cyclomatic_complexity: 0,
            parent: enumName,
            is_async: false,
            is_generic: false,
            is_abstract: false,
            is_readonly: true,
            type_params: [],
            return_type: '',
            modifiers: []
          });
        }
      });
    }
  }

  processTypeAlias(node) {
    const typeName = node.name ? node.name.text : 'AnonymousType';
    const typeParams = this.getTypeParameters(node);

    this.nodes.push({
      type: 'type',
      name: typeName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: 0,
      return_count: 0,
      cyclomatic_complexity: 0,
      parent: this.currentNamespace || '',
      is_async: false,
      is_generic: typeParams.length > 0,
      is_abstract: false,
      is_readonly: false,
      type_params: typeParams,
      return_type: '',
      modifiers: []
    });
  }

  processFunction(node) {
    const funcName = node.name ? node.name.text : 'anonymous';
    const modifiers = this.getModifiers(node);
    const typeParams = this.getTypeParameters(node);
    const returnType = node.type ? this.getTypeString(node.type) : 'any';

    // Count parameters
    const paramCount = node.parameters ? node.parameters.length : 0;

    // Count returns
    let returnCount = 0;
    const countReturns = (node) => {
      if (node.kind === ts.SyntaxKind.ReturnStatement) {
        returnCount++;
      }
      ts.forEachChild(node, countReturns);
    };
    if (node.body) {
      countReturns(node.body);
    }

    // Calculate complexity
    const complexity = node.body ? this.calculateComplexity(node.body) : 1;

    this.nodes.push({
      type: 'function',
      name: funcName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: paramCount,
      return_count: returnCount,
      cyclomatic_complexity: complexity,
      parent: this.currentClass || this.currentNamespace || '',
      is_async: modifiers.includes('async'),
      is_generic: typeParams.length > 0,
      is_abstract: false,
      is_readonly: false,
      type_params: typeParams,
      return_type: returnType,
      modifiers: modifiers
    });

    // Track function calls
    if (node.body) {
      this.extractCalls(node.body, funcName);
    }
  }

  processClassMember(member) {
    switch (member.kind) {
      case ts.SyntaxKind.Constructor:
        this.processConstructor(member);
        break;
      case ts.SyntaxKind.MethodDeclaration:
      case ts.SyntaxKind.GetAccessor:
      case ts.SyntaxKind.SetAccessor:
        this.processMethod(member);
        break;
      case ts.SyntaxKind.PropertyDeclaration:
        this.processProperty(member);
        break;
    }
  }

  processInterfaceMember(member) {
    switch (member.kind) {
      case ts.SyntaxKind.MethodSignature:
        this.processMethodSignature(member);
        break;
      case ts.SyntaxKind.PropertySignature:
        this.processPropertySignature(member);
        break;
    }
  }

  processConstructor(node) {
    const paramCount = node.parameters ? node.parameters.length : 0;
    const complexity = node.body ? this.calculateComplexity(node.body) : 1;

    this.nodes.push({
      type: 'constructor',
      name: 'constructor',
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: paramCount,
      return_count: 0,
      cyclomatic_complexity: complexity,
      parent: this.currentClass || '',
      is_async: false,
      is_generic: false,
      is_abstract: false,
      is_readonly: false,
      type_params: [],
      return_type: '',
      modifiers: this.getModifiers(node)
    });
  }

  processMethod(node) {
    const methodName = node.name ? this.getNodeText(node.name) : 'anonymous';
    const modifiers = this.getModifiers(node);
    const typeParams = this.getTypeParameters(node);
    const returnType = node.type ? this.getTypeString(node.type) : 'any';

    const paramCount = node.parameters ? node.parameters.length : 0;
    let returnCount = 0;

    if (node.body) {
      const countReturns = (node) => {
        if (node.kind === ts.SyntaxKind.ReturnStatement) {
          returnCount++;
        }
        ts.forEachChild(node, countReturns);
      };
      countReturns(node.body);
    }

    const complexity = node.body ? this.calculateComplexity(node.body) : 1;

    this.nodes.push({
      type: 'method',
      name: methodName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: paramCount,
      return_count: returnCount,
      cyclomatic_complexity: complexity,
      parent: this.currentClass || '',
      is_async: modifiers.includes('async'),
      is_generic: typeParams.length > 0,
      is_abstract: modifiers.includes('abstract'),
      is_readonly: false,
      type_params: typeParams,
      return_type: returnType,
      modifiers: modifiers
    });

    // Track method calls
    if (node.body) {
      const fullName = this.currentClass ? this.currentClass + '.' + methodName : methodName;
      this.extractCalls(node.body, fullName);
    }
  }

  processProperty(node) {
    const propName = node.name ? this.getNodeText(node.name) : 'anonymous';
    const modifiers = this.getModifiers(node);
    const propType = node.type ? this.getTypeString(node.type) : 'any';

    this.nodes.push({
      type: 'property',
      name: propName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: 0,
      return_count: 0,
      cyclomatic_complexity: 0,
      parent: this.currentClass || '',
      is_async: false,
      is_generic: false,
      is_abstract: false,
      is_readonly: modifiers.includes('readonly'),
      type_params: [],
      return_type: propType,
      modifiers: modifiers
    });
  }

  processMethodSignature(node) {
    const methodName = node.name ? this.getNodeText(node.name) : 'anonymous';
    const typeParams = this.getTypeParameters(node);
    const returnType = node.type ? this.getTypeString(node.type) : 'any';
    const paramCount = node.parameters ? node.parameters.length : 0;

    this.nodes.push({
      type: 'method',
      name: methodName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: paramCount,
      return_count: 0,
      cyclomatic_complexity: 0,
      parent: this.currentClass || '',
      is_async: false,
      is_generic: typeParams.length > 0,
      is_abstract: true,
      is_readonly: false,
      type_params: typeParams,
      return_type: returnType,
      modifiers: []
    });
  }

  processPropertySignature(node) {
    const propName = node.name ? this.getNodeText(node.name) : 'anonymous';
    const propType = node.type ? this.getTypeString(node.type) : 'any';
    const modifiers = this.getModifiers(node);

    this.nodes.push({
      type: 'property',
      name: propName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: 0,
      return_count: 0,
      cyclomatic_complexity: 0,
      parent: this.currentClass || '',
      is_async: false,
      is_generic: false,
      is_abstract: false,
      is_readonly: modifiers.includes('readonly'),
      type_params: [],
      return_type: propType,
      modifiers: modifiers
    });
  }

  processVariableStatement(node) {
    node.declarationList.declarations.forEach(declaration => {
      if (declaration.name && declaration.name.kind === ts.SyntaxKind.Identifier) {
        const varName = declaration.name.text;
        const varType = declaration.type ? this.getTypeString(declaration.type) : 'any';

        this.nodes.push({
          type: 'variable',
          name: varName,
          start_line: this.getLineNumber(declaration),
          end_line: this.getEndLineNumber(declaration),
          parameter_count: 0,
          return_count: 0,
          cyclomatic_complexity: 0,
          parent: this.currentNamespace || '',
          is_async: false,
          is_generic: false,
          is_abstract: false,
          is_readonly: !!(node.declarationList.flags & ts.NodeFlags.Const),
          type_params: [],
          return_type: varType,
          modifiers: []
        });
      }
    });
  }

  processNamespace(node) {
    const namespaceName = node.name ? node.name.text : 'AnonymousNamespace';

    this.nodes.push({
      type: 'namespace',
      name: namespaceName,
      start_line: this.getLineNumber(node),
      end_line: this.getEndLineNumber(node),
      parameter_count: 0,
      return_count: 0,
      cyclomatic_complexity: 0,
      parent: this.currentNamespace || '',
      is_async: false,
      is_generic: false,
      is_abstract: false,
      is_readonly: false,
      type_params: [],
      return_type: '',
      modifiers: this.getModifiers(node)
    });

    // Process namespace body
    const oldNamespace = this.currentNamespace;
    this.currentNamespace = namespaceName;

    if (node.body) {
      this.visitNode(node.body);
    }

    this.currentNamespace = oldNamespace;
  }

  extractCalls(node, fromEntity) {
    const visit = (node) => {
      if (node.kind === ts.SyntaxKind.CallExpression) {
        const calledName = this.getCallName(node.expression);
        if (calledName) {
          this.relationships.push({
            from_entity: fromEntity,
            to_entity: calledName,
            type: 'calls',
            line: this.getLineNumber(node),
            text: 'calls ' + calledName
          });
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(node);
  }

  getCallName(node) {
    switch (node.kind) {
      case ts.SyntaxKind.Identifier:
        return node.text;
      case ts.SyntaxKind.PropertyAccessExpression:
        return this.getPropertyAccessName(node);
      default:
        return null;
    }
  }

  getPropertyAccessName(node) {
    const parts = [];
    let current = node;

    while (current.kind === ts.SyntaxKind.PropertyAccessExpression) {
      parts.unshift(current.name.text);
      current = current.expression;
    }

    if (current.kind === ts.SyntaxKind.Identifier) {
      parts.unshift(current.text);
    } else if (current.kind === ts.SyntaxKind.ThisKeyword) {
      parts.unshift('this');
    }

    return parts.join('.');
  }

  getModifiers(node) {
    const modifiers = [];
    if (node.modifiers) {
      node.modifiers.forEach(modifier => {
        switch (modifier.kind) {
          case ts.SyntaxKind.PublicKeyword:
            modifiers.push('public');
            break;
          case ts.SyntaxKind.PrivateKeyword:
            modifiers.push('private');
            break;
          case ts.SyntaxKind.ProtectedKeyword:
            modifiers.push('protected');
            break;
          case ts.SyntaxKind.StaticKeyword:
            modifiers.push('static');
            break;
          case ts.SyntaxKind.AbstractKeyword:
            modifiers.push('abstract');
            break;
          case ts.SyntaxKind.AsyncKeyword:
            modifiers.push('async');
            break;
          case ts.SyntaxKind.ReadonlyKeyword:
            modifiers.push('readonly');
            break;
          case ts.SyntaxKind.ExportKeyword:
            modifiers.push('export');
            break;
          case ts.SyntaxKind.ConstKeyword:
            modifiers.push('const');
            break;
        }
      });
    }
    return modifiers;
  }

  getTypeParameters(node) {
    const params = [];
    if (node.typeParameters) {
      node.typeParameters.forEach(param => {
        params.push(param.name.text);
      });
    }
    return params;
  }

  getTypeString(typeNode) {
    // Simplified type string extraction
    if (!typeNode) return 'any';

    switch (typeNode.kind) {
      case ts.SyntaxKind.StringKeyword:
        return 'string';
      case ts.SyntaxKind.NumberKeyword:
        return 'number';
      case ts.SyntaxKind.BooleanKeyword:
        return 'boolean';
      case ts.SyntaxKind.VoidKeyword:
        return 'void';
      case ts.SyntaxKind.AnyKeyword:
        return 'any';
      case ts.SyntaxKind.UnknownKeyword:
        return 'unknown';
      case ts.SyntaxKind.NeverKeyword:
        return 'never';
      case ts.SyntaxKind.TypeReference:
        return this.getTypeName(typeNode.typeName);
      case ts.SyntaxKind.ArrayType:
        return this.getTypeString(typeNode.elementType) + '[]';
      default:
        return 'any';
    }
  }

  getTypeName(node) {
    if (!node) return 'unknown';

    if (node.kind === ts.SyntaxKind.Identifier) {
      return node.text;
    } else if (node.kind === ts.SyntaxKind.QualifiedName) {
      return this.getTypeName(node.left) + '.' + node.right.text;
    }
    return 'unknown';
  }

  getNodeText(node) {
    if (node.kind === ts.SyntaxKind.Identifier) {
      return node.text;
    } else if (node.kind === ts.SyntaxKind.StringLiteral) {
      return node.text;
    } else if (node.kind === ts.SyntaxKind.ComputedPropertyName && node.expression) {
      return '[computed]';
    }
    return 'unknown';
  }

  getLineNumber(node) {
    const sourceFile = node.getSourceFile();
    if (!sourceFile) return 0;

    const { line } = sourceFile.getLineAndCharacterOfPosition(node.getStart());
    return line + 1; // Convert to 1-based
  }

  getEndLineNumber(node) {
    const sourceFile = node.getSourceFile();
    if (!sourceFile) return 0;

    const { line } = sourceFile.getLineAndCharacterOfPosition(node.getEnd());
    return line + 1; // Convert to 1-based
  }
}

function main() {
  if (process.argv.length !== 3) {
    console.log(JSON.stringify({error: "Usage: node script.js <file>"}));
    process.exit(1);
  }

  const filepath = process.argv[2];

  try {
    const source = fs.readFileSync(filepath, 'utf8');

    // Create source file
    const sourceFile = ts.createSourceFile(
      filepath,
      source,
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TS
    );

    const extractor = new TypeScriptASTExtractor(filepath);
    const result = extractor.extract(sourceFile);

    console.log(JSON.stringify(result, null, 2));
  } catch (e) {
    console.log(JSON.stringify({error: e.toString()}));
    process.exit(1);
  }
}

main();
`
