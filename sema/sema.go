// Package sema is the ShadowRust semantic analyzer (spec 14.1, 14.6). It
// rejects unsafe programs before codegen runs: a value-moving tx with no
// ZKP-backed fee route, a non-numeric amount, an unbound Bank ATR operand,
// or a validate-stage body performing work illegal for that stage (spec
// 5.3, "it must emit only the checks legal for that stage").
//
// "Mutable shared state without a lock" (spec 14.1) is not a sema rule: the
// ShadowRust surface language never exposes a raw shared-mutable primitive
// to source programs — codegen (codegen/) is solely responsible for wrapping
// every generated queue/trait mutation in a mutex or single-owner goroutine
// (spec 14.4), and that guarantee is covered by codegen tests instead.
package sema

import (
	"fmt"

	"github.com/shadowforge/shadowforge-l1/ast"
)

// Diagnostic is one semantic-analysis finding.
type Diagnostic struct {
	Message string
}

func (d Diagnostic) String() string { return d.Message }

// Analyze walks the whole program and returns every diagnostic found. An
// empty result means the program is safe to hand to codegen.
func Analyze(prog *ast.Program) []Diagnostic {
	a := &analyzer{}
	a.checkStatements(prog.Statements, stageUnrestricted)
	return a.diags
}

// stageUnrestricted marks statements outside any validate{} block, where the
// per-stage whitelist (below) does not apply.
const stageUnrestricted = 0

type analyzer struct {
	diags []Diagnostic
}

func (a *analyzer) fail(format string, args ...interface{}) {
	a.diags = append(a.diags, Diagnostic{Message: fmt.Sprintf(format, args...)})
}

func (a *analyzer) checkStatements(stmts []ast.Statement, stage int) {
	for _, s := range stmts {
		a.checkStatement(s, stage)
	}
}

func (a *analyzer) checkStatement(s ast.Statement, stage int) {
	if stage != stageUnrestricted && !allowedInStage(stage, s) {
		a.fail("statement %s is not legal in validate stage %d (spec 5.3)", kindName(s), stage)
	}

	switch n := s.(type) {
	case *ast.TxStatement:
		a.checkNumericAmount("tx "+n.Name, n.Amount)
		if !bodyHasFeeRoute(n.Body) && !exprHasFeeRoute(n.Amount) {
			a.fail("tx %s moves value but has no fee route (`expr TO address`); every value-moving statement must route a fee (spec 14.4)", n.Name)
		}
		a.checkStatements(n.Body, stage)

	case *ast.MintStatement:
		a.checkNumericAmount("mint "+n.Name, n.Amount)

	case *ast.IfStatement:
		a.checkStatements(n.Body, stage)

	case *ast.ValidateStatement:
		if n.Stage < 1 || n.Stage > 5 {
			a.fail("validate %s: stage %d is out of range; the pipeline has exactly 5 stages (spec 5.3)", n.Name, n.Stage)
		}
		a.checkStatements(n.Body, n.Stage)

	case *ast.ResilienceStatement:
		a.checkStatements(n.Body, stage)

	case *ast.BankDepositStatement:
		if !exprBoundToIdent(n.ATR) {
			a.fail("bank deposit %s: atr operand must be bound to an identifier (an oracle mock or client), not a bare literal (spec 14.4)", n.Name)
		}

	case *ast.QueueInsertStatement:
		if len(n.Positions) == 0 {
			a.fail("queue insert %s: at least one position is required", n.Name)
		}
	}
}

// checkNumericAmount rejects an amount expression built from relational
// operators (>, >=, <, <=, ==, !=): those produce a 0/1 boolean, not a
// transferable quantity.
func (a *analyzer) checkNumericAmount(where string, e ast.Expr) {
	if e == nil {
		a.fail("%s: amount is required", where)
		return
	}
	if isRelational(e) {
		a.fail("%s: amount must be numeric, not a relational (comparison) expression", where)
	}
}

func isRelational(e ast.Expr) bool {
	b, ok := e.(*ast.Binary)
	if !ok {
		return false
	}
	switch b.Op {
	case ">", ">=", "<", "<=", "==", "!=":
		return true
	}
	return isRelational(b.Left) || isRelational(b.Right)
}

// bodyHasFeeRoute reports whether any assignment (directly, not nested
// inside a further tx) in the body routes a fee.
func bodyHasFeeRoute(stmts []ast.Statement) bool {
	for _, s := range stmts {
		switch n := s.(type) {
		case *ast.Assignment:
			if exprHasFeeRoute(n.Value) {
				return true
			}
		case *ast.IfStatement:
			if bodyHasFeeRoute(n.Body) {
				return true
			}
		}
	}
	return false
}

func exprHasFeeRoute(e ast.Expr) bool {
	switch n := e.(type) {
	case *ast.FeeRoute:
		return true
	case *ast.Binary:
		return exprHasFeeRoute(n.Left) || exprHasFeeRoute(n.Right)
	}
	return false
}

// exprBoundToIdent reports whether e contains at least one bare identifier
// reference (as opposed to being built entirely from numeric literals).
func exprBoundToIdent(e ast.Expr) bool {
	switch n := e.(type) {
	case *ast.Ident:
		return true
	case *ast.Binary:
		return exprBoundToIdent(n.Left) || exprBoundToIdent(n.Right)
	case *ast.FeeRoute:
		return exprBoundToIdent(n.Value)
	}
	return false
}

// allowedInStage implements the "what is checked / what is written" column
// of the five-stage table in spec 5.3:
//
//	1 Sender Leave    - ZKP existence/ownership/non-null checks only (read-only)
//	2 TX Offer        - well-formedness checks and admission (read-only)
//	3 Receiver Check  - receiver legality, compliance hook, container routing
//	4 Send Exec       - atomic state transition; Bank and mint math run here
//	5 Place Final     - BFT vote collection and commit
func allowedInStage(stage int, s ast.Statement) bool {
	switch stage {
	case 1, 2:
		switch s.(type) {
		case *ast.IfStatement, *ast.Assignment:
			return true
		}
		return false
	case 3:
		switch s.(type) {
		case *ast.IfStatement, *ast.Assignment, *ast.VoteStatement:
			return true
		}
		return false
	case 4:
		switch s.(type) {
		case *ast.IfStatement, *ast.Assignment, *ast.BankDepositStatement, *ast.UpdateTraitStatement, *ast.TxStatement, *ast.MintStatement, *ast.QueueInsertStatement:
			return true
		}
		return false
	case 5:
		switch s.(type) {
		case *ast.IfStatement, *ast.VoteStatement:
			return true
		}
		return false
	}
	return true
}

func kindName(s ast.Statement) string {
	return fmt.Sprintf("%T", s)
}
