package ast

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"
	"github.com/shadowforge/shadowforge-l1/parser"
)

// SyntaxError is one lexer/parser diagnostic collected while parsing a
// ShadowRust source file.
type SyntaxError struct {
	Line, Column int
	Message      string
}

func (e SyntaxError) String() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Column, e.Message)
}

type collectingErrorListener struct {
	*antlr.DefaultErrorListener
	errors []SyntaxError
}

func (l *collectingErrorListener) SyntaxError(_ antlr.Recognizer, _ interface{}, line, column int, msg string, _ antlr.RecognitionException) {
	l.errors = append(l.errors, SyntaxError{Line: line, Column: column, Message: msg})
}

// Parse lexes and parses ShadowRust source text using the ANTLR-generated
// ShadowRustExt grammar (grammar/ShadowRustExt.g4, which imports the pinned
// core grammar/ShadowRust.g4) and returns the typed AST. Any lexer or parser
// diagnostics are returned alongside; a non-empty error slice means the AST
// may be partial and must not be treated as valid ShadowRust.
func Parse(src string) (*Program, []SyntaxError) {
	input := antlr.NewInputStream(src)
	lexer := parser.NewShadowRustExtLexer(input)
	lexErrs := &collectingErrorListener{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(lexErrs)

	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewShadowRustExtParser(tokens)
	parseErrs := &collectingErrorListener{}
	p.RemoveErrorListeners()
	p.AddErrorListener(parseErrs)

	tree := p.Program()
	all := append(lexErrs.errors, parseErrs.errors...)
	if len(all) > 0 {
		return nil, all
	}
	return Build(tree), nil
}
