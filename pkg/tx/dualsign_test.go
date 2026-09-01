package tx_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/governance"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// mustDualSign builds a real, well-formed TxBankDeposit signed with both
// a real Dilithium key (always required) and a real ed25519 classical
// co-signature (spec 8.5's optional migration aid) over the identical
// TxID.
func mustDualSign(t *testing.T) types.ShieldedTx {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate dilithium key: %v", err)
	}
	classicalPK, classicalSK, err := crypto.GenerateClassicalKey()
	if err != nil {
		t.Fatalf("generate classical key: %v", err)
	}
	atr := decimal.MustFromString("2000")
	in := types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			Asset:     types.AssetSFG,
			ATRUSD:    atr,
			BufferUSD: decimal.MustFromString("2.5").Mul(atr),
		},
	}
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("dilithium sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	classicalSig, err := crypto.ClassicalSign(classicalSK, in.TxID[:])
	if err != nil {
		t.Fatalf("classical sign: %v", err)
	}
	in.ClassicalSig = []byte(classicalSig)
	in.ClassicalPubKey = []byte(classicalPK)
	return in
}

// TestPipelineDualSignedTxAccepted proves a real, correctly dual-signed
// transaction is accepted — the classical co-signature is a genuine
// addition Stage 2 verifies, not a decorative field nobody checks.
func TestPipelineDualSignedTxAccepted(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	txn := mustDualSign(t)
	if r := p.ProcessBatch([]tx.Entry{{Tx: txn, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected a real dual-signed transaction to be accepted: %v", r[0].Error)
	}
}

// TestPipelineDualSignedTxRejectsForgedClassicalSig proves the classical
// co-signature is really verified, not merely required to be present:
// swapping in an unrelated signature (still the right length, still
// structurally well-formed) must be rejected.
func TestPipelineDualSignedTxRejectsForgedClassicalSig(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)
	txn := mustDualSign(t)

	_, forgedSK, err := crypto.GenerateClassicalKey()
	if err != nil {
		t.Fatalf("generate forged classical key: %v", err)
	}
	forgedSig, err := crypto.ClassicalSign(forgedSK, txn.TxID[:])
	if err != nil {
		t.Fatalf("forge classical sig: %v", err)
	}
	txn.ClassicalSig = []byte(forgedSig) // signed with a DIFFERENT key than ClassicalPubKey claims

	results := p.ProcessBatch([]tx.Entry{{Tx: txn, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a classical signature from the wrong key to be rejected")
	}
}

// TestPipelineDualSignedTxRequiresBothFields proves ClassicalSig/
// ClassicalPubKey are an all-or-nothing pair, not independently optional.
func TestPipelineDualSignedTxRequiresBothFields(t *testing.T) {
	deps := newDeps(t)
	p := tx.NewPipeline(deps)

	withSigOnly := mustDualSign(t)
	withSigOnly.ClassicalPubKey = nil
	if r := p.ProcessBatch([]tx.Entry{{Tx: withSigOnly, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a classical sig with no matching pubkey to be rejected")
	}

	withKeyOnly := mustDualSign(t)
	withKeyOnly.ClassicalSig = nil
	if r := p.ProcessBatch([]tx.Entry{{Tx: withKeyOnly, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a classical pubkey with no matching sig to be rejected")
	}
}

// TestPipelineDualSignRejectedOnceRetired proves that once
// governance.Params.DualSignEnabled is false, Stage 2 rejects a
// transaction outright for merely carrying a classical co-signature —
// not just "ignores" it.
func TestPipelineDualSignRejectedOnceRetired(t *testing.T) {
	deps := newDeps(t)
	govParams := governance.Default()
	govParams.DualSignEnabled = false
	deps.Governance = &govParams
	p := tx.NewPipeline(deps)

	txn := mustDualSign(t)
	results := p.ProcessBatch([]tx.Entry{{Tx: txn, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a dual-signed transaction to be rejected once governance has retired dual-sign")
	}
}

// mustBuildUnwindDualSignVote builds a real, well-formed, signed TxVote
// binding proposalID to a real spec-8.5 dual-sign-retirement claim.
func mustBuildUnwindDualSignVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool) types.ShieldedTx {
	t.Helper()
	nonce := types.Hash{5, 5} // matches mustRevealVote's (slash_test.go) hardcoded reveal nonce
	commitment := types.ComputeVoteCommitment(elig.Nullifier, approve, nonce)
	return mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:     proposalID,
			Commitment:     commitment,
			UnwindDualSign: true,
		},
		VoteEligibility: elig,
	})
}

// TestPipelineUnwindDualSignProposalRetiresRealPath proves the real,
// end-to-end spec-8.5 outcome: a real dual-sign-retirement proposal,
// approved by real revealed ballots, makes governance.Params.
// DualSignEnabled genuinely false — and a dual-signed transaction that
// was accepted before the vote passed is rejected afterward.
func TestPipelineUnwindDualSignProposalRetiresRealPath(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	govParams := governance.Default()
	deps.Governance = &govParams
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "unwind-dual-sign-proposal-1")

	// Before the vote passes, a real dual-signed transaction is accepted.
	preVote := mustDualSign(t)
	if r := p.ProcessBatch([]tx.Entry{{Tx: preVote, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected a dual-signed transaction to be accepted before the vote passes: %v", r[0].Error)
	}

	commitTx := mustBuildUnwindDualSignVote(t, pk, sk, elig, "unwind-dual-sign-proposal-1", true)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the dual-sign-retirement-bound vote to be accepted: %v", r[0].Error)
	}
	revealTx := mustRevealVote(t, pk, sk, elig, "unwind-dual-sign-proposal-1", true)
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	tallied, err := p.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].UnwindDualSignApplied {
		t.Fatalf("expected the proposal to pass and the real retirement to be applied, got %+v", tallied)
	}
	if deps.Governance.DualSignEnabled {
		t.Fatalf("expected DualSignEnabled to be genuinely false after a passed retirement vote")
	}

	// The identical style of dual-signed transaction that was accepted
	// above is now rejected.
	postVote := mustDualSign(t)
	if r := p.ProcessBatch([]tx.Entry{{Tx: postVote, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a dual-signed transaction to be rejected after the retirement vote passes")
	}
}

// TestPipelineUnwindDualSignProposalRejectsAlreadyRetired proves a second
// vote to retire an already-retired dual-sign path is rejected outright
// as a pointless claim, rather than silently accepted.
func TestPipelineUnwindDualSignProposalRejectsAlreadyRetired(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	govParams := governance.Default()
	govParams.DualSignEnabled = false
	deps.Governance = &govParams
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "unwind-dual-sign-proposal-dup")

	commitTx := mustBuildUnwindDualSignVote(t, pk, sk, elig, "unwind-dual-sign-proposal-dup", true)
	results := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a retirement claim against an already-retired dual-sign path to be rejected")
	}
}
