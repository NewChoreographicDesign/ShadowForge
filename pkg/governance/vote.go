package governance

import (
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// ProposalKind enumerates what a governance vote can do. Slashing requires
// a vote (spec 10.3: "Slash execution is a governance vote that burns or
// freezes the NFT. Automatic silent burns are not in spec").
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
// "governance weight (via NFT + optional stake)".
type Ballot struct {
	Voter   types.NFTID
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
	seen := map[types.NFTID]bool{}
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
