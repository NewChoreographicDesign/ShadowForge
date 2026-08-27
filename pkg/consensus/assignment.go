package consensus

import (
	"sort"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// AssignCommittee deterministically selects up to count distinct
// validators from the online set for the batch at height, without
// requiring a globally-synchronized mutable queue.
//
// Why not just use Revolver directly for this: Revolver models one node's
// local view of a work queue (pop-front-per-stage, push-back-on-success),
// and spec 5.4.1 never specifies how that queue itself is meant to be kept
// identical bit-for-bit across every physical machine on the network. If
// two nodes' local Revolver instances ever pop in a different order —
// which network jitter alone can cause — they'd assign different
// committees to the same height and could never agree. Replicating a
// single logical mutable queue across an asynchronous network is itself a
// consensus problem, which would be circular to build assignment on top
// of.
//
// AssignCommittee sidesteps that: given the same sorted online set and the
// same height, every node computes the identical answer with no
// coordination at all — a pure function, not shared mutable state. It
// still rotates who's assigned as height increases (a rolling window over
// the sorted online set), which is the property spec 5.4.1's revolver is
// actually trying to achieve for BFT assignment (no fixed committee
// forever). Revolver itself is unchanged and still real: it is the
// correct model for spec 5.3.1's within-node stage popping and the
// unfair-insert onboarding behavior, which do not need cross-node
// agreement to be meaningful.
func AssignCommittee(onlineSorted []types.NFTID, height uint64, count int) []types.NFTID {
	n := len(onlineSorted)
	if n == 0 {
		return nil
	}
	if count > n {
		count = n
	}
	if count <= 0 {
		return nil
	}
	start := int(height % uint64(n))
	out := make([]types.NFTID, count)
	for i := 0; i < count; i++ {
		out[i] = onlineSorted[(start+i)%n]
	}
	return out
}

// SortNFTIDs returns a new, ascending-sorted copy of ids — the canonical
// ordering AssignCommittee's callers must feed it so every node derives
// the same "online set" ordering from what may be an unordered map/set of
// observed heartbeats.
func SortNFTIDs(ids []types.NFTID) []types.NFTID {
	out := make([]types.NFTID, len(ids))
	copy(out, ids)
	sort.Slice(out, func(i, j int) bool {
		for k := 0; k < len(out[i]); k++ {
			if out[i][k] != out[j][k] {
				return out[i][k] < out[j][k]
			}
		}
		return false
	})
	return out
}
