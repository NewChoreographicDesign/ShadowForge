package consensus_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestAssignCommitteeDeterministic(t *testing.T) {
	online := consensus.SortNFTIDs([]types.NFTID{nftID(3), nftID(1), nftID(2), nftID(4), nftID(5)})
	a := consensus.AssignCommittee(online, 7, 3)
	b := consensus.AssignCommittee(online, 7, 3)
	if len(a) != 3 || len(b) != 3 {
		t.Fatalf("expected 3 assigned, got %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("two calls with identical inputs must produce identical output: %v vs %v", a, b)
		}
	}
}

func TestAssignCommitteeRotatesWithHeight(t *testing.T) {
	online := consensus.SortNFTIDs([]types.NFTID{nftID(1), nftID(2), nftID(3), nftID(4), nftID(5)})
	a := consensus.AssignCommittee(online, 0, 2)
	b := consensus.AssignCommittee(online, 1, 2)
	if a[0] == b[0] && a[1] == b[1] {
		t.Fatalf("expected the committee to differ across heights, got the same set: %v", a)
	}
}

func TestAssignCommitteeDistinctMembers(t *testing.T) {
	online := consensus.SortNFTIDs([]types.NFTID{nftID(1), nftID(2), nftID(3)})
	got := consensus.AssignCommittee(online, 5, 3)
	seen := map[types.NFTID]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("expected distinct committee members, got a duplicate: %v", got)
		}
		seen[id] = true
	}
}

func TestAssignCommitteeCapsAtOnlineCount(t *testing.T) {
	online := consensus.SortNFTIDs([]types.NFTID{nftID(1), nftID(2)})
	got := consensus.AssignCommittee(online, 0, 5)
	if len(got) != 2 {
		t.Fatalf("expected assignment capped at 2 online validators, got %d", len(got))
	}
}

func TestAssignCommitteeEmptyOnline(t *testing.T) {
	if got := consensus.AssignCommittee(nil, 0, 5); got != nil {
		t.Fatalf("expected nil for an empty online set, got %v", got)
	}
}

// TestAssignCommitteeRefusesLoneValidator proves the real fix for a real
// fork found live: a single online validator must never get a non-empty
// committee, since BFTQuorumMet(1, 1) would let it unilaterally
// self-quorum with zero real agreement — see MinCommitteeSize's own doc
// for the full story (two nodes each alone during the heartbeat cold
// -start window independently committed different blocks at the same
// height).
func TestAssignCommitteeRefusesLoneValidator(t *testing.T) {
	online := consensus.SortNFTIDs([]types.NFTID{nftID(1)})
	if got := consensus.AssignCommittee(online, 0, 5); got != nil {
		t.Fatalf("expected a lone online validator to get an empty committee (must wait, not self-quorum), got %v", got)
	}
}

// TestAssignCommitteeAllowsTwoValidators proves MinCommitteeSize's floor
// doesn't block genuine progress once real two-party agreement is
// possible.
func TestAssignCommitteeAllowsTwoValidators(t *testing.T) {
	online := consensus.SortNFTIDs([]types.NFTID{nftID(1), nftID(2)})
	got := consensus.AssignCommittee(online, 0, 5)
	if len(got) != 2 {
		t.Fatalf("expected a 2-member committee at exactly MinCommitteeSize online validators, got %v", got)
	}
}

func TestSortNFTIDsIsOrderIndependent(t *testing.T) {
	a := consensus.SortNFTIDs([]types.NFTID{nftID(3), nftID(1), nftID(2)})
	b := consensus.SortNFTIDs([]types.NFTID{nftID(1), nftID(3), nftID(2)})
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("expected the same sorted order regardless of input order: %v vs %v", a, b)
		}
	}
}
