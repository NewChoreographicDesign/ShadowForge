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

// TallyVotes counts, for a candidate state root, how many of the supplied
// votes (already signature-verified by the caller) endorse it, and reports
// whether that reaches the BFTQuorumMet majority for the given assignment
// size. Votes for a different state root are counted as dissent, not as
// abstentions — an assigned validator who signs the wrong root does not
// help finalize any root.
func TallyVotes(assigned int, candidate types.Hash, votes []types.Vote) (endorsements int, quorum bool) {
	for _, v := range votes {
		if v.StateRoot == candidate {
			endorsements++
		}
	}
	return endorsements, BFTQuorumMet(assigned, endorsements)
}
