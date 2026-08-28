package txbuilder_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestProposeUnlockTransferThenRealTransferEndToEnd(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	voter := newVoterIdentity(t, deps)
	elig := voter.eligibilityFor(t, "builder-unlock-1")

	ownerPK, ownerSK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate owner key: %v", err)
	}
	ownerAddr := types.AddressFromPubkey(ownerPK)
	target := types.ValidatorNFT{ID: types.NFTID{0x66}, Owner: ownerAddr}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	votetx, err := voter.ProposeUnlockTransfer("builder-unlock-1", true, target.ID, elig)
	if err != nil {
		t.Fatalf("build propose-unlock-transfer: %v", err)
	}
	assertRealSignature(t, votetx)
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed unlock proposal: %v", err)
	}

	revealtx, err := voter.VoteReveal("builder-unlock-1", true, elig)
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
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].UnlockTransferApplied {
		t.Fatalf("expected the proposal to pass and the real unlock to be applied, got %+v", tallied)
	}

	owner := txbuilder.New(ownerPK, ownerSK)
	newOwner := types.Address{0x11}
	transferTx, err := owner.NFTTransfer(target.ID, newOwner)
	if err != nil {
		t.Fatalf("build nft-transfer: %v", err)
	}
	assertRealSignature(t, transferTx)
	if err := runOne(p, transferTx); err != nil {
		t.Fatalf("expected the real pipeline to accept the now-unlocked transfer: %v", err)
	}

	got, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("get transferred nft: found=%v err=%v", found, err)
	}
	if got.Owner != newOwner {
		t.Fatalf("expected Owner %s, got %s", newOwner, got.Owner)
	}
}

func TestProposeUnlockTransferRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.ProposeUnlockTransfer("", true, types.NFTID{0x1}, types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}

func TestProposeUnlockTransferRejectsEmptyTarget(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.ProposeUnlockTransfer("builder-unlock-empty", true, types.NFTID{}, types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty target to be rejected")
	}
}

func TestNFTTransferRejectsEmptyTarget(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.NFTTransfer(types.NFTID{}, types.Address{0x1}); err == nil {
		t.Fatalf("expected an empty transfer target to be rejected")
	}
}
