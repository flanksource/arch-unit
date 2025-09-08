package analysis

import (
	"context"
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

// JavaScriptASTExtractor extracts AST information from JavaScript source files
type JavaScriptASTExtractor struct {
	filePath    string
	packageName string
	depsManager *NodeDependenciesManager
}

// NewJavaScriptASTExtractor creates a new JavaScript AST extractor
func NewJavaScriptASTExtractor() *JavaScriptASTExtractor {
	return &JavaScriptASTExtractor{
		depsManager: NewNodeDependenciesManager(),
	}
}

// JavaScriptASTNode represents a node in JavaScript AST
type JavaScriptASTNode struct {
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
	IsGenerator          bool                 `json:"is_generator"`
	IsArrow              bool                 `json:"is_arrow"`
	ExportType           string               `json:"export_type"` // "default", "named", ""
}

// JavaScriptImport represents an import in JavaScript
type JavaScriptImport struct {
	Source   string   `json:"source"`
	Imported []string `json:"imported"`
	Line     int      `json:"line"`
	Type     string   `json:"type"` // "import", "require"
}

// JavaScriptRelationship represents a relationship between JavaScript entities
type JavaScriptRelationship struct {
	FromEntity string `json:"from_entity"`
	ToEntity   string `json:"to_entity"`
	Type       string `json:"type"`
	Line       int    `json:"line"`
	Text       string `json:"text"`
}

// JavaScriptASTResult contains the complete AST analysis result
type JavaScriptASTResult struct {
	Module        string                   `json:"module"`
	Nodes         []JavaScriptASTNode      `json:"nodes"`
	Imports       []JavaScriptImport       `json:"imports"`
	Relationships []JavaScriptRelationship `json:"relationships"`
}

// ExtractFile extracts AST information from a JavaScript file
func (e *JavaScriptASTExtractor) ExtractFile(cache cache.ReadOnlyCache, filePath string, content []byte) (*ASTResult, error) {
	e.filePath = filePath
	
	// Extract package name from file path or package.json
	e.packageName = e.extractPackageName(filePath)
	
	result := &ASTResult{
		Nodes:         []*models.ASTNode{},
		Relationships: []*models.ASTRelationship{},
	}

	// Run JavaScript AST extraction (write content to temp file for external tool)
	tempFile, err := os.CreateTemp("", "js_extract_*.js")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	
	if _, err := tempFile.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write content to temp file: %w", err)
	}
	tempFile.Close()

	jsResult, err := e.runJavaScriptASTExtraction(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to extract JavaScript AST: %w", err)
	}

	// Convert JavaScript nodes to AST nodes
	nodeMap := make(map[string]string) // fullName -> node key
	for _, node := range jsResult.Nodes {
		astNode := &models.ASTNode{
			FilePath:             filePath,
			PackageName:          e.packageName,
			TypeName:             "",
			MethodName:           "",
			FieldName:            "",
			NodeType:             e.mapJavaScriptNodeType(node.Type),
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
		case "variable", "property":
			if node.Parent != "" {
				astNode.TypeName = node.Parent
			}
			astNode.FieldName = node.Name
		}

		// Generate unique key for the node
		nodeKey := astNode.Key()
		fullName := e.getNodeFullName(node)
		nodeMap[fullName] = nodeKey

		result.Nodes = append(result.Nodes, astNode)
	}

	// Convert relationships
	for _, rel := range jsResult.Relationships {
		fromKey, fromExists := nodeMap[rel.FromEntity]
		if !fromExists {
			continue
		}

		var toKey *string
		if toNodeKey, toExists := nodeMap[rel.ToEntity]; toExists {
			toKey = &toNodeKey
		}

		// Look up existing node IDs from cache if available
		var fromID int64
		var toID *int64
		
		if id, exists := cache.GetASTId(fromKey); exists {
			fromID = id
		}
		
		if toKey != nil {
			if id, exists := cache.GetASTId(*toKey); exists {
				toID = &id
			}
		}

		astRel := &models.ASTRelationship{
			FromASTID:        fromID,
			ToASTID:          toID,
			LineNo:           rel.Line,
			RelationshipType: models.RelationshipType(e.mapRelationshipType(rel.Type)),
			Text:             rel.Text,
		}

		result.Relationships = append(result.Relationships, astRel)
	}

	return result, nil
}

// extractPackageName extracts package name from file path or package.json
func (e *JavaScriptASTExtractor) extractPackageName(filePath string) string {
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

// runJavaScriptASTExtraction runs the JavaScript AST extraction script
func (e *JavaScriptASTExtractor) runJavaScriptASTExtraction(filePath string) (*JavaScriptASTResult, error) {
	// Create parser script with proper module resolution
	ctx := flanksourceContext.NewContext(context.Background())
	scriptPath, err := e.depsManager.CreateParserScript(ctx, javascriptASTExtractorScript, "javascript")
	if err != nil {
		return nil, fmt.Errorf("failed to create parser script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Execute the script with Node.js
	cmd := exec.Command("node", scriptPath, filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("JavaScript AST extraction failed: %w - output: %s", err, string(output))
	}

	// Parse JSON output
	var result JavaScriptASTResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JavaScript AST JSON: %w", err)
	}

	return &result, nil
}

// mapJavaScriptNodeType maps JavaScript node types to generic AST node types
func (e *JavaScriptASTExtractor) mapJavaScriptNodeType(jsType string) string {
	switch jsType {
	case "class":
		return models.NodeTypeType
	case "function", "method":
		return models.NodeTypeMethod
	case "variable", "property":
		return models.NodeTypeField
	default:
		return models.NodeTypeVariable
	}
}

// mapRelationshipType maps JavaScript relationship types to generic relationship types
func (e *JavaScriptASTExtractor) mapRelationshipType(jsRelType string) string {
	switch jsRelType {
	case "extends":
		return models.RelationshipExtends
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

// getNodeFullName returns the full qualified name of a JavaScript node
func (e *JavaScriptASTExtractor) getNodeFullName(node JavaScriptASTNode) string {
	parts := []string{e.packageName}

	if node.Parent != "" {
		parts = append(parts, node.Parent)
	}

	parts = append(parts, node.Name)
	return strings.Join(parts, ".")
}

// JavaScript AST extractor script using Acorn parser
const javascriptASTExtractorScript = `
const fs = require('fs');
const acorn = require('acorn');
const walk = require('acorn-walk');

class JavaScriptASTExtractor {
  constructor(filename) {
    this.filename = filename;
    this.module = require('path').basename(filename, require('path').extname(filename));
    this.nodes = [];
    this.imports = [];
    this.relationships = [];
    this.currentClass = null;
    this.currentFunction = null;
    this.scopeStack = [];
  }
  
  extract(ast) {
    this.walkAST(ast);
    return {
      module: this.module,
      nodes: this.nodes,
      imports: this.imports,
      relationships: this.relationships
    };
  }
  
  calculateComplexity(node) {
    let complexity = 1;
    
    walk.simple(node, {
      IfStatement: () => complexity++,
      ConditionalExpression: () => complexity++,
      LogicalExpression: (node) => {
        if (node.operator === '&&' || node.operator === '||') complexity++;
      },
      ForStatement: () => complexity++,
      ForInStatement: () => complexity++,
      ForOfStatement: () => complexity++,
      WhileStatement: () => complexity++,
      DoWhileStatement: () => complexity++,
      CatchClause: () => complexity++,
      SwitchCase: (node) => {
        if (node.test) complexity++; // Don't count default case
      }
    });
    
    return complexity;
  }
  
  walkAST(ast) {
    const self = this;
    
    walk.ancestor(ast, {
      ImportDeclaration(node) {
        const imported = [];
        node.specifiers.forEach(spec => {
          if (spec.type === 'ImportDefaultSpecifier') {
            imported.push('default');
          } else if (spec.type === 'ImportSpecifier') {
            imported.push(spec.imported.name);
          } else if (spec.type === 'ImportNamespaceSpecifier') {
            imported.push('*');
          }
        });
        
        self.imports.push({
          source: node.source.value,
          imported: imported,
          line: node.loc ? node.loc.start.line : 0,
          type: 'import'
        });
      },
      
      CallExpression(node) {
        // Detect require() calls
        if (node.callee.name === 'require' && node.arguments.length > 0) {
          if (node.arguments[0].type === 'Literal') {
            self.imports.push({
              source: node.arguments[0].value,
              imported: [],
              line: node.loc ? node.loc.start.line : 0,
              type: 'require'
            });
          }
        }
        
        // Track function calls
        const calledName = self.getCallName(node);
        if (calledName && self.currentFunction) {
          self.relationships.push({
            from_entity: self.currentFunction,
            to_entity: calledName,
            type: 'calls',
            line: node.loc ? node.loc.start.line : 0,
            text: 'calls ' + calledName
          });
        }
      },
      
      ClassDeclaration(node) {
        const className = node.id ? node.id.name : 'AnonymousClass';
        
        self.nodes.push({
          type: 'class',
          name: className,
          start_line: node.loc ? node.loc.start.line : 0,
          end_line: node.loc ? node.loc.end.line : 0,
          parameter_count: 0,
          return_count: 0,
          cyclomatic_complexity: 1,
          parent: '',
          is_async: false,
          is_generator: false,
          is_arrow: false,
          export_type: ''
        });
        
        // Track inheritance
        if (node.superClass) {
          const superName = self.getNodeName(node.superClass);
          self.relationships.push({
            from_entity: className,
            to_entity: superName,
            type: 'extends',
            line: node.loc ? node.loc.start.line : 0,
            text: 'extends ' + superName
          });
        }
        
        // Process class body
        const oldClass = self.currentClass;
        self.currentClass = className;
        
        node.body.body.forEach(member => {
          if (member.type === 'MethodDefinition') {
            self.processMethod(member);
          } else if (member.type === 'PropertyDefinition') {
            self.processProperty(member);
          }
        });
        
        self.currentClass = oldClass;
      },
      
      FunctionDeclaration(node) {
        self.processFunction(node);
      },
      
      FunctionExpression(node) {
        if (node.id) {
          self.processFunction(node);
        }
      },
      
      ArrowFunctionExpression(node, ancestors) {
        // Check if this arrow function is assigned to a variable
        const parent = ancestors[ancestors.length - 2];
        if (parent && parent.type === 'VariableDeclarator' && parent.id.type === 'Identifier') {
          self.processFunction(node, parent.id.name, true);
        }
      },
      
      VariableDeclarator(node) {
        if (node.id.type === 'Identifier' && node.init) {
          // Check if it's a function
          if (node.init.type === 'FunctionExpression' || 
              node.init.type === 'ArrowFunctionExpression') {
            // Already handled by function visitors
            return;
          }
          
          // Regular variable
          self.nodes.push({
            type: 'variable',
            name: node.id.name,
            start_line: node.loc ? node.loc.start.line : 0,
            end_line: node.loc ? node.loc.end.line : 0,
            parameter_count: 0,
            return_count: 0,
            cyclomatic_complexity: 0,
            parent: self.currentClass || '',
            is_async: false,
            is_generator: false,
            is_arrow: false,
            export_type: ''
          });
        }
      }
    });
  }
  
  processFunction(node, name = null, isArrow = false) {
    const funcName = name || (node.id ? node.id.name : 'anonymous');
    const isMethod = this.currentClass !== null;
    
    // Count parameters
    const paramCount = node.params ? node.params.length : 0;
    
    // Count returns
    let returnCount = 0;
    walk.simple(node, {
      ReturnStatement: () => returnCount++
    });
    
    // Calculate complexity
    const complexity = this.calculateComplexity(node);
    
    this.nodes.push({
      type: isMethod ? 'method' : 'function',
      name: funcName,
      start_line: node.loc ? node.loc.start.line : 0,
      end_line: node.loc ? node.loc.end.line : 0,
      parameter_count: paramCount,
      return_count: returnCount,
      cyclomatic_complexity: complexity,
      parent: this.currentClass || '',
      is_async: node.async || false,
      is_generator: node.generator || false,
      is_arrow: isArrow,
      export_type: ''
    });
    
    // Track function scope for call relationships
    const oldFunction = this.currentFunction;
    this.currentFunction = this.currentClass ? 
      this.currentClass + '.' + funcName : funcName;
    
    // Process function body for relationships
    if (node.body) {
      walk.simple(node.body, {
        CallExpression: (callNode) => {
          const calledName = this.getCallName(callNode);
          if (calledName) {
            this.relationships.push({
              from_entity: this.currentFunction,
              to_entity: calledName,
              type: 'calls',
              line: callNode.loc ? callNode.loc.start.line : 0,
              text: 'calls ' + calledName
            });
          }
        }
      });
    }
    
    this.currentFunction = oldFunction;
  }
  
  processMethod(node) {
    const methodName = node.key.name || node.key.value;
    const isConstructor = node.kind === 'constructor';
    
    this.processFunction(node.value, isConstructor ? 'constructor' : methodName);
  }
  
  processProperty(node) {
    if (node.key.type === 'Identifier') {
      this.nodes.push({
        type: 'property',
        name: node.key.name,
        start_line: node.loc ? node.loc.start.line : 0,
        end_line: node.loc ? node.loc.end.line : 0,
        parameter_count: 0,
        return_count: 0,
        cyclomatic_complexity: 0,
        parent: this.currentClass || '',
        is_async: false,
        is_generator: false,
        is_arrow: false,
        export_type: ''
      });
    }
  }
  
  getCallName(node) {
    if (node.callee.type === 'Identifier') {
      return node.callee.name;
    } else if (node.callee.type === 'MemberExpression') {
      return this.getMemberName(node.callee);
    }
    return null;
  }
  
  getMemberName(node) {
    const parts = [];
    let current = node;
    
    while (current.type === 'MemberExpression') {
      if (current.property.type === 'Identifier') {
        parts.unshift(current.property.name);
      }
      current = current.object;
    }
    
    if (current.type === 'Identifier') {
      parts.unshift(current.name);
    } else if (current.type === 'ThisExpression') {
      parts.unshift('this');
    }
    
    return parts.join('.');
  }
  
  getNodeName(node) {
    if (node.type === 'Identifier') {
      return node.name;
    } else if (node.type === 'MemberExpression') {
      return this.getMemberName(node);
    }
    return 'unknown';
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
    
    // Parse with Acorn
    const ast = acorn.parse(source, {
      ecmaVersion: 'latest',
      sourceType: 'module',
      locations: true
    });
    
    const extractor = new JavaScriptASTExtractor(filepath);
    const result = extractor.extract(ast);
    
    console.log(JSON.stringify(result, null, 2));
  } catch (e) {
    console.log(JSON.stringify({error: e.toString()}));
    process.exit(1);
  }
}

main();
`
