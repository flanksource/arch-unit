package parser

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenType represents the type of token
type TokenType int

const (
	// Special tokens
	TokenEOF TokenType = iota
	TokenError
	TokenIdent
	TokenString
	TokenNumber

	// Keywords
	TokenRule
	TokenLimit
	TokenForbid
	TokenRequire
	TokenAllow

	// Operators
	TokenGT    // >
	TokenLT    // <
	TokenGTE   // >=
	TokenLTE   // <=
	TokenEQ    // ==
	TokenNE    // !=
	TokenArrow // ->

	// Delimiters
	TokenLBrace // {
	TokenRBrace // }
	TokenLParen // (
	TokenRParen // )
	TokenComma  // ,
	TokenDot    // .
	TokenColon  // :
)

// Token represents a lexical token
type Token struct {
	Type     TokenType
	Value    string
	Line     int
	Column   int
	Position int
}

// String returns string representation of token
func (t Token) String() string {
	if t.Type == TokenEOF {
		return "EOF"
	}
	if t.Type == TokenError {
		return fmt.Sprintf("ERROR: %s", t.Value)
	}
	return fmt.Sprintf("%s(%s)", tokenTypeNames[t.Type], t.Value)
}

var tokenTypeNames = map[TokenType]string{
	TokenEOF:     "EOF",
	TokenError:   "ERROR",
	TokenIdent:   "IDENT",
	TokenString:  "STRING",
	TokenNumber:  "NUMBER",
	TokenRule:    "RULE",
	TokenLimit:   "LIMIT",
	TokenForbid:  "FORBID",
	TokenRequire: "REQUIRE",
	TokenAllow:   "ALLOW",
	TokenGT:      ">",
	TokenLT:      "<",
	TokenGTE:     ">=",
	TokenLTE:     "<=",
	TokenEQ:      "==",
	TokenNE:      "!=",
	TokenArrow:   "->",
	TokenLBrace:  "{",
	TokenRBrace:  "}",
	TokenLParen:  "(",
	TokenRParen:  ")",
	TokenComma:   ",",
	TokenDot:     ".",
	TokenColon:   ":",
}

// Keywords map
var keywords = map[string]TokenType{
	"RULE":    TokenRule,
	"LIMIT":   TokenLimit,
	"FORBID":  TokenForbid,
	"REQUIRE": TokenRequire,
	"ALLOW":   TokenAllow,
}

// Lexer represents a lexical analyzer
type Lexer struct {
	input    string
	position int  // current position in input (points to current char)
	line     int  // current line number
	column   int  // current column number
	start    int  // start position of current token
	current  rune // current character under examination
}

// NewLexer creates a new lexer
func NewLexer(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

// readChar reads the next character and advances position
func (l *Lexer) readChar() {
	if l.position >= len(l.input) {
		l.current = 0 // EOF
	} else {
		var size int
		l.current, size = utf8.DecodeRuneInString(l.input[l.position:])
		l.position += size
		if l.current == '\n' {
			l.line++
			l.column = 0
		} else {
			l.column++
		}
	}
}

// peekChar returns the next character without advancing position
func (l *Lexer) peekChar() rune {
	if l.position >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.position:])
	return r
}

// skipWhitespace skips whitespace characters
func (l *Lexer) skipWhitespace() {
	for unicode.IsSpace(l.current) {
		l.readChar()
	}
}

// skipComment skips single-line and multi-line comments
func (l *Lexer) skipComment() {
	if l.current == '/' && l.peekChar() == '/' {
		// Single-line comment
		for l.current != '\n' && l.current != 0 {
			l.readChar()
		}
	} else if l.current == '/' && l.peekChar() == '*' {
		// Multi-line comment
		l.readChar() // skip '/'
		l.readChar() // skip '*'

		for {
			if l.current == 0 {
				break
			}
			if l.current == '*' && l.peekChar() == '/' {
				l.readChar() // skip '*'
				l.readChar() // skip '/'
				break
			}
			l.readChar()
		}
	}
}

// readIdentifier reads an identifier
func (l *Lexer) readIdentifier() string {
	start := l.position - 1 // Account for current character
	for unicode.IsLetter(l.current) || unicode.IsDigit(l.current) || l.current == '_' || l.current == '*' {
		l.readChar()
	}
	return l.input[start : l.position-1]
}

// readString reads a quoted string
func (l *Lexer) readString() (string, error) {
	var result strings.Builder
	quote := l.current
	l.readChar() // skip opening quote

	for l.current != quote && l.current != 0 {
		if l.current == '\\' {
			l.readChar()
			switch l.current {
			case 'n':
				result.WriteRune('\n')
			case 't':
				result.WriteRune('\t')
			case 'r':
				result.WriteRune('\r')
			case '\\':
				result.WriteRune('\\')
			case '"':
				result.WriteRune('"')
			case '\'':
				result.WriteRune('\'')
			default:
				result.WriteRune(l.current)
			}
		} else {
			result.WriteRune(l.current)
		}
		l.readChar()
	}

	if l.current != quote {
		return "", fmt.Errorf("unterminated string")
	}

	l.readChar() // skip closing quote
	return result.String(), nil
}

// readNumber reads a number
func (l *Lexer) readNumber() string {
	start := l.position - 1
	for unicode.IsDigit(l.current) {
		l.readChar()
	}

	// Check for decimal point
	if l.current == '.' && unicode.IsDigit(l.peekChar()) {
		l.readChar() // skip '.'
		for unicode.IsDigit(l.current) {
			l.readChar()
		}
	}

	return l.input[start : l.position-1]
}

// NextToken returns the next token
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	// Skip comments
	for l.current == '/' && (l.peekChar() == '/' || l.peekChar() == '*') {
		l.skipComment()
		l.skipWhitespace()
	}

	l.start = l.position - 1
	startLine := l.line
	startColumn := l.column

	switch l.current {
	case 0:
		return Token{TokenEOF, "", startLine, startColumn, l.start}
	case '{':
		l.readChar()
		return Token{TokenLBrace, "{", startLine, startColumn, l.start}
	case '}':
		l.readChar()
		return Token{TokenRBrace, "}", startLine, startColumn, l.start}
	case '(':
		l.readChar()
		return Token{TokenLParen, "(", startLine, startColumn, l.start}
	case ')':
		l.readChar()
		return Token{TokenRParen, ")", startLine, startColumn, l.start}
	case ',':
		l.readChar()
		return Token{TokenComma, ",", startLine, startColumn, l.start}
	case '.':
		l.readChar()
		return Token{TokenDot, ".", startLine, startColumn, l.start}
	case ':':
		l.readChar()
		return Token{TokenColon, ":", startLine, startColumn, l.start}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{TokenGTE, ">=", startLine, startColumn, l.start}
		}
		l.readChar()
		return Token{TokenGT, ">", startLine, startColumn, l.start}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{TokenLTE, "<=", startLine, startColumn, l.start}
		}
		l.readChar()
		return Token{TokenLT, "<", startLine, startColumn, l.start}
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{TokenEQ, "==", startLine, startColumn, l.start}
		}
		l.readChar()
		return Token{TokenError, "unexpected '='", startLine, startColumn, l.start}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			return Token{TokenNE, "!=", startLine, startColumn, l.start}
		}
		l.readChar()
		return Token{TokenError, "unexpected '!'", startLine, startColumn, l.start}
	case '-':
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			return Token{TokenArrow, "->", startLine, startColumn, l.start}
		}
		l.readChar()
		return Token{TokenError, "unexpected '-'", startLine, startColumn, l.start}
	case '"', '\'':
		str, err := l.readString()
		if err != nil {
			return Token{TokenError, err.Error(), startLine, startColumn, l.start}
		}
		return Token{TokenString, str, startLine, startColumn, l.start}
	default:
		if unicode.IsLetter(l.current) || l.current == '_' || l.current == '*' {
			ident := l.readIdentifier()
			tokenType := TokenIdent
			if kw, exists := keywords[strings.ToUpper(ident)]; exists {
				tokenType = kw
			}
			return Token{tokenType, ident, startLine, startColumn, l.start}
		}

		if unicode.IsDigit(l.current) {
			num := l.readNumber()
			return Token{TokenNumber, num, startLine, startColumn, l.start}
		}

		char := l.current
		l.readChar()
		return Token{TokenError, fmt.Sprintf("unexpected character: %c", char), startLine, startColumn, l.start}
	}
}

// TokenizeAll returns all tokens from the input
func (l *Lexer) TokenizeAll() []Token {
	var tokens []Token

	for {
		token := l.NextToken()
		tokens = append(tokens, token)
		if token.Type == TokenEOF || token.Type == TokenError {
			break
		}
	}

	return tokens
}
