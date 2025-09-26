package dependencies

import (
	"github.com/flanksource/arch-unit/models"
)

// DependencyScanner is the interface for language-specific dependency scanners
type DependencyScanner interface {
	// ScanFile scans a dependency file and returns dependencies
	ScanFile(ctx *models.ScanContext, filepath string, content []byte) ([]*models.Dependency, error)

	// SupportedFiles returns patterns for files this scanner can process
	// e.g., ["go.mod", "go.sum"] for Go, ["package.json", "package-lock.json"] for Node
	SupportedFiles() []string

	// Language returns the language/ecosystem this scanner supports
	Language() string
}

// BaseDependencyScanner provides common functionality for dependency scanners
type BaseDependencyScanner struct {
	language       string
	supportedFiles []string
}

// NewBaseDependencyScanner creates a new base dependency scanner
func NewBaseDependencyScanner(language string, supportedFiles []string) *BaseDependencyScanner {
	return &BaseDependencyScanner{
		language:       language,
		supportedFiles: supportedFiles,
	}
}

// Language returns the language/ecosystem this scanner supports
func (s *BaseDependencyScanner) Language() string {
	return s.language
}

// SupportedFiles returns patterns for files this scanner can process
func (s *BaseDependencyScanner) SupportedFiles() []string {
	return s.supportedFiles
}
