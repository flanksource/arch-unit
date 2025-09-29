package analysis

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// AnalysisSource represents the source of analysis data
type AnalysisSource struct {
	Type             string `json:"type"` // "file", "sql_connection", "openapi_url", "custom_output"
	Path             string `json:"path,omitempty"`
	ConnectionString string `json:"connection_string,omitempty"`
	URL              string `json:"url,omitempty"`
	OutputPath       string `json:"output_path,omitempty"`
}

// VirtualPathManager handles the creation and management of virtual paths
type VirtualPathManager struct{}

// NewVirtualPathManager creates a new virtual path manager
func NewVirtualPathManager() *VirtualPathManager {
	return &VirtualPathManager{}
}

// CreateVirtualPath creates a virtual path for different types of analysis sources
func (v *VirtualPathManager) CreateVirtualPath(source AnalysisSource) string {
	switch source.Type {
	case "sql_connection":
		return v.createSQLVirtualPath(source.ConnectionString)
	case "openapi_url":
		return v.createOpenAPIVirtualPath(source.URL)
	case "custom_output":
		if source.OutputPath != "" {
			return source.OutputPath
		}
		return v.createCustomVirtualPath(source.Path)
	case "file":
		// Regular files use their actual path
		return source.Path
	default:
		// Fallback for unknown types
		return v.createGenericVirtualPath(source.Type, source.Path)
	}
}

// createSQLVirtualPath creates a virtual path for SQL database connections
func (v *VirtualPathManager) createSQLVirtualPath(connectionString string) string {
	sanitized := v.sanitizeConnectionString(connectionString)
	return fmt.Sprintf("sql://%s", sanitized)
}

// createOpenAPIVirtualPath creates a virtual path for OpenAPI URLs
func (v *VirtualPathManager) createOpenAPIVirtualPath(apiURL string) string {
	sanitized := v.sanitizeURL(apiURL)
	return fmt.Sprintf("openapi://%s", sanitized)
}

// createCustomVirtualPath creates a virtual path for custom analyzers
func (v *VirtualPathManager) createCustomVirtualPath(sourcePath string) string {
	sanitized := v.sanitizePath(sourcePath)
	return fmt.Sprintf("virtual://custom/%s", sanitized)
}

// createGenericVirtualPath creates a generic virtual path
func (v *VirtualPathManager) createGenericVirtualPath(sourceType, identifier string) string {
	sanitized := v.sanitizePath(identifier)
	return fmt.Sprintf("virtual://%s/%s", sourceType, sanitized)
}

// sanitizeConnectionString sanitizes a database connection string for use in paths
func (v *VirtualPathManager) sanitizeConnectionString(connStr string) string {
	// Parse the connection string
	dbURL, err := url.Parse(connStr)
	if err != nil {
		// If parsing fails, create a hash-based identifier
		return v.createHashIdentifier(connStr)
	}

	// Extract components
	scheme := dbURL.Scheme
	host := dbURL.Hostname()
	port := dbURL.Port()
	dbName := strings.TrimPrefix(dbURL.Path, "/")

	// Build sanitized identifier
	var parts []string
	if scheme != "" {
		parts = append(parts, scheme)
	}
	if host != "" {
		parts = append(parts, host)
	}
	if port != "" {
		parts = append(parts, port)
	}
	if dbName != "" {
		parts = append(parts, dbName)
	}

	identifier := strings.Join(parts, "_")
	return v.sanitizeIdentifier(identifier)
}

// sanitizeURL sanitizes a URL for use in paths
func (v *VirtualPathManager) sanitizeURL(apiURL string) string {
	parsedURL, err := url.Parse(apiURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		// If parsing fails or URL lacks scheme/host, treat as invalid
		return "unknown"
	}

	// Build identifier from URL components
	host := parsedURL.Hostname()
	port := parsedURL.Port()
	path := strings.Trim(parsedURL.Path, "/")

	var parts []string
	if host != "" {
		parts = append(parts, host)
	}
	if port != "" {
		parts = append(parts, port)
	}
	if path != "" {
		// Replace path separators and common API path elements
		cleanPath := strings.ReplaceAll(path, "/", "_")
		cleanPath = strings.ReplaceAll(cleanPath, ".", "_")
		parts = append(parts, cleanPath)
	}

	identifier := strings.Join(parts, "_")
	return v.sanitizeIdentifier(identifier)
}

// sanitizePath sanitizes a file path for use in virtual paths
func (v *VirtualPathManager) sanitizePath(path string) string {
	// Get base name and remove extension
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	return v.sanitizeIdentifier(name)
}

// sanitizeIdentifier ensures an identifier is safe for use in paths
func (v *VirtualPathManager) sanitizeIdentifier(identifier string) string {
	// Replace hyphens with underscores first
	sanitized := strings.ReplaceAll(identifier, "-", "_")

	// Replace invalid characters with underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	sanitized = reg.ReplaceAllString(sanitized, "_")

	// Remove consecutive underscores
	reg2 := regexp.MustCompile(`_+`)
	sanitized = reg2.ReplaceAllString(sanitized, "_")

	// Trim underscores from ends
	sanitized = strings.Trim(sanitized, "_")

	// Ensure we have a valid identifier
	if sanitized == "" {
		sanitized = "unknown"
	}

	// Limit length to prevent overly long paths
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}

	return sanitized
}

// createHashIdentifier creates a hash-based identifier for complex strings
func (v *VirtualPathManager) createHashIdentifier(input string) string {
	hash := md5.Sum([]byte(input))
	return fmt.Sprintf("hash_%x", hash[:8]) // Use first 8 bytes of hash
}

// IsVirtualPath checks if a path is a virtual path
func (v *VirtualPathManager) IsVirtualPath(path string) bool {
	return strings.HasPrefix(path, "virtual://") ||
		strings.HasPrefix(path, "sql://") ||
		strings.HasPrefix(path, "openapi://")
}

// ParseVirtualPath parses a virtual path and returns its components
func (v *VirtualPathManager) ParseVirtualPath(virtualPath string) (string, string, error) {
	if !v.IsVirtualPath(virtualPath) {
		return "", "", fmt.Errorf("not a virtual path: %s", virtualPath)
	}

	var pathType, identifier string

	if strings.HasPrefix(virtualPath, "sql://") {
		pathType = "sql"
		identifier = strings.TrimPrefix(virtualPath, "sql://")
	} else if strings.HasPrefix(virtualPath, "openapi://") {
		pathType = "openapi"
		identifier = strings.TrimPrefix(virtualPath, "openapi://")
	} else if strings.HasPrefix(virtualPath, "virtual://") {
		// Remove virtual:// prefix
		path := strings.TrimPrefix(virtualPath, "virtual://")
		// Split into type and identifier
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid virtual path format: %s", virtualPath)
		}
		pathType = parts[0]
		identifier = parts[1]
	} else {
		return "", "", fmt.Errorf("invalid virtual path format: %s", virtualPath)
	}

	if identifier == "" {
		return "", "", fmt.Errorf("empty identifier in virtual path: %s", virtualPath)
	}

	return pathType, identifier, nil
}

// GetVirtualPathType extracts the type from a virtual path
func (v *VirtualPathManager) GetVirtualPathType(virtualPath string) string {
	pathType, _, err := v.ParseVirtualPath(virtualPath)
	if err != nil {
		return ""
	}
	return pathType
}

// GetVirtualPathIdentifier extracts the identifier from a virtual path
func (v *VirtualPathManager) GetVirtualPathIdentifier(virtualPath string) string {
	_, identifier, err := v.ParseVirtualPath(virtualPath)
	if err != nil {
		return ""
	}
	return identifier
}

// CreateSourceFromVirtualPath creates an AnalysisSource from a virtual path
// This is useful for reverse operations when you have a virtual path and need the source
func (v *VirtualPathManager) CreateSourceFromVirtualPath(virtualPath string) AnalysisSource {
	pathType := v.GetVirtualPathType(virtualPath)
	identifier := v.GetVirtualPathIdentifier(virtualPath)

	switch pathType {
	case "sql":
		return AnalysisSource{
			Type:             "sql_connection",
			ConnectionString: identifier, // Note: This is the sanitized version
		}
	case "openapi":
		return AnalysisSource{
			Type: "openapi_url",
			URL:  identifier, // Note: This is the sanitized version
		}
	case "custom":
		return AnalysisSource{
			Type: "custom_output",
			Path: identifier,
		}
	default:
		return AnalysisSource{
			Type: "file",
			Path: virtualPath,
		}
	}
}

// ValidateVirtualPath validates that a virtual path is well-formed
func (v *VirtualPathManager) ValidateVirtualPath(virtualPath string) error {
	if !v.IsVirtualPath(virtualPath) {
		return fmt.Errorf("path is not a virtual path: %s", virtualPath)
	}

	pathType, identifier, err := v.ParseVirtualPath(virtualPath)
	if err != nil {
		return err
	}

	if pathType == "" {
		return fmt.Errorf("virtual path missing type: %s", virtualPath)
	}

	if identifier == "" {
		return fmt.Errorf("virtual path missing identifier: %s", virtualPath)
	}

	// Validate known types
	validTypes := map[string]bool{
		"sql":     true,
		"openapi": true,
		"custom":  true,
	}

	if !validTypes[pathType] {
		return fmt.Errorf("unknown virtual path type: %s", pathType)
	}

	return nil
}