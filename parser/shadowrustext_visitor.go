// Code generated from grammar/ShadowRustExt.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // ShadowRustExt
import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by ShadowRustExtParser.
type ShadowRustExtVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by ShadowRustExtParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#containerStatement.
	VisitContainerStatement(ctx *ContainerStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#containerField.
	VisitContainerField(ctx *ContainerFieldContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#networkStatement.
	VisitNetworkStatement(ctx *NetworkStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#networkField.
	VisitNetworkField(ctx *NetworkFieldContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#resilienceStatement.
	VisitResilienceStatement(ctx *ResilienceStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#activateStatement.
	VisitActivateStatement(ctx *ActivateStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#updateTraitStatement.
	VisitUpdateTraitStatement(ctx *UpdateTraitStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#voteStatement.
	VisitVoteStatement(ctx *VoteStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#shardStatement.
	VisitShardStatement(ctx *ShardStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#asyncStaggerStatement.
	VisitAsyncStaggerStatement(ctx *AsyncStaggerStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#staggerField.
	VisitStaggerField(ctx *StaggerFieldContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#factor.
	VisitFactor(ctx *FactorContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#program.
	VisitProgram(ctx *ProgramContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#ifStatement.
	VisitIfStatement(ctx *IfStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#txStatement.
	VisitTxStatement(ctx *TxStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#mintStatement.
	VisitMintStatement(ctx *MintStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#validateStatement.
	VisitValidateStatement(ctx *ValidateStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#queueStatement.
	VisitQueueStatement(ctx *QueueStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#bankStatement.
	VisitBankStatement(ctx *BankStatementContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#assignment.
	VisitAssignment(ctx *AssignmentContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#condition.
	VisitCondition(ctx *ConditionContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#expr.
	VisitExpr(ctx *ExprContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#relExpr.
	VisitRelExpr(ctx *RelExprContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#arithExpr.
	VisitArithExpr(ctx *ArithExprContext) interface{}

	// Visit a parse tree produced by ShadowRustExtParser#term.
	VisitTerm(ctx *TermContext) interface{}
}
