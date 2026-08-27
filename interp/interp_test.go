package interp_test

import (
	"math/big"
	"testing"

	"github.com/shadowforge/shadowforge-l1/ast"
	"github.com/shadowforge/shadowforge-l1/interp"
)

// TestEval100Times005 covers 18.3 checkpoint: "Implement expr evaluation in
// the interpreter with tests for 100 * 0.05."
func TestEval100Times005(t *testing.T) {
	prog, errs := ast.Parse(`result = 100 * 0.05;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	it := interp.New(nil)
	if err := it.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, ok := it.Env.Get("result")
	if !ok {
		t.Fatalf("result not bound")
	}
	want := big.NewRat(5, 1)
	if v.Cmp(want) != 0 {
		t.Fatalf("100*0.05 = %s, want %s", v.RatString(), want.RatString())
	}
}

func TestFeeRouteEventRecorded(t *testing.T) {
	src := `tx buy x from sender to receiver amount 100 {
    project_fee = amount * 0.05 to vault_address;
}
`
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	it := interp.New(nil)
	if err := it.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(it.FeeRoutes) != 1 {
		t.Fatalf("expected 1 fee route event, got %d", len(it.FeeRoutes))
	}
	fr := it.FeeRoutes[0]
	if fr.To != "vault_address" || fr.Amount.Cmp(big.NewRat(5, 1)) != 0 {
		t.Fatalf("unexpected fee route: %+v", fr)
	}
	if len(it.Txs) != 1 || it.Txs[0].Amount.Cmp(big.NewRat(100, 1)) != 0 {
		t.Fatalf("unexpected tx event: %+v", it.Txs)
	}
}

func TestUndefinedIdentifierErrors(t *testing.T) {
	prog, errs := ast.Parse(`result = missing_var + 1;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	it := interp.New(nil)
	if err := it.Run(prog); err == nil {
		t.Fatalf("expected error for undefined identifier")
	}
}

func TestOracleSuppliesValue(t *testing.T) {
	prog, errs := ast.Parse(`bank deposit hold1 atr feed_atr;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	it := interp.New(interp.MapOracle{"feed_atr": big.NewRat(120, 1)})
	if err := it.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(it.Deposits) != 1 || it.Deposits[0].ATR.Cmp(big.NewRat(120, 1)) != 0 {
		t.Fatalf("unexpected deposits: %+v", it.Deposits)
	}
}

func TestAllStatementKindsExecuteWithoutError(t *testing.T) {
	src := `mint proposer1 amount 100 epoch 5;
validate v1 stage 1 {
    x = 1;
}
queue insert validator1 positions 4, 10, 2, 7;
container { id=finance; }
network { listen=1234; }
async_stagger { entry_ms=50; }
resilience if 1 {
    activate sentinels;
}
update_trait finance balance = 100;
update_trait finance balance += 50;
update_trait finance balance -= 20;
vote proposal1 commit1;
shard finance count 4;
shard finance;
`
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	it := interp.New(nil)
	if err := it.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, ok := it.Env.Get("finance.balance")
	if !ok || v.Cmp(big.NewRat(130, 1)) != 0 {
		t.Fatalf("expected finance.balance=130 after =100 +50 -20, got %v ok=%v", v, ok)
	}
	if len(it.QueueInsert) != 1 || len(it.QueueInsert[0].Positions) != 4 {
		t.Fatalf("unexpected queue insert events: %+v", it.QueueInsert)
	}
}

func TestMintMissingEpochOmittedIsFine(t *testing.T) {
	prog, errs := ast.Parse(`mint proposer1 amount 100;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if err := interp.New(nil).Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestResilienceConditionFalseSkipsBody(t *testing.T) {
	prog, errs := ast.Parse(`resilience if 0 {
    activate sentinels;
}
`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	it := interp.New(nil)
	if err := it.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestComparisonOperators(t *testing.T) {
	cases := []struct {
		expr string
		want int64
	}{
		{"5 > 3", 1}, {"3 > 5", 0},
		{"5 >= 5", 1}, {"4 >= 5", 0},
		{"3 < 5", 1}, {"5 < 3", 0},
		{"5 <= 5", 1}, {"6 <= 5", 0},
		{"5 == 5", 1}, {"5 == 6", 0},
		{"5 != 6", 1}, {"5 != 5", 0},
	}
	for _, c := range cases {
		prog, errs := ast.Parse("result = " + c.expr + ";")
		if len(errs) != 0 {
			t.Fatalf("%s: parse errors: %v", c.expr, errs)
		}
		it := interp.New(nil)
		if err := it.Run(prog); err != nil {
			t.Fatalf("%s: run: %v", c.expr, err)
		}
		v, _ := it.Env.Get("result")
		if v.Cmp(big.NewRat(c.want, 1)) != 0 {
			t.Errorf("%s = %s, want %d", c.expr, v.RatString(), c.want)
		}
	}
}

func TestDivisionByZeroErrors(t *testing.T) {
	prog, errs := ast.Parse(`result = 1 / 0;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if err := interp.New(nil).Run(prog); err == nil {
		t.Fatalf("expected division by zero to error")
	}
}

func TestUnknownIdentifierInMintAmountErrors(t *testing.T) {
	prog, errs := ast.Parse(`mint proposer1 amount missing_var;`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if err := interp.New(nil).Run(prog); err == nil {
		t.Fatalf("expected undefined identifier error")
	}
}

func TestIfStatementBranches(t *testing.T) {
	prog, errs := ast.Parse(`x = 5;
if x > 3 {
    y = 1;
}
`)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	it := interp.New(nil)
	if err := it.Run(prog); err != nil {
		t.Fatalf("run: %v", err)
	}
	if v, ok := it.Env.Get("y"); !ok || v.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("expected y=1 to be set, got %v ok=%v", v, ok)
	}
}
