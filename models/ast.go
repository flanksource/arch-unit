// Note to claude:  DO NOT MODIFY any structs in this file without consulting the user, Add/Remove/Modify functions as needed without change the fields.

package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/flanksource/clicky/api"
)

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
	IsExported bool        `json:"is_exported,omitempty"` // For Go visibility
}

// Field represents a struct/class field
type Field struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	NameLength int       `json:"name_length"`
	Comments   []Comment `json:"comments"`
	IsExported bool      `json:"is_exported,omitempty"`
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
	IsExported bool       `json:"is_exported,omitempty"`
}

// Variable represents a variable declaration
type Variable struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	NameLength int       `json:"name_length"`
	Line       int       `json:"line"`
	IsConstant bool      `json:"is_constant"`
	IsExported bool      `json:"is_exported,omitempty"`
	Comments   []Comment `json:"comments"`
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
	ID                   int64         `json:"id"`
	ParentID             int64         `json:"parent_id,omitempty"`     // Nullable for root nodes, For a field, parent is the struct/class, for a struct/class parent is package,
	DependencyID         *int64        `json:"dependency_id,omitempty"` // Id of the dependency that contains this node
	FilePath             string        `json:"file_path"`
	PackageName          string        `json:"package_name,omitempty"`
	TypeName             string        `json:"type_name,omitempty"`
	MethodName           string        `json:"method_name,omitempty"`
	FieldName            string        `json:"field_name,omitempty"`
	NodeType             NodeType      `json:"node_type"` // "package", "type", "method", "field", "variable"
	StartLine            int           `json:"start_line"`
	EndLine              int           `json:"end_line"`
	CyclomaticComplexity int           `json:"cyclomatic_complexity"`
	LineCount            int           `json:"line_count"`
	Imports              []string      `json:"imports,omitempty"`       // List of import paths
	Parameters           []Parameter   `json:"parameters,omitempty"`    // Detailed parameter information
	ReturnValues         []ReturnValue `json:"return_values,omitempty"` // Return value information
	LastModified         time.Time     `json:"last_modified"`
	FileHash             string        `json:"file_hash,omitempty"`
	// Summary is an AI generated/enhanced summary of the node,
	// For fields, its a max of 5 words, for method, a max of 20 works, and for types a maximum of 50
	Summary string `json:"summary,omitempty"`
}

// ASTRelationship represents a relationship between AST nodes
type ASTRelationship struct {
	ID               int64            `json:"id"`
	FromASTID        int64            `json:"from_ast_id"`
	ToASTID          *int64           `json:"to_ast_id,omitempty"` // Nullable for external calls
	LineNo           int              `json:"line_no,omitempty"`
	RelationshipType RelationshipType `json:"relationship_type"`
	Comments         string           `json:"comments,omitempty"` // Additional comments or context found in the code
	Text             string           `json:"text"`               // Text of the relationship, could be the line(s) with the function call, the line in a go.mod or Chart.yaml=
}

type RelationshipType string

const (
	RelationshipTypeImport      RelationshipType = "import"      // Import statement in code
	RelationshipTypeCall        RelationshipType = "call"        // Function/method call
	RelationshipTypeReference   RelationshipType = "reference"   // Variable reference
	RelationshipTypeInheritance RelationshipType = "inheritance" // Class inheritance, Docker FROM
	RelationshipTypeImplements  RelationshipType = "implements"  // Interface implementation
	RelationshipTypeIncludes    RelationshipType = "includes"    // e.g. For a chart including a subchart
)

// DependencyRelationship represents a relationship between AST node and dependency node
type DependencyRelationship struct {
	ID           int64 `json:"id"`
	ASTID        int64 `json:"ast_id"`
	DependencyID int64 `json:"dependency_id"`
	LineNo       int   `json:"line_no"`
}

// LibraryNode represents a node in an external library/framework
type LibraryNode struct {
	ID        int64  `json:"id"`
	Package   string `json:"package"`
	Class     string `json:"class,omitempty"`
	Method    string `json:"method,omitempty"`
	Field     string `json:"field,omitempty"`
	NodeType  string `json:"node_type"`           // 'package', 'class', 'method', 'field'
	Language  string `json:"language,omitempty"`  // 'go', 'python', 'javascript', etc.
	Framework string `json:"framework,omitempty"` // 'stdlib', 'gin', 'django', 'react', etc.
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
	ID               int64        `json:"id"`
	ASTID            int64        `json:"ast_id"`
	LibraryID        int64        `json:"library_id"`
	LineNo           int          `json:"line_no"`
	RelationshipType string       `json:"relationship_type"` // 'import', 'call', 'reference', 'extends'
	Text             string       `json:"text,omitempty"`    // The actual usage text
	LibraryNode      *LibraryNode `json:"library_node,omitempty"`
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
)

// RelationshipType constants for relationship types
const (
	RelationshipCall        = "call"
	RelationshipReference   = "reference"
	RelationshipInheritance = "inheritance"
	RelationshipImplements  = "implements"
	RelationshipImport      = "import"
	RelationshipExtends     = "extends"
)

// GetFullName returns the full qualified name of an AST node
func (n *ASTNode) GetFullName() string {
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

// GetSignature returns the .ARCHUNIT format signature for the node
// Format: package:method or package:Type.method
func (n *ASTNode) GetSignature() string {
	if n.PackageName == "" {
		return ""
	}

	var signature strings.Builder
	signature.WriteString(n.PackageName)

	if n.MethodName != "" {
		signature.WriteString(":")
		if n.TypeName != "" {
			signature.WriteString(n.TypeName)
			signature.WriteString(".")
		}
		signature.WriteString(n.MethodName)
	} else if n.TypeName != "" {
		signature.WriteString(":")
		signature.WriteString(n.TypeName)
		if n.FieldName != "" {
			signature.WriteString(".")
			signature.WriteString(n.FieldName)
		}
	} else if n.FieldName != "" {
		signature.WriteString(":")
		signature.WriteString(n.FieldName)
	}

	return signature.String()
}

func (n *ASTNode) Pretty() api.Text {
	// implement pretty for the current node, adding an apropriate icon by node type
	icon := "📄" // default icon
	switch n.NodeType {
	case NodeTypePackage:
		icon = "📦"
	case NodeTypeType:
		icon = "🔤"
	case NodeTypeMethod:
		icon = "🔧"
	case NodeTypeField:
		icon = "🔑"
	}

	return api.Text{
		Content:  fmt.Sprintf("%s %s", icon, n.GetFullName()),
		Children: nil, // No children for AST nodes
		Style:    "ast-node",
	}
}

func (n *ASTNode) AsMap() map[string]interface{} {
	if n == nil {
		return nil
	}
	return map[string]interface{}{
		"id":                    n.ID,
		"file_path":             n.FilePath,
		"package_name":          n.PackageName,
		"type_name":             n.TypeName,
		"method_name":           n.MethodName,
		"field_name":            n.FieldName,
		"node_type":             string(n.NodeType),
		"start_line":            n.StartLine,
		"end_line":              n.EndLine,
		"cyclomatic_complexity": n.CyclomaticComplexity,
		"parameter_count":       len(n.Parameters),
		"return_count":          len(n.ReturnValues),
		"line_count":            n.LineCount,
		"import_count":          len(n.Imports),
		"call_count":            0, // This would need to be calculated from relationships
		"imports":               n.Imports,
		"parameters":            n.Parameters,
		"return_values":         n.ReturnValues,
	}
}

// IsComplex returns true if the node exceeds complexity thresholds
func (n *ASTNode) IsComplex(cyclomaticThreshold, parameterThreshold, lineThreshold int) bool {
	return n.CyclomaticComplexity > cyclomaticThreshold ||
		len(n.Parameters) > parameterThreshold ||
		n.LineCount > lineThreshold
}
