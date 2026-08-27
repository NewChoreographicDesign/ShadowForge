// Package interp is the ShadowRust sandbox interpreter (spec 14.1, 14.6,
// 13.2 "Sandbox preview"). It evaluates expr, assignment, and mocked tx/bank
// statements against an in-memory environment and a mocked oracle/state
// backend so ShadeLang can preview a program without touching a real node.
// It never produces a proof; it is a preview tool, not a production path.
package interp

import (
	"fmt"
	"math/big"

	"github.com/shadowforge/shadowforge-l1/ast"
)

// Env is the mutable variable environment a program executes against.
// ShadowRust has no block scoping (spec 14.2's grammar has no let/var
// binding form narrower than a whole program), so a single flat map is
// the right shape: an if-block's assignments are visible after the block,
// same as everywhere else in the language.
type Env struct {
	vars map[string]*big.Rat
}

func NewEnv() *Env {
	return &Env{vars: map[string]*big.Rat{}}
}

func (e *Env) Get(name string) (*big.Rat, bool) {
	v, ok := e.vars[name]
	return v, ok
}

func (e *Env) Set(name string, v *big.Rat) {
	e.vars[name] = v
}

// FeeRouteEvent records a `expr TO address` evaluation, mirroring the
// codegen contract in spec 14.4 ("Routes any `expr TO address` as a fee
// commitment to that address").
type FeeRouteEvent struct {
	To     string
	Amount *big.Rat
}

// TxEvent records a mocked `tx buy` execution.
type TxEvent struct {
	Name, From, To string
	Amount         *big.Rat
}

// BankDepositEvent records a mocked `bank deposit` execution, using the
// exact ATR math from spec 19.3 via pkg/bank in the real node; here it is
// evaluated against a caller-supplied ATR oracle mock only.
type BankDepositEvent struct {
	Name string
	ATR  *big.Rat
}

// QueueInsertEvent records a mocked `queue insert` execution.
type QueueInsertEvent struct {
	Name      string
	Positions []*big.Rat
}

// Oracle supplies external values (e.g. `atr`, `online`) the interpreter
// cannot compute itself. Sandbox preview wires a mock; production wiring is
// pkg/bank / pkg/consensus.
type Oracle interface {
	Lookup(name string) (*big.Rat, bool)
}

// MapOracle is the simplest Oracle: a static value table, sufficient for
// ShadeLang's "Run 100 mock TXs" sandbox preview (spec 13.2).
type MapOracle map[string]*big.Rat

func (m MapOracle) Lookup(name string) (*big.Rat, bool) {
	v, ok := m[name]
	return v, ok
}

// Interpreter walks an ast.Program and evaluates it against an Env + Oracle,
// collecting side-effect events for the caller to inspect (sandbox preview)
// or assert on (tests).
type Interpreter struct {
	Env         *Env
	Oracle      Oracle
	FeeRoutes   []FeeRouteEvent
	Txs         []TxEvent
	Deposits    []BankDepositEvent
	QueueInsert []QueueInsertEvent
}

func New(oracle Oracle) *Interpreter {
	if oracle == nil {
		oracle = MapOracle{}
	}
	return &Interpreter{Env: NewEnv(), Oracle: oracle}
}

// Run executes every top-level statement in order. It stops and returns the
// first runtime error encountered (undefined identifier, division by zero,
// or an unsupported node), matching the atomicity philosophy of the L1
// pipeline: nothing partially commits.
func (it *Interpreter) Run(prog *ast.Program) error {
	return it.runStatements(prog.Statements)
}

func (it *Interpreter) runStatements(stmts []ast.Statement) error {
	for _, s := range stmts {
		if err := it.runStatement(s); err != nil {
			return err
		}
	}
	return nil
}

func (it *Interpreter) runStatement(s ast.Statement) error {
	switch n := s.(type) {
	case *ast.Assignment:
		v, err := it.eval(n.Value)
		if err != nil {
			return fmt.Errorf("assignment %s: %w", n.Name, err)
		}
		it.Env.Set(n.Name, v)
		return nil

	case *ast.IfStatement:
		v, err := it.eval(n.Condition)
		if err != nil {
			return fmt.Errorf("if condition: %w", err)
		}
		if v.Sign() != 0 {
			return it.runStatements(n.Body)
		}
		return nil

	case *ast.TxStatement:
		amt, err := it.eval(n.Amount)
		if err != nil {
			return fmt.Errorf("tx %s amount: %w", n.Name, err)
		}
		it.Env.Set("amount", amt)
		it.Txs = append(it.Txs, TxEvent{Name: n.Name, From: n.From, To: n.To, Amount: amt})
		return it.runStatements(n.Body)

	case *ast.MintStatement:
		if _, err := it.eval(n.Amount); err != nil {
			return fmt.Errorf("mint %s amount: %w", n.Name, err)
		}
		if n.Epoch != nil {
			if _, err := it.eval(n.Epoch); err != nil {
				return fmt.Errorf("mint %s epoch: %w", n.Name, err)
			}
		}
		return nil

	case *ast.ValidateStatement:
		return it.runStatements(n.Body)

	case *ast.QueueInsertStatement:
		vals := make([]*big.Rat, 0, len(n.Positions))
		for _, p := range n.Positions {
			v, err := it.eval(p)
			if err != nil {
				return fmt.Errorf("queue insert %s: %w", n.Name, err)
			}
			vals = append(vals, v)
		}
		it.QueueInsert = append(it.QueueInsert, QueueInsertEvent{Name: n.Name, Positions: vals})
		return nil

	case *ast.BankDepositStatement:
		atr, err := it.eval(n.ATR)
		if err != nil {
			return fmt.Errorf("bank deposit %s: %w", n.Name, err)
		}
		it.Deposits = append(it.Deposits, BankDepositEvent{Name: n.Name, ATR: atr})
		return nil

	case *ast.ContainerStatement, *ast.NetworkStatement, *ast.AsyncStaggerStatement:
		return nil // declarative config blocks; no runtime effect in the sandbox

	case *ast.ResilienceStatement:
		v, err := it.eval(n.Condition)
		if err != nil {
			return fmt.Errorf("resilience condition: %w", err)
		}
		if v.Sign() != 0 {
			return it.runStatements(n.Body)
		}
		return nil

	case *ast.ActivateSentinelsStatement:
		return nil

	case *ast.UpdateTraitStatement:
		v, err := it.eval(n.Value)
		if err != nil {
			return fmt.Errorf("update_trait %s.%s: %w", n.Target, n.Key, err)
		}
		key := n.Target + "." + n.Key
		switch n.Op {
		case "+=":
			cur, _ := it.Env.Get(key)
			if cur == nil {
				cur = new(big.Rat)
			}
			v = new(big.Rat).Add(cur, v)
		case "-=":
			cur, _ := it.Env.Get(key)
			if cur == nil {
				cur = new(big.Rat)
			}
			v = new(big.Rat).Sub(cur, v)
		}
		it.Env.Set(key, v)
		return nil

	case *ast.VoteStatement:
		return nil

	case *ast.ShardStatement:
		if n.Count != nil {
			if _, err := it.eval(n.Count); err != nil {
				return fmt.Errorf("shard %s count: %w", n.Name, err)
			}
		}
		return nil

	default:
		return fmt.Errorf("interp: unsupported statement %T", s)
	}
}

func (it *Interpreter) eval(e ast.Expr) (*big.Rat, error) {
	switch n := e.(type) {
	case *ast.NumberLit:
		v, ok := new(big.Rat).SetString(n.Text)
		if !ok {
			return nil, fmt.Errorf("invalid number literal %q", n.Text)
		}
		return v, nil

	case *ast.Ident:
		if v, ok := it.Env.Get(n.Name); ok {
			return v, nil
		}
		if v, ok := it.Oracle.Lookup(n.Name); ok {
			return v, nil
		}
		return nil, fmt.Errorf("undefined identifier %q", n.Name)

	case *ast.FeeRoute:
		v, err := it.eval(n.Value)
		if err != nil {
			return nil, err
		}
		it.FeeRoutes = append(it.FeeRoutes, FeeRouteEvent{To: n.To, Amount: v})
		return v, nil

	case *ast.Binary:
		l, err := it.eval(n.Left)
		if err != nil {
			return nil, err
		}
		r, err := it.eval(n.Right)
		if err != nil {
			return nil, err
		}
		return evalBinary(n.Op, l, r)

	default:
		return nil, fmt.Errorf("interp: unsupported expr %T", e)
	}
}

func evalBinary(op string, l, r *big.Rat) (*big.Rat, error) {
	out := new(big.Rat)
	switch op {
	case "+":
		return out.Add(l, r), nil
	case "-":
		return out.Sub(l, r), nil
	case "*":
		return out.Mul(l, r), nil
	case "/":
		if r.Sign() == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return out.Quo(l, r), nil
	case ">", ">=", "<", "<=", "==", "!=":
		return boolRat(compareRat(op, l, r)), nil
	default:
		return nil, fmt.Errorf("unknown operator %q", op)
	}
}

func compareRat(op string, l, r *big.Rat) bool {
	c := l.Cmp(r)
	switch op {
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case "==":
		return c == 0
	case "!=":
		return c != 0
	}
	return false
}

func boolRat(b bool) *big.Rat {
	if b {
		return big.NewRat(1, 1)
	}
	return new(big.Rat)
}
