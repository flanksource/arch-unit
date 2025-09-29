package openapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"gopkg.in/yaml.v3"
)

// OpenAPIExtractor extracts AST information from OpenAPI specifications
type OpenAPIExtractor struct {
	virtualPathMgr *analysis.VirtualPathManager
	httpClient     *http.Client
}

// NewOpenAPIExtractor creates a new OpenAPI AST extractor
func NewOpenAPIExtractor() *OpenAPIExtractor {
	return &OpenAPIExtractor{
		virtualPathMgr: analysis.NewVirtualPathManager(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ExtractFile extracts AST from an OpenAPI specification file
func (e *OpenAPIExtractor) ExtractFile(cache cache.ReadOnlyCache, filepath string, content []byte) (*types.ASTResult, error) {
	// Parse the OpenAPI specification
	spec, err := e.parseOpenAPISpec(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	// Convert to AST nodes
	result := types.NewASTResult(filepath, "openapi")
	e.convertSpecToASTNodes(spec, filepath, result)

	return result, nil
}

// ExtractFromURL extracts AST from an OpenAPI specification URL
func (e *OpenAPIExtractor) ExtractFromURL(url string) (*types.ASTResult, error) {
	// Create virtual path for this URL
	virtualPath := e.virtualPathMgr.CreateVirtualPath(analysis.AnalysisSource{
		Type: "openapi_url",
		URL:  url,
	})

	// Fetch the OpenAPI specification from URL
	resp, err := e.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OpenAPI spec from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d when fetching %s", resp.StatusCode, url)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the OpenAPI specification
	spec, err := e.parseOpenAPISpec(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	// Convert to AST nodes
	result := types.NewASTResult(virtualPath, "openapi")
	e.convertSpecToASTNodes(spec, virtualPath, result)

	return result, nil
}

// OpenAPI specification structures (simplified)
type OpenAPISpec struct {
	OpenAPI    string                 `json:"openapi" yaml:"openapi"`
	Info       Info                   `json:"info" yaml:"info"`
	Paths      map[string]PathItem    `json:"paths" yaml:"paths"`
	Components *Components            `json:"components,omitempty" yaml:"components,omitempty"`
	Tags       []Tag                  `json:"tags,omitempty" yaml:"tags,omitempty"`
}

type Info struct {
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Version     string `json:"version" yaml:"version"`
}

type PathItem struct {
	Get    *Operation `json:"get,omitempty" yaml:"get,omitempty"`
	Post   *Operation `json:"post,omitempty" yaml:"post,omitempty"`
	Put    *Operation `json:"put,omitempty" yaml:"put,omitempty"`
	Delete *Operation `json:"delete,omitempty" yaml:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty" yaml:"patch,omitempty"`
}

type Operation struct {
	OperationID string      `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	Summary     string      `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        []string    `json:"tags,omitempty" yaml:"tags,omitempty"`
	Parameters  []Parameter `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody `json:"requestBody,omitempty" yaml:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses,omitempty" yaml:"responses,omitempty"`
}

type Parameter struct {
	Name        string `json:"name" yaml:"name"`
	In          string `json:"in" yaml:"in"` // "query", "header", "path", "cookie"
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

type RequestBody struct {
	Description string                `json:"description,omitempty" yaml:"description,omitempty"`
	Content     map[string]MediaType  `json:"content,omitempty" yaml:"content,omitempty"`
	Required    bool                  `json:"required,omitempty" yaml:"required,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty" yaml:"schema,omitempty"`
}

type Response struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

type Components struct {
	Schemas map[string]Schema `json:"schemas,omitempty" yaml:"schemas,omitempty"`
}

type Schema struct {
	Type        string            `json:"type,omitempty" yaml:"type,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema           `json:"items,omitempty" yaml:"items,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Required    []string          `json:"required,omitempty" yaml:"required,omitempty"`
	Ref         string            `json:"$ref,omitempty" yaml:"$ref,omitempty"`
}

type Tag struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// parseOpenAPISpec parses content as either JSON or YAML
func (e *OpenAPIExtractor) parseOpenAPISpec(content []byte) (*OpenAPISpec, error) {
	var spec OpenAPISpec

	// Try JSON first
	if err := json.Unmarshal(content, &spec); err == nil {
		return &spec, nil
	}

	// Try YAML
	if err := yaml.Unmarshal(content, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse as JSON or YAML: %w", err)
	}

	return &spec, nil
}

// convertSpecToASTNodes converts OpenAPI spec to AST nodes
func (e *OpenAPIExtractor) convertSpecToASTNodes(spec *OpenAPISpec, filePath string, result *types.ASTResult) {
	// Determine API namespace from title or tags
	apiNamespace := e.extractAPINamespace(spec)

	// Process schemas as types
	if spec.Components != nil {
		for schemaName, schema := range spec.Components.Schemas {
			schemaNode := e.convertSchemaToASTNode(schemaName, schema, apiNamespace, filePath)
			result.AddNode(schemaNode)

			// Process schema properties as fields
			for propName, propSchema := range schema.Properties {
				fieldNode := e.convertSchemaPropertyToASTNode(propName, propSchema, schemaName, apiNamespace, filePath)
				result.AddNode(fieldNode)
			}
		}
	}

	// Process paths as methods
	for path, pathItem := range spec.Paths {
		operations := map[string]*Operation{
			"GET":    pathItem.Get,
			"POST":   pathItem.Post,
			"PUT":    pathItem.Put,
			"DELETE": pathItem.Delete,
			"PATCH":  pathItem.Patch,
		}

		for method, operation := range operations {
			if operation != nil {
				methodNode := e.convertOperationToASTNode(path, method, operation, apiNamespace, filePath)
				result.AddNode(methodNode)
			}
		}
	}
}

// extractAPINamespace extracts a namespace from the OpenAPI spec
func (e *OpenAPIExtractor) extractAPINamespace(spec *OpenAPISpec) string {
	// Try to extract version from info
	if spec.Info.Version != "" {
		// Clean version string to use as namespace
		version := strings.ReplaceAll(spec.Info.Version, ".", "_")
		return "v" + version
	}

	// Fallback to title-based namespace
	if spec.Info.Title != "" {
		// Convert title to a valid namespace
		namespace := strings.ToLower(spec.Info.Title)
		namespace = strings.ReplaceAll(namespace, " ", "_")
		namespace = strings.ReplaceAll(namespace, "-", "_")
		return namespace
	}

	return "api"
}

// convertSchemaToASTNode converts an OpenAPI schema to an AST type node
func (e *OpenAPIExtractor) convertSchemaToASTNode(schemaName string, schema Schema, namespace, filePath string) *models.ASTNode {
	return &models.ASTNode{
		FilePath:     filePath,
		PackageName:  namespace,
		TypeName:     schemaName,
		NodeType:     models.NodeTypeTypeHTTPSchema,
		StartLine:    -1,
		LastModified: time.Now(),
		Summary:      fmt.Sprintf("API schema with %d properties", len(schema.Properties)),
	}
}

// convertSchemaPropertyToASTNode converts a schema property to an AST field node
func (e *OpenAPIExtractor) convertSchemaPropertyToASTNode(propName string, propSchema Schema, parentSchema, namespace, filePath string) *models.ASTNode {
	return &models.ASTNode{
		FilePath:     filePath,
		PackageName:  namespace,
		TypeName:     parentSchema,
		FieldName:    propName,
		NodeType:     models.NodeTypeField,
		StartLine:    -1,
		LastModified: time.Now(),
		Summary:      fmt.Sprintf("%s field", propSchema.Type),
	}
}

// convertOperationToASTNode converts an OpenAPI operation to an AST method node
func (e *OpenAPIExtractor) convertOperationToASTNode(path, method string, operation *Operation, namespace, filePath string) *models.ASTNode {
	// Determine node type based on HTTP method
	var nodeType models.NodeType
	switch strings.ToUpper(method) {
	case "GET":
		nodeType = models.NodeTypeMethodHTTPGet
	case "POST":
		nodeType = models.NodeTypeMethodHTTPPost
	case "PUT":
		nodeType = models.NodeTypeMethodHTTPPut
	case "DELETE":
		nodeType = models.NodeTypeMethodHTTPDelete
	default:
		nodeType = models.NodeTypeMethod
	}

	// Create method name from operation ID or path
	methodName := operation.OperationID
	if methodName == "" {
		methodName = fmt.Sprintf("%s %s", method, path)
	}

	// Convert OpenAPI parameters to AST parameters
	parameters := make([]models.Parameter, 0)

	// Add path, query, and header parameters
	for _, param := range operation.Parameters {
		astParam := models.Parameter{
			Name:       param.Name,
			Type:       e.getParameterType(param),
			NameLength: len(param.Name),
		}
		parameters = append(parameters, astParam)
	}

	// Add request body parameters if present
	if operation.RequestBody != nil {
		for contentType := range operation.RequestBody.Content {
			astParam := models.Parameter{
				Name:       "body",
				Type:       contentType,
				NameLength: 4,
			}
			parameters = append(parameters, astParam)
		}
	}

	return &models.ASTNode{
		FilePath:       filePath,
		PackageName:    namespace,
		MethodName:     methodName,
		NodeType:       nodeType,
		StartLine:      -1,
		Parameters:     parameters,
		ParameterCount: len(parameters),
		LastModified:   time.Now(),
		Summary:        fmt.Sprintf("%s endpoint with %d parameters", method, len(parameters)),
	}
}

// getParameterType returns the type of an OpenAPI parameter
func (e *OpenAPIExtractor) getParameterType(param Parameter) string {
	if param.Schema != nil && param.Schema.Type != "" {
		return param.Schema.Type
	}

	// Default based on parameter location
	switch param.In {
	case "path":
		return "string" // Path parameters are typically strings
	case "query":
		return "string" // Query parameters default to string
	case "header":
		return "string" // Header parameters are strings
	case "cookie":
		return "string" // Cookie parameters are strings
	default:
		return "unknown"
	}
}

// ExtractFromSpec extracts AST from an already parsed OpenAPI spec
func (e *OpenAPIExtractor) ExtractFromSpec(spec *OpenAPISpec, sourcePath string) (*types.ASTResult, error) {
	result := types.NewASTResult(sourcePath, "openapi")
	e.convertSpecToASTNodes(spec, sourcePath, result)
	return result, nil
}

// ValidateSpec performs basic validation on an OpenAPI spec
func (e *OpenAPIExtractor) ValidateSpec(spec *OpenAPISpec) error {
	if spec.OpenAPI == "" {
		return fmt.Errorf("missing openapi version")
	}

	if spec.Info.Title == "" {
		return fmt.Errorf("missing api title")
	}

	if spec.Info.Version == "" {
		return fmt.Errorf("missing api version")
	}

	if len(spec.Paths) == 0 {
		return fmt.Errorf("no paths defined")
	}

	return nil
}

// GetSupportedVersions returns the OpenAPI versions supported by this extractor
func (e *OpenAPIExtractor) GetSupportedVersions() []string {
	return []string{"3.0.0", "3.0.1", "3.0.2", "3.0.3", "3.1.0"}
}

// IsVersionSupported checks if the given OpenAPI version is supported
func (e *OpenAPIExtractor) IsVersionSupported(version string) bool {
	for _, supported := range e.GetSupportedVersions() {
		if version == supported {
			return true
		}
	}
	return false
}