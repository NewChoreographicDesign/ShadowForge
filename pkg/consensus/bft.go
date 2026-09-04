package consensus

import "github.com/shadowforge/shadowforge-l1/pkg/types"

// BFTQuorumMet reports whether votes distinct, valid signatures for the
// same candidate state root reach real Byzantine-safe quorum out of
// assigned total validator slots (validatorsPerStage * 5).
//
// Phase 2 independent audit finding (see
// TestBFTQuorumUnsafeAgainstClaimedFaultTolerance): spec 5.7's own literal
// arithmetic ("a majority ... with one validator per stage that is 3 of 5;
// with two per stage that is 6 of 10") is a simple-majority rule, but
// BFTFaultTolerance next door claims tolerance of up to assigned/3
// *Byzantine* (not merely crash-faulty) validators — one who can equivocate
// by signing two different candidates at the same height. A simple
// majority is not safe against that: with a 5-validator committee and
// exactly BFTFaultTolerance(5)=1 equivocating validator, two conflicting
// candidates can each independently collect "3 of 5" and both be
// considered quorum-reached, i.e. two different blocks finalized at the
// same height. Preventing that requires any two quora to overlap by more
// votes than the tolerated fault count, which means votes must exceed
// (assigned+f)/2, not assigned/2. For f == assigned/3 that reduces to the
// classic ">2/3" BFT supermajority (votes*3 > assigned*2) used here, which
// this package's TestBFTQuorumUnsafeAgainstClaimedFaultTolerance and the
// updated TestBFTQuorum*PerStage tests both verify holds for every
// committee size this codebase actually produces (2, 4, 5, 10, 15).
func BFTQuorumMet(assigned, votes int) bool {
	if assigned <= 0 {
		return false
	}
	return votes*3 > assigned*2
}

// BFTFaultTolerance implements spec 5.1: "the protocol tolerates up to one
// third faulty nodes among those who are currently online and assigned."
func BFTFaultTolerance(assigned int) int {
	return assigned / 3
}

// TallyVotes counts, for a candidate state root, how many *distinct*
// committee members endorse it, and reports whether that reaches
// BFTQuorumMet's majority for len(committee).
//
// Two defenses live here that a naive "just count matching votes" tally
// would miss: a vote from an identity not in committee is ignored (an
// outsider cannot pad the count), and a committee member's second vote for
// the same root is not counted twice (one validator, one vote — otherwise
// a single validator could unilaterally manufacture "quorum" by submitting
// duplicate votes). votes must already be signature-verified by the
// caller — TallyVotes only knows about Vote.Validator/StateRoot, not
// cryptographic validity.
func TallyVotes(committee []types.NFTID, candidate types.Hash, votes []types.Vote) (endorsements int, quorum bool) {
	inCommittee := make(map[types.NFTID]bool, len(committee))
	for _, id := range committee {
		inCommittee[id] = true
	}
	counted := make(map[types.NFTID]bool, len(votes))
	for _, v := range votes {
		if v.StateRoot != candidate {
			continue
		}
		if !inCommittee[v.Validator] {
			continue
		}
		if counted[v.Validator] {
			continue
		}
		counted[v.Validator] = true
		endorsements++
	}
	return endorsements, BFTQuorumMet(len(committee), endorsements)
}
