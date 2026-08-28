package txbuilder_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestProposeSlashFreezesRealNFTEndToEnd(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "builder-slash-1")

	target := types.ValidatorNFT{ID: types.NFTID{0x55}, Owner: types.Address{0x11}}
	if err := deps.Store.PutNFT(target); err != nil {
		t.Fatalf("seed target nft: %v", err)
	}

	votetx, err := b.ProposeSlash("builder-slash-1", true, target.ID, false, elig)
	if err != nil {
		t.Fatalf("build propose-slash: %v", err)
	}
	assertRealSignature(t, votetx)
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed slash proposal: %v", err)
	}

	revealtx, err := b.VoteReveal("builder-slash-1", true, elig)
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
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].SlashApplied {
		t.Fatalf("expected the proposal to pass and the real slash to be applied, got %+v", tallied)
	}

	got, found, err := deps.Store.GetNFT(target.ID)
	if err != nil || !found {
		t.Fatalf("expected the target NFT record to still exist: found=%v err=%v", found, err)
	}
	if !got.Slashed {
		t.Fatalf("expected the real NFT record to be marked Slashed")
	}
}

func TestProposeSlashRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.ProposeSlash("", true, types.NFTID{0x1}, false, types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}

func TestProposeSlashRejectsEmptyTarget(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.ProposeSlash("builder-slash-empty", true, types.NFTID{}, false, types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty slash target to be rejected")
	}
}
