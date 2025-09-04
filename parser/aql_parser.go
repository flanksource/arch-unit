package parser

import (
	"fmt"
	"strconv"

	"github.com/flanksource/arch-unit/models"
)

// Parser represents an AQL parser
type Parser struct {
	lexer        *Lexer
	currentToken Token
	peekToken    Token
	errors       []string
}

// NewParser creates a new AQL parser
func NewParser(input string) *Parser {
	lexer := NewLexer(input)
	parser := &Parser{
		lexer:  lexer,
		errors: []string{},
	}

	// Read two tokens, so currentToken and peekToken are both set
	parser.nextToken()
	parser.nextToken()

	return parser
}

// nextToken advances to the next token
func (p *Parser) nextToken() {
	p.currentToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

// addError adds an error message
func (p *Parser) addError(msg string) {
	p.errors = append(p.errors, fmt.Sprintf("line %d, col %d: %s",
		p.currentToken.Line, p.currentToken.Column, msg))
}

// expectToken checks if current token matches expected type and consumes it
func (p *Parser) expectToken(tokenType TokenType) bool {
	if p.currentToken.Type == tokenType {
		p.nextToken()
		return true
	}

	p.addError(fmt.Sprintf("expected %s, got %s",
		tokenTypeNames[tokenType], tokenTypeNames[p.currentToken.Type]))
	return false
}

// currentTokenIs checks if current token is of given type
func (p *Parser) currentTokenIs(tokenType TokenType) bool {
	return p.currentToken.Type == tokenType
}

// peekTokenIs checks if peek token is of given type
func (p *Parser) peekTokenIs(tokenType TokenType) bool {
	return p.peekToken.Type == tokenType
}

// ParseRuleSet parses a complete AQL rule set
func (p *Parser) ParseRuleSet() (*models.AQLRuleSet, error) {
	ruleSet := &models.AQLRuleSet{
		Rules: []*models.AQLRule{},
	}

	for !p.currentTokenIs(TokenEOF) {
		if p.currentTokenIs(TokenError) {
			p.addError(p.currentToken.Value)
			p.nextToken()
			continue
		}

		rule, err := p.parseRule()
		if err != nil {
			return nil, err
		}

		if rule != nil {
			ruleSet.AddRule(rule)
		}
	}

	if len(p.errors) > 0 {
		return nil, fmt.Errorf("parsing errors: %v", p.errors)
	}

	return ruleSet, nil
}

// parseRule parses a single AQL rule
func (p *Parser) parseRule() (*models.AQLRule, error) {
	if !p.expectToken(TokenRule) {
		return nil, fmt.Errorf("expected RULE keyword")
	}

	if !p.currentTokenIs(TokenString) {
		p.addError("expected rule name as string")
		return nil, fmt.Errorf("expected rule name")
	}

	ruleName := p.currentToken.Value
	lineNumber := p.currentToken.Line
	p.nextToken()

	if !p.expectToken(TokenLBrace) {
		return nil, fmt.Errorf("expected '{' after rule name")
	}

	rule := &models.AQLRule{
		Name:       ruleName,
		LineNumber: lineNumber,
		Statements: []*models.AQLStatement{},
	}

	// Parse statements until we hit '}'
	for !p.currentTokenIs(TokenRBrace) && !p.currentTokenIs(TokenEOF) {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}

		if stmt != nil {
			rule.Statements = append(rule.Statements, stmt)
		}

		// Optional comma between statements
		if p.currentTokenIs(TokenComma) {
			p.nextToken()
		}
	}

	if !p.expectToken(TokenRBrace) {
		return nil, fmt.Errorf("expected '}' to close rule")
	}

	return rule, nil
}

// parseStatement parses an AQL statement
func (p *Parser) parseStatement() (*models.AQLStatement, error) {
	switch p.currentToken.Type {
	case TokenLimit:
		return p.parseLimitStatement()
	case TokenForbid:
		return p.parseForbidStatement()
	case TokenRequire:
		return p.parseRequireStatement()
	case TokenAllow:
		return p.parseAllowStatement()
	default:
		p.addError(fmt.Sprintf("unexpected token: %s", p.currentToken.Value))
		p.nextToken() // Skip invalid token
		return nil, nil
	}
}

// parseLimitStatement parses a LIMIT statement
func (p *Parser) parseLimitStatement() (*models.AQLStatement, error) {
	p.nextToken() // consume LIMIT

	if !p.expectToken(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after LIMIT")
	}

	condition, err := p.parseCondition()
	if err != nil {
		return nil, err
	}

	if !p.expectToken(TokenRParen) {
		return nil, fmt.Errorf("expected ')' after condition")
	}

	return &models.AQLStatement{
		Type:      models.AQLStatementLimit,
		Pattern:   condition.Pattern,
		Condition: condition,
	}, nil
}

// parseForbidStatement parses a FORBID statement
func (p *Parser) parseForbidStatement() (*models.AQLStatement, error) {
	p.nextToken() // consume FORBID

	if !p.expectToken(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after FORBID")
	}

	// Check if it's a relationship pattern (contains ->)
	if p.isRelationshipPattern() {
		fromPattern, toPattern, err := p.parseRelationshipPattern()
		if err != nil {
			return nil, err
		}

		if !p.expectToken(TokenRParen) {
			return nil, fmt.Errorf("expected ')' after relationship pattern")
		}

		return &models.AQLStatement{
			Type:        models.AQLStatementForbid,
			FromPattern: fromPattern,
			ToPattern:   toPattern,
		}, nil
	}

	// Single pattern
	pattern, err := p.parsePattern()
	if err != nil {
		return nil, err
	}

	if !p.expectToken(TokenRParen) {
		return nil, fmt.Errorf("expected ')' after pattern")
	}

	return &models.AQLStatement{
		Type:    models.AQLStatementForbid,
		Pattern: pattern,
	}, nil
}

// parseRequireStatement parses a REQUIRE statement
func (p *Parser) parseRequireStatement() (*models.AQLStatement, error) {
	p.nextToken() // consume REQUIRE

	if !p.expectToken(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after REQUIRE")
	}

	// Check if it's a relationship pattern
	if p.isRelationshipPattern() {
		fromPattern, toPattern, err := p.parseRelationshipPattern()
		if err != nil {
			return nil, err
		}

		if !p.expectToken(TokenRParen) {
			return nil, fmt.Errorf("expected ')' after relationship pattern")
		}

		return &models.AQLStatement{
			Type:        models.AQLStatementRequire,
			FromPattern: fromPattern,
			ToPattern:   toPattern,
		}, nil
	}

	// Single pattern
	pattern, err := p.parsePattern()
	if err != nil {
		return nil, err
	}

	if !p.expectToken(TokenRParen) {
		return nil, fmt.Errorf("expected ')' after pattern")
	}

	return &models.AQLStatement{
		Type:    models.AQLStatementRequire,
		Pattern: pattern,
	}, nil
}

// parseAllowStatement parses an ALLOW statement
func (p *Parser) parseAllowStatement() (*models.AQLStatement, error) {
	p.nextToken() // consume ALLOW

	if !p.expectToken(TokenLParen) {
		return nil, fmt.Errorf("expected '(' after ALLOW")
	}

	// Check if it's a relationship pattern
	if p.isRelationshipPattern() {
		fromPattern, toPattern, err := p.parseRelationshipPattern()
		if err != nil {
			return nil, err
		}

		if !p.expectToken(TokenRParen) {
			return nil, fmt.Errorf("expected ')' after relationship pattern")
		}

		return &models.AQLStatement{
			Type:        models.AQLStatementAllow,
			FromPattern: fromPattern,
			ToPattern:   toPattern,
		}, nil
	}

	// Single pattern
	pattern, err := p.parsePattern()
	if err != nil {
		return nil, err
	}

	if !p.expectToken(TokenRParen) {
		return nil, fmt.Errorf("expected ')' after pattern")
	}

	return &models.AQLStatement{
		Type:    models.AQLStatementAllow,
		Pattern: pattern,
	}, nil
}

// isRelationshipPattern checks if the current position contains a relationship pattern (->)
func (p *Parser) isRelationshipPattern() bool {
	// Simple lookahead to check for arrow
	// This is a simplified check - in a full parser we'd need better lookahead
	lexerCopy := NewLexer(p.lexer.input[p.currentToken.Position:])
	depth := 0

	for {
		token := lexerCopy.NextToken()
		if token.Type == TokenEOF || token.Type == TokenError {
			break
		}

		if token.Type == TokenLParen {
			depth++
		} else if token.Type == TokenRParen {
			depth--
			if depth < 0 {
				break
			}
		} else if token.Type == TokenArrow && depth == 0 {
			return true
		}
	}

	return false
}

// parseRelationshipPattern parses a relationship pattern (pattern -> pattern)
func (p *Parser) parseRelationshipPattern() (*models.AQLPattern, *models.AQLPattern, error) {
	fromPattern, err := p.parsePattern()
	if err != nil {
		return nil, nil, err
	}

	if !p.expectToken(TokenArrow) {
		return nil, nil, fmt.Errorf("expected '->' in relationship pattern")
	}

	toPattern, err := p.parsePattern()
	if err != nil {
		return nil, nil, err
	}

	return fromPattern, toPattern, nil
}

// parseCondition parses a condition expression
func (p *Parser) parseCondition() (*models.AQLCondition, error) {
	pattern, err := p.parsePattern()
	if err != nil {
		return nil, err
	}

	operator, err := p.parseOperator()
	if err != nil {
		return nil, err
	}

	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	// Extract metric from pattern for backward compatibility
	property := ""
	conditionPattern := pattern
	if pattern.Metric != "" {
		property = pattern.Metric
		// Create a clean pattern without the metric for the condition
		conditionPattern = &models.AQLPattern{
			Package:    pattern.Package,
			Type:       pattern.Type,
			Method:     pattern.Method,
			Field:      pattern.Field,
			IsWildcard: pattern.IsWildcard,
			Original:   pattern.Original[:len(pattern.Original)-len("."+pattern.Metric)],
		}
	}

	return &models.AQLCondition{
		Pattern:  conditionPattern,
		Property: property, // For backward compatibility
		Operator: operator,
		Value:    value,
	}, nil
}

// parsePattern parses a pattern expression
func (p *Parser) parsePattern() (*models.AQLPattern, error) {
	if !p.currentTokenIs(TokenIdent) {
		p.addError("expected pattern identifier")
		return nil, fmt.Errorf("expected pattern")
	}

	patternText := p.currentToken.Value
	p.nextToken()

	// Handle dot notation for metrics (e.g., *.cyclomatic)
	for p.currentTokenIs(TokenDot) || p.currentTokenIs(TokenColon) {
		delimiter := p.currentToken.Value
		p.nextToken()

		if !p.currentTokenIs(TokenIdent) {
			p.addError("expected identifier after " + delimiter)
			return nil, fmt.Errorf("expected identifier")
		}

		patternText += delimiter + p.currentToken.Value
		p.nextToken()
	}

	pattern, err := models.ParsePattern(patternText)
	if err != nil {
		p.addError(fmt.Sprintf("invalid pattern: %v", err))
		return nil, err
	}

	return pattern, nil
}

// parseOperator parses a comparison operator
func (p *Parser) parseOperator() (models.ComparisonOperator, error) {
	switch p.currentToken.Type {
	case TokenGT:
		p.nextToken()
		return models.OpGreaterThan, nil
	case TokenLT:
		p.nextToken()
		return models.OpLessThan, nil
	case TokenGTE:
		p.nextToken()
		return models.OpGreaterThanEqual, nil
	case TokenLTE:
		p.nextToken()
		return models.OpLessThanEqual, nil
	case TokenEQ:
		p.nextToken()
		return models.OpEqual, nil
	case TokenNE:
		p.nextToken()
		return models.OpNotEqual, nil
	default:
		p.addError(fmt.Sprintf("expected comparison operator, got %s",
			tokenTypeNames[p.currentToken.Type]))
		return "", fmt.Errorf("expected operator")
	}
}

// parseValue parses a value expression and returns raw values for backward compatibility
func (p *Parser) parseValue() (interface{}, error) {
	switch p.currentToken.Type {
	case TokenNumber:
		numStr := p.currentToken.Value
		p.nextToken()

		// Try parsing as float first to match test expectations
		if floatNum, err := strconv.ParseFloat(numStr, 64); err == nil {
			return floatNum, nil
		}

		num, err := strconv.Atoi(numStr)
		if err != nil {
			p.addError(fmt.Sprintf("invalid number: %s", numStr))
			return nil, err
		}

		return float64(num), nil // Convert to float64 for consistency

	case TokenString:
		str := p.currentToken.Value
		p.nextToken()
		return str, nil

	case TokenIdent:
		// Handle boolean literals
		ident := p.currentToken.Value
		p.nextToken()

		switch ident {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			p.addError(fmt.Sprintf("unexpected identifier in value: %s", ident))
			return nil, fmt.Errorf("unexpected identifier")
		}

	default:
		p.addError(fmt.Sprintf("expected value, got %s", tokenTypeNames[p.currentToken.Type]))
		return nil, fmt.Errorf("expected value")
	}
}

// ParseAQLFile parses an AQL file
func ParseAQLFile(content string) (*models.AQLRuleSet, error) {
	parser := NewParser(content)
	return parser.ParseRuleSet()
}

// ParseAQLRule parses a single AQL rule
func ParseAQLRule(content string) (*models.AQLRule, error) {
	parser := NewParser(content)
	return parser.parseRule()
}
