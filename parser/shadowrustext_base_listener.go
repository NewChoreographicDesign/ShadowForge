// Code generated from grammar/ShadowRustExt.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // ShadowRustExt
import "github.com/antlr4-go/antlr/v4"

// BaseShadowRustExtListener is a complete listener for a parse tree produced by ShadowRustExtParser.
type BaseShadowRustExtListener struct{}

var _ ShadowRustExtListener = &BaseShadowRustExtListener{}

// VisitTerminal is called when a terminal node is visited.
func (s *BaseShadowRustExtListener) VisitTerminal(node antlr.TerminalNode) {}

// VisitErrorNode is called when an error node is visited.
func (s *BaseShadowRustExtListener) VisitErrorNode(node antlr.ErrorNode) {}

// EnterEveryRule is called when any rule is entered.
func (s *BaseShadowRustExtListener) EnterEveryRule(ctx antlr.ParserRuleContext) {}

// ExitEveryRule is called when any rule is exited.
func (s *BaseShadowRustExtListener) ExitEveryRule(ctx antlr.ParserRuleContext) {}

// EnterStatement is called when production statement is entered.
func (s *BaseShadowRustExtListener) EnterStatement(ctx *StatementContext) {}

// ExitStatement is called when production statement is exited.
func (s *BaseShadowRustExtListener) ExitStatement(ctx *StatementContext) {}

// EnterContainerStatement is called when production containerStatement is entered.
func (s *BaseShadowRustExtListener) EnterContainerStatement(ctx *ContainerStatementContext) {}

// ExitContainerStatement is called when production containerStatement is exited.
func (s *BaseShadowRustExtListener) ExitContainerStatement(ctx *ContainerStatementContext) {}

// EnterContainerField is called when production containerField is entered.
func (s *BaseShadowRustExtListener) EnterContainerField(ctx *ContainerFieldContext) {}

// ExitContainerField is called when production containerField is exited.
func (s *BaseShadowRustExtListener) ExitContainerField(ctx *ContainerFieldContext) {}

// EnterNetworkStatement is called when production networkStatement is entered.
func (s *BaseShadowRustExtListener) EnterNetworkStatement(ctx *NetworkStatementContext) {}

// ExitNetworkStatement is called when production networkStatement is exited.
func (s *BaseShadowRustExtListener) ExitNetworkStatement(ctx *NetworkStatementContext) {}

// EnterNetworkField is called when production networkField is entered.
func (s *BaseShadowRustExtListener) EnterNetworkField(ctx *NetworkFieldContext) {}

// ExitNetworkField is called when production networkField is exited.
func (s *BaseShadowRustExtListener) ExitNetworkField(ctx *NetworkFieldContext) {}

// EnterResilienceStatement is called when production resilienceStatement is entered.
func (s *BaseShadowRustExtListener) EnterResilienceStatement(ctx *ResilienceStatementContext) {}

// ExitResilienceStatement is called when production resilienceStatement is exited.
func (s *BaseShadowRustExtListener) ExitResilienceStatement(ctx *ResilienceStatementContext) {}

// EnterActivateStatement is called when production activateStatement is entered.
func (s *BaseShadowRustExtListener) EnterActivateStatement(ctx *ActivateStatementContext) {}

// ExitActivateStatement is called when production activateStatement is exited.
func (s *BaseShadowRustExtListener) ExitActivateStatement(ctx *ActivateStatementContext) {}

// EnterUpdateTraitStatement is called when production updateTraitStatement is entered.
func (s *BaseShadowRustExtListener) EnterUpdateTraitStatement(ctx *UpdateTraitStatementContext) {}

// ExitUpdateTraitStatement is called when production updateTraitStatement is exited.
func (s *BaseShadowRustExtListener) ExitUpdateTraitStatement(ctx *UpdateTraitStatementContext) {}

// EnterVoteStatement is called when production voteStatement is entered.
func (s *BaseShadowRustExtListener) EnterVoteStatement(ctx *VoteStatementContext) {}

// ExitVoteStatement is called when production voteStatement is exited.
func (s *BaseShadowRustExtListener) ExitVoteStatement(ctx *VoteStatementContext) {}

// EnterShardStatement is called when production shardStatement is entered.
func (s *BaseShadowRustExtListener) EnterShardStatement(ctx *ShardStatementContext) {}

// ExitShardStatement is called when production shardStatement is exited.
func (s *BaseShadowRustExtListener) ExitShardStatement(ctx *ShardStatementContext) {}

// EnterAsyncStaggerStatement is called when production asyncStaggerStatement is entered.
func (s *BaseShadowRustExtListener) EnterAsyncStaggerStatement(ctx *AsyncStaggerStatementContext) {}

// ExitAsyncStaggerStatement is called when production asyncStaggerStatement is exited.
func (s *BaseShadowRustExtListener) ExitAsyncStaggerStatement(ctx *AsyncStaggerStatementContext) {}

// EnterStaggerField is called when production staggerField is entered.
func (s *BaseShadowRustExtListener) EnterStaggerField(ctx *StaggerFieldContext) {}

// ExitStaggerField is called when production staggerField is exited.
func (s *BaseShadowRustExtListener) ExitStaggerField(ctx *StaggerFieldContext) {}

// EnterFactor is called when production factor is entered.
func (s *BaseShadowRustExtListener) EnterFactor(ctx *FactorContext) {}

// ExitFactor is called when production factor is exited.
func (s *BaseShadowRustExtListener) ExitFactor(ctx *FactorContext) {}

// EnterProgram is called when production program is entered.
func (s *BaseShadowRustExtListener) EnterProgram(ctx *ProgramContext) {}

// ExitProgram is called when production program is exited.
func (s *BaseShadowRustExtListener) ExitProgram(ctx *ProgramContext) {}

// EnterIfStatement is called when production ifStatement is entered.
func (s *BaseShadowRustExtListener) EnterIfStatement(ctx *IfStatementContext) {}

// ExitIfStatement is called when production ifStatement is exited.
func (s *BaseShadowRustExtListener) ExitIfStatement(ctx *IfStatementContext) {}

// EnterTxStatement is called when production txStatement is entered.
func (s *BaseShadowRustExtListener) EnterTxStatement(ctx *TxStatementContext) {}

// ExitTxStatement is called when production txStatement is exited.
func (s *BaseShadowRustExtListener) ExitTxStatement(ctx *TxStatementContext) {}

// EnterMintStatement is called when production mintStatement is entered.
func (s *BaseShadowRustExtListener) EnterMintStatement(ctx *MintStatementContext) {}

// ExitMintStatement is called when production mintStatement is exited.
func (s *BaseShadowRustExtListener) ExitMintStatement(ctx *MintStatementContext) {}

// EnterValidateStatement is called when production validateStatement is entered.
func (s *BaseShadowRustExtListener) EnterValidateStatement(ctx *ValidateStatementContext) {}

// ExitValidateStatement is called when production validateStatement is exited.
func (s *BaseShadowRustExtListener) ExitValidateStatement(ctx *ValidateStatementContext) {}

// EnterQueueStatement is called when production queueStatement is entered.
func (s *BaseShadowRustExtListener) EnterQueueStatement(ctx *QueueStatementContext) {}

// ExitQueueStatement is called when production queueStatement is exited.
func (s *BaseShadowRustExtListener) ExitQueueStatement(ctx *QueueStatementContext) {}

// EnterBankStatement is called when production bankStatement is entered.
func (s *BaseShadowRustExtListener) EnterBankStatement(ctx *BankStatementContext) {}

// ExitBankStatement is called when production bankStatement is exited.
func (s *BaseShadowRustExtListener) ExitBankStatement(ctx *BankStatementContext) {}

// EnterAssignment is called when production assignment is entered.
func (s *BaseShadowRustExtListener) EnterAssignment(ctx *AssignmentContext) {}

// ExitAssignment is called when production assignment is exited.
func (s *BaseShadowRustExtListener) ExitAssignment(ctx *AssignmentContext) {}

// EnterCondition is called when production condition is entered.
func (s *BaseShadowRustExtListener) EnterCondition(ctx *ConditionContext) {}

// ExitCondition is called when production condition is exited.
func (s *BaseShadowRustExtListener) ExitCondition(ctx *ConditionContext) {}

// EnterExpr is called when production expr is entered.
func (s *BaseShadowRustExtListener) EnterExpr(ctx *ExprContext) {}

// ExitExpr is called when production expr is exited.
func (s *BaseShadowRustExtListener) ExitExpr(ctx *ExprContext) {}

// EnterRelExpr is called when production relExpr is entered.
func (s *BaseShadowRustExtListener) EnterRelExpr(ctx *RelExprContext) {}

// ExitRelExpr is called when production relExpr is exited.
func (s *BaseShadowRustExtListener) ExitRelExpr(ctx *RelExprContext) {}

// EnterArithExpr is called when production arithExpr is entered.
func (s *BaseShadowRustExtListener) EnterArithExpr(ctx *ArithExprContext) {}

// ExitArithExpr is called when production arithExpr is exited.
func (s *BaseShadowRustExtListener) ExitArithExpr(ctx *ArithExprContext) {}

// EnterTerm is called when production term is entered.
func (s *BaseShadowRustExtListener) EnterTerm(ctx *TermContext) {}

// ExitTerm is called when production term is exited.
func (s *BaseShadowRustExtListener) ExitTerm(ctx *TermContext) {}
