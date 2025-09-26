package analysis

import (
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/analysis/types"
	"github.com/flanksource/arch-unit/models"
)

// RuleViolationAnalyzer checks AST data against architecture rules
type RuleViolationAnalyzer struct{}

// NewRuleViolationAnalyzer creates a new rule violation analyzer
func NewRuleViolationAnalyzer() *RuleViolationAnalyzer {
	return &RuleViolationAnalyzer{}
}

// AnalyzeViolations checks AST results against rules and returns violations
func (r *RuleViolationAnalyzer) AnalyzeViolations(astResult *types.ASTResult, ruleSets []models.RuleSet) ([]models.Violation, error) {
	if astResult == nil || len(astResult.Nodes) == 0 {
		return nil, nil
	}

	var violations []models.Violation

	// Get rules for this file from all rule sets
	rules := r.getRulesForFile(astResult.FilePath, ruleSets)
	if rules == nil || len(rules.Rules) == 0 {
		return nil, nil // No rules apply to this file
	}

	// Check each relationship for violations
	for _, relationship := range astResult.Relationships {
		if relationship.RelationshipType == models.RelationshipCall {
			if violation := r.checkCallRelationship(astResult, relationship, rules); violation != nil {
				violations = append(violations, *violation)
			}
		}
	}

	// Also check library relationships (external package calls like fmt.Println)
	for _, libRel := range astResult.Libraries {
		if libRel.RelationshipType == models.RelationshipCall {
			if violation := r.checkLibraryCallRelationship(astResult, libRel, rules); violation != nil {
				violations = append(violations, *violation)
			}
		}
	}

	return violations, nil
}

// getRulesForFile combines rules from all rule sets that apply to the given file
func (r *RuleViolationAnalyzer) getRulesForFile(filePath string, ruleSets []models.RuleSet) *models.RuleSet {
	if len(ruleSets) == 0 {
		return nil
	}

	// For now, just return the first rule set
	// TODO: Implement proper rule merging and file-specific overrides
	return &ruleSets[0]
}

// checkCallRelationship checks if a call relationship violates any rules
func (r *RuleViolationAnalyzer) checkCallRelationship(astResult *types.ASTResult, relationship *models.ASTRelationship, rules *models.RuleSet) *models.Violation {
	// Find the caller and called nodes
	var callerNode, calledNode *models.ASTNode

	for _, node := range astResult.Nodes {
		if node.ID == relationship.FromASTID {
			callerNode = node
		}
		if relationship.ToASTID != nil && node.ID == *relationship.ToASTID {
			calledNode = node
		}
	}

	if callerNode == nil {
		return nil // Cannot check without caller information
	}

	// Extract package and method information for the call
	var pkgName, methodName string

	if calledNode != nil {
		// We have full AST information
		pkgName = calledNode.PackageName
		if calledNode.NodeType == models.NodeTypeMethod {
			if calledNode.TypeName != "" {
				methodName = calledNode.TypeName + "." + calledNode.MethodName
			} else {
				methodName = calledNode.MethodName
			}
		} else if calledNode.NodeType == models.NodeTypeField {
			if calledNode.TypeName != "" {
				methodName = calledNode.TypeName + "." + calledNode.FieldName
			} else {
				methodName = calledNode.FieldName
			}
		}
	} else {
		// Fall back to parsing from relationship text
		// This handles cases where the called function is external
		parts := strings.Split(relationship.Text, ".")
		if len(parts) >= 2 {
			pkgName = strings.Join(parts[:len(parts)-1], ".")
			methodName = parts[len(parts)-1]
		} else if len(parts) == 1 {
			pkgName = astResult.PackageName
			methodName = parts[0]
		}
	}

	if pkgName == "" || methodName == "" {
		return nil // Cannot check without package/method information
	}

	// Check if the call is allowed by the rules
	allowed, rule := rules.IsAllowedForFile(pkgName, methodName, astResult.FilePath)
	if !allowed {
		violationMsg := fmt.Sprintf("Call to %s.%s violates architecture rule", pkgName, methodName)
		if rule.FilePattern != "" {
			violationMsg = fmt.Sprintf("Call to %s.%s violates file-specific rule [%s]", pkgName, methodName, rule.FilePattern)
		}

		violation := &models.Violation{
			File:     astResult.FilePath,
			Line:     relationship.LineNo,
			Column:   0, // TODO: Extract column information if available
			CallerID: &relationship.FromASTID,
			Called:   calledNode, // Will be nil for external calls
			Rule:     rule,
			Message:  violationMsg,
			Source:   "arch-unit",
		}

		// Set CalledID if we have the called node
		if calledNode != nil {
			violation.CalledID = relationship.ToASTID
		}

		return violation
	}

	return nil
}

// checkLibraryCallRelationship checks if a library call relationship violates any rules
func (r *RuleViolationAnalyzer) checkLibraryCallRelationship(astResult *types.ASTResult, libRel *models.LibraryRelationship, rules *models.RuleSet) *models.Violation {
	// Parse library relationship text to extract package and method info
	// Format: "fmt.Println() (pkg=fmt;class=;method=Println;framework=stdlib)"
	text := libRel.Text

	// Extract package name and method name from the text
	var pkgName, methodName string

	// Look for pkg= and method= in the text
	if strings.Contains(text, "(pkg=") {
		// Find the metadata parentheses, not the function call parentheses
		start := strings.Index(text, "(pkg=")
		end := strings.LastIndex(text, ")")
		if start != -1 && end != -1 && end > start {
			params := text[start+1 : end]
			parts := strings.Split(params, ";")
			for _, part := range parts {
				if strings.HasPrefix(part, "pkg=") {
					pkgName = strings.TrimPrefix(part, "pkg=")
				} else if strings.HasPrefix(part, "method=") {
					methodName = strings.TrimPrefix(part, "method=")
				}
			}
		}
	} else {
		// Fallback: parse from the call text (e.g., "fmt.Println()")
		if strings.Contains(text, ".") && strings.Contains(text, "()") {
			callPart := strings.Split(text, " ")[0] // Get "fmt.Println()" part
			callPart = strings.TrimSuffix(callPart, "()")
			parts := strings.Split(callPart, ".")
			if len(parts) >= 2 {
				pkgName = parts[0]
				methodName = parts[1]
			}
		}
	}

	if pkgName == "" || methodName == "" {
		return nil // Could not parse library call
	}

	// Check if the call is allowed by the rules
	allowed, rule := rules.IsAllowedForFile(pkgName, methodName, astResult.FilePath)
	if !allowed {
		violationMsg := fmt.Sprintf("Call to %s.%s violates architecture rule", pkgName, methodName)
		if rule.FilePattern != "" {
			violationMsg = fmt.Sprintf("Call to %s.%s violates file-specific rule [%s]", pkgName, methodName, rule.FilePattern)
		}

		violation := &models.Violation{
			File:    astResult.FilePath,
			Line:    libRel.LineNo,
			Column:  0,
			Rule:    rule,
			Message: violationMsg,
			Source:  "arch-unit",
		}

		return violation
	}

	return nil
}