package txbuilder_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/governance"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// TestWithClassicalKeyCoSignsRealTransaction proves Builder.
// WithClassicalKey genuinely attaches a real, independently-verifiable
// ed25519 co-signature — not just a stored field nobody checks — by
// verifying it directly, and separately confirming the real pipeline
// accepts the resulting dual-signed transaction.
func TestWithClassicalKeyCoSignsRealTransaction(t *testing.T) {
	classicalPK, classicalSK, err := crypto.GenerateClassicalKey()
	if err != nil {
		t.Fatalf("generate classical key: %v", err)
	}
	b := newIdentity(t).WithClassicalKey(classicalPK, classicalSK)

	quorum := staticQuorum(t, "60000", "1500")
	txn, err := b.BankDeposit(quorum, "SFG")
	if err != nil {
		t.Fatalf("build deposit: %v", err)
	}
	assertRealSignature(t, txn)

	if len(txn.ClassicalSig) == 0 || len(txn.ClassicalPubKey) == 0 {
		t.Fatalf("expected a real classical co-signature to be attached")
	}
	if string(txn.ClassicalPubKey) != string(classicalPK) {
		t.Fatalf("expected the attached classical public key to match")
	}
	ok, err := crypto.ClassicalVerify(classicalPK, txn.TxID[:], crypto.ClassicalSignature(txn.ClassicalSig))
	if err != nil {
		t.Fatalf("verify classical sig: %v", err)
	}
	if !ok {
		t.Fatalf("expected the real classical co-signature to verify against TxID")
	}

	p, _ := newRealPipeline(t, tx.Deps{Oracle: quorum})
	if err := runOne(p, txn); err != nil {
		t.Fatalf("expected the real pipeline to accept a genuinely dual-signed transaction: %v", err)
	}
}

// TestBuilderWithoutClassicalKeyBuildsSingleSignedTransaction proves
// dual-sign stays genuinely optional: a Builder that never calls
// WithClassicalKey builds an ordinary, single-signed (Dilithium only)
// transaction, exactly as before this feature existed.
func TestBuilderWithoutClassicalKeyBuildsSingleSignedTransaction(t *testing.T) {
	b := newIdentity(t)
	quorum := staticQuorum(t, "60000", "1500")
	txn, err := b.BankDeposit(quorum, "SFG")
	if err != nil {
		t.Fatalf("build deposit: %v", err)
	}
	if len(txn.ClassicalSig) != 0 || len(txn.ClassicalPubKey) != 0 {
		t.Fatalf("expected no classical co-signature when WithClassicalKey was never called")
	}
}

func TestProposeUnwindDualSignThenRealRetirementEndToEnd(t *testing.T) {
	govParams := governance.Default()
	_, deps := newRealPipeline(t, tx.Deps{})
	deps.Governance = &govParams
	p := tx.NewPipeline(deps)
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "builder-unwind-dual-sign-1")

	votetx, err := b.ProposeUnwindDualSign("builder-unwind-dual-sign-1", true, elig)
	if err != nil {
		t.Fatalf("build propose-unwind-dual-sign: %v", err)
	}
	assertRealSignature(t, votetx)
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed dual-sign-retirement proposal: %v", err)
	}

	revealtx, err := b.VoteReveal("builder-unwind-dual-sign-1", true, elig)
	if err != nil {
		t.Fatalf("build reveal: %v", err)
	}
	if err := runOne(p, revealtx); err != nil {
		t.Fatalf("expected the real pipeline to accept the matching reveal: %v", err)
	}

	tallied, err := p.TallyDueProposals(deps.Epoch + 1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].UnwindDualSignApplied {
		t.Fatalf("expected the proposal to pass and the real retirement to be applied, got %+v", tallied)
	}
	if deps.Governance.DualSignEnabled {
		t.Fatalf("expected DualSignEnabled to be genuinely false after a passed retirement vote")
	}
}

func TestProposeUnwindDualSignRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.ProposeUnwindDualSign("", true, types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}
