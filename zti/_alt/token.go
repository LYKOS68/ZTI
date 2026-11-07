package main

import "fmt"

type TokenType uint16

const (
	// Spezielle Kontroll-Tokens (0..9)
	TOK_EOF     TokenType = 0
	TOK_ILLEGAL TokenType = 1
	TOK_NEWLINE TokenType = 2

	// 3+1 Transaktions-Gates (100..109)
	TOK_ERFASSE TokenType = 100 // 0x01: OP_READ   (Zustand erfassen)
	TOK_FORME   TokenType = 101 // 0x02: OP_XFORM  (Transformation vollziehen)
	TOK_PRUEFE  TokenType = 102 // 0x03: OP_CHECK  (Invariante prüfen)
	TOK_BIND    TokenType = 103 // 0x04: OP_COMMIT (Atomar comitten)
	TOK_STOP    TokenType = 104 // 0xFF: OP_HALT   (Terminieren)

	// Kontrollfluss & Routing (110..119)
	TOK_WENN    TokenType = 110
	TOK_DANN    TokenType = 111
	TOK_SONST   TokenType = 112
	TOK_SOLANGE TokenType = 113
	TOK_MACHE   TokenType = 114
	TOK_ENDE    TokenType = 115
	TOK_SENDE   TokenType = 116

	// Relationen & Positionsanker (120..129)
	TOK_VON  TokenType = 120
	TOK_NACH TokenType = 121
	TOK_MIT  TokenType = 122
	TOK_ALS  TokenType = 123

	// Datentypen (130..139)
	TOK_TYPE_U16  TokenType = 130
	TOK_TYPE_BOOL TokenType = 131
	TOK_TYPE_MSG  TokenType = 132

	// Bezeichner & Literale (200..299)
	TOK_IDENT  TokenType = 200
	TOK_INT    TokenType = 201
	TOK_STRING TokenType = 202

	// Operatoren & Begrenzer (300..399)
	TOK_PLUS     TokenType = 301 // +
	TOK_MINUS    TokenType = 302 // -
	TOK_ASTERISK TokenType = 303 // *
	TOK_SLASH    TokenType = 304 // /
	TOK_PERCENT  TokenType = 305 // %

	TOK_EQ     TokenType = 310 // ==
	TOK_NOT_EQ TokenType = 311 // !=
	TOK_LT     TokenType = 312 // <
	TOK_LTE    TokenType = 313 // <=
	TOK_GT     TokenType = 314 // >
	TOK_GTE    TokenType = 315 // >=

	TOK_AND TokenType = 320 // UND
	TOK_OR  TokenType = 321 // ODER
	TOK_NOT TokenType = 322 // NICHT

	TOK_LPAREN TokenType = 330 // (
	TOK_RPAREN TokenType = 331 // )
	TOK_COMMA  TokenType = 332 // ,
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%d, %q, L:%d, C:%d)", t.Type, t.Literal, t.Line, t.Col)
}

var keywords = map[string]TokenType{
	// 3+1 Gates
	"ERFASSE": TOK_ERFASSE,
	"FORME":   TOK_FORME,
	"PRÜFE":   TOK_PRUEFE,
	"PRUEFE":  TOK_PRUEFE,
	"BIND":    TOK_BIND,
	"STOP":    TOK_STOP,

	// Kontrollfluss
	"WENN":    TOK_WENN,
	"DANN":    TOK_DANN,
	"SONST":   TOK_SONST,
	"SOLANGE": TOK_SOLANGE,
	"MACHE":   TOK_MACHE,
	"ENDE":    TOK_ENDE,
	"SENDE":   TOK_SENDE,

	// Relationen
	"VON":  TOK_VON,
	"NACH": TOK_NACH,
	"MIT":  TOK_MIT,
	"ALS":  TOK_ALS,

	// Typen
	"U16":  TOK_TYPE_U16,
	"BOOL": TOK_TYPE_BOOL,
	"MSG":  TOK_TYPE_MSG,

	// Logische Operatoren als Schlüsselwörter
	"UND":   TOK_AND,
	"ODER":  TOK_OR,
	"NICHT": TOK_NOT,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOK_IDENT
}
