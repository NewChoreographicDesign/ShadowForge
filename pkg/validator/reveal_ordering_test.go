package validator

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// These tests prove filterReadyReveals closes a real network-liveness bug
// found live under sustained 3-node TxVote/TxVoteReveal traffic: a reveal
// gossiped to a node before that node has seen its own commit (no
// ordering guarantee across independent libp2p connections) used to
// poison the entire proposal it was drained into — see
// filterReadyReveals' own doc for the full story.

// mustSignRevealWithKey builds a real, correctly-signed TxVoteReveal for
// proposalID/approve/nonce, signed by the given keypair — mirroring
// mustSignVote's TxVote construction so a matching commit/reveal pair can
// be built by the same voter. elig carries the (real or, for these
// filterReadyReveals-only tests, merely consistent-Nullifier) eligibility
// proof filterReadyReveals reads to correlate a reveal with its earlier
// commit — filterReadyReveals itself never verifies the proof
// cryptographically (that's Stage 4's job), only pkg/tx's pipeline does,
// so these tests don't need a real Groth16 proof, just a matching
// Nullifier.
func mustSignRevealWithKey(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, proposalID string, approve bool, nonce types.Hash, elig *types.VoteEligibilityProof) types.ShieldedTx {
	t.Helper()
	in := types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID(proposalID),
			Approve:    approve,
			Nonce:      nonce,
		},
		VoteEligibility: elig,
		Nullifier:       types.SumHash([]byte(pk), []byte(proposalID), []byte("reveal")),
	}
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	return in
}

// mustSignVoteWithKey mirrors mustSignVote but with a caller-supplied
// keypair, so a commit and its later reveal come from the same voter.
func mustSignVoteWithKey(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, proposalID string, commitment types.Hash, elig *types.VoteEligibilityProof) types.ShieldedTx {
	t.Helper()
	in := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID(proposalID),
			Commitment: commitment,
		},
		VoteEligibility: elig,
		Nullifier:       types.SumHash([]byte(pk), []byte(proposalID), []byte("commit")),
	}
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	return in
}

func TestFilterReadyRevealsDefersRevealWithNoDurableCommit(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	// A reveal arrives with no commit anywhere — neither durably
	// committed nor present in the same candidate batch.
	elig := &types.VoteEligibilityProof{Nullifier: types.Hash{42}}
	reveal := mustSignRevealWithKey(t, pk, sk, "race-proposal", true, types.Hash{1}, elig)

	ready, deferred := n.filterReadyReveals([]types.ShieldedTx{reveal})
	if len(ready) != 0 {
		t.Fatalf("expected the reveal to be deferred, not included, got ready=%+v", ready)
	}
	if len(deferred) != 1 || deferred[0].TxID != reveal.TxID {
		t.Fatalf("expected exactly the reveal to be deferred, got %+v", deferred)
	}
}

func TestFilterReadyRevealsAllowsRevealWhoseCommitIsEarlierInSameBatch(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	elig := &types.VoteEligibilityProof{Nullifier: types.Hash{55}}
	nonce := types.Hash{2}
	commitment := types.ComputeVoteCommitment(elig.Nullifier, true, nonce)
	commit := mustSignVoteWithKey(t, pk, sk, "same-batch-proposal", commitment, elig)
	reveal := mustSignRevealWithKey(t, pk, sk, "same-batch-proposal", true, nonce, elig)

	// Commit appears earlier in the SAME candidate batch as its reveal —
	// this is the ordinary, common case (gossip delivered in order) and
	// must not be needlessly deferred.
	ready, deferred := n.filterReadyReveals([]types.ShieldedTx{commit, reveal})
	if len(deferred) != 0 {
		t.Fatalf("expected nothing to be deferred, got %+v", deferred)
	}
	if len(ready) != 2 || ready[0].TxID != commit.TxID || ready[1].TxID != reveal.TxID {
		t.Fatalf("expected both commit and reveal ready in order, got %+v", ready)
	}
}

func TestFilterReadyRevealsAllowsRevealWhoseCommitIsDurablyCommitted(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	elig := &types.VoteEligibilityProof{Nullifier: types.Hash{77}}
	nonce := types.Hash{3}
	commitment := types.ComputeVoteCommitment(elig.Nullifier, true, nonce)

	// Simulate an already-durably-committed commit for this voter,
	// exactly the shape pipeline.go's TxVote case would have persisted.
	record := state.ProposalRecord{
		ProposalID:  "durable-proposal",
		Commitments: map[types.Hash]types.Hash{elig.Nullifier: commitment},
		Reveals:     map[types.Hash]bool{},
	}
	if err := n.store.PutProposal(record); err != nil {
		t.Fatalf("seed committed proposal: %v", err)
	}

	reveal := mustSignRevealWithKey(t, pk, sk, "durable-proposal", true, nonce, elig)
	ready, deferred := n.filterReadyReveals([]types.ShieldedTx{reveal})
	if len(deferred) != 0 {
		t.Fatalf("expected nothing to be deferred, got %+v", deferred)
	}
	if len(ready) != 1 || ready[0].TxID != reveal.TxID {
		t.Fatalf("expected the reveal to be ready, got %+v", ready)
	}
}

func TestFilterReadyRevealsUnrelatedTxsAlwaysReady(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	other := mustSignVote(t, n, "unrelated", 1) // an ordinary TxVote commit, nothing to defer
	ready, deferred := n.filterReadyReveals([]types.ShieldedTx{other})
	if len(deferred) != 0 || len(ready) != 1 {
		t.Fatalf("expected the unrelated tx to pass through as ready, got ready=%+v deferred=%+v", ready, deferred)
	}
}

// TestBuildProposalBatchDefersNotYetReadyRevealAndReinserts proves the
// full wiring: buildProposalBatch excludes a not-yet-ready reveal from
// the batch it returns, and the reveal is still pending in the mempool
// afterward (retried in a later round) rather than lost.
func TestBuildProposalBatchDefersNotYetReadyRevealAndReinserts(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	stranded := mustSignRevealWithKey(t, pk, sk, "stranded-proposal", true, types.Hash{9}, &types.VoteEligibilityProof{Nullifier: types.Hash{88}})
	if err := n.mempool.Submit(stranded, time.Now()); err != nil {
		t.Fatalf("submit: %v", err)
	}
	normal := mustSignVote(t, n, "normal-proposal", 5)
	if err := n.mempool.Submit(normal, time.Now()); err != nil {
		t.Fatalf("submit: %v", err)
	}

	batch := n.buildProposalBatch(false)

	for _, bt := range batch {
		if bt.TxID == stranded.TxID {
			t.Fatalf("expected the not-yet-ready reveal to be excluded from the proposal batch")
		}
	}
	foundNormal := false
	for _, bt := range batch {
		if bt.TxID == normal.TxID {
			foundNormal = true
		}
	}
	if !foundNormal {
		t.Fatalf("expected the unrelated, ready tx to still be included")
	}
	if n.mempool.Len() != 1 {
		t.Fatalf("expected the deferred reveal to be back in the mempool for a later round, len=%d", n.mempool.Len())
	}
}
