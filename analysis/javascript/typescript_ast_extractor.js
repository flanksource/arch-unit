
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
    console.log(JSON.stringify({ error: "Usage: node script.js <file>" }));
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
    console.log(JSON.stringify({ error: e.toString() }));
    process.exit(1);
  }
}

main();
