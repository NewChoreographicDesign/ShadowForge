package ast

import (
	"strconv"

	"github.com/antlr4-go/antlr/v4"
	"github.com/shadowforge/shadowforge-l1/parser"
)

// Build walks an ANTLR parse tree produced by parser.ShadowRustExtParser and
// constructs the typed AST described in ast.go. This is the "Visitor builds a
// typed AST" step of the pipeline (spec 14.1).
func Build(tree parser.IProgramContext) *Program {
	p := &Program{}
	for _, s := range tree.AllStatement() {
		if st := buildStatement(s); st != nil {
			p.Statements = append(p.Statements, st)
		}
	}
	return p
}

func buildStatements(ctxs []parser.IStatementContext) []Statement {
	out := make([]Statement, 0, len(ctxs))
	for _, c := range ctxs {
		if st := buildStatement(c); st != nil {
			out = append(out, st)
		}
	}
	return out
}

func buildStatement(ctx parser.IStatementContext) Statement {
	s, ok := ctx.(*parser.StatementContext)
	if !ok || s == nil {
		return nil
	}
	switch {
	case s.IfStatement() != nil:
		return buildIfStatement(s.IfStatement().(*parser.IfStatementContext))
	case s.TxStatement() != nil:
		return buildTxStatement(s.TxStatement().(*parser.TxStatementContext))
	case s.MintStatement() != nil:
		return buildMintStatement(s.MintStatement().(*parser.MintStatementContext))
	case s.ValidateStatement() != nil:
		return buildValidateStatement(s.ValidateStatement().(*parser.ValidateStatementContext))
	case s.QueueStatement() != nil:
		return buildQueueStatement(s.QueueStatement().(*parser.QueueStatementContext))
	case s.BankStatement() != nil:
		return buildBankStatement(s.BankStatement().(*parser.BankStatementContext))
	case s.ContainerStatement() != nil:
		return buildContainerStatement(s.ContainerStatement().(*parser.ContainerStatementContext))
	case s.NetworkStatement() != nil:
		return buildNetworkStatement(s.NetworkStatement().(*parser.NetworkStatementContext))
	case s.ResilienceStatement() != nil:
		return buildResilienceStatement(s.ResilienceStatement().(*parser.ResilienceStatementContext))
	case s.ActivateStatement() != nil:
		return &ActivateSentinelsStatement{}
	case s.UpdateTraitStatement() != nil:
		return buildUpdateTraitStatement(s.UpdateTraitStatement().(*parser.UpdateTraitStatementContext))
	case s.VoteStatement() != nil:
		return buildVoteStatement(s.VoteStatement().(*parser.VoteStatementContext))
	case s.ShardStatement() != nil:
		return buildShardStatement(s.ShardStatement().(*parser.ShardStatementContext))
	case s.AsyncStaggerStatement() != nil:
		return buildAsyncStaggerStatement(s.AsyncStaggerStatement().(*parser.AsyncStaggerStatementContext))
	case s.Assignment() != nil:
		return buildAssignment(s.Assignment().(*parser.AssignmentContext))
	}
	return nil
}

func buildIfStatement(ctx *parser.IfStatementContext) *IfStatement {
	return &IfStatement{
		Condition: buildExpr(ctx.Condition().(*parser.ConditionContext).Expr()),
		Body:      buildStatements(ctx.AllStatement()),
	}
}

func buildTxStatement(ctx *parser.TxStatementContext) *TxStatement {
	ids := ctx.AllID()
	tx := &TxStatement{Body: buildStatements(ctx.AllStatement())}
	if len(ids) > 0 {
		tx.Name = ids[0].GetText()
	}
	if len(ids) > 1 {
		tx.From = ids[1].GetText()
	}
	if len(ids) > 2 {
		tx.To = ids[2].GetText()
	}
	tx.Amount = buildExpr(ctx.Expr())
	return tx
}

func buildMintStatement(ctx *parser.MintStatementContext) *MintStatement {
	m := &MintStatement{Name: ctx.ID().GetText()}
	exprs := ctx.AllExpr()
	if len(exprs) > 0 {
		m.Amount = buildExpr(exprs[0])
	}
	if len(exprs) > 1 {
		m.Epoch = buildExpr(exprs[1])
	}
	return m
}

func buildValidateStatement(ctx *parser.ValidateStatementContext) *ValidateStatement {
	stage, _ := strconv.Atoi(ctx.NUMBER().GetText())
	return &ValidateStatement{
		Name:  ctx.ID().GetText(),
		Stage: stage,
		Body:  buildStatements(ctx.AllStatement()),
	}
}

func buildQueueStatement(ctx *parser.QueueStatementContext) *QueueInsertStatement {
	q := &QueueInsertStatement{Name: ctx.ID().GetText()}
	for _, e := range ctx.AllExpr() {
		q.Positions = append(q.Positions, buildExpr(e))
	}
	return q
}

func buildBankStatement(ctx *parser.BankStatementContext) *BankDepositStatement {
	return &BankDepositStatement{
		Name: ctx.ID().GetText(),
		ATR:  buildExpr(ctx.Expr()),
	}
}

func buildAssignment(ctx *parser.AssignmentContext) *Assignment {
	return &Assignment{Name: ctx.ID().GetText(), Value: buildExpr(ctx.Expr())}
}

func buildFields(ctxs []parser.IContainerFieldContext) []Field {
	out := make([]Field, 0, len(ctxs))
	for _, c := range ctxs {
		out = append(out, Field{Key: c.ID().GetText(), Value: buildExpr(c.Expr())})
	}
	return out
}

func buildContainerStatement(ctx *parser.ContainerStatementContext) *ContainerStatement {
	return &ContainerStatement{Fields: buildFields(ctx.AllContainerField())}
}

func buildNetworkStatement(ctx *parser.NetworkStatementContext) *NetworkStatement {
	out := make([]Field, 0, len(ctx.AllNetworkField()))
	for _, c := range ctx.AllNetworkField() {
		out = append(out, Field{Key: c.ID().GetText(), Value: buildExpr(c.Expr())})
	}
	return &NetworkStatement{Fields: out}
}

func buildResilienceStatement(ctx *parser.ResilienceStatementContext) *ResilienceStatement {
	return &ResilienceStatement{
		Condition: buildExpr(ctx.Condition().(*parser.ConditionContext).Expr()),
		Body:      buildStatements(ctx.AllStatement()),
	}
}

func buildUpdateTraitStatement(ctx *parser.UpdateTraitStatementContext) *UpdateTraitStatement {
	ids := ctx.AllID()
	u := &UpdateTraitStatement{Value: buildExpr(ctx.Expr())}
	if len(ids) > 0 {
		u.Target = ids[0].GetText()
	}
	if len(ids) > 1 {
		u.Key = ids[1].GetText()
	}
	if ctx.GetOp() != nil {
		u.Op = ctx.GetOp().GetText()
	}
	return u
}

func buildVoteStatement(ctx *parser.VoteStatementContext) *VoteStatement {
	ids := ctx.AllID()
	v := &VoteStatement{}
	if len(ids) > 0 {
		v.Proposal = ids[0].GetText()
	}
	if len(ids) > 1 {
		v.Commitment = ids[1].GetText()
	}
	return v
}

func buildShardStatement(ctx *parser.ShardStatementContext) *ShardStatement {
	s := &ShardStatement{Name: ctx.ID().GetText()}
	if ctx.COUNT() != nil {
		s.Count = buildExpr(ctx.Expr())
	}
	return s
}

func buildAsyncStaggerStatement(ctx *parser.AsyncStaggerStatementContext) *AsyncStaggerStatement {
	out := make([]Field, 0, len(ctx.AllStaggerField()))
	for _, c := range ctx.AllStaggerField() {
		out = append(out, Field{Key: c.ID().GetText(), Value: buildExpr(c.Expr())})
	}
	return &AsyncStaggerStatement{Fields: out}
}

// ---- Expressions ----

func buildExpr(ctx parser.IExprContext) Expr {
	e, ok := ctx.(*parser.ExprContext)
	if !ok || e == nil {
		return nil
	}
	rel := buildRelExpr(e.RelExpr())
	if e.TO() != nil && e.ID() != nil {
		return &FeeRoute{Value: rel, To: e.ID().GetText()}
	}
	return rel
}

// operatorsOf returns the direct TerminalNode children of a rule context, in
// order. For relExpr/arithExpr/term these are exactly the operator tokens,
// since operands are always sub-rule contexts, never bare terminals.
func operatorsOf(ctx antlr.RuleContext) []string {
	var ops []string
	rc, ok := ctx.(antlr.RuleNode)
	if !ok {
		return ops
	}
	for i := 0; i < rc.GetChildCount(); i++ {
		if tn, ok := rc.GetChild(i).(antlr.TerminalNode); ok {
			ops = append(ops, tn.GetText())
		}
	}
	return ops
}

func leftAssoc(operands []Expr, ops []string) Expr {
	if len(operands) == 0 {
		return nil
	}
	result := operands[0]
	for i, op := range ops {
		if i+1 >= len(operands) {
			break
		}
		result = &Binary{Op: op, Left: result, Right: operands[i+1]}
	}
	return result
}

func buildRelExpr(ctx parser.IRelExprContext) Expr {
	c := ctx.(*parser.RelExprContext)
	arith := c.AllArithExpr()
	operands := make([]Expr, len(arith))
	for i, a := range arith {
		operands[i] = buildArithExpr(a)
	}
	return leftAssoc(operands, operatorsOf(c))
}

func buildArithExpr(ctx parser.IArithExprContext) Expr {
	c := ctx.(*parser.ArithExprContext)
	terms := c.AllTerm()
	operands := make([]Expr, len(terms))
	for i, t := range terms {
		operands[i] = buildTerm(t)
	}
	return leftAssoc(operands, operatorsOf(c))
}

func buildTerm(ctx parser.ITermContext) Expr {
	c := ctx.(*parser.TermContext)
	factors := c.AllFactor()
	operands := make([]Expr, len(factors))
	for i, f := range factors {
		operands[i] = buildFactor(f)
	}
	return leftAssoc(operands, operatorsOf(c))
}

func buildFactor(ctx parser.IFactorContext) Expr {
	c := ctx.(*parser.FactorContext)
	if id := c.ID(); id != nil {
		return &Ident{Name: id.GetText()}
	}
	if amt := c.AMOUNT(); amt != nil {
		return &Ident{Name: amt.GetText()}
	}
	if num := c.NUMBER(); num != nil {
		return &NumberLit{Text: num.GetText()}
	}
	if e := c.Expr(); e != nil {
		return buildExpr(e)
	}
	return nil
}
