package main

import (
	"fmt"
	"unicode"
)

const maxTokenLen = 256

type Lexer struct {
	input        []rune
	position     int
	readPosition int
	ch           rune
	line         int
	col          int
}

func NewLexer(input string) *Lexer {
	l := &Lexer{
		input: []rune(input),
		line:  1,
		col:   0,
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.col++
}

func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) NextToken() (Token, error) {
	l.skipWhitespaceExceptNewline()

	tok := Token{
		Line: l.line,
		Col:  l.col,
	}

	// Kommentare überspringen (// ...)
	if l.ch == '/' && l.peekChar() == '/' {
		l.skipComment()
		return l.NextToken()
	}

	switch l.ch {
	case '\n':
		tok.Type = TOK_NEWLINE
		tok.Literal = "\n"
		l.line++
		l.col = 0
		l.readChar()
		return tok, nil
	case 0:
		tok.Type = TOK_EOF
		tok.Literal = ""
		return tok, nil
	case '+':
		tok.Type = TOK_PLUS
		tok.Literal = "+"
	case '-':
		tok.Type = TOK_MINUS
		tok.Literal = "-"
	case '*':
		tok.Type = TOK_ASTERISK
		tok.Literal = "*"
	case '/':
		tok.Type = TOK_SLASH
		tok.Literal = "/"
	case '%':
		tok.Type = TOK_PERCENT
		tok.Literal = "%"
	case '(':
		tok.Type = TOK_LPAREN
		tok.Literal = "("
	case ')':
		tok.Type = TOK_RPAREN
		tok.Literal = ")"
	case ',':
		tok.Type = TOK_COMMA
		tok.Literal = ","
	case '=':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = TOK_EQ
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = TOK_ILLEGAL
			tok.Literal = "="
			l.readChar()
			return tok, fmt.Errorf("zti: einzelnes '=' unzulaessig in Zeile %d:%d (nutze '==')", tok.Line, tok.Col)
		}
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = TOK_NOT_EQ
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = TOK_ILLEGAL
			tok.Literal = "!"
			l.readChar()
			return tok, fmt.Errorf("zti: unerwartetes '!' in Zeile %d:%d (nutze '!=' oder 'NICHT')", tok.Line, tok.Col)
		}
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = TOK_LTE
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = TOK_LT
			tok.Literal = "<"
		}
	case '>':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = TOK_GTE
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = TOK_GT
			tok.Literal = ">"
		}
	default:
		if isLetter(l.ch) {
			lit, err := l.readIdentifier()
			if err != nil {
				return tok, err
			}
			tok.Literal = lit
			tok.Type = LookupIdent(tok.Literal)
			return tok, nil
		} else if isDigit(l.ch) {
			lit, err := l.readNumber()
			if err != nil {
				return tok, err
			}
			tok.Literal = lit
			tok.Type = TOK_INT
			return tok, nil
		}

		tok.Type = TOK_ILLEGAL
		tok.Literal = string(l.ch)
		l.readChar()
		return tok, fmt.Errorf("zti: ungueltiges Zeichen %q in Zeile %d:%d", tok.Literal, tok.Line, tok.Col)
	}

	l.readChar()
	return tok, nil
}

func (l *Lexer) skipWhitespaceExceptNewline() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
}

func (l *Lexer) readIdentifier() (string, error) {
	start := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		if l.position-start >= maxTokenLen {
			return "", fmt.Errorf("zti: Token-Laengenlimit (%d) in Zeile %d ueberschritten", maxTokenLen, l.line)
		}
		l.readChar()
	}
	return string(l.input[start:l.position]), nil
}

func (l *Lexer) readNumber() (string, error) {
	start := l.position
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		l.readChar() // 0
		l.readChar() // x
		for isHexDigit(l.ch) {
			if l.position-start >= maxTokenLen {
				return "", fmt.Errorf("zti: Token-Laengenlimit (%d) in Zeile %d ueberschritten", maxTokenLen, l.line)
			}
			l.readChar()
		}
		return string(l.input[start:l.position]), nil
	}
	for isDigit(l.ch) {
		if l.position-start >= maxTokenLen {
			return "", fmt.Errorf("zti: Token-Laengenlimit (%d) in Zeile %d ueberschritten", maxTokenLen, l.line)
		}
		l.readChar()
	}
	return string(l.input[start:l.position]), nil
}

func isLetter(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_' || ch == 'Ü' || ch == 'ü' || ch == 'Ä' || ch == 'ä' || ch == 'Ö' || ch == 'ö'
}

func isDigit(ch rune) bool {
	return unicode.IsDigit(ch)
}

func isHexDigit(ch rune) bool {
	return unicode.IsDigit(ch) ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}
