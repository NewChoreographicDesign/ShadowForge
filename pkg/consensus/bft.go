package consensus

import "github.com/shadowforge/shadowforge-l1/pkg/types"

// BFTQuorumMet implements spec 5.7: "For a batch to finalize at Stage 5, a
// majority of the validators assigned to that batch's stages must sign the
// candidate state root. With one validator per stage that is 3 of 5. With
// two per stage that is 6 of 10." assigned is the total number of
// validator slots for the batch (validatorsPerStage * 5); votes is how many
// distinct, valid signatures were collected for the same state root.
func BFTQuorumMet(assigned, votes int) bool {
	if assigned <= 0 {
		return false
	}
	return votes*2 > assigned
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
