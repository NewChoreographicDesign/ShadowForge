package state_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func leaf(b byte) types.Hash {
	var h types.Hash
	h[0] = b
	return h
}

func TestEmptyTreeRootIsZero(t *testing.T) {
	m := state.NewMerkleTree()
	if !m.Root().IsZero() {
		t.Fatalf("expected zero root for empty tree")
	}
}

func TestRootChangesOnAppend(t *testing.T) {
	m := state.NewMerkleTree()
	r0 := m.Root()
	m.Append(leaf(1))
	r1 := m.Root()
	if r0 == r1 {
		t.Fatalf("root should change after appending a leaf")
	}
	m.Append(leaf(2))
	r2 := m.Root()
	if r1 == r2 {
		t.Fatalf("root should change after appending a second leaf")
	}
}

func TestRootDeterministicForOddLeafCount(t *testing.T) {
	m1 := state.NewMerkleTree()
	m2 := state.NewMerkleTree()
	for _, b := range []byte{1, 2, 3} {
		m1.Append(leaf(b))
		m2.Append(leaf(b))
	}
	if m1.Root() != m2.Root() {
		t.Fatalf("two trees built from the same odd-length leaf sequence must match")
	}
}

func TestMerkleProofRoundTrip(t *testing.T) {
	m := state.NewMerkleTree()
	leaves := []byte{10, 20, 30, 40, 50}
	for _, b := range leaves {
		m.Append(leaf(b))
	}
	root := m.Root()
	for i := range leaves {
		proof, ok := m.Proof(i)
		if !ok {
			t.Fatalf("expected proof for index %d", i)
		}
		if !state.VerifyMerkleProof(leaf(leaves[i]), proof, root) {
			t.Fatalf("proof for leaf %d did not verify against root", i)
		}
	}
}

func TestMerkleProofFailsForWrongLeaf(t *testing.T) {
	m := state.NewMerkleTree()
	m.Append(leaf(1))
	m.Append(leaf(2))
	m.Append(leaf(3))
	root := m.Root()
	proof, _ := m.Proof(0)
	if state.VerifyMerkleProof(leaf(99), proof, root) {
		t.Fatalf("proof must not verify for a substituted leaf")
	}
}

func TestTruncateToRollsBackAppends(t *testing.T) {
	m := state.NewMerkleTree()
	m.Append(leaf(1))
	m.Append(leaf(2))
	snapshot := m.Len()
	rootBefore := m.Root()

	m.Append(leaf(3))
	m.Append(leaf(4))
	if m.Root() == rootBefore {
		t.Fatalf("root should have changed after the tentative appends")
	}

	m.TruncateTo(snapshot)
	if m.Len() != snapshot {
		t.Fatalf("expected len %d after truncate, got %d", snapshot, m.Len())
	}
	if m.Root() != rootBefore {
		t.Fatalf("expected root to match pre-append snapshot after rollback")
	}
}

func TestTruncateToClampsOutOfRange(t *testing.T) {
	m := state.NewMerkleTree()
	m.Append(leaf(1))
	m.TruncateTo(-5)
	if m.Len() != 0 {
		t.Fatalf("negative truncate target should clamp to 0, got len=%d", m.Len())
	}
	m.Append(leaf(2))
	m.TruncateTo(1000)
	if m.Len() != 1 {
		t.Fatalf("truncate target beyond current length should clamp to current length, got len=%d", m.Len())
	}
}

func TestMerkleProofOutOfRange(t *testing.T) {
	m := state.NewMerkleTree()
	m.Append(leaf(1))
	if _, ok := m.Proof(5); ok {
		t.Fatalf("expected Proof to fail for out-of-range index")
	}
}
