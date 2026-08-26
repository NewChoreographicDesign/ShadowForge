// Package codegen is the ShadowRust Go emitter (spec 14.1, 14.4, 14.6). It
// turns a sema-checked AST into a compilable Go source file: one function
// per top-level tx/mint/bank-deposit/queue-insert/update-trait statement,
// each calling the runtime.Node interface (codegen/runtime) that the real
// node wires up to gnark, Badger, and the 5-stage pipeline.
//
// Scope, honestly stated: this emits the statement kinds spec 14.4 gives an
// explicit code-generation contract for (tx buy, queue insert, bank
// deposit), plus mint and update_trait. container/network/resilience/vote/
// shard/async_stagger are declarative configuration, consumed directly by
// the node's config loader (pkg/container, pkg/net) rather than compiled
// per-statement; Generate skips them. A statement nested where this v0.1
// emitter cannot yet compile it (e.g. a tx nested inside another tx) is a
// hard codegen error rather than a silent miscompile.
package codegen

import (
	"fmt"
	"go/format"
	"sort"
	"strings"

	"github.com/shadowforge/shadowforge-l1/ast"
)

// Generate compiles prog into a single Go source file in package pkgName.
// Callers should run sema.Analyze first; Generate does not re-check safety.
func Generate(pkgName string, prog *ast.Program) (string, error) {
	g := &generator{pkgName: pkgName}
	for i, s := range prog.Statements {
		if err := g.topLevel(i, s); err != nil {
			return "", err
		}
	}
	src := g.render()
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", fmt.Errorf("codegen produced invalid Go: %w\n---\n%s", err, src)
	}
	return string(formatted), nil
}

type generator struct {
	pkgName string
	funcs   []string
}

func (g *generator) topLevel(idx int, s ast.Statement) error {
	switch n := s.(type) {
	case *ast.TxStatement:
		return g.genTx(idx, n)
	case *ast.MintStatement:
		return g.genMint(idx, n)
	case *ast.BankDepositStatement:
		return g.genBank(idx, n)
	case *ast.QueueInsertStatement:
		return g.genQueue(idx, n)
	case *ast.UpdateTraitStatement:
		return g.genUpdateTrait(idx, n)
	case *ast.ContainerStatement, *ast.NetworkStatement, *ast.ResilienceStatement,
		*ast.VoteStatement, *ast.ShardStatement, *ast.AsyncStaggerStatement,
		*ast.ValidateStatement, *ast.IfStatement, *ast.Assignment:
		return nil // declarative / top-level scratch — no function emitted
	default:
		return fmt.Errorf("codegen: unsupported top-level statement %T", s)
	}
}

func funcName(kind string, idx int, name string) string {
	name = sanitize(name)
	return fmt.Sprintf("%s%d_%s", kind, idx, name)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "anon"
	}
	return b.String()
}

// scope tracks which identifiers are already bound as Go locals within the
// function body being generated, so free identifiers can be turned into
// function parameters.
type scope struct {
	bound map[string]bool
	free  map[string]bool
}

func newScope(initial ...string) *scope {
	s := &scope{bound: map[string]bool{}, free: map[string]bool{}}
	for _, n := range initial {
		s.bound[n] = true
	}
	return s
}

func (s *scope) use(name string) {
	if !s.bound[name] {
		s.free[name] = true
	}
}

func (s *scope) bind(name string) { s.bound[name] = true }

func (s *scope) sortedFree() []string {
	out := make([]string, 0, len(s.free))
	for n := range s.free {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (g *generator) genTx(idx int, n *ast.TxStatement) error {
	fn := funcName("Tx", idx, n.Name)
	sc := newScope("amount")

	var body strings.Builder
	amtExpr, err := exprGo(n.Amount, sc)
	if err != nil {
		return fmt.Errorf("tx %s: amount: %w", n.Name, err)
	}
	fmt.Fprintf(&body, "\tamount := %s\n", amtExpr)

	feeTo, feeAmt := "\"\"", "nil"
	for _, st := range n.Body {
		switch bs := st.(type) {
		case *ast.Assignment:
			ex, err := exprGo(bs.Value, sc)
			if err != nil {
				return fmt.Errorf("tx %s: %s: %w", n.Name, bs.Name, err)
			}
			fmt.Fprintf(&body, "\t%s := %s\n", goVar(bs.Name), ex)
			sc.bind(bs.Name)
			if fr, ok := bs.Value.(*ast.FeeRoute); ok {
				feeTo = fmt.Sprintf("%q", fr.To)
				feeAmt = goVar(bs.Name)
			}
		default:
			return fmt.Errorf("tx %s: codegen v0.1 does not support nested %T in a tx body", n.Name, st)
		}
	}

	params := sc.sortedFree()
	sig := paramSig(params)
	call := fmt.Sprintf("func %s(node runtime.Node%s) (string, error) {\n%s\treturn node.Transfer(%q, %q, amount, %s, %s)\n}\n",
		fn, sig, body.String(), n.From, n.To, feeTo, feeAmt)
	g.funcs = append(g.funcs, call)
	return nil
}

func (g *generator) genMint(idx int, n *ast.MintStatement) error {
	fn := funcName("Mint", idx, n.Name)
	sc := newScope()
	amtExpr, err := exprGo(n.Amount, sc)
	if err != nil {
		return fmt.Errorf("mint %s: amount: %w", n.Name, err)
	}
	epochExpr := "nil"
	if n.Epoch != nil {
		e, err := exprGo(n.Epoch, sc)
		if err != nil {
			return fmt.Errorf("mint %s: epoch: %w", n.Name, err)
		}
		epochExpr = e
	}
	sig := paramSig(sc.sortedFree())
	g.funcs = append(g.funcs, fmt.Sprintf(
		"func %s(node runtime.Node%s) error {\n\treturn node.MintProposal(%q, %s, %s)\n}\n",
		fn, sig, n.Name, amtExpr, epochExpr))
	return nil
}

func (g *generator) genBank(idx int, n *ast.BankDepositStatement) error {
	fn := funcName("Bank", idx, n.Name)
	sc := newScope()
	atrExpr, err := exprGo(n.ATR, sc)
	if err != nil {
		return fmt.Errorf("bank deposit %s: %w", n.Name, err)
	}
	sig := paramSig(sc.sortedFree())
	g.funcs = append(g.funcs, fmt.Sprintf(
		"func %s(node runtime.Node%s) (string, error) {\n\treturn node.BankDeposit(%q, %s)\n}\n",
		fn, sig, n.Name, atrExpr))
	return nil
}

func (g *generator) genQueue(idx int, n *ast.QueueInsertStatement) error {
	fn := funcName("Queue", idx, n.Name)
	var lits []string
	for _, p := range n.Positions {
		nl, ok := p.(*ast.NumberLit)
		if !ok {
			return fmt.Errorf("queue insert %s: codegen v0.1 requires literal positions, got %T", n.Name, p)
		}
		lits = append(lits, nl.Text)
	}
	g.funcs = append(g.funcs, fmt.Sprintf(
		"func %s(node runtime.Node) error {\n\treturn node.QueueInsert(%q, []int{%s})\n}\n",
		fn, n.Name, strings.Join(intLiterals(lits), ", ")))
	return nil
}

func intLiterals(nums []string) []string {
	out := make([]string, len(nums))
	for i, n := range nums {
		// Positions are integers per spec 5.4.1; truncate any fractional text.
		if dot := strings.IndexByte(n, '.'); dot >= 0 {
			n = n[:dot]
		}
		out[i] = n
	}
	return out
}

func (g *generator) genUpdateTrait(idx int, n *ast.UpdateTraitStatement) error {
	fn := funcName("Trait", idx, n.Target+"_"+n.Key)
	sc := newScope()
	valExpr, err := exprGo(n.Value, sc)
	if err != nil {
		return fmt.Errorf("update_trait %s.%s: %w", n.Target, n.Key, err)
	}
	sig := paramSig(sc.sortedFree())
	g.funcs = append(g.funcs, fmt.Sprintf(
		"func %s(node runtime.Node%s) error {\n\treturn node.UpdateTrait(%q, %q, %q, %s)\n}\n",
		fn, sig, n.Target, n.Key, n.Op, valExpr))
	return nil
}

func paramSig(free []string) string {
	if len(free) == 0 {
		return ""
	}
	parts := make([]string, len(free))
	for i, f := range free {
		parts[i] = goVar(f) + " *big.Rat"
	}
	return ", " + strings.Join(parts, ", ")
}

func goVar(name string) string { return sanitize(name) }

func exprGo(e ast.Expr, sc *scope) (string, error) {
	switch n := e.(type) {
	case *ast.NumberLit:
		return fmt.Sprintf("runtime.N(%q)", n.Text), nil
	case *ast.Ident:
		v := goVar(n.Name)
		sc.use(n.Name)
		return v, nil
	case *ast.Binary:
		l, err := exprGo(n.Left, sc)
		if err != nil {
			return "", err
		}
		r, err := exprGo(n.Right, sc)
		if err != nil {
			return "", err
		}
		switch n.Op {
		case "+":
			return fmt.Sprintf("runtime.Add(%s, %s)", l, r), nil
		case "-":
			return fmt.Sprintf("runtime.Sub(%s, %s)", l, r), nil
		case "*":
			return fmt.Sprintf("runtime.Mul(%s, %s)", l, r), nil
		case "/":
			return fmt.Sprintf("runtime.Div(%s, %s)", l, r), nil
		case ">", ">=", "<", "<=", "==", "!=":
			return fmt.Sprintf("runtime.Cmp(%q, %s, %s)", n.Op, l, r), nil
		}
		return "", fmt.Errorf("unknown operator %q", n.Op)
	case *ast.FeeRoute:
		return exprGo(n.Value, sc)
	default:
		return "", fmt.Errorf("codegen: unsupported expr %T", e)
	}
}

func (g *generator) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by shadowc (ShadowRust codegen). DO NOT EDIT.\n")
	fmt.Fprintf(&b, "package %s\n\n", g.pkgName)
	if len(g.funcs) > 0 {
		fmt.Fprintf(&b, "import (\n\t\"math/big\"\n\n\t\"github.com/shadowforge/shadowforge-l1/codegen/runtime\"\n)\n\n")
		// Guaranteed reference so packages compile even when no emitted
		// function happens to need a *big.Rat parameter.
		fmt.Fprintf(&b, "var _ = big.NewRat\nvar _ runtime.Node\n\n")
	}
	for _, f := range g.funcs {
		b.WriteString(f)
		b.WriteString("\n")
	}
	return b.String()
}
