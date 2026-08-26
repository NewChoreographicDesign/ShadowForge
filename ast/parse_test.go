package ast_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/ast"
)

// TestParseSampleTxBuy exercises the exact example from spec section 14.5.
func TestParseSampleTxBuy(t *testing.T) {
	src := `tx buy x from sender to receiver amount 100 {
    project_fee = amount * 0.05 to vault_address;
}
`
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected syntax errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	tx, ok := prog.Statements[0].(*ast.TxStatement)
	if !ok {
		t.Fatalf("expected *ast.TxStatement, got %T", prog.Statements[0])
	}
	if tx.Name != "x" || tx.From != "sender" || tx.To != "receiver" {
		t.Fatalf("unexpected tx header: %+v", tx)
	}
	if len(tx.Body) != 1 {
		t.Fatalf("expected 1 body statement, got %d", len(tx.Body))
	}
	assign, ok := tx.Body[0].(*ast.Assignment)
	if !ok {
		t.Fatalf("expected *ast.Assignment, got %T", tx.Body[0])
	}
	fee, ok := assign.Value.(*ast.FeeRoute)
	if !ok {
		t.Fatalf("expected *ast.FeeRoute, got %T", assign.Value)
	}
	if fee.To != "vault_address" {
		t.Fatalf("expected fee routed to vault_address, got %s", fee.To)
	}
	bin, ok := fee.Value.(*ast.Binary)
	if !ok || bin.Op != "*" {
		t.Fatalf("expected amount * 0.05 binary expr, got %+v", fee.Value)
	}
}

// TestParseQueueInsert exercises the whitepaper queue-insert snippet
// (spec 5.4.1 / 18.3 review step): queue insert ID positions expr, expr...
func TestParseQueueInsert(t *testing.T) {
	src := `queue insert validator1 positions 4, 10, 2, 7;`
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected syntax errors: %v", errs)
	}
	q, ok := prog.Statements[0].(*ast.QueueInsertStatement)
	if !ok {
		t.Fatalf("expected *ast.QueueInsertStatement, got %T", prog.Statements[0])
	}
	if q.Name != "validator1" {
		t.Fatalf("unexpected queue target: %s", q.Name)
	}
	if len(q.Positions) != 4 {
		t.Fatalf("expected 4 positions, got %d", len(q.Positions))
	}
	for i, want := range []string{"4", "10", "2", "7"} {
		num, ok := q.Positions[i].(*ast.NumberLit)
		if !ok || num.Text != want {
			t.Fatalf("position %d: expected %s, got %+v", i, want, q.Positions[i])
		}
	}
}

func TestParseExtensions(t *testing.T) {
	src := `container { id=finance; validators=5; hybrid=50; }
network { listen=1234; }
resilience if online < 10 { activate sentinels; }
update_trait finance balance += 500;
vote proposal1 commit1;
shard finance count 4;
async_stagger { entry_ms=50; step_ms=200; }
`
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("unexpected syntax errors: %v", errs)
	}
	if len(prog.Statements) != 7 {
		t.Fatalf("expected 7 statements, got %d", len(prog.Statements))
	}
	if _, ok := prog.Statements[0].(*ast.ContainerStatement); !ok {
		t.Fatalf("statement 0: expected ContainerStatement, got %T", prog.Statements[0])
	}
	if _, ok := prog.Statements[1].(*ast.NetworkStatement); !ok {
		t.Fatalf("statement 1: expected NetworkStatement, got %T", prog.Statements[1])
	}
	res, ok := prog.Statements[2].(*ast.ResilienceStatement)
	if !ok {
		t.Fatalf("statement 2: expected ResilienceStatement, got %T", prog.Statements[2])
	}
	if len(res.Body) != 1 {
		t.Fatalf("expected 1 resilience body statement, got %d", len(res.Body))
	}
	if _, ok := res.Body[0].(*ast.ActivateSentinelsStatement); !ok {
		t.Fatalf("expected ActivateSentinelsStatement, got %T", res.Body[0])
	}
	trait, ok := prog.Statements[3].(*ast.UpdateTraitStatement)
	if !ok || trait.Target != "finance" || trait.Key != "balance" || trait.Op != "+=" {
		t.Fatalf("unexpected update_trait: %+v", trait)
	}
	vote, ok := prog.Statements[4].(*ast.VoteStatement)
	if !ok || vote.Proposal != "proposal1" || vote.Commitment != "commit1" {
		t.Fatalf("unexpected vote: %+v", vote)
	}
	shard, ok := prog.Statements[5].(*ast.ShardStatement)
	if !ok || shard.Name != "finance" {
		t.Fatalf("unexpected shard: %+v", shard)
	}
	if _, ok := prog.Statements[6].(*ast.AsyncStaggerStatement); !ok {
		t.Fatalf("statement 6: expected AsyncStaggerStatement, got %T", prog.Statements[6])
	}
}

func TestParseInvalidTxRejected(t *testing.T) {
	// Missing `amount` clause entirely — must be a syntax error, not a silent parse.
	src := `tx buy x from sender to receiver {
    project_fee = 5;
}
`
	_, errs := ast.Parse(src)
	if len(errs) == 0 {
		t.Fatalf("expected syntax error for tx missing amount clause")
	}
}
