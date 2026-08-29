package txbuilder_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestProposeAuthorizeAssetThenBankDepositEndToEnd(t *testing.T) {
	p, deps := newRealPipeline(t, tx.Deps{})
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "builder-authorize-asset-1")

	quote := oracle.Quote{PriceUSD: decimal.MustFromString("60000"), ATRUSD: decimal.MustFromString("2000")}

	// Before the vote passes, a real BankDeposit naming this asset is
	// rejected outright.
	preVoteDeposit, err := newIdentity(t).BankDepositFromQuote("BTC", quote)
	if err != nil {
		t.Fatalf("build pre-vote deposit: %v", err)
	}
	if err := runOne(p, preVoteDeposit); err == nil {
		t.Fatalf("expected a BankDeposit naming an unauthorized asset to be rejected before the vote passes")
	}

	votetx, err := b.ProposeAuthorizeAsset("builder-authorize-asset-1", true, types.AssetID("BTC"), elig)
	if err != nil {
		t.Fatalf("build propose-authorize-asset: %v", err)
	}
	assertRealSignature(t, votetx)
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed asset-authorization proposal: %v", err)
	}

	revealtx, err := b.VoteReveal("builder-authorize-asset-1", true, elig)
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
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].ContainerAssetApplied {
		t.Fatalf("expected the proposal to pass and the real authorization to be applied, got %+v", tallied)
	}

	authorized, err := deps.Store.IsAssetAuthorized(types.AssetID("BTC"))
	if err != nil {
		t.Fatalf("check authorization: %v", err)
	}
	if !authorized {
		t.Fatalf("expected BTC to be authorized in the real store after a passed proposal")
	}

	postVoteDeposit, err := newIdentity(t).BankDepositFromQuote("BTC", quote)
	if err != nil {
		t.Fatalf("build post-vote deposit: %v", err)
	}
	if err := runOne(p, postVoteDeposit); err != nil {
		t.Fatalf("expected a BankDeposit naming a now-authorized asset to be accepted: %v", err)
	}
}

func TestProposeAuthorizeAssetRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.ProposeAuthorizeAsset("", true, types.AssetID("BTC"), types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}

func TestProposeAuthorizeAssetRejectsEmptyAsset(t *testing.T) {
	b := newIdentity(t)
	if _, err := b.ProposeAuthorizeAsset("builder-authorize-asset-empty", true, types.AssetID(""), types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty asset target to be rejected")
	}
}
