// Package ast defines the typed abstract syntax tree for ShadowRust programs,
// per ShadowForge L1 Master Specification section 14.1 ("Visitor builds a typed
// AST, then an IR") and section 18.3 ("AST structs for every statement kind").
package ast

// Program is the root of a parsed ShadowRust source file.
type Program struct {
	Statements []Statement
}

// Statement is implemented by every ShadowRust statement kind.
type Statement interface {
	statementNode()
}

// Expr is implemented by every ShadowRust expression node.
type Expr interface {
	exprNode()
}

// ---- Statements ----

type IfStatement struct {
	Condition Expr
	Body      []Statement
}

// TxStatement models `tx buy ID from ID to ID amount expr { ... }`.
type TxStatement struct {
	Name   string
	From   string
	To     string
	Amount Expr
	Body   []Statement
}

// MintStatement models `mint ID amount expr (epoch expr)? ;`.
type MintStatement struct {
	Name   string
	Amount Expr
	Epoch  Expr // nil if absent
}

// ValidateStatement models `validate ID stage NUMBER { ... }`.
type ValidateStatement struct {
	Name  string
	Stage int
	Body  []Statement
}

// QueueInsertStatement models `queue insert ID positions expr (, expr)* ;`.
type QueueInsertStatement struct {
	Name      string
	Positions []Expr
}

// BankDepositStatement models `bank deposit ID atr expr ;`.
type BankDepositStatement struct {
	Name string
	ATR  Expr
}

// Assignment models `ID = expr ;`.
type Assignment struct {
	Name  string
	Value Expr
}

// ContainerStatement models `container { field=expr; ... }` (14.3 extension).
type ContainerStatement struct {
	Fields []Field
}

// NetworkStatement models `network { field=expr; ... }` (14.3 extension).
type NetworkStatement struct {
	Fields []Field
}

// ResilienceStatement models `resilience if condition { ... }` (14.3 extension).
type ResilienceStatement struct {
	Condition Expr
	Body      []Statement
}

// ActivateSentinelsStatement models `activate sentinels;` inside a resilience block.
type ActivateSentinelsStatement struct{}

// UpdateTraitStatement models `update_trait ID KEY op expr ;` (14.3 extension).
type UpdateTraitStatement struct {
	Target string
	Key    string
	Op     string // "=" | "+=" | "-="
	Value  Expr
}

// VoteStatement models `vote PROPOSAL commitment ;` (14.3 extension).
type VoteStatement struct {
	Proposal   string
	Commitment string
}

// ShardStatement models `shard ID (count expr)? ;` (high_tps.g4 dialect).
type ShardStatement struct {
	Name  string
	Count Expr // nil if absent
}

// AsyncStaggerStatement models `async_stagger { field=expr; ... }` (high_tps.g4 dialect).
type AsyncStaggerStatement struct {
	Fields []Field
}

// Field is a `key=value;` pair used by container/network/async_stagger blocks.
type Field struct {
	Key   string
	Value Expr
}

func (*IfStatement) statementNode()                {}
func (*TxStatement) statementNode()                {}
func (*MintStatement) statementNode()              {}
func (*ValidateStatement) statementNode()          {}
func (*QueueInsertStatement) statementNode()       {}
func (*BankDepositStatement) statementNode()       {}
func (*Assignment) statementNode()                 {}
func (*ContainerStatement) statementNode()         {}
func (*NetworkStatement) statementNode()           {}
func (*ResilienceStatement) statementNode()        {}
func (*ActivateSentinelsStatement) statementNode() {}
func (*UpdateTraitStatement) statementNode()       {}
func (*VoteStatement) statementNode()              {}
func (*ShardStatement) statementNode()             {}
func (*AsyncStaggerStatement) statementNode()      {}

// ---- Expressions ----

// Ident is a bare identifier reference.
type Ident struct {
	Name string
}

// NumberLit is a numeric literal, kept as source text so callers can choose
// integer, float, or exact decimal parsing (see pkg/decimal for chain math).
type NumberLit struct {
	Text string
}

// Binary is a left-associative binary operation: relational, additive, or
// multiplicative, per the arithExpr/term/relExpr grammar rules.
type Binary struct {
	Op    string
	Left  Expr
	Right Expr
}

// FeeRoute models the top-level `expr TO ID` production, e.g.
// `amount * 0.05 to vault_address`. Value is the routed amount; To is the
// destination address identifier. Per spec 14.4, codegen must route this as
// a fee commitment to the named address (Vault by default).
type FeeRoute struct {
	Value Expr
	To    string
}

func (*Ident) exprNode()     {}
func (*NumberLit) exprNode() {}
func (*Binary) exprNode()    {}
func (*FeeRoute) exprNode()  {}
