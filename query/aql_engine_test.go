package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flanksource/arch-unit/analysis"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/arch-unit/parser"
	flanksourceContext "github.com/flanksource/commons/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestData(t *testing.T, astCache *cache.ASTCache) {
	// Create test AST nodes
	nodes := []*models.ASTNode{
		// Simple controller with low complexity
		{
			FilePath:             "/test/SimpleController.go",
			PackageName:          "controller",
			TypeName:             "SimpleController",
			MethodName:           "GetUser",
			NodeType:             models.NodeTypeMethod,
			StartLine:            10,
			EndLine:              15,
			CyclomaticComplexity: 2,
			LastModified:         time.Now(),
		},
		// Complex controller with high complexity
		{
			FilePath:             "/test/ComplexController.go",
			PackageName:          "controller",
			TypeName:             "ComplexController",
			MethodName:           "ProcessOrder",
			NodeType:             models.NodeTypeMethod,
			StartLine:            20,
			EndLine:              80,
			CyclomaticComplexity: 25,
			LastModified:         time.Now(),
		},
		// Service layer
		{
			FilePath:             "/test/UserService.go",
			PackageName:          "service",
			TypeName:             "UserService",
			MethodName:           "CreateUser",
			NodeType:             models.NodeTypeMethod,
			StartLine:            5,
			EndLine:              20,
			CyclomaticComplexity: 5,
			LastModified:         time.Now(),
		},
		// Repository layer
		{
			FilePath:             "/test/UserRepository.go",
			PackageName:          "repository",
			TypeName:             "UserRepository",
			MethodName:           "Save",
			NodeType:             models.NodeTypeMethod,
			StartLine:            8,
			EndLine:              12,
			CyclomaticComplexity: 1,
			LastModified:         time.Now(),
		},
		// Model layer
		{
			FilePath:             "/test/User.go",
			PackageName:          "model",
			TypeName:             "User",
			MethodName:           "",
			NodeType:             models.NodeTypeType,
			StartLine:            1,
			EndLine:              10,
			CyclomaticComplexity: 0,
			LastModified:         time.Now(),
		},
	}

	// Store nodes and get their IDs
	nodeIDs := make([]int64, len(nodes))
	for i, node := range nodes {
		id, err := astCache.StoreASTNode(node)
		require.NoError(t, err)
		nodeIDs[i] = id
		node.ID = id // Update the node with the ID for relationship creation
	}

	// Create relationships
	// SimpleController -> UserService
	err := astCache.StoreASTRelationship(nodeIDs[0], &nodeIDs[2], 12, models.RelationshipCall, "userService.CreateUser()")
	require.NoError(t, err)

	// ComplexController -> UserService
	err = astCache.StoreASTRelationship(nodeIDs[1], &nodeIDs[2], 45, models.RelationshipCall, "userService.CreateUser()")
	require.NoError(t, err)

	// UserService -> UserRepository
	err = astCache.StoreASTRelationship(nodeIDs[2], &nodeIDs[3], 18, models.RelationshipCall, "userRepo.Save()")
	require.NoError(t, err)

	// Store library relationships
	fmtLibID, err := astCache.StoreLibraryNode("fmt", "", "Printf", "", models.NodeTypeMethod, "go", "stdlib")
	require.NoError(t, err)

	err = astCache.StoreLibraryRelationship(nodeIDs[0], fmtLibID, 11, models.RelationshipCall, "fmt.Printf()")
	require.NoError(t, err)
}

func TestAQLEngine_LimitStatements(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	setupTestData(t, astCache)
	engine := NewAQLEngine(astCache)

	tests := []struct {
		name          string
		aql           string
		expectedCount int
		description   string
	}{
		{
			name: "Complexity greater than 10",
			aql: `RULE "High Complexity" {
				LIMIT(*.cyclomatic > 10)
			}`,
			expectedCount: 1, // Only ComplexController.ProcessOrder has complexity > 10
			description:   "Should find methods with high complexity",
		},
		{
			name: "Complexity less than 5",
			aql: `RULE "Low Complexity" {
				LIMIT(*.cyclomatic < 5)
			}`,
			expectedCount: 3, // SimpleController.GetUser(2), UserRepository.Save(1), and UserService.CreateUser should not match (5)
			description:   "Should find methods with low complexity",
		},
		{
			name: "Controller pattern with high complexity",
			aql: `RULE "Complex Controllers" {
				LIMIT(*Controller*.cyclomatic > 5)
			}`,
			expectedCount: 1, // Only ComplexController.ProcessOrder
			description:   "Should find complex controller methods",
		},
		{
			name: "Service layer complexity",
			aql: `RULE "Service Complexity" {
				LIMIT(*Service*.cyclomatic >= 5)
			}`,
			expectedCount: 1, // UserService.CreateUser has complexity 5
			description:   "Should find service methods meeting complexity threshold",
		},
		{
			name: "Parameter count check",
			aql: `RULE "Too Many Parameters" {
				LIMIT(*.params > 2)
			}`,
			expectedCount: 1, // ComplexController.ProcessOrder has 3 parameters
			description:   "Should find methods with too many parameters",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ruleSet, err := parser.ParseAQLFile(test.aql)
			require.NoError(t, err)

			violations, err := engine.ExecuteRuleSet(ruleSet)
			require.NoError(t, err)

			assert.Len(t, violations, test.expectedCount, test.description)

			// Verify violation details
			for _, violation := range violations {
				assert.NotEmpty(t, violation.File)
				assert.Greater(t, violation.Line, int64(0))
				assert.NotEmpty(t, violation.Message)
				assert.Equal(t, "aql", violation.Source)
			}
		})
	}
}

func TestAQLEngine_ForbidStatements(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	setupTestData(t, astCache)
	engine := NewAQLEngine(astCache)

	tests := []struct {
		name          string
		aql           string
		expectedCount int
		description   string
	}{
		{
			name: "Controllers should not access repositories directly",
			aql: `RULE "Layer Violation" {
				FORBID(*Controller* -> *Repository*)
			}`,
			expectedCount: 0, // No direct controller->repository relationships in test data
			description:   "Should not find direct controller to repository calls",
		},
		{
			name: "Services should not access controllers",
			aql: `RULE "Reverse Dependency" {
				FORBID(*Service* -> *Controller*)
			}`,
			expectedCount: 0, // No service->controller relationships in test data
			description:   "Should not find service to controller calls",
		},
		{
			name: "Models should not access anything",
			aql: `RULE "Model Isolation" {
				FORBID(*model* -> *)
			}`,
			expectedCount: 0, // Model layer should not have outgoing calls
			description:   "Models should be isolated from other layers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ruleSet, err := parser.ParseAQLFile(test.aql)
			require.NoError(t, err)

			violations, err := engine.ExecuteRuleSet(ruleSet)
			require.NoError(t, err)

			assert.Len(t, violations, test.expectedCount, test.description)
		})
	}
}

func TestAQLEngine_RequireStatements(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	setupTestData(t, astCache)
	engine := NewAQLEngine(astCache)

	tests := []struct {
		name          string
		aql           string
		expectedCount int
		description   string
	}{
		{
			name: "Controllers must use services",
			aql: `RULE "Controller Service Dependency" {
				REQUIRE(*Controller* -> *Service*)
			}`,
			expectedCount: 0, // Both controllers use services, so no violations
			description:   "Controllers should use services",
		},
		{
			name: "Services must use repositories",
			aql: `RULE "Service Repository Dependency" {
				REQUIRE(*Service* -> *Repository*)
			}`,
			expectedCount: 0, // UserService uses UserRepository, so no violations
			description:   "Services should use repositories",
		},
		{
			name: "Controllers must use models (this should create violations)",
			aql: `RULE "Controller Model Dependency" {
				REQUIRE(*Controller* -> *model*)
			}`,
			expectedCount: 2, // Neither controller directly uses models
			description:   "Controllers should use models (missing dependency)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ruleSet, err := parser.ParseAQLFile(test.aql)
			require.NoError(t, err)

			violations, err := engine.ExecuteRuleSet(ruleSet)
			require.NoError(t, err)

			assert.Len(t, violations, test.expectedCount, test.description)

			// Verify violation structure for missing dependencies
			for _, violation := range violations {
				assert.NotEmpty(t, violation.File)
				assert.Greater(t, violation.Line, int64(0))
				assert.Contains(t, violation.Message, "missing required dependency")
				assert.Equal(t, "aql", violation.Source)
			}
		})
	}
}

func TestAQLEngine_ComplexRules(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	setupTestData(t, astCache)
	engine := NewAQLEngine(astCache)

	aql := `RULE "Architecture Compliance" {
		LIMIT(*Controller*.cyclomatic > 15)
		FORBID(*Controller* -> *Repository*)
		REQUIRE(*Controller* -> *Service*)
		REQUIRE(*Service* -> *Repository*)
	}`

	ruleSet, err := parser.ParseAQLFile(aql)
	require.NoError(t, err)

	violations, err := engine.ExecuteRuleSet(ruleSet)
	require.NoError(t, err)

	// Should have violations for:
	// 1. ComplexController.ProcessOrder exceeds complexity limit
	// Total expected: 1 violation
	assert.Greater(t, len(violations), 0)

	// Verify we have the complexity violation
	hasComplexityViolation := false
	for _, violation := range violations {
		if strings.Contains(violation.Message, "violated limit") {
			hasComplexityViolation = true
		}
	}
	assert.True(t, hasComplexityViolation, "Should have complexity violation")
}

func TestAQLEngine_PatternMatching(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	setupTestData(t, astCache)
	engine := NewAQLEngine(astCache)

	tests := []struct {
		pattern     string
		expectedMin int
		description string
	}{
		{"*", 4, "Wildcard should match all methods"},
		{"*Controller*", 2, "Should match controller methods"},
		{"*Service*", 1, "Should match service methods"},
		{"*Repository*", 1, "Should match repository methods"},
		{"controller.*", 2, "Should match methods in controller package"},
		{"*.UserService", 1, "Should match UserService type"},
		{"*.UserService:CreateUser", 1, "Should match specific method"},
	}

	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			aql := `RULE "Pattern Test" {
				LIMIT(` + test.pattern + `.cyclomatic >= 0)
			}`

			ruleSet, err := parser.ParseAQLFile(aql)
			require.NoError(t, err)

			violations, err := engine.ExecuteRuleSet(ruleSet)
			require.NoError(t, err)

			assert.GreaterOrEqual(t, len(violations), test.expectedMin, test.description)
		})
	}
}

func TestAQLEngine_ErrorHandling(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	engine := NewAQLEngine(astCache)

	// Test with empty rule set
	emptyRuleSet := &models.AQLRuleSet{Rules: []*models.AQLRule{}}
	violations, err := engine.ExecuteRuleSet(emptyRuleSet)
	require.NoError(t, err)
	assert.Len(t, violations, 0)

	// Test with nil rule set
	violations, err = engine.ExecuteRuleSet(nil)
	assert.Error(t, err)
	assert.Nil(t, violations)
}

func TestAQLEngine_WithRealGoCode(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Create real Go files for testing
	tmpDir := t.TempDir()

	// Controller file
	controllerFile := filepath.Join(tmpDir, "controller.go")
	controllerContent := `package controller

import "fmt"

type UserController struct{}

func (c *UserController) GetUser(id string) (*User, error) {
	if id == "" {
		return nil, fmt.Errorf("invalid id")
	}
	// Simple logic - complexity should be 2
	return &User{ID: id}, nil
}

func (c *UserController) ComplexMethod(a, b, c int) int {
	result := 0
	for i := 0; i < a; i++ {
		if i%2 == 0 {
			switch b {
			case 1:
				result += i
			case 2:
				result += i * 2
			default:
				result += i * c
			}
		} else {
			result += i
		}
	}
	// This should have high complexity
	return result
}`

	require.NoError(t, os.WriteFile(controllerFile, []byte(controllerContent), 0644))

	// Extract AST using real extractor
	extractor := analysis.NewGoASTExtractor(astCache)
	err = extractor.ExtractFile(flanksourceContext.NewContext(context.Background()), controllerFile)
	require.NoError(t, err)

	// Test AQL rules on real extracted data
	engine := NewAQLEngine(astCache)

	aql := `RULE "Real Code Test" {
		LIMIT(*Controller*.cyclomatic > 5)
	}`

	ruleSet, err := parser.ParseAQLFile(aql)
	require.NoError(t, err)

	violations, err := engine.ExecuteRuleSet(ruleSet)
	require.NoError(t, err)

	// Should find the ComplexMethod as a violation
	assert.Greater(t, len(violations), 0)

	// Verify violation details
	found := false
	for _, violation := range violations {
		if strings.Contains(violation.Message, "ComplexMethod") || strings.Contains(violation.File, controllerFile) {
			found = true
			assert.Greater(t, violation.Line, int64(0))
			assert.Equal(t, "aql", violation.Source)
		}
	}
	assert.True(t, found, "Should find ComplexMethod violation")
}

func TestAQLEngine_Performance(t *testing.T) {
	astCache, err := cache.NewASTCache()
	require.NoError(t, err)
	defer astCache.Close()

	// Create many test nodes for performance testing
	nodeCount := 1000
	for i := 0; i < nodeCount; i++ {
		node := &models.ASTNode{
			FilePath:             fmt.Sprintf("/test/file%d.go", i),
			PackageName:          fmt.Sprintf("pkg%d", i%10),
			TypeName:             fmt.Sprintf("Type%d", i),
			MethodName:           fmt.Sprintf("Method%d", i),
			NodeType:             models.NodeTypeMethod,
			StartLine:            i % 100,
			EndLine:              i%100 + 10,
			CyclomaticComplexity: i % 20, // 0-19 complexity
			LastModified:         time.Now(),
		}
		_, err := astCache.StoreASTNode(node)
		require.NoError(t, err)
	}

	engine := NewAQLEngine(astCache)

	// Test complex rule performance
	aql := `RULE "Performance Test" {
		LIMIT(*.cyclomatic > 15)
		FORBID(Type1* -> Type2*)
		REQUIRE(Type3* -> Type4*)
	}`

	start := time.Now()
	ruleSet, err := parser.ParseAQLFile(aql)
	require.NoError(t, err)

	violations, err := engine.ExecuteRuleSet(ruleSet)
	require.NoError(t, err)
	elapsed := time.Since(start)

	t.Logf("Processed %d nodes in %v, found %d violations", nodeCount, elapsed, len(violations))

	// Performance should be reasonable (less than 5 seconds for 1000 nodes)
	assert.Less(t, elapsed, 5*time.Second, "Query should complete within 5 seconds")
	assert.Greater(t, len(violations), 0, "Should find some violations")
}
