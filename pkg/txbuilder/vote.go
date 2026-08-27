package txbuilder

import (
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// voteNonceDomain separates the deterministic vote-nonce derivation below
// from any other use of SumHash(sk, ...) elsewhere, so the same private
// key can never accidentally produce the same output for two different
// purposes.
var voteNonceDomain = []byte("shadowforge-txbuilder-vote-nonce-v1")

// voteNonce deterministically derives this identity's sealed-ballot nonce
// for proposalID from its own private key, rather than drawing a fresh
// random one and requiring the caller to remember it until reveal time.
// This is a real, deliberate design choice: a wallet that stores nothing
// between casting a vote and revealing it — no local "pending votes"
// database to lose or restore from backup — can still reveal correctly,
// because VoteReveal recomputes the identical nonce from the same
// (private key, proposal ID) pair. It also makes retried Vote/VoteReveal
// submissions naturally idempotent: the same call always produces the
// same TxID, so the mempool's own dedup absorbs a network-hiccup replay
// instead of creating a second ballot.
//
// The nonce derivation intentionally hashes the raw private key together
// with a fixed domain tag and the proposal ID (types.SumHash's own
// length-prefixing already gives real domain separation between fields)
// — a standard deterministic-nonce pattern, not a novel one; nothing
// about this reveals sk, and a different proposal always yields an
// unlinkable nonce.
func (b *Builder) voteNonce(proposalID types.ID) types.Hash {
	return types.SumHash(b.sk, voteNonceDomain, []byte(proposalID))
}

// Vote casts a real sealed ballot for proposalID: approve stays hidden
// inside Commitment until a later VoteReveal opens it, matching exactly
// what pkg/tx's pipeline (Stage 4, TxVote case) checks — the same real
// commit-reveal scheme cmd/walletsim already exercises for one kind,
// extended here into a reusable, tested builder.
//
// paramKey/newValue optionally bind this proposal to a real
// governance.Params field change (see types.VotePublicInputs' own doc):
// pass "" for both for a plain up/down vote. They only matter on the
// first Vote to ever reference proposalID — a later voter's own values
// are ignored by the pipeline, not by this builder, so passing something
// here doesn't guarantee it takes effect if someone else voted first.
func (b *Builder) Vote(proposalID types.ID, approve bool, paramKey, newValue string) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	nonce := b.voteNonce(proposalID)
	commitment := types.ComputeVoteCommitment(b.Identity(), approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: proposalID,
			Commitment: commitment,
			ParamKey:   paramKey,
			NewValue:   newValue,
		},
		// Distinct from VoteReveal's nullifier for the same proposal (see
		// that function) — reusing one would collide the pair's TxIDs and
		// have the mempool's TxID-based dedup silently drop the second.
		Nullifier: types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("commit")),
	}
	return b.finalize(t)
}

// VoteReveal opens the sealed ballot Vote(proposalID, approve, ...)
// earlier committed, by recomputing the same deterministic nonce and
// handing back the (approve, nonce) pair the pipeline checks against the
// stored commitment. approve must be the exact same value passed to the
// matching Vote call — VoteReveal has no way to recover it on its own
// (that's the whole point of a sealed ballot), so the caller (a wallet's
// own UI, or its own record of what it voted) is responsible for
// supplying it correctly; passing the wrong value produces a
// well-formed but honestly-rejected reveal, exactly as if a stranger
// tried to open someone else's ballot.
func (b *Builder) VoteReveal(proposalID types.ID, approve bool) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	nonce := b.voteNonce(proposalID)

	t := types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: proposalID,
			Approve:    approve,
			Nonce:      nonce,
		},
		Nullifier: types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("reveal")),
	}
	return b.finalize(t)
}
