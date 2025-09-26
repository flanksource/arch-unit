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
    console.log(JSON.stringify({ error: "Usage: node script.js <file>" }));
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
    console.log(JSON.stringify({ error: e.toString() }));
    process.exit(1);
  }
}

main();
