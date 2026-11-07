package main

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrSyntax            = errors.New("zti: syntaxfehler in positionsgrammatik")
	ErrUnboundState      = errors.New("zti: zustand nicht erfasst (ohne Z kein T)")
	ErrGateClosed        = errors.New("zti: commit abgewiesen - gate geschlossen (BIT[ZTIS] != 1111)")
	ErrCommitWithoutCheck = errors.New("zti: commit ohne vorangehende pruefung unzulaessig")
	ErrInvalidType       = errors.New("zti: ungueltiger typ")
	ErrUnknownOperator   = errors.New("zti: unbekannter operator")
	ErrUnknownPredicate  = errors.New("zti: unbekanntes praedikat")
)

type Parser struct {
	l         *Lexer
	curToken  Token
	peekToken Token
}

func NewParser(l *Lexer) *Parser {
	p := &Parser{l: l}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	tok, _ := p.l.NextToken()
	p.peekToken = tok
}

func (p *Parser) ParseProgram() (*Program, error) {
	prog := &Program{Statements: []Statement{}}

	for p.curToken.Type != TOK_EOF {
		if p.curToken.Type == TOK_NEWLINE {
			p.nextToken()
			continue
		}

		stmt, err := p.ParseStatement()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			prog.Statements = append(prog.Statements, stmt)
		}

		if p.curToken.Type == TOK_NEWLINE {
			p.nextToken()
		} else if p.curToken.Type != TOK_EOF {
			return nil, fmt.Errorf("%w: unerwartetes Token '%s' in Zeile %d", ErrSyntax, p.curToken.Literal, p.curToken.Line)
		}
	}

	return prog, nil
}

func (p *Parser) ParseStatement() (Statement, error) {
	switch p.curToken.Type {
	case TOK_ERFASSE:
		return p.parseReadStmt()
	case TOK_FORME:
		return p.parseTransformStmt()
	case TOK_PRUEFE:
		return p.parseCheckStmt()
	case TOK_BIND:
		return p.parseCommitStmt()
	case TOK_STOP:
		return p.parseHaltStmt()
	default:
		return nil, fmt.Errorf("%w: unerwarteter Befehl '%s' in Zeile %d", ErrSyntax, p.curToken.Literal, p.curToken.Line)
	}
}

// ERFASSE <Ident> ALS <Type> [VON <Literal>]
func (p *Parser) parseReadStmt() (*ReadStmt, error) {
	stmt := &ReadStmt{Token: p.curToken}

	if !p.expectPeek(TOK_IDENT) {
		return nil, fmt.Errorf("%w: Bezeichner nach ERFASSE erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}
	stmt.Ident = p.curToken.Literal

	if !p.expectPeek(TOK_ALS) {
		return nil, fmt.Errorf("%w: 'ALS' nach Bezeichner erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}

	p.nextToken()
	stmt.Type = p.curToken.Literal

	if p.peekToken.Type == TOK_VON {
		p.nextToken() // VON
		p.nextToken() // Literal
		stmt.From = p.curToken.Literal
	}

	p.nextToken()
	return stmt, nil
}

// FORME VON <Src> NACH <Dst> MIT <Op>
func (p *Parser) parseTransformStmt() (*TransformStmt, error) {
	stmt := &TransformStmt{Token: p.curToken}

	if !p.expectPeek(TOK_VON) {
		return nil, fmt.Errorf("%w: 'VON' nach FORME erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}

	if !p.expectPeek(TOK_IDENT) {
		return nil, fmt.Errorf("%w: Quell-Register nach VON erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}
	stmt.Src = p.curToken.Literal

	if !p.expectPeek(TOK_NACH) {
		return nil, fmt.Errorf("%w: 'NACH' nach Quelle erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}

	if !p.expectPeek(TOK_IDENT) {
		return nil, fmt.Errorf("%w: Ziel-Register nach NACH erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}
	stmt.Dst = p.curToken.Literal

	if !p.expectPeek(TOK_MIT) {
		return nil, fmt.Errorf("%w: 'MIT' nach Ziel erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}

	p.nextToken()
	stmt.Op = p.curToken.Literal

	p.nextToken()
	return stmt, nil
}

// PRÜFE <Ident> WENN <Predicate> [Literal]
func (p *Parser) parseCheckStmt() (*CheckStmt, error) {
	stmt := &CheckStmt{Token: p.curToken}

	if !p.expectPeek(TOK_IDENT) {
		return nil, fmt.Errorf("%w: Register nach PRÜFE erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}
	stmt.Ident = p.curToken.Literal

	if !p.expectPeek(TOK_WENN) {
		return nil, fmt.Errorf("%w: 'WENN' nach Register erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}

	p.nextToken()
	stmt.Predicate = p.curToken.Literal

	if p.peekToken.Type == TOK_INT || p.peekToken.Type == TOK_IDENT {
		p.nextToken()
		stmt.Value = p.curToken.Literal
	}

	p.nextToken()
	return stmt, nil
}

// BIND <Ident>
func (p *Parser) parseCommitStmt() (*CommitStmt, error) {
	stmt := &CommitStmt{Token: p.curToken}

	if !p.expectPeek(TOK_IDENT) {
		return nil, fmt.Errorf("%w: Register nach BIND erwartet (Zeile %d)", ErrSyntax, p.curToken.Line)
	}
	stmt.Ident = p.curToken.Literal

	p.nextToken()
	return stmt, nil
}

// STOP [Code]
func (p *Parser) parseHaltStmt() (*HaltStmt, error) {
	stmt := &HaltStmt{Token: p.curToken, Code: 0}

	if p.peekToken.Type == TOK_INT {
		p.nextToken()
		code, err := strconv.Atoi(p.curToken.Literal)
		if err == nil {
			stmt.Code = code
		}
	}

	p.nextToken()
	return stmt, nil
}

func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekToken.Type == t {
		p.nextToken()
		return true
	}
	return false
}
