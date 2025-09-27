package query

import (
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

// AQLEngine executes AQL queries against the AST database
type AQLEngine struct {
	cache *cache.ASTCache
}

// NewAQLEngine creates a new AQL engine
func NewAQLEngine(astCache *cache.ASTCache) *AQLEngine {
	return &AQLEngine{
		cache: astCache,
	}
}

// ExecuteRuleSet executes a set of AQL rules and returns violations
func (e *AQLEngine) ExecuteRuleSet(ruleSet *models.AQLRuleSet) ([]*models.Violation, error) {
	if ruleSet == nil {
		return nil, fmt.Errorf("ruleSet cannot be nil")
	}

	if ruleSet.Rules == nil {
		return nil, fmt.Errorf("ruleSet.Rules cannot be nil")
	}

	var allViolations []*models.Violation

	for _, rule := range ruleSet.Rules {
		violations, err := e.ExecuteRule(rule)
		if err != nil {
			return nil, fmt.Errorf("failed to execute rule %s: %w", rule.Name, err)
		}
		allViolations = append(allViolations, violations...)
	}

	return allViolations, nil
}

// ExecuteRule executes a single AQL rule and returns violations
func (e *AQLEngine) ExecuteRule(rule *models.AQLRule) ([]*models.Violation, error) {
	var violations []*models.Violation

	for _, stmt := range rule.Statements {
		stmtViolations, err := e.executeStatement(rule, stmt)
		if err != nil {
			return nil, fmt.Errorf("failed to execute statement in rule %s: %w", rule.Name, err)
		}
		violations = append(violations, stmtViolations...)
	}

	return violations, nil
}

// executeStatement executes a single AQL statement
func (e *AQLEngine) executeStatement(rule *models.AQLRule, stmt *models.AQLStatement) ([]*models.Violation, error) {
	switch stmt.Type {
	case models.AQLStatementLimit:
		return e.executeLimitStatement(rule, stmt)
	case models.AQLStatementForbid:
		return e.executeForbidStatement(rule, stmt)
	case models.AQLStatementRequire:
		return e.executeRequireStatement(rule, stmt)
	case models.AQLStatementAllow:
		// ALLOW statements don't generate violations directly
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown statement type: %s", stmt.Type)
	}
}

// executeLimitStatement executes a LIMIT statement
func (e *AQLEngine) executeLimitStatement(rule *models.AQLRule, stmt *models.AQLStatement) ([]*models.Violation, error) {
	if stmt.Condition == nil {
		return nil, fmt.Errorf("LIMIT statement missing condition")
	}

	// Get all AST nodes that match the pattern
	nodes, err := e.findMatchingNodes(stmt.Condition.Pattern)
	if err != nil {
		return nil, err
	}

	var violations []*models.Violation
	for _, node := range nodes {
		// Evaluate condition against the node
		violated, err := stmt.Condition.Evaluate(node)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate condition: %w", err)
		}

		if violated {
			callerNode := &models.ASTNode{
				FilePath:    node.FilePath,
				PackageName: node.PackageName,
				StartLine:   node.StartLine,
				NodeType:    models.NodeTypePackage,
			}
			calledNode := &models.ASTNode{
				FilePath:    node.FilePath,
				PackageName: node.GetFullName(),
				StartLine:   node.StartLine,
				NodeType:    models.NodeTypeMethod,
			}
			violation := &models.Violation{
				File:    node.FilePath,
				Line:    node.StartLine,
				Caller:  callerNode,
				Called:  calledNode,
				Message: fmt.Sprintf("Rule '%s': %s violated limit", rule.Name, node.GetFullName()),
				Source:  "aql",
			}
			violations = append(violations, violation)
		}
	}

	return violations, nil
}

// executeForbidStatement executes a FORBID statement
func (e *AQLEngine) executeForbidStatement(rule *models.AQLRule, stmt *models.AQLStatement) ([]*models.Violation, error) {
	if stmt.FromPattern != nil && stmt.ToPattern != nil {
		// Relationship pattern: FORBID(A -> B)
		return e.executeForbidRelationship(rule, stmt.FromPattern, stmt.ToPattern)
	} else if stmt.Pattern != nil {
		// Single pattern: FORBID(A)
		return e.executeForbidPattern(rule, stmt.Pattern)
	}

	return nil, fmt.Errorf("FORBID statement missing pattern")
}

// executeForbidRelationship executes a FORBID relationship statement
func (e *AQLEngine) executeForbidRelationship(rule *models.AQLRule, fromPattern, toPattern *models.AQLPattern) ([]*models.Violation, error) {
	// Find all nodes matching the from pattern
	fromNodes, err := e.findMatchingNodes(fromPattern)
	if err != nil {
		return nil, err
	}

	var violations []*models.Violation

	for _, fromNode := range fromNodes {
		// Get relationships from this node
		relationships, err := e.cache.GetASTRelationships(fromNode.ID, models.RelationshipCall)
		if err != nil {
			return nil, err
		}

		for _, rel := range relationships {
			if rel.ToASTID == nil {
				// External call - check library relationships
				// libRels, err := e.cache.GetLibraryRelationships(fromNode.ID, models.RelationshipCall)
				// if err != nil {
				// 	continue
				// }

				// for _, libRel := range libRels {
				// 	if e.matchesLibraryPattern(libRel.LibraryNode, toPattern) {
				// 		violation := &models.Violation{
				// 			File:          fromNode.FilePath,
				// 			Line:          libRel.LineNo,
				// 			Caller: fromNode.PackageName,
				// 			Called:  fromNode.GetFullName(),
				// 			CalledPackage: libRel.LibraryNode.Package,
				// 			CalledMethod:  libRel.LibraryNode.GetFullName(),
				// 			Message:       fmt.Sprintf("Rule '%s': Forbidden call from %s to %s", rule.Name, fromNode.GetFullName(), libRel.LibraryNode.GetFullName()),
				// 			Source:        "aql",
				// 		}
				// 		violations = append(violations, violation)
				// 	}
				// }
			} else {
				// Internal call
				toNode, err := e.cache.GetASTNode(*rel.ToASTID)
				if err != nil {
					continue
				}

				if toPattern.Matches(toNode) {
					callerNode := &models.ASTNode{
						FilePath:    fromNode.FilePath,
						PackageName: fromNode.PackageName,
						StartLine:   rel.LineNo,
						NodeType:    models.NodeTypeMethod,
					}
					calledNode := &models.ASTNode{
						FilePath:    toNode.FilePath,
						PackageName: toNode.PackageName,
						StartLine:   rel.LineNo,
						NodeType:    models.NodeTypeMethod,
					}
					violation := &models.Violation{
						File:    fromNode.FilePath,
						Line:    rel.LineNo,
						Caller:  callerNode,
						Called:  calledNode,
						Message: fmt.Sprintf("Rule '%s': Forbidden call from %s to %s", rule.Name, fromNode.GetFullName(), toNode.GetFullName()),
						Source:  "aql",
					}
					violations = append(violations, violation)
				}
			}
		}
	}

	return violations, nil
}

// executeForbidPattern executes a FORBID pattern statement
func (e *AQLEngine) executeForbidPattern(rule *models.AQLRule, pattern *models.AQLPattern) ([]*models.Violation, error) {
	// Find all nodes that match the forbidden pattern
	nodes, err := e.findMatchingNodes(pattern)
	if err != nil {
		return nil, err
	}

	var violations []*models.Violation
	for _, node := range nodes {
		callerNode := &models.ASTNode{
			FilePath:    node.FilePath,
			PackageName: node.PackageName,
			StartLine:   node.StartLine,
			NodeType:    models.NodeTypePackage,
		}
		calledNode := &models.ASTNode{
			FilePath:    node.FilePath,
			PackageName: node.GetFullName(),
			StartLine:   node.StartLine,
			NodeType:    models.NodeTypeMethod,
		}
		violation := &models.Violation{
			File:    node.FilePath,
			Line:    node.StartLine,
			Caller:  callerNode,
			Called:  calledNode,
			Message: fmt.Sprintf("Rule '%s': Forbidden pattern %s found in %s", rule.Name, pattern.String(), node.GetFullName()),
			Source:  "aql",
		}
		violations = append(violations, violation)
	}

	return violations, nil
}

// executeRequireStatement executes a REQUIRE statement
func (e *AQLEngine) executeRequireStatement(rule *models.AQLRule, stmt *models.AQLStatement) ([]*models.Violation, error) {
	if stmt.FromPattern != nil && stmt.ToPattern != nil {
		// Relationship pattern: REQUIRE(A -> B)
		return e.executeRequireRelationship(rule, stmt.FromPattern, stmt.ToPattern)
	} else if stmt.Pattern != nil {
		// Single pattern: REQUIRE(A)
		return e.executeRequirePattern(rule, stmt.Pattern)
	}

	return nil, fmt.Errorf("REQUIRE statement missing pattern")
}

// executeRequireRelationship executes a REQUIRE relationship statement
func (e *AQLEngine) executeRequireRelationship(rule *models.AQLRule, fromPattern, toPattern *models.AQLPattern) ([]*models.Violation, error) {
	// Find all nodes matching the from pattern
	fromNodes, err := e.findMatchingNodes(fromPattern)
	if err != nil {
		return nil, err
	}

	var violations []*models.Violation

	for _, fromNode := range fromNodes {
		// Check if this node has any relationship to nodes matching toPattern
		hasRequiredRelationship := false

		// Check internal relationships
		relationships, err := e.cache.GetASTRelationships(fromNode.ID, models.RelationshipCall)
		if err != nil {
			return nil, err
		}

		for _, rel := range relationships {
			if rel.ToASTID != nil {
				toNode, err := e.cache.GetASTNode(*rel.ToASTID)
				if err != nil {
					continue
				}

				if toPattern.Matches(toNode) {
					hasRequiredRelationship = true
					break
				}
			}
		}

		// // Check library relationships if no internal relationship found
		// if !hasRequiredRelationship {
		// 	libRels, err := e.cache.GetLibraryRelationships(fromNode.ID, models.RelationshipCall)
		// 	if err != nil {
		// 		return nil, err
		// 	}

		// 	// for _, libRel := range libRels {
		// 	// 	if e.matchesLibraryPattern(libRel.LibraryNode, toPattern) {
		// 	// 		hasRequiredRelationship = true
		// 	// 		break
		// 	// 	}
		// 	// }
		// }

		if !hasRequiredRelationship {
			callerNode := &models.ASTNode{
				FilePath:    fromNode.FilePath,
				PackageName: fromNode.PackageName,
				StartLine:   fromNode.StartLine,
				NodeType:    models.NodeTypePackage,
			}
			calledNode := &models.ASTNode{
				FilePath:    fromNode.FilePath,
				PackageName: fromNode.GetFullName(),
				StartLine:   fromNode.StartLine,
				NodeType:    models.NodeTypeMethod,
			}
			violation := &models.Violation{
				File:    fromNode.FilePath,
				Line:    fromNode.StartLine,
				Caller:  callerNode,
				Called:  calledNode,
				Message: fmt.Sprintf("Rule '%s': Required relationship from %s to %s not found", rule.Name, fromNode.GetFullName(), toPattern.String()),
				Source:  "aql",
			}
			violations = append(violations, violation)
		}
	}

	return violations, nil
}

// executeRequirePattern executes a REQUIRE pattern statement
func (e *AQLEngine) executeRequirePattern(rule *models.AQLRule, pattern *models.AQLPattern) ([]*models.Violation, error) {
	// Find all nodes that match the required pattern
	nodes, err := e.findMatchingNodes(pattern)
	if err != nil {
		return nil, err
	}

	// If no nodes match the required pattern, it's a violation
	if len(nodes) == 0 {
		violation := &models.Violation{
			File:    "",
			Line:    0,
			Message: fmt.Sprintf("Rule '%s': Required pattern %s not found in codebase", rule.Name, pattern.String()),
			Source:  "aql",
		}
		return []*models.Violation{violation}, nil
	}

	return nil, nil
}

// findMatchingNodes finds AST nodes that match a pattern
func (e *AQLEngine) findMatchingNodes(pattern *models.AQLPattern) ([]*models.ASTNode, error) {
	// Build a query based on the pattern
	query := "SELECT id, file_path, package_name, type_name, method_name, field_name, node_type, start_line, end_line, cyclomatic_complexity, parameter_count, return_count, line_count, last_modified FROM ast_nodes WHERE 1=1"
	args := []interface{}{}

	if pattern.Package != "" && pattern.Package != "*" {
		if strings.Contains(pattern.Package, "*") {
			query += " AND package_name LIKE ?"
			args = append(args, strings.ReplaceAll(pattern.Package, "*", "%"))
		} else {
			query += " AND package_name = ?"
			args = append(args, pattern.Package)
		}
	}

	if pattern.Type != "" && pattern.Type != "*" {
		if strings.Contains(pattern.Type, "*") {
			query += " AND type_name LIKE ?"
			args = append(args, strings.ReplaceAll(pattern.Type, "*", "%"))
		} else {
			query += " AND type_name = ?"
			args = append(args, pattern.Type)
		}
	}

	if pattern.Method != "" && pattern.Method != "*" {
		if strings.Contains(pattern.Method, "*") {
			query += " AND method_name LIKE ?"
			args = append(args, strings.ReplaceAll(pattern.Method, "*", "%"))
		} else {
			query += " AND method_name = ?"
			args = append(args, pattern.Method)
		}
	}

	if pattern.Field != "" && pattern.Field != "*" {
		if strings.Contains(pattern.Field, "*") {
			query += " AND field_name LIKE ?"
			args = append(args, strings.ReplaceAll(pattern.Field, "*", "%"))
		} else {
			query += " AND field_name = ?"
			args = append(args, pattern.Field)
		}
	}

	rows, err := e.cache.QueryRaw(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query AST nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var allNodes []*models.ASTNode
	for rows.Next() {
		var node models.ASTNode
		err := rows.Scan(&node.ID, &node.FilePath, &node.PackageName, &node.TypeName,
			&node.MethodName, &node.FieldName, &node.NodeType, &node.StartLine,
			&node.EndLine, &node.CyclomaticComplexity, &node.ParameterCount,
			&node.ReturnCount, &node.LineCount, &node.LastModified)
		if err != nil {
			return nil, err
		}
		allNodes = append(allNodes, &node)
	}

	// Filter by file path pattern if specified
	if pattern.FilePath != "" && pattern.FilePath != "*" {
		var filteredNodes []*models.ASTNode
		for _, node := range allNodes {
			if pattern.Matches(node) {
				filteredNodes = append(filteredNodes, node)
			}
		}
		return filteredNodes, nil
	}

	return allNodes, nil
}
