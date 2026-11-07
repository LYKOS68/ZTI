package main

import (
	"testing"
)

func TestLexer_Tokens(t *testing.T) {
	input := `ERFASSE x ALS U16 VON 10
FORME VON x NACH y MIT x + 5 * 2
PRÜFE y WENN y == 20
BIND y
SENDE bus MIT 1, 10, 20
WENN y > 10 DANN
    STOP 0
ENDE
`

	expected := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{TOK_ERFASSE, "ERFASSE"},
		{TOK_IDENT, "x"},
		{TOK_ALS, "ALS"},
		{TOK_TYPE_U16, "U16"},
		{TOK_VON, "VON"},
		{TOK_INT, "10"},
		{TOK_NEWLINE, "\n"},

		{TOK_FORME, "FORME"},
		{TOK_VON, "VON"},
		{TOK_IDENT, "x"},
		{TOK_NACH, "NACH"},
		{TOK_IDENT, "y"},
		{TOK_MIT, "MIT"},
		{TOK_IDENT, "x"},
		{TOK_PLUS, "+"},
		{TOK_INT, "5"},
		{TOK_ASTERISK, "*"},
		{TOK_INT, "2"},
		{TOK_NEWLINE, "\n"},

		{TOK_PRUEFE, "PRÜFE"},
		{TOK_IDENT, "y"},
		{TOK_WENN, "WENN"},
		{TOK_IDENT, "y"},
		{TOK_EQ, "=="},
		{TOK_INT, "20"},
		{TOK_NEWLINE, "\n"},

		{TOK_BIND, "BIND"},
		{TOK_IDENT, "y"},
		{TOK_NEWLINE, "\n"},

		{TOK_SENDE, "SENDE"},
		{TOK_IDENT, "bus"},
		{TOK_MIT, "MIT"},
		{TOK_INT, "1"},
		{TOK_COMMA, ","},
		{TOK_INT, "10"},
		{TOK_COMMA, ","},
		{TOK_INT, "20"},
		{TOK_NEWLINE, "\n"},

		{TOK_WENN, "WENN"},
		{TOK_IDENT, "y"},
		{TOK_GT, ">"},
		{TOK_INT, "10"},
		{TOK_DANN, "DANN"},
		{TOK_NEWLINE, "\n"},

		{TOK_STOP, "STOP"},
		{TOK_INT, "0"},
		{TOK_NEWLINE, "\n"},

		{TOK_ENDE, "ENDE"},
		{TOK_NEWLINE, "\n"},

		{TOK_EOF, ""},
	}

	lexer := NewLexer(input)
	for i, tt := range expected {
		tok, err := lexer.NextToken()
		if err != nil {
			t.Fatalf("Test[%d] Unerwarteter Lexer-Fehler: %v", i, err)
		}

		if tok.Type != tt.expectedType {
			t.Fatalf("Test[%d] - Falscher TokenType. Erwartet=%d, Erhalten=%d (%q)",
				i, tt.expectedType, tok.Type, tok.Literal)
		}

		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("Test[%d] - Falsches Literal. Erwartet=%q, Erhalten=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}
