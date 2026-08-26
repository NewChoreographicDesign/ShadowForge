// Package state is the ShadowForge L1 state layer (spec section 7): an
// encrypted account/note KV store backed by Badger, plus the incremental
// Merkle tree whose root becomes the block header's StateRoot. It is not a
// UTXO-only model and not a transparent Ethereum-style account model — the
// user-visible "balance" is a set of unspent shielded notes keyed by
// commitment (spec 4.4).
package state

import (
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// MerkleTree is an append-only binary Merkle tree over leaf hashes
// (commitments or DA blob digests). It keeps every level in memory so a
// Merkle proof can be produced for any leaf, matching spec 7's "light
// clients verify Merkle proofs against DARoot without seeing note
// contents."
//
// This is intentionally the simple "recompute levels on append" design
// rather than a sparse/incremental-hash-chain optimization: spec 7 only
// requires that the tree "is rebuilt or incrementally updated at Stage 4",
// leaving the strategy open, and the simple version is easiest to audit.
// pkg/tx recomputes the root once per batch (not per transaction), which
// keeps this cheap enough for the 1-second batch cadence at Year-1 scale.
type MerkleTree struct {
	leaves []types.Hash
}

func NewMerkleTree() *MerkleTree {
	return &MerkleTree{}
}

// Append adds a new leaf (typically a note commitment) and returns its index.
func (m *MerkleTree) Append(leaf types.Hash) int {
	m.leaves = append(m.leaves, leaf)
	return len(m.leaves) - 1
}

func (m *MerkleTree) Len() int { return len(m.leaves) }

// Root computes the current Merkle root. An empty tree's root is the zero
// hash. A tree with an odd node count at any level duplicates the last node
// (the standard Bitcoin-style padding rule), so Root is deterministic for
// any leaf count.
func (m *MerkleTree) Root() types.Hash {
	return computeRoot(m.leaves)
}

func computeRoot(leaves []types.Hash) types.Hash {
	if len(leaves) == 0 {
		return types.Hash{}
	}
	level := make([]types.Hash, len(leaves))
	copy(level, leaves)
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		next := make([]types.Hash, len(level)/2)
		for i := 0; i < len(next); i++ {
			next[i] = types.SumHash(level[2*i][:], level[2*i+1][:])
		}
		level = next
	}
	return level[0]
}

// MerkleProof is a sibling-hash path from a leaf to the root.
type MerkleProof struct {
	LeafIndex int
	Siblings  []types.Hash // bottom-up
	// SiblingOnRight[i] is true if Siblings[i] is the right child at that
	// level (i.e. the leaf-side node was the left child).
	SiblingOnRight []bool
}

// Proof builds a MerkleProof for the leaf at index. ok is false if index is
// out of range.
func (m *MerkleTree) Proof(index int) (MerkleProof, bool) {
	if index < 0 || index >= len(m.leaves) {
		return MerkleProof{}, false
	}
	level := make([]types.Hash, len(m.leaves))
	copy(level, m.leaves)
	proof := MerkleProof{LeafIndex: index}
	idx := index
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		var sibling types.Hash
		var siblingOnRight bool
		if idx%2 == 0 {
			sibling = level[idx+1]
			siblingOnRight = true
		} else {
			sibling = level[idx-1]
			siblingOnRight = false
		}
		proof.Siblings = append(proof.Siblings, sibling)
		proof.SiblingOnRight = append(proof.SiblingOnRight, siblingOnRight)

		next := make([]types.Hash, len(level)/2)
		for i := 0; i < len(next); i++ {
			next[i] = types.SumHash(level[2*i][:], level[2*i+1][:])
		}
		level = next
		idx /= 2
	}
	return proof, true
}

// VerifyMerkleProof recomputes the root from leaf and proof and checks it
// against root.
func VerifyMerkleProof(leaf types.Hash, proof MerkleProof, root types.Hash) bool {
	cur := leaf
	for i, sib := range proof.Siblings {
		if proof.SiblingOnRight[i] {
			cur = types.SumHash(cur[:], sib[:])
		} else {
			cur = types.SumHash(sib[:], cur[:])
		}
	}
	return cur == root
}
