package tx_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/bank"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// mustBuildContainerAssetVote builds a real, well-formed, signed TxVote
// binding proposalID to a real spec-11/19.3 Bank-asset-authorization
// claim against asset.
func mustBuildContainerAssetVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool, asset types.AssetID) types.ShieldedTx {
	t.Helper()
	// mustRevealVote (slash_test.go) hardcodes Nonce{5,5} for every
	// reveal in this package — matching it here, not a distinct value,
	// is what lets that shared helper open this proposal's ballot too.
	nonce := types.Hash{5, 5}
	commitment := types.ComputeVoteCommitment(elig.Nullifier, approve, nonce)
	return mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:           proposalID,
			Commitment:           commitment,
			ContainerAssetTarget: asset,
		},
		VoteEligibility: elig,
	})
}

// TestPipelineContainerAssetProposalAuthorizesRealAsset proves the real,
// end-to-end spec-11/19.3 outcome: a real asset-authorization proposal,
// approved by real revealed ballots, makes the target asset genuinely
// usable for BankDeposit/BankWithdraw — not merely a persisted tally
// outcome.
func TestPipelineContainerAssetProposalAuthorizesRealAsset(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "container-asset-proposal-1")

	// Before the vote passes, a real BankDeposit naming this asset is
	// rejected outright.
	preVoteDeposit := bankDepositTx(t, types.AssetBTC, "60000", "2000")
	if r := p.ProcessBatch([]tx.Entry{{Tx: preVoteDeposit, SubmittedAt: time.Now()}}); r[0].Error == nil {
		t.Fatalf("expected a BankDeposit naming an unauthorized asset to be rejected before the vote passes")
	}

	commitTx := mustBuildContainerAssetVote(t, pk, sk, elig, "container-asset-proposal-1", true, types.AssetBTC)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the asset-authorization-bound vote to be accepted: %v", r[0].Error)
	}
	revealTx := mustRevealVote(t, pk, sk, elig, "container-asset-proposal-1", true)
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	tallied, err := p.TallyDueProposals(1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].ContainerAssetApplied {
		t.Fatalf("expected the proposal to pass and the real authorization to be applied, got %+v", tallied)
	}

	authorized, err := deps.Store.IsAssetAuthorized(types.AssetBTC)
	if err != nil {
		t.Fatalf("check authorization: %v", err)
	}
	if !authorized {
		t.Fatalf("expected BTC to be authorized in the real store after a passed proposal")
	}

	// Now the identical BankDeposit that was rejected above is accepted.
	postVoteDeposit := bankDepositTx(t, types.AssetBTC, "60000", "2000")
	if r := p.ProcessBatch([]tx.Entry{{Tx: postVoteDeposit, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected a BankDeposit naming a now-authorized asset to be accepted: %v", r[0].Error)
	}
}

// TestPipelineContainerAssetProposalRejectsSFG proves the native asset
// can never be the target of an authorization vote — it needs none, and
// the claim is rejected outright rather than silently accepted as a
// no-op.
func TestPipelineContainerAssetProposalRejectsSFG(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "container-asset-proposal-sfg")

	commitTx := mustBuildContainerAssetVote(t, pk, sk, elig, "container-asset-proposal-sfg", true, types.AssetSFG)
	results := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected an asset-authorization claim naming the native SFG asset to be rejected")
	}
	if _, found, err := deps.Store.GetProposal("container-asset-proposal-sfg"); err != nil {
		t.Fatalf("get proposal: %v", err)
	} else if found {
		t.Fatalf("expected no proposal record to exist after a rejected claim")
	}
}

// TestPipelineContainerAssetProposalRejectsAlreadyAuthorized proves a
// second vote to authorize an already-authorized asset is rejected
// outright as a pointless claim, rather than silently accepted.
func TestPipelineContainerAssetProposalRejectsAlreadyAuthorized(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)
	if err := deps.Store.PutAuthorizedAsset(types.AssetBTC); err != nil {
		t.Fatalf("pre-authorize BTC: %v", err)
	}

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "container-asset-proposal-dup")

	commitTx := mustBuildContainerAssetVote(t, pk, sk, elig, "container-asset-proposal-dup", true, types.AssetBTC)
	results := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected an asset-authorization claim against an already-authorized asset to be rejected")
	}
}

// TestPipelineContainerAssetProposalNotAppliedWhenRejected proves an
// asset-authorization claim bound to a proposal that fails its vote
// never executes: the asset stays unauthorized, ContainerAssetApplied
// stays false.
func TestPipelineContainerAssetProposalNotAppliedWhenRejected(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "container-asset-proposal-rejected")

	commitTx := mustBuildContainerAssetVote(t, pk, sk, elig, "container-asset-proposal-rejected", false, types.AssetBTC)
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the asset-authorization-bound vote (rejecting) to be accepted as a real ballot: %v", r[0].Error)
	}
	revealTx := mustRevealVote(t, pk, sk, elig, "container-asset-proposal-rejected", false)
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
	if tallied[0].ContainerAssetApplied {
		t.Fatalf("expected ContainerAssetApplied to stay false for a failed proposal")
	}
	authorized, err := deps.Store.IsAssetAuthorized(types.AssetBTC)
	if err != nil {
		t.Fatalf("check authorization: %v", err)
	}
	if authorized {
		t.Fatalf("expected BTC to remain unauthorized after a failed proposal")
	}
}

// TestPipelineBankDepositAcceptsNativeAssetWithoutAuthorization proves
// the native SFG asset (and the unset zero-value claim existing tests
// rely on) never need a governance vote at all — the gate only applies
// to a real, named external asset.
func TestPipelineBankDepositAcceptsNativeAssetWithoutAuthorization(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	atr := decimal.MustFromString("2000")
	sfgDeposit := mustSign(t, types.ShieldedTx{
		Kind: types.TxBankDeposit,
		BankPublicInputs: &types.BankPublicInputs{
			Asset:     types.AssetSFG,
			ATRUSD:    atr,
			BufferUSD: bank.DepositATRMultiple.Mul(atr),
		},
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: sfgDeposit, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected a BankDeposit naming the native SFG asset to need no authorization: %v", r[0].Error)
	}
}
