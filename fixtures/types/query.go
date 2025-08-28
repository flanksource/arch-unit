package types

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/flanksource/arch-unit/ast"
	"github.com/flanksource/arch-unit/fixtures"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/commons/logger"
)

// QueryFixture implements FixtureType for AST query tests
type QueryFixture struct{}

// ensure QueryFixture implements FixtureType
var _ fixtures.FixtureType = (*QueryFixture)(nil)

// Name returns the type identifier
func (q *QueryFixture) Name() string {
	return "query"
}

// Run executes the AST query test
func (q *QueryFixture) Run(ctx context.Context, fixture fixtures.FixtureTest, opts fixtures.RunOptions) fixtures.FixtureResult {
	start := time.Now()
	result := fixtures.FixtureResult{
		Test:     fixture,
		Name:     fixture.Name,
		Type:     "query",
		Metadata: make(map[string]interface{}),
	}

	// Determine the base directory for working directory resolution
	// Prefer fixture.SourceDir (directory containing fixture file) over opts.WorkDir
	baseDir := opts.WorkDir
	if fixture.SourceDir != "" {
		baseDir = fixture.SourceDir
	}

	// Use the base directory as default working directory
	// If fixture.CWD is specified, resolve it relative to base directory
	testWorkDir := baseDir
	if fixture.CWD != "" && fixture.CWD != "." {
		if filepath.IsAbs(fixture.CWD) {
			// If CWD is absolute, use it directly
			testWorkDir = fixture.CWD
		} else {
			// If CWD is relative, resolve it from the base directory (fixture file location)
			testWorkDir = filepath.Join(baseDir, fixture.CWD)
		}
	}

	analyzer := ast.NewAnalyzer(cache.MustGetASTCache(), testWorkDir)

	// Analyze files in the directory
	if opts.Verbose {
		logger.Debugf("Analyzing files in %s", testWorkDir)
	}

	if err := analyzer.AnalyzeFiles(); err != nil {
		result.Status = "FAIL"
		result.Error = fmt.Sprintf("failed to analyze files: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// Execute the query
	nodes, err := analyzer.ExecuteAQLQuery(fixture.Query)
	if err != nil {
		if fixture.Expected.Error != "" {
			// Expected error case
			if strings.Contains(err.Error(), fixture.Expected.Error) {
				result.Status = "PASS"
				result.Metadata["details"] = fmt.Sprintf("Got expected error: %v", err)
				result.Duration = time.Since(start)
				return result
			}
		}
		result.Status = "FAIL"
		result.Error = fmt.Sprintf("query failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// Store actual count
	actualCount := len(nodes)
	result.Actual = actualCount
	result.Metadata["node_count"] = actualCount

	if opts.Verbose {
		logger.Debugf("Query returned %d nodes", actualCount)
	}

	// Check expected count
	if fixture.Expected.Count != nil {
		result.Expected = *fixture.Expected.Count
		if actualCount != *fixture.Expected.Count {
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("expected %d nodes, got %d", *fixture.Expected.Count, actualCount)
			result.Duration = time.Since(start)
			return result
		}
	}

	// Evaluate CEL expression if provided
	if fixture.CEL != "" && fixture.CEL != "true" && opts.Evaluator != nil {
		valid, err := opts.Evaluator.EvaluateNodes(fixture.CEL, nodes)
		if err != nil {
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("CEL evaluation failed: %v", err)
			result.Duration = time.Since(start)
			return result
		}
		result.CELResult = valid
		if !valid {
			result.Status = "FAIL"
			result.Error = fmt.Sprintf("CEL validation failed: %s", fixture.CEL)
			result.Duration = time.Since(start)
			return result
		}
	}

	result.Status = "PASS"
	result.Duration = time.Since(start)
	return result
}

// ValidateFixture validates that the fixture has required fields
func (q *QueryFixture) ValidateFixture(fixture fixtures.FixtureTest) error {
	if fixture.Query == "" {
		return fmt.Errorf("query fixture requires 'Query' field")
	}
	return nil
}

// GetRequiredFields returns required fields
func (q *QueryFixture) GetRequiredFields() []string {
	return []string{"Query"}
}

// GetOptionalFields returns optional fields
func (q *QueryFixture) GetOptionalFields() []string {
	return []string{"CWD", "CEL", "Expected.Count", "Expected.Error"}
}

func init() {
	// Register the query fixture type
	fixtures.Register(&QueryFixture{})
}
