package sema_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/ast"
	"github.com/shadowforge/shadowforge-l1/sema"
)

func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := ast.Parse(src)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return prog
}

func TestValidTxPassesSema(t *testing.T) {
	prog := mustParse(t, `tx buy x from sender to receiver amount 100 {
    project_fee = amount * 0.05 to vault_address;
}
`)
	if diags := sema.Analyze(prog); len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diags)
	}
}

func TestTxMissingFeeRouteRejected(t *testing.T) {
	prog := mustParse(t, `tx buy x from sender to receiver amount 100 {
    project_fee = amount * 0.05;
}
`)
	diags := sema.Analyze(prog)
	if len(diags) == 0 {
		t.Fatalf("expected a missing-fee-route diagnostic")
	}
}

func TestNonNumericAmountRejected(t *testing.T) {
	prog := mustParse(t, `tx buy x from sender to receiver amount (5 > 3) {
    fee = 1 to vault;
}
`)
	diags := sema.Analyze(prog)
	if len(diags) == 0 {
		t.Fatalf("expected a non-numeric-amount diagnostic")
	}
}

func TestBankDepositUnboundATRRejected(t *testing.T) {
	prog := mustParse(t, `bank deposit hold1 atr 5;`)
	diags := sema.Analyze(prog)
	if len(diags) == 0 {
		t.Fatalf("expected an unbound-atr diagnostic")
	}
}

func TestBankDepositBoundATRAccepted(t *testing.T) {
	prog := mustParse(t, `bank deposit hold1 atr atr_feed;`)
	if diags := sema.Analyze(prog); len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diags)
	}
}

func TestStageIllegalStatementRejected(t *testing.T) {
	// Stage 1 (Sender Leave) is read-only per spec 5.3; a bank deposit here
	// is illegal.
	prog := mustParse(t, `validate v1 stage 1 {
    bank deposit hold1 atr atr_feed;
}
`)
	diags := sema.Analyze(prog)
	if len(diags) == 0 {
		t.Fatalf("expected a stage-legality diagnostic")
	}
}

func TestStageLegalStatementAccepted(t *testing.T) {
	prog := mustParse(t, `validate v1 stage 4 {
    bank deposit hold1 atr atr_feed;
}
`)
	if diags := sema.Analyze(prog); len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diags)
	}
}

func TestStageOutOfRangeRejected(t *testing.T) {
	prog := mustParse(t, `validate v1 stage 6 {
}
`)
	diags := sema.Analyze(prog)
	if len(diags) == 0 {
		t.Fatalf("expected an out-of-range stage diagnostic")
	}
}
