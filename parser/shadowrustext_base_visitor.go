// Code generated from grammar/ShadowRustExt.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // ShadowRustExt
import "github.com/antlr4-go/antlr/v4"

type BaseShadowRustExtVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseShadowRustExtVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitContainerStatement(ctx *ContainerStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitContainerField(ctx *ContainerFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitNetworkStatement(ctx *NetworkStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitNetworkField(ctx *NetworkFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitResilienceStatement(ctx *ResilienceStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitActivateStatement(ctx *ActivateStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitUpdateTraitStatement(ctx *UpdateTraitStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitVoteStatement(ctx *VoteStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitShardStatement(ctx *ShardStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitAsyncStaggerStatement(ctx *AsyncStaggerStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitStaggerField(ctx *StaggerFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitFactor(ctx *FactorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitProgram(ctx *ProgramContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitIfStatement(ctx *IfStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitTxStatement(ctx *TxStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitMintStatement(ctx *MintStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitValidateStatement(ctx *ValidateStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitQueueStatement(ctx *QueueStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitBankStatement(ctx *BankStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitAssignment(ctx *AssignmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitCondition(ctx *ConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitExpr(ctx *ExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitRelExpr(ctx *RelExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitArithExpr(ctx *ArithExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseShadowRustExtVisitor) VisitTerm(ctx *TermContext) interface{} {
	return v.VisitChildren(ctx)
}
