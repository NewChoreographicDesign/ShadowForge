package tx_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// mustBuildUnlockTransferVote builds a real, well-formed, signed TxVote
// binding proposalID to a real spec-10.1 transfer-unlock claim against
// target.
func mustBuildUnlockTransferVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool, target types.NFTID) types.ShieldedTx {
	t.Helper()
	nonce := types.Hash{6, 6}
	commitment := types.ComputeVoteCommitment(elig.Nullifier, approve, nonce)
	return mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:           proposalID,
			Commitment:           commitment,
			UnlockTransferTarget: target,
		},
		VoteEligibility: elig,
	})
}

func mustRevealUnlockVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool) types.ShieldedTx {
	t.Helper()
	return mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: proposalID, Approve: approve, Nonce: types.Hash{6, 6},
		},
		VoteEligibility: elig,
	})
}

// mustBuildNFTTransfer builds and signs a real Kind NFTTransfer tx with
// ownerPK/ownerSK — the claimed current owner's own real key.
func mustBuildNFTTransfer(t *testing.T, ownerPK crypto.DilithiumPublicKey, ownerSK crypto.DilithiumPrivateKey, target types.NFTID, newOwner types.Address) types.ShieldedTx {
	t.Helper()
	return mustSignWithKey(t, ownerPK, ownerSK, types.ShieldedTx{
		Kind:        types.TxNFTTransfer,
		Commitments: []types.Hash{types.Hash(target)},
		NFTTransferPublicInputs: &types.NFTTransferPublicInputs{
			Target:   target,
			NewOwner: newOwner,
		},
		Nullifier: types.Hash{0xaa, byte(target[0])},
	})
}

// TestPipelineUnlockTransferProposalThenRealTransfer proves the real,
// end-to-end spec-10.1 flow: a real transfer-unlock proposal passes,
// setting the real "transferable" trait, and only THEN does a real Kind
// NFTTransfer signed by the current owner actually move the NFT — before
// the unlock, the identical transfer is rejected outright.
func TestPipelineUnlockTransferProposalThenRealTransfer(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "unlock-proposal-1")

	ownerPK, ownerSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate nft owner key: %v", err)
	}
	ownerAddr := types.AddressFromPubkey(ownerPK)
	target := types.ValidatorNFT{ID: types.NFTID{0x33}, Owner: ownerAddr, TP: 5}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}
	newOwner := types.Address{0x77}

	// Before the unlock: an otherwise well-formed, correctly-signed
	// transfer from the real current owner is still rejected.
	tooEarly := mustBuildNFTTransfer(t, ownerPK, ownerSK, target.ID, newOwner)
	if r := p.ProcessBatch([]tx.Entry{{Tx: tooEarly, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a transfer to be rejected before governance unlocks it")
	}

	commitTx := mustBuildUnlockTransferVote(t, pk, sk, elig, "unlock-proposal-1", true, target.ID)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the unlock-bound vote to be accepted: %v", r[0].Error)
	}
	revealTx := mustRevealUnlockVote(t, pk, sk, elig, "unlock-proposal-1", true)
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}
	tallied, err := p.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].UnlockTransferApplied {
		t.Fatalf("expected the proposal to pass and the real unlock to be applied, got %+v", tallied)
	}

	// Real, independent proof the trait actually landed.
	unlocked, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("get target nft: found=%v err=%v", found, err)
	}
	if unlocked.Traits["transferable"] != "true" {
		t.Fatalf("expected the real transferable trait to be set, got %+v", unlocked.Traits)
	}

	// Now the identical transfer succeeds.
	transferTx := mustBuildNFTTransfer(t, ownerPK, ownerSK, target.ID, newOwner)
	if r := p.ProcessBatch([]tx.Entry{{Tx: transferTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the real transfer to be accepted once unlocked: %v", r[0].Error)
	}

	moved, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("get transferred nft: found=%v err=%v", found, err)
	}
	if moved.Owner != newOwner {
		t.Fatalf("expected Owner to be %s, got %s", newOwner, moved.Owner)
	}
	if moved.TP != target.TP {
		t.Fatalf("expected TP to be preserved across transfer, got %d want %d", moved.TP, target.TP)
	}

	// The real old-owner index entry must be gone.
	if _, found, err := deps.Store.GetNFTByOwner(ownerAddr); err != nil {
		t.Fatalf("get by old owner: %v", err)
	} else if found {
		t.Fatalf("expected the old owner's index entry to be removed after a real transfer")
	}
	byNewOwner, found, err := deps.Store.GetNFTByOwner(newOwner)
	if err != nil || !found {
		t.Fatalf("expected the new owner's index entry to resolve: found=%v err=%v", found, err)
	}
	if byNewOwner.ID != target.ID {
		t.Fatalf("expected new owner's indexed NFT id %s, got %s", target.ID, byNewOwner.ID)
	}
}

// TestPipelineNFTTransferRejectsNonOwnerSigner proves the real
// authorization check: a transfer signed by anyone other than the NFT's
// current real owner is rejected outright, even for an already-unlocked
// NFT.
func TestPipelineNFTTransferRejectsNonOwnerSigner(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	ownerAddr := types.Address{0x1}
	target := types.ValidatorNFT{ID: types.NFTID{0x34}, Owner: ownerAddr}
	target.Traits = map[string]string{"transferable": "true"}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	impostorPK, impostorSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate impostor key: %v", err)
	}
	transferTx := mustBuildNFTTransfer(t, impostorPK, impostorSK, target.ID, types.Address{0x2})
	results := p.ProcessBatch([]tx.Entry{{Tx: transferTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a transfer signed by a non-owner to be rejected")
	}

	still, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("get target nft: found=%v err=%v", found, err)
	}
	if still.Owner != ownerAddr {
		t.Fatalf("expected Owner to be unchanged after a rejected transfer, got %s", still.Owner)
	}
}

// TestPipelineNFTTransferRejectsWhenNewOwnerAlreadyHasNFT proves spec
// 10.1's "one per wallet" invariant survives transfers: a receiving
// wallet that already holds a different, real NFT cannot receive a
// second one.
func TestPipelineNFTTransferRejectsWhenNewOwnerAlreadyHasNFT(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	ownerPK, ownerSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate owner key: %v", err)
	}
	ownerAddr := types.AddressFromPubkey(ownerPK)
	target := types.ValidatorNFT{ID: types.NFTID{0x35}, Owner: ownerAddr}
	target.Traits = map[string]string{"transferable": "true"}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	receiverAddr := types.Address{0x9}
	if err := deps.Store.PutNFT(types.ValidatorNFT{ID: types.NFTID{0x99}, Owner: receiverAddr}); err != nil {
		t.Fatalf("seed receiver's existing nft: %v", err)
	}

	transferTx := mustBuildNFTTransfer(t, ownerPK, ownerSK, target.ID, receiverAddr)
	results := p.ProcessBatch([]tx.Entry{{Tx: transferTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a transfer to a wallet that already holds a different NFT to be rejected")
	}
}

// TestPipelineUnlockTransferNotAppliedWhenRejected proves an unlock
// proposal that fails its vote never sets the trait.
func TestPipelineUnlockTransferNotAppliedWhenRejected(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "unlock-proposal-rejected")

	target := types.ValidatorNFT{ID: types.NFTID{0x36}, Owner: types.Address{0x5}}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	commitTx := mustBuildUnlockTransferVote(t, pk, sk, elig, "unlock-proposal-rejected", false, target.ID)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the unlock-bound vote (rejecting) to be accepted as a real ballot: %v", r[0].Error)
	}
	revealTx := mustRevealUnlockVote(t, pk, sk, elig, "unlock-proposal-rejected", false)
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
	if tallied[0].UnlockTransferApplied {
		t.Fatalf("expected UnlockTransferApplied to stay false for a failed proposal")
	}
	got, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("get target nft: found=%v err=%v", found, err)
	}
	if got.Traits["transferable"] == "true" {
		t.Fatalf("expected the target NFT to remain locked")
	}
}
