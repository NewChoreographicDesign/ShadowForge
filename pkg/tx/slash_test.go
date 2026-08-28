package tx_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// mustBuildSlashVote builds a real, well-formed, signed TxVote binding
// proposalID to a real spec-10.3 slash claim against target.
func mustBuildSlashVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool, target types.NFTID, burn bool) types.ShieldedTx {
	t.Helper()
	nonce := types.Hash{5, 5}
	commitment := types.ComputeVoteCommitment(elig.Nullifier, approve, nonce)
	return mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:     proposalID,
			Commitment:     commitment,
			SlashTargetNFT: target,
			SlashBurn:      burn,
		},
		VoteEligibility: elig,
	})
}

func mustRevealVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool) types.ShieldedTx {
	t.Helper()
	return mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: proposalID, Approve: approve, Nonce: types.Hash{5, 5},
		},
		VoteEligibility: elig,
	})
}

// TestPipelineSlashProposalFreezesRealNFT proves the real, end-to-end
// spec-10.3 freeze outcome: a real slash proposal, approved by real
// revealed ballots, marks the target NFT's real record Slashed — not
// merely a persisted tally outcome.
func TestPipelineSlashProposalFreezesRealNFT(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "slash-proposal-1")

	target := types.ValidatorNFT{ID: types.NFTID{0x42}, Owner: types.Address{0x99}, TP: 10}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	commitTx := mustBuildSlashVote(t, pk, sk, elig, "slash-proposal-1", true, target.ID, false)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the slash-bound vote to be accepted: %v", r[0].Error)
	}
	revealTx := mustRevealVote(t, pk, sk, elig, "slash-proposal-1", true)
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	tallied, err := p.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].SlashApplied {
		t.Fatalf("expected the proposal to pass and the real slash to be applied, got %+v", tallied)
	}

	got, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("expected the target NFT record to still exist (frozen, not burned): found=%v err=%v", found, err)
	}
	if !got.Slashed {
		t.Fatalf("expected the real NFT record to be marked Slashed")
	}
	if got.TP != target.TP {
		t.Fatalf("expected TP to be unchanged on freeze, got %d want %d", got.TP, target.TP)
	}
}

// TestPipelineSlashProposalBurnsRealNFT proves the real burn outcome:
// the target's record is permanently removed, including its
// owner-index entry (so the owner could mint a fresh NFT afterward).
func TestPipelineSlashProposalBurnsRealNFT(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "slash-proposal-2")

	target := types.ValidatorNFT{ID: types.NFTID{0x43}, Owner: types.Address{0x98}, TP: 10}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	commitTx := mustBuildSlashVote(t, pk, sk, elig, "slash-proposal-2", true, target.ID, true)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the slash-bound vote to be accepted: %v", r[0].Error)
	}
	revealTx := mustRevealVote(t, pk, sk, elig, "slash-proposal-2", true)
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	tallied, err := p.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].SlashApplied {
		t.Fatalf("expected the proposal to pass and the real burn to be applied, got %+v", tallied)
	}

	if _, found, err := deps.Store.GetNFT(target.ID); err != nil {
		t.Fatalf("get nft: %v", err)
	} else if found {
		t.Fatalf("expected the burned NFT's record to be permanently removed")
	}
	if _, found, err := deps.Store.GetNFTByOwner(target.Owner); err != nil {
		t.Fatalf("get nft by owner: %v", err)
	} else if found {
		t.Fatalf("expected the burned NFT's owner-index entry to be removed too")
	}
}

// TestPipelineSlashProposalRejectsNonexistentTarget proves the real
// existence check, not a bare-field-presence check, is what Stage 4
// enforces: a slash claim against an NFT that was never minted is
// rejected outright, and no proposal record is created.
func TestPipelineSlashProposalRejectsNonexistentTarget(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "slash-proposal-ghost")

	ghostTarget := types.NFTID{0xff, 0xff, 0xff}
	commitTx := mustBuildSlashVote(t, pk, sk, elig, "slash-proposal-ghost", true, ghostTarget, false)
	results := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a slash claim against a nonexistent NFT to be rejected")
	}
	if _, found, err := deps.Store.GetProposal("slash-proposal-ghost"); err != nil {
		t.Fatalf("get proposal: %v", err)
	} else if found {
		t.Fatalf("expected no proposal record to exist after a rejected slash claim")
	}
}

// TestPipelineSlashProposalNotAppliedWhenRejected proves a slash bound
// to a proposal that fails its vote never executes: the target NFT
// record is untouched, SlashApplied stays false.
func TestPipelineSlashProposalNotAppliedWhenRejected(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "slash-proposal-rejected")

	target := types.ValidatorNFT{ID: types.NFTID{0x44}, Owner: types.Address{0x97}}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	commitTx := mustBuildSlashVote(t, pk, sk, elig, "slash-proposal-rejected", false, target.ID, false)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the slash-bound vote (rejecting) to be accepted as a real ballot: %v", r[0].Error)
	}
	revealTx := mustRevealVote(t, pk, sk, elig, "slash-proposal-rejected", false)
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	tallied, err := p.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || tallied[0].Passed {
		t.Fatalf("expected the proposal to fail (sole voter rejected), got %+v", tallied)
	}
	if tallied[0].SlashApplied {
		t.Fatalf("expected SlashApplied to stay false for a failed proposal")
	}
	got, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("expected the target NFT record to be untouched: found=%v err=%v", found, err)
	}
	if got.Slashed {
		t.Fatalf("expected the target NFT to not be slashed")
	}
}
