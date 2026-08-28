package governance

import (
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// MinTurnout is the spec's real quorum floor ("governance turnout above
// 20% of eligible NFTs" — docs/SPEC_SOURCE.md). A fixed protocol
// constant, not a
// governance.Params field a passed proposal could adjust — turnout
// itself gating whether a proposal passes must not be something a
// low-turnout proposal could first vote down to make every later
// proposal easier to pass.
var MinTurnout = decimal.MustFromString("0.20")

// ProposalKind enumerates what a governance vote can do. Slashing requires
// a vote (spec 10.3: "Slash execution is a governance vote that burns or
// freezes the NFT. Automatic silent burns are not in spec").
//
// Execution status: ProposalParamChange is real end-to-end — a passed
// proposal actually mutates live Params (see ApplyParamChange, and
// pkg/tx's TallyDueProposals, which is what calls it). ProposalSlashNFT
// is real end-to-end too (types.VotePublicInputs.SlashTargetNFT/
// SlashBurn; TallyDueProposals calls pkg/nft.ApplySlash on a pass, plus
// a real state.Store.DeleteNFT for the burn outcome). ProposalUnlockNFTTransfer
// is real end-to-end too (types.VotePublicInputs.UnlockTransferTarget;
// TallyDueProposals calls the real pkg/nft.UnlockTransfer on a pass,
// and a real types.TxNFTTransfer transaction kind is what actually lets
// the now-unlocked NFT change hands afterward — see that type's own
// doc). The remaining two kinds are tallied identically (real ballots,
// real majority, real persisted Approve/Reject/Passed) but this build
// wires no execution step for a pass: ProposalContainerAsset and
// ProposalUpgradeUnwind have no automated effect at all. A real
// deployment needs a wallet/App-layer or operator step to read a passed
// proposal's Kind and act on it for those two, the same "tally is real,
// automatic execution is a separate later step" split
// TallyDueProposals' own doc already draws for SFG minting.
type ProposalKind uint8

const (
	ProposalParamChange ProposalKind = iota
	ProposalSlashNFT
	ProposalUnlockNFTTransfer
	ProposalContainerAsset
	ProposalUpgradeUnwind // e.g. removing a dual-sign migration path (spec 8.5)
)

// Proposal is one governance item. Votes accumulate as ZKP ballots during
// an epoch and finalize at epoch end (spec 5.2, 17.4).
type Proposal struct {
	ID          types.ID
	Kind        ProposalKind
	Description string
	Epoch       types.EpochNumber
}

// Ballot is one NFT holder's vote. One NFT, one vote — spec 9.1:
// "governance weight (via NFT + optional stake)". Voter is a real
// anonymous ZK eligibility proof's Nullifier (types.
// VoteEligibilityProof.Nullifier), not a named identity — see
// pkg/state.ProposalRecord's own doc for why dedup keys off it.
type Ballot struct {
	Voter   types.Hash
	Approve bool
}

// TallyResult reports the outcome of counting a proposal's ballots.
type TallyResult struct {
	Approve int
	Reject  int
	Turnout decimal.Decimal // fraction of EligibleNFTs that voted
	Passed  bool
}

// Tally counts ballots (deduplicating by voter — only the first ballot from
// each NFT counts) and applies simple majority among cast votes, gated by a
// minimum-turnout requirement. minTurnout of zero means no turnout floor.
func Tally(ballots []Ballot, eligibleNFTs int, minTurnout decimal.Decimal) TallyResult {
	seen := map[types.Hash]bool{}
	var approve, reject int
	for _, b := range ballots {
		if seen[b.Voter] {
			continue
		}
		seen[b.Voter] = true
		if b.Approve {
			approve++
		} else {
			reject++
		}
	}
	cast := approve + reject
	turnout := decimal.Zero
	if eligibleNFTs > 0 {
		turnout = decimal.FromInt(int64(cast)).Div(decimal.FromInt(int64(eligibleNFTs)))
	}
	passed := approve > reject
	if minTurnout.Sign() > 0 && turnout.Cmp(minTurnout) < 0 {
		passed = false
	}
	return TallyResult{Approve: approve, Reject: reject, Turnout: turnout, Passed: passed}
}
