package aql

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/linters"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	"github.com/flanksource/arch-unit/query"
	"github.com/flanksource/clicky"
	commonsContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
)

// AQL represents the AQL linter
type AQL struct {
	linters.RunOptions
	astCache  *cache.ASTCache
	extractor *analysis.GoASTExtractor
	resolver  *analysis.LibraryResolver
	config    *models.Config // Store the main config to access AQL rules
}

// NewAQL creates a new AQL linter
func NewAQL(workingDir string) *AQL {
	return &AQL{
		RunOptions: linters.RunOptions{
			WorkDir: workingDir,
		},
	}
}

// SetOptions sets the run options for the linter
func (a *AQL) SetOptions(opts linters.RunOptions) {
	a.RunOptions = opts
}

// NewAQLWithConfig creates a new AQL linter with configuration
func NewAQLWithConfig(workingDir string, config *models.Config) *AQL {
	return &AQL{
		RunOptions: linters.RunOptions{
			WorkDir: workingDir,
		},
		config: config,
	}
}

// Name returns the linter name
func (a *AQL) Name() string {
	return "aql"
}

// DefaultIncludes returns default file patterns this linter should process
func (a *AQL) DefaultIncludes() []string {
	return []string{"**/*.go"}
}

// DefaultExcludes returns patterns this linter should ignore by default
func (a *AQL) DefaultExcludes() []string {
	return []string{
		"vendor/**",
		"*_test.go", // Skip test files by default for AQL analysis
	}
}

// SupportsJSON returns false as AQL doesn't use JSON output format
func (a *AQL) SupportsJSON() bool {
	return false
}

// JSONArgs returns empty as AQL doesn't use JSON output
func (a *AQL) JSONArgs() []string {
	return []string{}
}

// SupportsFix returns false as AQL doesn't support automatic fixes
func (a *AQL) SupportsFix() bool {
	return false
}

// FixArgs returns empty as AQL doesn't support fixes
func (a *AQL) FixArgs() []string {
	return []string{}
}

// ValidateConfig validates AQL linter configuration
func (a *AQL) ValidateConfig(config *models.LinterConfig) error {
	// AQL configuration is validated through the main configuration
	return nil
}

// Run executes AQL linting using the modern interface
func (a *AQL) Run(ctx commonsContext.Context, task *clicky.Task) ([]models.Violation, error) {
	// Initialize AST cache if not already done
	if a.astCache == nil {
		a.astCache = cache.MustGetASTCache()

	}

	// Initialize components
	if a.extractor == nil {
		a.extractor = analysis.NewGoASTExtractor(a.astCache)
	}
	if a.resolver == nil {
		a.resolver = analysis.NewLibraryResolver(a.astCache)
		// Pre-populate library nodes
		if err := a.resolver.StoreLibraryNodes(); err != nil {
			logger.Warnf("Failed to store library nodes: %v", err)
		}
	}

	// Analyze Go files if any are provided
	goFiles := filterGoFiles(a.Files)
	if len(goFiles) > 0 {
		for _, file := range goFiles {
			if err := a.extractor.ExtractFile(ctx, file); err != nil {
				logger.Warnf("Failed to extract AST from %s: %v", file, err)
			}
		}
	}

	// Get AQL rules from config
	var aqlRuleConfigs []models.AQLRuleConfig
	if a.config != nil && len(a.config.AQLRules) > 0 {
		// Use AQL rules from main configuration
		aqlRuleConfigs = a.config.AQLRules
	} else if a.ArchConfig != nil && len(a.ArchConfig.AQLRules) > 0 {
		// Use AQL rules from arch config in run options
		aqlRuleConfigs = a.ArchConfig.AQLRules
	}

	if len(aqlRuleConfigs) == 0 {
		return []models.Violation{}, nil
	}

	// Parse and execute AQL rules
	var allViolations []models.Violation

	for _, ruleConfig := range aqlRuleConfigs {
		// Skip disabled rules
		if !ruleConfig.Enabled {
			continue
		}

		var ruleText string
		var sourceFile string

		if ruleConfig.File != "" {
			// Load from file
			sourceFile = ruleConfig.File
			if !filepath.IsAbs(sourceFile) {
				sourceFile = filepath.Join(a.WorkDir, sourceFile)
			}

			content, err := os.ReadFile(sourceFile)
			if err != nil {
				logger.Warnf("Failed to read AQL rule file %s: %v", sourceFile, err)
				continue
			}
			ruleText = string(content)
		} else if ruleConfig.Inline != "" {
			// Use inline rule
			ruleText = ruleConfig.Inline
			sourceFile = "inline"
		}

		if ruleText == "" {
			continue
		}

		// Parse AQL rules
		ruleSet, err := parser.ParseAQLFile(ruleText)
		if err != nil {
			violation := models.Violation{
				File:    sourceFile,
				Line:    1,
				Message: fmt.Sprintf("AQL parsing error: %v", err),
				Source:  "aql",
			}
			allViolations = append(allViolations, violation)
			continue
		}

		// Execute AQL rules
		engine := query.NewAQLEngine(a.astCache)
		violations, err := engine.ExecuteRuleSet(ruleSet)
		if err != nil {
			violation := models.Violation{
				File:    sourceFile,
				Line:    1,
				Message: fmt.Sprintf("AQL execution error: %v", err),
				Source:  "aql",
			}
			allViolations = append(allViolations, violation)
			continue
		}

		// Add source file information to violations
		for _, v := range violations {
			if v.Source == "" {
				v.Source = "aql"
			}
			allViolations = append(allViolations, *v)
		}
	}

	return allViolations, nil
}

// Close cleans up resources
func (a *AQL) Close() error {
	if a.astCache != nil {
		return a.astCache.Close()
	}
	return nil
}

// filterGoFiles returns only Go files from the provided list
func filterGoFiles(filePaths []string) []string {
	var goFiles []string
	for _, file := range filePaths {
		if filepath.Ext(file) == ".go" {
			goFiles = append(goFiles, file)
		}
	}
	return goFiles
}
