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

// voteNonce deterministically derives this ballot's sealed nonce for
// proposalID from the voter's real eligibility nullifier, rather than
// drawing a fresh random one and requiring the caller to remember it
// until reveal time. This is a real, deliberate design choice: a wallet
// that stores nothing between casting a vote and revealing it — no local
// "pending votes" database to lose or restore from backup — can still
// reveal correctly, because VoteReveal recomputes the identical nonce
// from the same (voterNullifier, proposal ID) pair.
//
// This intentionally does NOT derive from b.sk (unlike, e.g., NFTMint's
// nonce elsewhere in this package): Vote and VoteReveal are built by two
// separate Builder values wrapping two different, unlinked keys (see
// Vote's own doc on why b should hold a fresh throwaway key per call,
// distinct from the identity that minted the NFT) — deriving from b.sk
// would make VoteReveal recompute a DIFFERENT nonce than the Vote it's
// meant to open, since it's a different b. voterNullifier is exactly the
// one value the real design guarantees is identical across both calls
// (deterministic in VoterSK+proposalID — see pkg/govwallet.Wallet.
// BuildEligibilityProof), so it is what ties nonce (and therefore
// Commitment) to one specific voter/proposal pair without touching a
// signing key at all.
func voteNonce(proposalID types.ID, voterNullifier types.Hash) types.Hash {
	return types.SumHash(voteNonceDomain, []byte(proposalID), voterNullifier[:])
}

// Vote casts a real sealed ballot for proposalID: approve stays hidden
// inside Commitment until a later VoteReveal opens it, matching exactly
// what pkg/tx's pipeline (Stage 4, TxVote case) checks — the same real
// commit-reveal scheme cmd/walletsim already exercises for one kind,
// extended here into a reusable, tested builder.
//
// eligibility is a real, pre-built anonymous ZK proof that this caller
// holds a minted NFT (pkg/govwallet.Wallet.BuildEligibilityProof) —
// Builder itself never touches the network or a Merkle tree (see the
// package doc), so unlike Identity() it cannot build one on its own. Its
// Nullifier, not b.Identity(), is what Commitment binds and what the
// pipeline dedups ballots by (types.VoteEligibilityProof's own doc): this
// is what makes a ballot anonymous rather than tied to b's own signing
// key. For that anonymity to mean anything, b should hold a fresh key
// generated just for this vote, unlinked from whatever identity minted
// the NFT eligibility proves — pkg/govwallet.Wallet.BuildEligibilityProof
// derives its proof from a separate, deterministic secret of its own,
// independent of whatever key signs the transaction this method returns.
//
// paramKey/newValue optionally bind this proposal to a real
// governance.Params field change (see types.VotePublicInputs' own doc):
// pass "" for both for a plain up/down vote. They only matter on the
// first Vote to ever reference proposalID — a later voter's own values
// are ignored by the pipeline, not by this builder, so passing something
// here doesn't guarantee it takes effect if someone else voted first.
func (b *Builder) Vote(proposalID types.ID, approve bool, paramKey, newValue string, eligibility types.VoteEligibilityProof) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	nonce := voteNonce(proposalID, eligibility.Nullifier)
	commitment := types.ComputeVoteCommitment(eligibility.Nullifier, approve, nonce)

	t := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: proposalID,
			Commitment: commitment,
			ParamKey:   paramKey,
			NewValue:   newValue,
		},
		VoteEligibility: &eligibility,
		// Distinct from VoteReveal's nullifier for the same proposal (see
		// that function) — reusing one would collide the pair's TxIDs and
		// have the mempool's TxID-based dedup silently drop the second.
		// This is the top-level spec-4.1 TxID nullifier (b's own signing
		// key, purely for TxID uniqueness), not eligibility.Nullifier
		// (the real anonymous per-proposal ballot nullifier) — the two
		// serve entirely different purposes and must never be confused.
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
//
// eligibility must resolve to the exact same Nullifier as the matching
// Vote call's own eligibility proof — pkg/govwallet.Wallet.
// BuildEligibilityProof(sys, proposalID) reproduces it deterministically,
// so calling it again here (even with a freshly built proof) still ties
// this reveal to the same earlier commitment; a proof for a different
// proposalID or a different underlying VoterSK cannot open it.
func (b *Builder) VoteReveal(proposalID types.ID, approve bool, eligibility types.VoteEligibilityProof) (types.ShieldedTx, error) {
	if proposalID == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: proposal id must not be empty")
	}
	nonce := voteNonce(proposalID, eligibility.Nullifier)

	t := types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: proposalID,
			Approve:    approve,
			Nonce:      nonce,
		},
		VoteEligibility: &eligibility,
		Nullifier:       types.SumHash(b.sk, voteNonceDomain, []byte(proposalID), []byte("reveal")),
	}
	return b.finalize(t)
}
