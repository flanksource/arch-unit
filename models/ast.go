// Note to claude:  DO NOT MODIFY any structs in this file without consulting the user, Add/Remove/Modify functions as needed without change the fields.

package models

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/internal/source"
	"github.com/flanksource/clicky"
	"github.com/flanksource/clicky/api"
	"github.com/flanksource/clicky/api/icons"
)

// Global source reader for on-demand source code retrieval
var globalSourceReader = source.NewReader()

// CommentType represents the type of comment
type CommentType string

const (
	CommentTypeSingleLine    CommentType = "single_line"
	CommentTypeMultiLine     CommentType = "multi_line"
	CommentTypeDocumentation CommentType = "documentation"
)

// Comment represents a comment in the code
type Comment struct {
	Text      string      `json:"text"`
	StartLine int         `json:"start_line"`
	EndLine   int         `json:"end_line"`
	WordCount int         `json:"word_count"`
	Type      CommentType `json:"type"`
	Context   string      `json:"context"` // Function/Type/Variable it's associated with
}

// Parameter represents a function parameter
type Parameter struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	NameLength int    `json:"name_length"`
}

// ReturnValue represents a function return value
type ReturnValue struct {
	Name string `json:"name"` // Can be empty for unnamed returns
	Type string `json:"type"`
}

// Function represents a function definition
type Function struct {
	Name       string      `json:"name"`
	NameLength int         `json:"name_length"`
	StartLine  int         `json:"start_line"`
	EndLine    int         `json:"end_line"`
	LineCount  int         `json:"line_count"`
	Parameters []Parameter `json:"parameters"`
	ReturnType string      `json:"return_type,omitempty"`
	Comments   []Comment   `json:"comments"`
	IsPrivate  bool        `json:"is_private,omitempty"` // Unified visibility across languages
}

// Field represents a struct/class field
type Field struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	NameLength int       `json:"name_length"`
	Comments   []Comment `json:"comments"`
	IsPrivate  bool      `json:"is_private,omitempty"` // Unified visibility across languages
}

// TypeDefinition represents a type definition (struct, class, interface, etc.)
type TypeDefinition struct {
	Name       string     `json:"name"`
	NameLength int        `json:"name_length"`
	Kind       string     `json:"kind"` // "struct", "interface", "class", "enum", etc.
	StartLine  int        `json:"start_line"`
	EndLine    int        `json:"end_line"`
	LineCount  int        `json:"line_count"`
	Fields     []Field    `json:"fields"`
	Methods    []Function `json:"methods"`
	Comments   []Comment  `json:"comments"`
	IsPrivate  bool       `json:"is_private,omitempty"` // Unified visibility across languages
}

// Variable represents a variable declaration
type Variable struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	NameLength int       `json:"name_length"`
	Line       int       `json:"line"`
	IsConstant bool      `json:"is_constant"`
	Comments   []Comment `json:"comments"`
	IsPrivate  bool      `json:"is_private,omitempty"` // Unified visibility across languages
}

// Import represents an import statement
type Import struct {
	Path   string `json:"path"`
	Alias  string `json:"alias,omitempty"`
	Line   int    `json:"line"`
	IsUsed bool   `json:"is_used,omitempty"`
}

// GenericAST represents a language-agnostic AST
type GenericAST struct {
	Language    string           `json:"language"`
	FilePath    string           `json:"file_path"`
	LineCount   int              `json:"line_count"`
	Functions   []Function       `json:"functions"`
	Types       []TypeDefinition `json:"types"`
	Variables   []Variable       `json:"variables"`
	Comments    []Comment        `json:"comments"`
	Imports     []Import         `json:"imports"`
	PackageName string           `json:"package_name,omitempty"`
}

// GetAllNames returns all identifiers in the AST for name analysis
func (ast *GenericAST) GetAllNames() []string {
	var names []string

	// Function names
	for _, fn := range ast.Functions {
		names = append(names, fn.Name)
		// Parameter names
		for _, param := range fn.Parameters {
			names = append(names, param.Name)
		}
	}

	// Type names
	for _, typ := range ast.Types {
		names = append(names, typ.Name)
		// Field names
		for _, field := range typ.Fields {
			names = append(names, field.Name)
		}
	}

	// Variable names
	for _, variable := range ast.Variables {
		names = append(names, variable.Name)
	}

	return names
}

// GetLongNames returns names that exceed the specified length
func (ast *GenericAST) GetLongNames(maxLength int) []string {
	var longNames []string
	names := ast.GetAllNames()

	for _, name := range names {
		if len(name) > maxLength {
			longNames = append(longNames, name)
		}
	}

	return longNames
}

// CountWords counts words in a text string
func CountWords(text string) int {
	if text == "" {
		return 0
	}

	// Clean the text and split on whitespace
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return 0
	}

	words := strings.Fields(cleaned)
	return len(words)
}

// NewComment creates a new comment with calculated word count
func NewComment(text string, startLine, endLine int, commentType CommentType, context string) Comment {
	return Comment{
		Text:      text,
		StartLine: startLine,
		EndLine:   endLine,
		WordCount: CountWords(text),
		Type:      commentType,
		Context:   context,
	}
}

// IsSimpleComment returns true if the comment is under the word limit
func (c *Comment) IsSimpleComment(wordLimit int) bool {
	return c.WordCount <= wordLimit
}

// GetMultiLineComments returns only multi-line comments from the AST
func (ast *GenericAST) GetMultiLineComments() []Comment {
	var multiLine []Comment

	for _, comment := range ast.Comments {
		if comment.Type == CommentTypeMultiLine || comment.Type == CommentTypeDocumentation {
			multiLine = append(multiLine, comment)
		}
	}

	return multiLine
}

// GetComplexComments returns comments that exceed the word limit
func (ast *GenericAST) GetComplexComments(wordLimit int) []Comment {
	var complex []Comment

	for _, comment := range ast.Comments {
		if !comment.IsSimpleComment(wordLimit) {
			complex = append(complex, comment)
		}
	}

	return complex
}

// ASTExporter defines the interface for exporting language-specific ASTs to GenericAST
type ASTExporter interface {
	ExportAST(filePath string) (*GenericAST, error)
}

// Extended AST models for database storage

// ASTNode represents a node in the AST stored in database
type ASTNode struct {
	ID                   int64         `json:"id,omitempty" gorm:"primaryKey;autoIncrement" pretty:"hide"`
	Parent               *ASTNode      `json:"-" pretty:"hide"`
	ParentID             *int64        `json:"parent_id,omitempty" gorm:"column:parent_id;index" pretty:"hide"`         // Nullable for root nodes, For a field, parent is the struct/class, for a struct/class parent is package,
	DependencyID         *int64        `json:"dependency_id,omitempty" gorm:"column:dependency_id;index" pretty:"hide"` // Id of the dependency that contains this node
	FilePath             string        `json:"file_path,omitempty" gorm:"column:file_path;not null;index" pretty:"label=File,style=text-blue-500"`
	PackageName          string        `json:"package_name,omitempty" gorm:"column:package_name;index" pretty:"label=Package"`
	TypeName             string        `json:"type_name,omitempty" gorm:"column:type_name;index" pretty:"label=Type,style=text-green-600"`
	MethodName           string        `json:"method_name,omitempty" gorm:"column:method_name;index" pretty:"label=Method,style=text-purple-600"`
	FieldName            string        `json:"field_name,omitempty" gorm:"column:field_name" pretty:"label=Field,style=text-orange-600"`
	NodeType             NodeType      `json:"node_type,omitempty" gorm:"column:node_type;not null;index" pretty:"label=Type,style=text-gray-600"` // "package", "type", "method", "field", "variable"
	Language             *string       `json:"language,omitempty" gorm:"column:language;index" pretty:"label=Language"`                            // "go", "python", "sql", "openapi", etc. (optional)
	StartLine            int           `json:"start_line,omitempty" gorm:"column:start_line" pretty:"label=Line"`
	EndLine              int           `json:"end_line,omitempty" gorm:"column:end_line" pretty:"hide"`
	CyclomaticComplexity int           `json:"cyclomatic_complexity,omitempty" gorm:"column:cyclomatic_complexity;default:0;index" pretty:"label=Complexity,green=1-5,yellow=6-10,red=11+"`
	ParameterCount       int           `json:"parameter_count,omitempty" gorm:"column:parameter_count;default:0" pretty:"label=Params"`
	ReturnCount          int           `json:"return_count,omitempty" gorm:"column:return_count;default:0" pretty:"label=Returns"`
	LineCount            int           `json:"line_count,omitempty" gorm:"column:line_count;default:0" pretty:"label=Lines"`
	Imports              []string      `json:"imports,omitempty" gorm:"-" pretty:"hide"`                     // List of import paths - not stored in DB
	Parameters           []Parameter   `json:"parameters,omitempty" gorm:"serializer:json" pretty:"hide"`    // Detailed parameter information
	ReturnValues         []ReturnValue `json:"return_values,omitempty" gorm:"serializer:json" pretty:"hide"` // Return value information
	LastModified         time.Time     `json:"last_modified,omitempty" gorm:"column:last_modified;index" pretty:"hide"`
	FileHash             string        `json:"file_hash,omitempty" gorm:"column:file_hash" pretty:"hide"`
	// Summary is an AI generated/enhanced summary of the node,
	// For fields, its a max of 5 words, for method, a max of 20 works, and for types a maximum of 50
	Summary      *string           `json:"summary,omitempty" gorm:"column:summary" pretty:"label=Summary,style=text-gray-700"`
	FieldType    *string           `json:"field_type,omitempty" gorm:"column:field_type" pretty:"label=Field Type"`                  // Go type or SQL column type
	DefaultValue *string           `json:"default_value,omitempty" gorm:"column:default_value" pretty:"label=Default"`               // Default value for fields
	IsPrivate    bool              `json:"is_private,omitempty" gorm:"column:is_private;default:false;index" pretty:"label=Private"` // Unified visibility across languages
	Metatdata    map[string]string `json:"metadata,omitempty" gorm:"serializer:json"`                                                // Additional metadata specific to language or analysis

	// Hydrated relationships for easy printing
	Relationships []*ASTRelationship `json:"-" gorm:"-"`
	Statements    []ASTStatement     `json:"statements,omitempty" gorm:"-"`
}

type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeNumber  FieldType = "number"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeArray   FieldType = "array"
	FieldTypeObject  FieldType = "object"
	FieldTypeEnum    FieldType = "enum"
	FieldTypeDate    FieldType = "date"
	FieldTypeFloat   FieldType = "float"
	FieldTypeMap     FieldType = "map"
	// Result of an expression evaluation
	FieldTypeExpression FieldType = "expression"
	// Result of a SQL query
	FieldTypeSQL FieldType = "sql"
	// i.e. lambda, closure, arrow function, etc.
	FieldTypeFunction FieldType = "function"
	// Result of a method call
	FieldTypeMethodResult FieldType = "method_result"
)

func (f FieldType) Pretty() api.Text {
	switch f {
	case FieldTypeString:
		return clicky.Text("string", "text-green-600")
	case FieldTypeNumber, FieldTypeFloat:
		return clicky.Text("number", "text-blue-600")
	case FieldTypeBoolean:
		return clicky.Text("boolean", "text-red-600")
	case FieldTypeArray:
		return clicky.Text("array", "text-purple-600")
	case FieldTypeObject:
		return clicky.Text("object", "text-pink-600")
	case FieldTypeEnum:
		return clicky.Text("enum", "text-indigo-600")
	case FieldTypeDate:
		return clicky.Text("date", "text-gray-600")
	case FieldTypeMap:
		return clicky.Text("map", "text-teal-600")
	default:
		return clicky.Text(string(f), "text-yellow-600")
	}
}

type Value struct {
	// Literal value as string, or SQL,  pkg.Type.method for method call, expr for expression
	Value     string    `json:"value"`
	FieldType FieldType `json:"field_type,omitempty"`
	//being called
	Constant bool `json:"constant,omitempty"`
	// params to a method call
	Params map[string]Value `json:"params,omitempty"`
}

func (v Value) Pretty() api.Text {
	if v.Value == "" {
		return api.Text{}
	}
	// get icon and color for field type

	p := v.FieldType.Pretty()
	if v.Value != "" {
		p = p.Append(" = ", "text-gray-500").Append(v.Value, "text-gray-700")
	}
	return p
}

func (a ASTNode) Key() string {
	k := fmt.Sprintf("%s/%s:%s%s", a.FilePath, a.TypeName, a.MethodName, a.FieldName)
	if a.DependencyID != nil {
		k = fmt.Sprintf("%d#%s", *a.DependencyID, k)
	}
	return k
}

// TableName specifies the table name for ASTNode
func (ASTNode) TableName() string {
	return "ast_nodes"
}

type ASTStatementType string

const (
	ASTStatementTypeFunctionCall ASTStatementType = "function_call"
	ASTStatementTypeIf           ASTStatementType = "if"
	ASTStatementTypeLoop         ASTStatementType = "loop"
	ASTStatementTypeImport       ASTStatementType = "import"
	ASTStatementTypeSQLQuery     ASTStatementType = "sql"
	ASTStatementTypeExpression   ASTStatementType = "expression"
	ASTStatementTypeAssignment   ASTStatementType = "assignment"
	ASTStatementTypeHttpCall     ASTStatementType = "http_call"
	ASTStatementTypeFileOp       ASTStatementType = "file_op"
	ASTStatementTypeMessageQueue ASTStatementType = "message_queue"
	ASTStatementTypeOther        ASTStatementType = "other"
)

func (t ASTStatementType) Pretty() api.Text {
	switch t {
	case ASTStatementTypeFunctionCall:
		return clicky.Text("").Add(icons.ArrowRight)
	case ASTStatementTypeIf:
		return clicky.Text("").Add(icons.If)
	case ASTStatementTypeLoop:
		return clicky.Text("").Add(icons.Loop)
	case ASTStatementTypeSQLQuery:
		return clicky.Text("").Add(icons.DB)
	case ASTStatementTypeExpression:
		return clicky.Text("").Add(icons.Lambda)
	case ASTStatementTypeAssignment:
		return clicky.Text("").Add(icons.Variable)
	case ASTStatementTypeHttpCall:
		return clicky.Text("").Add(icons.Http)
	case ASTStatementTypeFileOp:
		return clicky.Text("").Add(icons.Folder)
	case ASTStatementTypeMessageQueue:
		return clicky.Text("").Add(icons.Queue)
	case ASTStatementTypeImport:
		return clicky.Text("").Add(icons.ArrowDown)
	default:
		return clicky.Text(" other", "text-gray-600")
	}
}

type Params map[string]Value

func (p Params) Pretty() api.Text {
	if len(p) == 0 {
		return api.Text{}
	}

	var parts = api.Text{}
	if len(p) > 3 {
		// Multi-line format for more than 3 params
		first := true
		for k, v := range p {
			if !first {
				parts = parts.Append("\n  ", "")
			}
			parts = parts.Append(k, "text-gray-500").Append("=", "text-gray-300").Add(v.Pretty())
			first = false
		}
	} else {
		// Single line format for 3 or fewer params
		for k, v := range p {
			parts = parts.Append(k, "text-gray-500").Append("=", "text-gray-300").Add(v.Pretty())
		}
	}
	return parts
}

// Location represents a file location with line range
type Location struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line,omitempty"`
}

func (l Location) Pretty() api.Text {
	var result api.Text

	// Add filename if present
	if l.Path != "" {
		fileName := filepath.Base(l.Path)
		result = clicky.Text(fileName, "text-blue-500 font-medium")
	}

	// Add line numbers even if path is empty
	if l.EndLine > 0 && l.EndLine != l.StartLine {
		// Range format: :L1-23
		if l.Path != "" {
			result = result.Append(":", "text-gray-400")
		}
		result = result.Append("L", "text-gray-500 text-xs")
		result = result.Append(fmt.Sprintf("%d", l.StartLine), "text-purple-600 font-mono")
		result = result.Append("-", "text-gray-400")
		result = result.Append(fmt.Sprintf("%d", l.EndLine), "text-purple-600 font-mono")
	} else if l.StartLine > 0 {
		// Single line format: :L1
		if l.Path != "" {
			result = result.Append(":", "text-gray-400")
		}
		result = result.Append("L", "text-gray-500 text-xs")
		result = result.Append(fmt.Sprintf("%d", l.StartLine), "text-purple-600 font-mono")
	}

	return result
}

// ASTStatement represents specific function calls, flow control statements, or important lines within an AST node
type ASTStatement struct {
	From      *ASTNode         `json:"from,omitempty" gorm:"-"`
	To        *ASTNode         `json:"to,omitempty" gorm:"-"`
	StartLine int              `json:"start_line,omitempty"`
	EndLine   int              `json:"end_line,omitempty"`
	Text      string           `json:"text"`
	Input     Params           `json:"input,omitempty"`
	Output    Params           `json:"output,omitempty"`
	Type      ASTStatementType `json:"type"`
	Children  []ASTStatement   `json:"children,omitempty"`
}

func (s ASTStatement) GetChildren() []api.TreeNode {
	children := make([]api.TreeNode, len(s.Children))
	for i, child := range s.Children {
		children[i] = child
	}
	return children
}

func (s ASTStatement) Pretty() api.Text {
	p := s.Type.Pretty()
	if s.From != nil {
		p = p.Append(" ").Add(s.From.ShortName()).Append(" = ", "text-muted")
	}
	if s.To != nil {
		// Determine whether to show full qualified name based on context
		var targetName api.Text
		if s.From != nil {
			fromPkg := s.From.GetPackage().FullName().String()
			toPkg := s.To.GetPackage().FullName().String()
			fromType := s.From.GetType().FullName().String()
			toType := s.To.GetType().FullName().String()

			// If packages differ, use full qualified name
			if fromPkg != toPkg {
				targetName = s.To.FullName()
			} else if fromType != toType {
				// If packages match but types differ, show Type.Method
				targetName = clicky.Text("")
				if s.To.TypeName != "" {
					targetName = targetName.Append(s.To.TypeName)
					if s.To.MethodName != "" {
						targetName = targetName.Append(".", "text-gray-500").Append(s.To.MethodName)
					} else if s.To.FieldName != "" {
						targetName = targetName.Append(".", "text-gray-500").Append(s.To.FieldName)
					}
				} else {
					targetName = s.To.ShortName()
				}
			} else {
				// Same package and type, use short name
				targetName = s.To.ShortName()
			}
		} else {
			// No From context, use short name
			targetName = s.To.ShortName()
		}
		p = p.Append(" → ", "text-gray-400").Add(targetName)
	}
	if s.Input != nil && len(s.Input) > 0 {
		p = p.Append("( ", "text-gray-400").Add(s.Input.Pretty()).Append(" )", "text-gray-400")
	}
	if s.Text != "" {
		p = p.Append(" ").Append(s.Text, "text-gray-700")
	}
	if s.Output != nil && len(s.Output) > 0 {
		p = p.Append(" ").Add(icons.ArrowDoubleRight).Append(" ( ", "text-gray-400").Add(s.Output.Pretty()).Append(" )", "text-gray-400")
	}

	return p
}

// ASTRelationship represents a relationship between AST nodes
type ASTRelationship struct {
	ID               int64             `json:"id" gorm:"primaryKey;autoIncrement"`
	FromAST          *ASTNode          `json:"-"`
	ToAST            *ASTNode          `json:"-"`
	FromASTID        int64             `json:"from_ast_id" gorm:"column:from_ast_id;not null;index"`
	ToASTID          *int64            `json:"to_ast_id,omitempty" gorm:"column:to_ast_id;index"` // Nullable for external calls
	LineNo           int               `json:"line_no,omitempty" gorm:"column:line_no;index"`
	RelationshipType RelationshipType  `json:"relationship_type" gorm:"column:relationship_type;not null;index"`
	Comments         string            `json:"comments,omitempty" gorm:"column:comments"` // Additional comments or context found in the code
	Text             string            `json:"text" gorm:"column:text"`                   // Text of the relationship, could be the line(s) with the function call, the line in a go.mod or Chart.yaml=
	Metadata         map[string]string `json:"metadata,omitempty" gorm:"serializer:json"`
}

// TableName specifies the table name for ASTRelationship
func (ASTRelationship) TableName() string {
	return "ast_relationships"
}

type RelationshipType string

const (
	RelationshipTypeImport      RelationshipType = "import"      // Import statement in code
	RelationshipTypeCall        RelationshipType = "call"        // Function/method call
	RelationshipTypeReference   RelationshipType = "reference"   // Variable reference
	RelationshipTypeInheritance RelationshipType = "inheritance" // Class inheritance, Docker FROM
	RelationshipTypeImplements  RelationshipType = "implements"  // Interface implementation
	RelationshipTypeIncludes    RelationshipType = "includes"    // e.g. For a chart including a subchart
	RelationshipTypeForeignKey  RelationshipType = "foreign_key" // Database foreign key constraint
)

func (r RelationshipType) Pretty() api.Text {
	switch r {
	case RelationshipTypeImport:
		return clicky.Text("").Add(icons.ArrowDown).Append(" import", "text-blue-600")
	case RelationshipTypeCall:
		return clicky.Text("").Add(icons.ArrowRight).Append(" call", "text-green-600")
	case RelationshipTypeInheritance:
		return clicky.Text("").Add(icons.ArrowRight).Append(" extends", "text-purple-600")
	case RelationshipTypeImplements:
		return clicky.Text("").Add(icons.ArrowRight).Append(" implements", "text-indigo-600")
	case RelationshipTypeIncludes:
		return clicky.Text("").Add(icons.ArrowRight).Append(" includes", "text-pink-600")
	case RelationshipTypeForeignKey:
		return clicky.Text("").Add(icons.ArrowRight).Append(" foreign key", "text-red-600")
	default:
		return clicky.Text("").Add(icons.ArrowRight).Append(" reference", "text-yellow-600")
	}
}

// DependencyRelationship represents a relationship between AST node and dependency node
type DependencyRelationship struct {
	ID           int64 `json:"id"`
	ASTID        int64 `json:"ast_id"`
	DependencyID int64 `json:"dependency_id"`
	LineNo       int   `json:"line_no"`
}

// LibraryNode represents a node in an external library/framework
type LibraryNode struct {
	ID        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Package   string `json:"package" gorm:"column:package;not null;index"`
	Class     string `json:"class,omitempty" gorm:"column:class;index"`
	Method    string `json:"method,omitempty" gorm:"column:method;index"`
	Field     string `json:"field,omitempty" gorm:"column:field"`
	NodeType  string `json:"node_type" gorm:"column:node_type;not null;index"`  // 'package', 'class', 'method', 'field'
	Language  string `json:"language,omitempty" gorm:"column:language"`         // 'go', 'python', 'javascript', etc.
	Framework string `json:"framework,omitempty" gorm:"column:framework;index"` // 'stdlib', 'gin', 'django', 'react', etc.
}

// TableName specifies the table name for LibraryNode
func (LibraryNode) TableName() string {
	return "library_nodes"
}

// GetFullName returns the full qualified name of a library node
func (n *LibraryNode) GetFullName() string {
	parts := []string{}

	if n.Package != "" {
		parts = append(parts, n.Package)
	}

	if n.Class != "" {
		parts = append(parts, n.Class)
	}

	if n.Method != "" {
		parts = append(parts, n.Method)
	}

	if n.Field != "" {
		parts = append(parts, n.Field)
	}

	return strings.Join(parts, ".")
}

// LibraryRelationship represents a relationship between AST node and library node
type LibraryRelationship struct {
	ID               int64        `json:"id" gorm:"primaryKey;autoIncrement"`
	ASTID            int64        `json:"ast_id" gorm:"column:ast_id;not null;index"`
	LibraryID        int64        `json:"library_id" gorm:"column:library_id;not null;index"`
	LineNo           int          `json:"line_no" gorm:"column:line_no;index"`
	RelationshipType string       `json:"relationship_type" gorm:"column:relationship_type;not null;index"` // 'import', 'call', 'reference', 'extends'
	Text             string       `json:"text,omitempty" gorm:"column:text"`                                // The actual usage text
	LibraryNode      *LibraryNode `json:"library_node,omitempty" gorm:"foreignKey:LibraryID;references:ID"`
}

// TableName specifies the table name for LibraryRelationship
func (LibraryRelationship) TableName() string {
	return "library_relationships"
}

// ComplexityViolation represents a violation of complexity constraints
type ComplexityViolation struct {
	Node        *ASTNode `json:"node"`
	Threshold   int      `json:"threshold"`
	ActualValue int      `json:"actual_value"`
	MetricType  string   `json:"metric_type"` // "cyclomatic", "parameters", "lines"
}

// CallPath represents a path of method calls between nodes
type CallPath struct {
	FromNode    *ASTNode           `json:"from_node"`
	ToNode      *ASTNode           `json:"to_node"`
	Path        []*ASTRelationship `json:"path"`
	PathLength  int                `json:"path_length"`
	CallPattern string             `json:"call_pattern"` // e.g., "Controller -> Service -> Repository"
}

// NodeType is an alias for backward compatibility
type NodeType = string

// ASTNodeType constants for node types
const (
	NodeTypePackage    NodeType = "package"
	NodeTypeType       NodeType = "type"
	NodeTypeMethod     NodeType = "method"
	NodeTypeField      NodeType = "field"
	NodeTypeVariable   NodeType = "variable"
	NodeTypeDependency NodeType = "dependency"

	// SQL Database node types (as sub-types)
	NodeTypeTypeTable        NodeType = "type_table"         // Tables as sub-type of "type"
	NodeTypeTypeView         NodeType = "type_view"          // Views as sub-type of "type"
	NodeTypeMethodStoredProc NodeType = "method_stored_proc" // Stored procedures as sub-type of "method"
	NodeTypeMethodFunction   NodeType = "method_function"    // SQL functions as sub-type of "method"
	NodeTypeFieldColumn      NodeType = "field_column"       // Columns as sub-type of "field"

	// HTTP/REST node types (as sub-types)
	NodeTypeMethodHTTPGet    NodeType = "method_http_get"    // GET endpoints as sub-type of "method"
	NodeTypeMethodHTTPPost   NodeType = "method_http_post"   // POST endpoints as sub-type of "method"
	NodeTypeMethodHTTPPut    NodeType = "method_http_put"    // PUT endpoints as sub-type of "method"
	NodeTypeMethodHTTPDelete NodeType = "method_http_delete" // DELETE endpoints as sub-type of "method"
	NodeTypeTypeHTTPSchema   NodeType = "type_http_schema"   // Schemas as sub-type of "type"
)

// RelationshipType constants for relationship types
const (
	RelationshipCall        = "call"
	RelationshipReference   = "reference"
	RelationshipInheritance = "inheritance"
	RelationshipImplements  = "implements"
	RelationshipImport      = "import"
	RelationshipExtends     = "extends"
	RelationshipForeignKey  = "foreign_key"
)

// nodeTypeIconCache caches NodeType -> api.Text mappings to avoid repeated lookups
var nodeTypeIconCache = make(map[NodeType]api.Text)

// nodeTypeMap defines icon and style mappings ordered by key length (longest first) for prefix matching
var nodeTypeMap = []struct {
	prefix string
	icon   icons.Icon
	style  string
}{
	{"method_stored_proc", icons.DB, "text-blue-700 font-semibold"},
	{"method_http_delete", icons.Http, "text-red-600"},
	{"method_http_post", icons.Http, "text-blue-600"},
	{"method_http_put", icons.Http, "text-orange-600"},
	{"method_http_get", icons.Http, "text-green-600"},
	{"method_function", icons.Lambda, "text-blue-500"},
	{"type_http_schema", icons.Http, "text-purple-600 italic"},
	{"field_column", icons.DB, "text-green-700"},
	{"type_table", icons.DB, "text-purple-700 font-semibold"},
	{"type_view", icons.DB, "text-purple-500"},
	{"method", icons.Method, "text-blue-600"},
	{"variable", icons.Variable, "text-green-500"},
	{"package", icons.Package, "text-orange-600"},
	{"dependency", icons.Link, "text-gray-600"},
	{"field", icons.Variable, "text-green-600"},
	{"type", icons.Type, "text-purple-600"},
}

// getNodeTypeIconStyle returns formatted icon and style for a NodeType using longest-match-first with caching
func getNodeTypeIconStyle(nt NodeType) api.Text {
	// Check cache first
	if cached, ok := nodeTypeIconCache[nt]; ok {
		return cached
	}

	// Find longest matching prefix
	var result api.Text
	for _, mapping := range nodeTypeMap {
		if strings.HasPrefix(string(nt), mapping.prefix) {
			result = clicky.Text("").Add(mapping.icon)
			if mapping.style != "" {
				result.Style = mapping.style
			}
			nodeTypeIconCache[nt] = result
			return result
		}
	}

	// Default fallback
	result = clicky.Text("")
	nodeTypeIconCache[nt] = result
	return result
}

func (n ASTNode) String() string {
	parts := []string{}

	if n.PackageName != "" {
		parts = append(parts, n.PackageName)
	}

	if n.TypeName != "" {
		parts = append(parts, n.TypeName)
	}

	if n.MethodName != "" {
		parts = append(parts, n.MethodName)
	}

	if n.FieldName != "" {
		parts = append(parts, n.FieldName)
	}

	return strings.Join(parts, ".")
}

// GetFullName returns the full qualified name of an AST node
// Deprecated: Use String()
func (n *ASTNode) GetFullName() string {
	return n.String()
}

// GetSignature returns the .ARCHUNIT format signature for the node
// Format: package:method or package:Type.method
// Deprecated: Use String()
func (n *ASTNode) GetSignature() string {
	return n.String()
}

func (n *ASTNode) Pretty() api.Text {
	// Get icon and style from NodeType
	iconStyle := getNodeTypeIconStyle(n.NodeType)

	// Show only current level name, not full path
	var displayName string
	switch {
	case n.NodeType == NodeTypePackage:
		displayName = n.PackageName
	case n.NodeType == NodeTypeType || strings.HasPrefix(string(n.NodeType), "type_"):
		displayName = n.TypeName
	case n.NodeType == NodeTypeMethod || strings.HasPrefix(string(n.NodeType), "method_"):
		displayName = n.MethodName
	case n.NodeType == NodeTypeField || strings.HasPrefix(string(n.NodeType), "field_") || n.NodeType == NodeTypeVariable:
		// For variables and fields, prefer FieldName if available, otherwise use TypeName
		displayName = n.FieldName
		if displayName == "" {
			displayName = n.TypeName
		}
	default:
		// Fallback: use the first available name
		if n.MethodName != "" {
			displayName = n.MethodName
		} else if n.FieldName != "" {
			displayName = n.FieldName
		} else if n.TypeName != "" {
			displayName = n.TypeName
		} else if n.PackageName != "" {
			displayName = n.PackageName
		}
	}

	// Build styled content starting with icon
	content := iconStyle.Append(" ", "")

	if displayName != "" {
		content = content.Append(displayName, iconStyle.Style)

		// Add parentheses for methods with subtle styling
		if n.NodeType == NodeTypeMethod || strings.HasPrefix(string(n.NodeType), "method_") {
			content = content.Append("()", "text-gray-500 text-sm")
		}
	}

	// Add field type if available
	if n.FieldType != nil && *n.FieldType != "" {
		content = content.Append(" : ", "text-gray-400 text-xs")
		content = content.Append(*n.FieldType, "text-blue-500 text-xs")
	}

	// Add default value if available
	if n.DefaultValue != nil && *n.DefaultValue != "" {
		content = content.Append(" = ", "text-gray-400 text-xs")
		content = content.Append(*n.DefaultValue, "text-green-500 text-xs")
	}

	// Add line number with subdued styling (only for real source files, not virtual nodes)
	if n.StartLine > 0 {
		content = content.Append(" L", "text-gray-400 text-xs")
		content = content.Append(fmt.Sprintf("%d", n.StartLine), "text-gray-500 text-xs")
	}

	return content
}

func (n *ASTNode) FullName() api.Text {
	content := clicky.Text("")

	// Include package name if present
	if n.PackageName != "" {
		content = content.Append(n.PackageName)
	}

	if n.TypeName != "" {
		if !content.IsEmpty() {
			content = content.Append(".", "text-gray-500")
		}
		content = content.Append(n.TypeName)
	}

	if n.FieldName != "" {
		if !content.IsEmpty() {
			content = content.Append(".", "text-gray-500")
		}
		content = content.Append(n.FieldName, "font-bold")
	}

	if n.MethodName != "" {
		if !content.IsEmpty() {
			content = content.Append(".", "text-gray-500")
		}
		content = content.Append(n.MethodName, "font-bold")
	}

	return content
}

// ShortClone creates a shallow clone of ASTNode suitable for PrettyShort() display
// without any fields that could cause cycles (Parent, Statements, Relationships)
func (n *ASTNode) ShortClone() *ASTNode {
	if n == nil {
		return nil
	}
	return &ASTNode{
		ID:          n.ID,
		FilePath:    n.FilePath,
		PackageName: n.PackageName,
		TypeName:    n.TypeName,
		MethodName:  n.MethodName,
		FieldName:   n.FieldName,
		NodeType:    n.NodeType,
		Language:    n.Language,
		StartLine:   n.StartLine,
		FieldType:   n.FieldType,
		// Explicitly omit: Parent, Statements, Relationships, and other circular references
	}
}

// GetPackage returns a new ASTNode representing just the package level
func (n *ASTNode) GetPackage() *ASTNode {
	return &ASTNode{
		PackageName: n.PackageName,
	}
}

// GetType returns a new ASTNode representing the package and type level
func (n *ASTNode) GetType() *ASTNode {
	return &ASTNode{
		PackageName: n.PackageName,
		TypeName:    n.TypeName,
	}
}

// ShortName returns a formatted name with icon and type-based coloring using NodeType constants
func (n *ASTNode) ShortName() api.Text {
	// Get icon and style from NodeType
	iconStyle := getNodeTypeIconStyle(n.NodeType)

	// Determine the display name based on node type
	var displayName string
	switch {
	case n.NodeType == NodeTypePackage:
		displayName = n.PackageName
	case n.NodeType == NodeTypeType || strings.HasPrefix(string(n.NodeType), "type_"):
		displayName = n.TypeName
	case n.NodeType == NodeTypeMethod || strings.HasPrefix(string(n.NodeType), "method_"):
		displayName = n.MethodName
	case n.NodeType == NodeTypeField || strings.HasPrefix(string(n.NodeType), "field_") || n.NodeType == NodeTypeVariable:
		displayName = n.FieldName
		if displayName == "" {
			displayName = n.TypeName
		}
	default:
		// Fallback: use the first available name
		if n.MethodName != "" {
			displayName = n.MethodName
		} else if n.FieldName != "" {
			displayName = n.FieldName
		} else if n.TypeName != "" {
			displayName = n.TypeName
		} else if n.PackageName != "" {
			displayName = n.PackageName
		}
	}

	// If we have a display name, append it to the icon
	if displayName != "" {
		return iconStyle.Append(" "+displayName, iconStyle.Style)
	}

	// If no display name, just return the icon
	return iconStyle
}

// PrettyWithConfig returns a detailed Pretty representation based on DisplayConfig and parent context
func (n *ASTNode) PrettyWithConfig(config DisplayConfig, parentContext string) api.Text {
	// Start with base pretty output
	content := n.Pretty()

	// Add detailed information based on config
	if config.ShowComplexity && n.CyclomaticComplexity > 0 {
		var complexityIcon string
		var complexityStyle string
		switch {
		case n.CyclomaticComplexity > 10:
			complexityIcon = "🔥"
			complexityStyle = "text-red-600"
		case n.CyclomaticComplexity > 5:
			complexityIcon = "⚠️"
			complexityStyle = "text-yellow-600"
		default:
			complexityIcon = "📊"
			complexityStyle = "text-green-600"
		}
		content = content.Append(" ", "").Append(complexityIcon, complexityStyle)
		content = content.Append(fmt.Sprintf("%d", n.CyclomaticComplexity), complexityStyle+" text-xs")
	}

	// Add parameter information for methods
	if (n.NodeType == NodeTypeMethod || strings.HasPrefix(string(n.NodeType), "method_")) && len(n.Parameters) > 0 {
		content = content.Append(" (", "text-gray-400 text-xs")

		if config.ShowParams {
			// Show detailed parameter information
			var paramStrs []string
			for _, param := range n.Parameters {
				if param.Name != "_" && param.Name != "" {
					paramStrs = append(paramStrs, fmt.Sprintf("%s %s", param.Name, param.Type))
				} else {
					paramStrs = append(paramStrs, param.Type)
				}
			}
			content = content.Append(strings.Join(paramStrs, ", "), "text-blue-500 text-xs")
		} else {
			// Show just parameter count
			content = content.Append(fmt.Sprintf("%d params", len(n.Parameters)), "text-blue-500 text-xs")
		}

		content = content.Append(")", "text-gray-400 text-xs")
	}

	// Add return information for methods
	if (n.NodeType == NodeTypeMethod || strings.HasPrefix(string(n.NodeType), "method_")) && n.ReturnCount > 0 {
		content = content.Append(" → ", "text-gray-400 text-xs")

		if len(n.ReturnValues) > 0 {
			// Show detailed return information
			var returnStrs []string
			for _, ret := range n.ReturnValues {
				if ret.Name != "" {
					returnStrs = append(returnStrs, fmt.Sprintf("%s %s", ret.Name, ret.Type))
				} else {
					returnStrs = append(returnStrs, ret.Type)
				}
			}
			content = content.Append(strings.Join(returnStrs, ", "), "text-purple-500 text-xs")
		} else {
			// Fallback to count if detailed info not available
			content = content.Append(fmt.Sprintf("%d ret", n.ReturnCount), "text-purple-500 text-xs")
		}
	}

	// Add line count for larger items
	if config.ShowFileStats && n.LineCount > 1 {
		var lineStyle string
		switch {
		case n.LineCount > 100:
			lineStyle = "text-red-500 text-xs"
		case n.LineCount > 50:
			lineStyle = "text-yellow-500 text-xs"
		default:
			lineStyle = "text-gray-500 text-xs"
		}
		content = content.Append(" [", "text-gray-400 text-xs")
		content = content.Append(fmt.Sprintf("%d lines", n.LineCount), lineStyle)
		content = content.Append("]", "text-gray-400 text-xs")
	}

	// Add context-aware information if parentContext is provided
	if parentContext != "" && parentContext != n.PackageName {
		// Show relative context (e.g., method shown within its class context)
		content = content.Append(" in ", "text-gray-400 text-xs")
		content = content.Append(parentContext, "text-gray-600 text-xs")
	}

	return content
}

func (n *ASTNode) AsMap() map[string]interface{} {
	if n == nil {
		return nil
	}
	return map[string]interface{}{
		"call_count":            0, // This would need to be calculated from relationships
		"cyclomatic_complexity": n.CyclomaticComplexity,
		"end_line":              n.EndLine,
		"field_name":            n.FieldName,
		"file_path":             n.FilePath,
		"id":                    n.ID,
		"import_count":          len(n.Imports),
		"imports":               n.Imports,
		"line_count":            n.LineCount,
		"method_name":           n.MethodName,
		"node_type":             string(n.NodeType),
		"package_name":          n.PackageName,
		"parameter_count":       len(n.Parameters),
		"parameters":            n.Parameters,
		"return_count":          len(n.ReturnValues),
		"return_values":         n.ReturnValues,
		"start_line":            n.StartLine,
		"type_name":             n.TypeName,
		"field_type":            n.FieldType,
		"default_value":         n.DefaultValue,
	}
}

// IsComplex returns true if the node exceeds complexity thresholds
func (n *ASTNode) IsComplex(cyclomaticThreshold, parameterThreshold, lineThreshold int) bool {
	return n.CyclomaticComplexity > cyclomaticThreshold ||
		len(n.Parameters) > parameterThreshold ||
		n.LineCount > lineThreshold
}

// GetSourceCode retrieves the source code line for this node
func (n *ASTNode) GetSourceCode() (string, error) {
	return globalSourceReader.GetLine(n.FilePath, n.StartLine)
}

// GetSourceCodeLines retrieves a range of source code lines for this node
func (n *ASTNode) GetSourceCodeLines(start, end int) ([]string, error) {
	return globalSourceReader.GetLines(n.FilePath, start, end)
}

// GetFullSourceCode retrieves all source lines for this node (from StartLine to EndLine)
func (n *ASTNode) GetFullSourceCode() ([]string, error) {
	if n.EndLine == 0 {
		// If EndLine is not set, just get the start line
		line, err := n.GetSourceCode()
		if err != nil {
			return nil, err
		}
		return []string{line}, nil
	}
	return n.GetSourceCodeLines(n.StartLine, n.EndLine)
}

// Global node registry for tree building - cleared and rebuilt as needed
var globalNodeChildren = make(map[int64][]*ASTNode)

// PopulateNodeHierarchy builds parent-child relationships from a flat list of nodes
func PopulateNodeHierarchy(nodes []*ASTNode) {
	// Clear existing registry
	globalNodeChildren = make(map[int64][]*ASTNode)

	// Build parent->children mapping
	for _, node := range nodes {
		if node.ParentID != nil {
			globalNodeChildren[*node.ParentID] = append(globalNodeChildren[*node.ParentID], node)
		}
	}
}

// GetChildren implements api.TreeNode interface for ASTNode
func (n *ASTNode) GetChildren() []api.TreeNode {
	children := globalNodeChildren[n.ID]
	result := make([]api.TreeNode, len(children))
	for i, child := range children {
		result[i] = child
	}
	for _, statement := range n.Statements {
		result = append(result, statement)
	}
	return result
}

// DirectoryTreeNode represents a directory in the filesystem hierarchy
type DirectoryTreeNode struct {
	Path     string
	Children []api.TreeNode
}

// MultiRootTreeNode represents multiple root directories without a wrapper
type MultiRootTreeNode struct {
	Children []api.TreeNode
}

func (d *DirectoryTreeNode) Pretty() api.Text {
	displayName := filepath.Base(d.Path)

	// Handle virtual paths - extract the meaningful part after the protocol
	if strings.Contains(d.Path, "://") {
		parts := strings.SplitN(d.Path, "://", 2)
		if len(parts) == 2 {
			displayName = parts[1] // Use the part after the ://
		}
	}

	return clicky.Text("📁", "text-blue-600").Append(" "+displayName, "text-blue-600 font-medium")
}

func (d *DirectoryTreeNode) GetChildren() []api.TreeNode {
	return d.Children
}

// MultiRootTreeNode implements TreeNode interface
func (m *MultiRootTreeNode) Pretty() api.Text {
	// Return empty text since we don't want to display the root node
	return api.Text{}
}

func (m *MultiRootTreeNode) GetChildren() []api.TreeNode {
	return m.Children
}

// FileTreeNode represents a source file in the hierarchy
type FileTreeNode struct {
	FilePath string
	Children []api.TreeNode
}

func (f *FileTreeNode) Pretty() api.Text {
	displayName := filepath.Base(f.FilePath)

	// Handle virtual paths - extract the meaningful part after the protocol
	if strings.Contains(f.FilePath, "://") {
		parts := strings.SplitN(f.FilePath, "://", 2)
		if len(parts) == 2 {
			displayName = parts[1] // Use the part after the ://
		}
	}

	return clicky.Text("📄", "text-green-600").Append(" "+displayName, "text-green-600 font-medium")
}

func (f *FileTreeNode) GetChildren() []api.TreeNode {
	return f.Children
}

// PackageTreeNode represents a package within a file
type PackageTreeNode struct {
	PackageName string
	Children    []api.TreeNode
}

func (p *PackageTreeNode) Pretty() api.Text {
	return clicky.Text("📦", "text-purple-600").Append(" "+p.PackageName, "text-purple-600 font-medium")
}

func (p *PackageTreeNode) GetChildren() []api.TreeNode {
	return p.Children
}

// EnhancedASTNodeTreeNode wraps ASTNode to provide enhanced display with config
type EnhancedASTNodeTreeNode struct {
	*ASTNode
	Config        DisplayConfig `json:"-" yaml:"-"`
	ParentContext string        `json:"-" yaml:"-"`
}

func (e *EnhancedASTNodeTreeNode) Pretty() api.Text {
	return e.ASTNode.PrettyWithConfig(e.Config, e.ParentContext)
}

func (e *EnhancedASTNodeTreeNode) GetChildren() []api.TreeNode {
	children := e.ASTNode.GetChildren()

	// Filter and wrap children with enhanced display if they're ASTNodes
	var enhanced []api.TreeNode
	for _, child := range children {
		if astChild, ok := child.(*ASTNode); ok {
			// Apply display config filtering to children
			shouldShow := false
			switch {
			case astChild.NodeType == NodeTypeTypeTable || astChild.NodeType == NodeTypeTypeView || astChild.NodeType == NodeTypeType:
				shouldShow = e.Config.ShowTypes
			case astChild.NodeType == NodeTypeMethod || strings.HasPrefix(string(astChild.NodeType), "method_"):
				shouldShow = e.Config.ShowMethods
			case astChild.NodeType == NodeTypeFieldColumn || astChild.NodeType == NodeTypeField || astChild.NodeType == NodeTypeVariable:
				shouldShow = e.Config.ShowFields
			case astChild.NodeType == NodeTypePackage:
				shouldShow = e.Config.ShowPackages
			default:
				shouldShow = true // Show unknown types by default
			}

			if shouldShow {
				enhanced = append(enhanced, &EnhancedASTNodeTreeNode{
					ASTNode:       astChild,
					Config:        e.Config,
					ParentContext: e.getContextForChild(astChild),
				})
			}
		} else {
			enhanced = append(enhanced, child)
		}
	}
	return enhanced
}

func (e *EnhancedASTNodeTreeNode) getContextForChild(child *ASTNode) string {
	// Provide context based on the current node type
	switch {
	case e.ASTNode.TypeName != "":
		return e.ASTNode.TypeName
	case e.ASTNode.PackageName != "":
		return e.ASTNode.PackageName
	default:
		return e.ParentContext
	}
}

// DisplayConfig controls what elements are shown in the AST tree
type DisplayConfig struct {
	// Structure control
	ShowDirs     bool // Show/hide directory structure (default: true)
	ShowFiles    bool // Show/hide individual files (default: true)
	ShowPackages bool // Show/hide package nodes (default: true)

	// Content control
	ShowTypes   bool // Show/hide type definitions (default: true)
	ShowMethods bool // Show/hide methods (default: true)
	ShowFields  bool // Show/hide struct fields (default: false)
	ShowParams  bool // Show/hide method parameters (default: false)
	ShowImports bool // Show/hide import statements (default: false)

	// Display details
	ShowLineNo     bool // Show/hide line numbers (default: true)
	ShowFileStats  bool // Show file-level stats (lines, types, etc.)
	ShowComplexity bool // Show complexity metrics
}

// DefaultDisplayConfig returns sensible defaults for AST display
func DefaultDisplayConfig() DisplayConfig {
	return DisplayConfig{
		ShowDirs:       true,
		ShowFiles:      true,
		ShowPackages:   true,
		ShowTypes:      true,
		ShowMethods:    true,
		ShowFields:     false,
		ShowParams:     false,
		ShowImports:    false,
		ShowLineNo:     true,
		ShowFileStats:  false,
		ShowComplexity: false,
	}
}

// ASTRootTreeNode represents the root of an AST tree for display
type ASTRootTreeNode struct {
	nodes        []*ASTNode
	total        int
	availableIDs map[int64]bool // Map of node IDs available in current result set
}

func (art *ASTRootTreeNode) Pretty() api.Text {
	content := fmt.Sprintf("🏗️ AST Nodes (%d)", art.total)
	return api.Text{
		Content: content,
		Style:   "text-blue-600 font-bold",
	}
}

func (art *ASTRootTreeNode) GetChildren() []api.TreeNode {
	// Smart root detection: return nodes that either have no parent OR whose parent is not in the current result set
	var roots []api.TreeNode
	for _, node := range art.nodes {
		// Treat as root if:
		// 1. Node has no parent (true root), OR
		// 2. Node's parent is not available in current result set (virtual root)
		if node.ParentID == nil || !art.availableIDs[*node.ParentID] {
			roots = append(roots, node)
		}
	}
	return roots
}

// BuildASTNodeTree creates a tree structure from AST nodes for clicky formatting
func BuildASTNodeTree(nodes []*ASTNode) api.TreeNode {
	if len(nodes) == 0 {
		return &ASTRootTreeNode{
			nodes:        []*ASTNode{},
			total:        0,
			availableIDs: make(map[int64]bool),
		}
	}

	// Build map of available node IDs for smart root detection
	availableIDs := make(map[int64]bool)
	for _, node := range nodes {
		availableIDs[node.ID] = true
	}

	// Populate the parent-child relationships
	PopulateNodeHierarchy(nodes)

	return &ASTRootTreeNode{
		nodes:        nodes,
		total:        len(nodes),
		availableIDs: availableIDs,
	}
}

// BuildHierarchicalASTTree creates a filesystem-aware hierarchical tree
func BuildHierarchicalASTTree(nodes []*ASTNode, config DisplayConfig, workingDir string) api.TreeNode {
	if len(nodes) == 0 {
		return &MultiRootTreeNode{Children: []api.TreeNode{}}
	}

	// Build parent-child relationships for better AST hierarchy
	PopulateNodeHierarchy(nodes)

	// Group nodes by directory structure
	dirMap := make(map[string]*DirectoryTreeNode)
	fileMap := make(map[string]*FileTreeNode)
	packageMap := make(map[string]*PackageTreeNode)

	// Build the hierarchy
	for _, node := range nodes {
		// Check if this is a SQL virtual path
		if strings.HasPrefix(node.FilePath, "sql://") {
			// For SQL nodes, use built-in parent-child relationships
			// Skip the package layer and let the ASTNode parent relationships handle the hierarchy
			addSQLNodeToHierarchy(node, fileMap, dirMap, config)
			continue
		}

		// Convert absolute file path to relative path from working directory
		relFilePath, err := filepath.Rel(workingDir, node.FilePath)
		if err != nil {
			// Fallback to original path if conversion fails
			relFilePath = node.FilePath
		}

		// Extract directory path from relative file path
		dirPath := filepath.Dir(relFilePath)
		if dirPath == "." {
			dirPath = ""
		}

		// Ensure directory structure exists
		ensureDirectoryStructure(dirPath, dirMap, config)

		// Ensure file exists in directory
		ensureFileInDirectory(relFilePath, dirPath, fileMap, dirMap, config)

		// Create a copy of the node with relative file path for package handling
		nodeWithRelPath := *node
		nodeWithRelPath.FilePath = relFilePath

		// Ensure package exists in file
		ensurePackageInFile(&nodeWithRelPath, fileMap, packageMap, dirMap, config)

		// Add the AST node to its package (filtered by config)
		addASTNodeToPackage(&nodeWithRelPath, packageMap, fileMap, dirMap, config)
	}

	// Collect all root-level nodes with comprehensive hoisting
	var rootChildren []api.TreeNode

	if config.ShowDirs {
		// Start with directories and apply hoisting logic
		if rootDir, exists := dirMap[""]; exists {
			for _, child := range rootDir.Children {
				hoistedChildren := hoistChildrenFromHiddenLevels(child, config)
				rootChildren = append(rootChildren, hoistedChildren...)
			}
		}

		// Also include virtual files (SQL databases, OpenAPI specs, etc.) that don't have directory structure
		for filePath, fileNode := range fileMap {
			if strings.Contains(filePath, "://") { // Virtual path indicator
				hoistedChildren := hoistChildrenFromHiddenLevels(fileNode, config)
				rootChildren = append(rootChildren, hoistedChildren...)
			}
		}
	} else {
		// If not showing directories, start from files and apply hoisting
		for _, fileNode := range fileMap {
			hoistedChildren := hoistChildrenFromHiddenLevels(fileNode, config)
			rootChildren = append(rootChildren, hoistedChildren...)
		}
	}

	// Return children directly without a root wrapper
	return &MultiRootTreeNode{Children: rootChildren}
}

// FilterASTNodes applies display configuration filtering to a slice of nodes
func FilterASTNodes(nodes []*ASTNode, config DisplayConfig) []*ASTNode {
	var filtered []*ASTNode

	for _, node := range nodes {
		// Apply filtering logic based on node type and config
		shouldShow := false
		switch {
		// SQL node types
		case node.NodeType == NodeTypeTypeTable || node.NodeType == NodeTypeTypeView:
			shouldShow = config.ShowTypes
		case node.NodeType == NodeTypeMethod || strings.HasPrefix(string(node.NodeType), "method_"):
			shouldShow = config.ShowMethods
		case node.NodeType == NodeTypeFieldColumn || node.NodeType == NodeTypeField || node.NodeType == NodeTypeVariable:
			shouldShow = config.ShowFields
		// Regular Go node types
		case node.NodeType == NodeTypeType:
			shouldShow = config.ShowTypes
		default:
			shouldShow = true // Show unknown types by default
		}

		if shouldShow {
			filtered = append(filtered, node)
		}
	}

	return filtered
}

// Helper functions for hierarchical tree building

// addSQLNodeToHierarchy adds SQL nodes directly to file structure, bypassing package layer
func addSQLNodeToHierarchy(node *ASTNode, fileMap map[string]*FileTreeNode, dirMap map[string]*DirectoryTreeNode, config DisplayConfig) {
	// Ensure file exists for SQL virtual path
	if _, exists := fileMap[node.FilePath]; !exists {
		fileMap[node.FilePath] = &FileTreeNode{
			FilePath: node.FilePath,
			Children: []api.TreeNode{},
		}
	}

	// Filter nodes based on type and config
	shouldShow := false
	switch {
	case node.NodeType == NodeTypeTypeTable || node.NodeType == NodeTypeTypeView:
		shouldShow = config.ShowTypes
	case node.NodeType == NodeTypeMethod || strings.HasPrefix(string(node.NodeType), "method_"):
		shouldShow = config.ShowMethods
	case node.NodeType == NodeTypeFieldColumn || node.NodeType == NodeTypeField || node.NodeType == NodeTypeVariable:
		shouldShow = config.ShowFields
	default:
		shouldShow = true // Show unknown types by default
	}

	// If this node type should not be shown, don't add it to hierarchy at all
	if !shouldShow {
		return
	}

	// For SQL nodes, check if they have a parent and should be children of another node
	if node.ParentID != nil {
		// This node has a parent, it will be added via the GetChildren() method
		// Don't add it directly to the file structure
		return
	}

	// This is a root-level SQL node (typically a table, view, or stored procedure)
	// Add it directly to the file
	if file, exists := fileMap[node.FilePath]; exists {
		enhancedNode := &EnhancedASTNodeTreeNode{
			ASTNode:       node,
			Config:        config,
			ParentContext: "",
		}
		file.Children = append(file.Children, enhancedNode)
	}
}

// ensureDirectoryStructure creates directory nodes for the given path
func ensureDirectoryStructure(dirPath string, dirMap map[string]*DirectoryTreeNode, config DisplayConfig) {
	if !config.ShowDirs || dirPath == "" {
		return
	}

	// Create directory if it doesn't exist
	if _, exists := dirMap[dirPath]; !exists {
		dirMap[dirPath] = &DirectoryTreeNode{
			Path:     dirPath,
			Children: []api.TreeNode{},
		}

		// Ensure parent directories exist
		parentDir := filepath.Dir(dirPath)
		if parentDir != "." && parentDir != dirPath {
			ensureDirectoryStructure(parentDir, dirMap, config)
			// Add this directory to its parent
			if parent, exists := dirMap[parentDir]; exists {
				parent.Children = append(parent.Children, dirMap[dirPath])
			}
		} else {
			// This is a root-level directory, add to empty string key
			if _, exists := dirMap[""]; !exists {
				dirMap[""] = &DirectoryTreeNode{Path: ".", Children: []api.TreeNode{}}
			}
			dirMap[""].Children = append(dirMap[""].Children, dirMap[dirPath])
		}
	}
}

// ensureFileInDirectory creates file nodes and links them to directories
func ensureFileInDirectory(filePath, dirPath string, fileMap map[string]*FileTreeNode, dirMap map[string]*DirectoryTreeNode, config DisplayConfig) {
	if !config.ShowFiles {
		return
	}

	if _, exists := fileMap[filePath]; !exists {
		fileMap[filePath] = &FileTreeNode{
			FilePath: filePath,
			Children: []api.TreeNode{},
		}

		// Add file to its directory
		if config.ShowDirs {
			if dir, exists := dirMap[dirPath]; exists {
				dir.Children = append(dir.Children, fileMap[filePath])
			}
		}
	}
}

// ensurePackageInFile creates package nodes and links them to files or directories
func ensurePackageInFile(node *ASTNode, fileMap map[string]*FileTreeNode, packageMap map[string]*PackageTreeNode, dirMap map[string]*DirectoryTreeNode, config DisplayConfig) {
	if !config.ShowPackages || node.PackageName == "" {
		return
	}

	packageKey := node.FilePath + ":" + node.PackageName
	if _, exists := packageMap[packageKey]; !exists {
		packageMap[packageKey] = &PackageTreeNode{
			PackageName: node.PackageName,
			Children:    []api.TreeNode{},
		}

		// Add package to its file if showing files
		if config.ShowFiles {
			if file, exists := fileMap[node.FilePath]; exists {
				file.Children = append(file.Children, packageMap[packageKey])
			}
		} else {
			// If not showing files, hoist package directly to its directory
			dirPath := filepath.Dir(node.FilePath)
			if dirPath == "." {
				dirPath = ""
			}
			if dir, exists := dirMap[dirPath]; exists {
				dir.Children = append(dir.Children, packageMap[packageKey])
			}
		}
	}
}

// addASTNodeToPackage adds AST nodes to their packages, files, or directories based on display config
func addASTNodeToPackage(node *ASTNode, packageMap map[string]*PackageTreeNode, fileMap map[string]*FileTreeNode, dirMap map[string]*DirectoryTreeNode, config DisplayConfig) {
	packageKey := node.FilePath + ":" + node.PackageName

	// Filter nodes based on type and config
	shouldShow := false
	switch node.NodeType {
	case NodeTypeType:
		shouldShow = config.ShowTypes
	case NodeTypeMethod:
		shouldShow = config.ShowMethods
	case NodeTypeField, NodeTypeVariable:
		// Both struct fields and global variables are "field-like" data elements
		shouldShow = config.ShowFields
	default:
		shouldShow = true // Show unknown types by default
	}

	if shouldShow {
		// Create enhanced node wrapper with appropriate context
		var enhancedNode api.TreeNode
		var parentContext string

		if config.ShowPackages {
			// Normal case: add to package
			if pkg, exists := packageMap[packageKey]; exists {
				parentContext = pkg.PackageName
				enhancedNode = &EnhancedASTNodeTreeNode{
					ASTNode:       node,
					Config:        config,
					ParentContext: parentContext,
				}
				pkg.Children = append(pkg.Children, enhancedNode)
			}
		} else if config.ShowFiles {
			// Hoist to file level when packages are hidden
			if file, exists := fileMap[node.FilePath]; exists {
				parentContext = filepath.Base(file.FilePath)
				enhancedNode = &EnhancedASTNodeTreeNode{
					ASTNode:       node,
					Config:        config,
					ParentContext: parentContext,
				}
				file.Children = append(file.Children, enhancedNode)
			}
		} else {
			// Hoist to directory level when both packages and files are hidden
			dirPath := filepath.Dir(node.FilePath)
			if dirPath == "." {
				dirPath = ""
			}
			if dir, exists := dirMap[dirPath]; exists {
				parentContext = filepath.Base(dirPath)
				enhancedNode = &EnhancedASTNodeTreeNode{
					ASTNode:       node,
					Config:        config,
					ParentContext: parentContext,
				}
				dir.Children = append(dir.Children, enhancedNode)
			}
		}
	}
}

// hoistChildrenFromHiddenLevels recursively collects children from hidden intermediate levels
func hoistChildrenFromHiddenLevels(node api.TreeNode, config DisplayConfig) []api.TreeNode {
	var result []api.TreeNode

	switch n := node.(type) {
	case *DirectoryTreeNode:
		// Always include directories if we're showing them
		if config.ShowDirs {
			result = append(result, n)
		} else {
			// If not showing directories, hoist their children
			for _, child := range n.Children {
				result = append(result, hoistChildrenFromHiddenLevels(child, config)...)
			}
		}
	case *FileTreeNode:
		// Include files only if showing them
		if config.ShowFiles {
			result = append(result, n)
		} else {
			// If not showing files, hoist their children
			for _, child := range n.Children {
				result = append(result, hoistChildrenFromHiddenLevels(child, config)...)
			}
		}
	case *PackageTreeNode:
		// Include packages only if showing them
		if config.ShowPackages {
			result = append(result, n)
		} else {
			// If not showing packages, hoist their children
			for _, child := range n.Children {
				result = append(result, hoistChildrenFromHiddenLevels(child, config)...)
			}
		}
	default:
		// For AST nodes and other types, always include them
		result = append(result, n)
	}

	return result
}

// ClearNodeHierarchy cleans up the global registry
func ClearNodeHierarchy() {
	globalNodeChildren = make(map[int64][]*ASTNode)
}

// PrettyRow implements the api.PrettyRow interface for custom table rendering
func (n ASTNode) PrettyRow(opts interface{}) map[string]api.Text {
	row := make(map[string]api.Text)

	// Name column using ShortName() - replaces Package/Type/Member columns
	shortName := n.ShortName()
	row["Name"] = shortName

	// Package column
	if n.PackageName != "" {
		row["Package"] = api.Text{
			Content: n.PackageName,
			Style:   "text-orange-600 max-w-[80ch] truncate",
		}
	}

	// File column using basename:L1-23 format
	if n.FilePath != "" {
		fileName := filepath.Base(n.FilePath)
		var lineRef string
		if n.EndLine > 0 && n.EndLine != n.StartLine {
			lineRef = fmt.Sprintf(":L%d-%d", n.StartLine, n.EndLine)
		} else if n.StartLine > 0 {
			lineRef = fmt.Sprintf(":L%d", n.StartLine)
		}
		displayName := fileName + lineRef

		// Truncate from beginning if too long (ellipsis-prefix style)
		if len(displayName) > 77 {
			displayName = "..." + displayName[len(displayName)-74:]
		}

		row["File"] = api.Text{
			Content: displayName,
			Style:   "text-blue-500 max-w-[80ch] ellipsis-prefix",
		}
	}

	// Lines column with enhanced display (range if available)
	if n.LineCount > 0 {
		var linesContent string
		if n.EndLine > 0 && n.EndLine != n.StartLine {
			// Show range: "45-67 (23)"
			linesContent = fmt.Sprintf("%d-%d (%d)", n.StartLine, n.EndLine, n.LineCount)
		} else if n.StartLine > 0 {
			// Show start line with count: "45 (23)"
			linesContent = fmt.Sprintf("%d (%d)", n.StartLine, n.LineCount)
		} else {
			// Just show count: "23"
			linesContent = fmt.Sprintf("%d", n.LineCount)
		}

		row["Lines"] = api.Text{
			Content: linesContent,
			Style:   "text-gray-600 max-w-[80ch] truncate font-mono",
		}
	} else if n.StartLine > 0 {
		// Show just the start line if no count available
		row["Lines"] = api.Text{
			Content: fmt.Sprintf("%d", n.StartLine),
			Style:   "text-gray-600 max-w-[80ch] truncate font-mono",
		}
	}

	// Complexity column with color coding (keep existing logic)
	if n.CyclomaticComplexity > 0 {
		var style string
		if n.CyclomaticComplexity <= 5 {
			style = "text-green-600 max-w-[80ch] truncate"
		} else if n.CyclomaticComplexity <= 10 {
			style = "text-yellow-600 max-w-[80ch] truncate"
		} else {
			style = "text-red-600 max-w-[80ch] truncate"
		}
		row["Complexity"] = api.Text{
			Content: fmt.Sprintf("%d", n.CyclomaticComplexity),
			Style:   style,
		}
	}

	// Parameters column - show names and types if available, otherwise count
	if n.ParameterCount > 0 {
		var content string
		if len(n.Parameters) > 0 {
			// Show parameter names and types
			var parts []string
			for _, param := range n.Parameters {
				if param.Name != "" && param.Name != "_" {
					parts = append(parts, fmt.Sprintf("%s %s", param.Name, param.Type))
				} else {
					parts = append(parts, param.Type)
				}
			}
			content = strings.Join(parts, ", ")
			// Truncate if too long
			if len(content) > 77 {
				content = content[:74] + "..."
			}
		} else {
			// Fallback to count
			content = fmt.Sprintf("%d", n.ParameterCount)
		}
		row["Params"] = api.Text{
			Content: content,
			Style:   "max-w-[80ch] truncate",
		}
	}

	// Returns column - show return names and types if available, otherwise count
	if n.ReturnCount > 0 {
		var content string
		if len(n.ReturnValues) > 0 {
			// Show return names and types
			var parts []string
			for _, ret := range n.ReturnValues {
				if ret.Name != "" {
					parts = append(parts, fmt.Sprintf("%s %s", ret.Name, ret.Type))
				} else {
					parts = append(parts, ret.Type)
				}
			}
			content = strings.Join(parts, ", ")
			// Truncate if too long
			if len(content) > 77 {
				content = content[:74] + "..."
			}
		} else {
			// Fallback to count
			content = fmt.Sprintf("%d", n.ReturnCount)
		}
		row["Returns"] = api.Text{
			Content: content,
			Style:   "max-w-[80ch] truncate",
		}
	}

	// Field Type column - show type for fields
	if n.FieldType != nil && *n.FieldType != "" {
		row["Type"] = api.Text{
			Content: *n.FieldType,
			Style:   "text-blue-500 max-w-[80ch] truncate",
		}
	}

	// Default Value column - show default value for fields
	if n.DefaultValue != nil && *n.DefaultValue != "" {
		row["Default"] = api.Text{
			Content: *n.DefaultValue,
			Style:   "text-green-500 max-w-[80ch] truncate",
		}
	}

	return row
}
