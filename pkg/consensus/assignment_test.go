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

func TestSortNFTIDsIsOrderIndependent(t *testing.T) {
	a := consensus.SortNFTIDs([]types.NFTID{nftID(3), nftID(1), nftID(2)})
	b := consensus.SortNFTIDs([]types.NFTID{nftID(1), nftID(3), nftID(2)})
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("expected the same sorted order regardless of input order: %v vs %v", a, b)
		}
	}
}
