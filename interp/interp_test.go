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
