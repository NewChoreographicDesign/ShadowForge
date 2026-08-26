// Code generated from grammar/ShadowRustExt.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // ShadowRustExt
import "github.com/antlr4-go/antlr/v4"

// ShadowRustExtListener is a complete listener for a parse tree produced by ShadowRustExtParser.
type ShadowRustExtListener interface {
	antlr.ParseTreeListener

	// EnterStatement is called when entering the statement production.
	EnterStatement(c *StatementContext)

	// EnterContainerStatement is called when entering the containerStatement production.
	EnterContainerStatement(c *ContainerStatementContext)

	// EnterContainerField is called when entering the containerField production.
	EnterContainerField(c *ContainerFieldContext)

	// EnterNetworkStatement is called when entering the networkStatement production.
	EnterNetworkStatement(c *NetworkStatementContext)

	// EnterNetworkField is called when entering the networkField production.
	EnterNetworkField(c *NetworkFieldContext)

	// EnterResilienceStatement is called when entering the resilienceStatement production.
	EnterResilienceStatement(c *ResilienceStatementContext)

	// EnterActivateStatement is called when entering the activateStatement production.
	EnterActivateStatement(c *ActivateStatementContext)

	// EnterUpdateTraitStatement is called when entering the updateTraitStatement production.
	EnterUpdateTraitStatement(c *UpdateTraitStatementContext)

	// EnterVoteStatement is called when entering the voteStatement production.
	EnterVoteStatement(c *VoteStatementContext)

	// EnterShardStatement is called when entering the shardStatement production.
	EnterShardStatement(c *ShardStatementContext)

	// EnterAsyncStaggerStatement is called when entering the asyncStaggerStatement production.
	EnterAsyncStaggerStatement(c *AsyncStaggerStatementContext)

	// EnterStaggerField is called when entering the staggerField production.
	EnterStaggerField(c *StaggerFieldContext)

	// EnterFactor is called when entering the factor production.
	EnterFactor(c *FactorContext)

	// EnterProgram is called when entering the program production.
	EnterProgram(c *ProgramContext)

	// EnterIfStatement is called when entering the ifStatement production.
	EnterIfStatement(c *IfStatementContext)

	// EnterTxStatement is called when entering the txStatement production.
	EnterTxStatement(c *TxStatementContext)

	// EnterMintStatement is called when entering the mintStatement production.
	EnterMintStatement(c *MintStatementContext)

	// EnterValidateStatement is called when entering the validateStatement production.
	EnterValidateStatement(c *ValidateStatementContext)

	// EnterQueueStatement is called when entering the queueStatement production.
	EnterQueueStatement(c *QueueStatementContext)

	// EnterBankStatement is called when entering the bankStatement production.
	EnterBankStatement(c *BankStatementContext)

	// EnterAssignment is called when entering the assignment production.
	EnterAssignment(c *AssignmentContext)

	// EnterCondition is called when entering the condition production.
	EnterCondition(c *ConditionContext)

	// EnterExpr is called when entering the expr production.
	EnterExpr(c *ExprContext)

	// EnterRelExpr is called when entering the relExpr production.
	EnterRelExpr(c *RelExprContext)

	// EnterArithExpr is called when entering the arithExpr production.
	EnterArithExpr(c *ArithExprContext)

	// EnterTerm is called when entering the term production.
	EnterTerm(c *TermContext)

	// ExitStatement is called when exiting the statement production.
	ExitStatement(c *StatementContext)

	// ExitContainerStatement is called when exiting the containerStatement production.
	ExitContainerStatement(c *ContainerStatementContext)

	// ExitContainerField is called when exiting the containerField production.
	ExitContainerField(c *ContainerFieldContext)

	// ExitNetworkStatement is called when exiting the networkStatement production.
	ExitNetworkStatement(c *NetworkStatementContext)

	// ExitNetworkField is called when exiting the networkField production.
	ExitNetworkField(c *NetworkFieldContext)

	// ExitResilienceStatement is called when exiting the resilienceStatement production.
	ExitResilienceStatement(c *ResilienceStatementContext)

	// ExitActivateStatement is called when exiting the activateStatement production.
	ExitActivateStatement(c *ActivateStatementContext)

	// ExitUpdateTraitStatement is called when exiting the updateTraitStatement production.
	ExitUpdateTraitStatement(c *UpdateTraitStatementContext)

	// ExitVoteStatement is called when exiting the voteStatement production.
	ExitVoteStatement(c *VoteStatementContext)

	// ExitShardStatement is called when exiting the shardStatement production.
	ExitShardStatement(c *ShardStatementContext)

	// ExitAsyncStaggerStatement is called when exiting the asyncStaggerStatement production.
	ExitAsyncStaggerStatement(c *AsyncStaggerStatementContext)

	// ExitStaggerField is called when exiting the staggerField production.
	ExitStaggerField(c *StaggerFieldContext)

	// ExitFactor is called when exiting the factor production.
	ExitFactor(c *FactorContext)

	// ExitProgram is called when exiting the program production.
	ExitProgram(c *ProgramContext)

	// ExitIfStatement is called when exiting the ifStatement production.
	ExitIfStatement(c *IfStatementContext)

	// ExitTxStatement is called when exiting the txStatement production.
	ExitTxStatement(c *TxStatementContext)

	// ExitMintStatement is called when exiting the mintStatement production.
	ExitMintStatement(c *MintStatementContext)

	// ExitValidateStatement is called when exiting the validateStatement production.
	ExitValidateStatement(c *ValidateStatementContext)

	// ExitQueueStatement is called when exiting the queueStatement production.
	ExitQueueStatement(c *QueueStatementContext)

	// ExitBankStatement is called when exiting the bankStatement production.
	ExitBankStatement(c *BankStatementContext)

	// ExitAssignment is called when exiting the assignment production.
	ExitAssignment(c *AssignmentContext)

	// ExitCondition is called when exiting the condition production.
	ExitCondition(c *ConditionContext)

	// ExitExpr is called when exiting the expr production.
	ExitExpr(c *ExprContext)

	// ExitRelExpr is called when exiting the relExpr production.
	ExitRelExpr(c *RelExprContext)

	// ExitArithExpr is called when exiting the arithExpr production.
	ExitArithExpr(c *ArithExprContext)

	// ExitTerm is called when exiting the term production.
	ExitTerm(c *TermContext)
}
