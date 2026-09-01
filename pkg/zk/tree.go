package zk

import (
	"errors"
	"fmt"
	"hash"
	"sync"

	gchash "github.com/consensys/gnark-crypto/hash"

	_ "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc" // registers hash.MIMC_BN254
)

// TreeSize is the fixed leaf capacity implied by MerkleDepth.
const TreeSize = 1 << MerkleDepth

const segmentSize = 32 // bn254 fr.Element byte width

// ErrTreeFull is returned by Insert once TreeSize leaves are occupied.
var ErrTreeFull = errors.New("zk: commitment tree is full for this MerkleDepth")

// Tree is the ZK-circuit-native (MiMC/BN254) note-commitment accumulator
// the TransferCircuit proves membership against. It is intentionally
// separate from pkg/state's general-purpose SHA256 Merkle tree — see the
// pkg/zk package doc.
//
// Root()/Prove() compute against a full, conceptually TreeSize-leaf tree
// (every unused slot a zero leaf) — the fixed-depth shape TransferCircuit's
// in-circuit merkle.MerkleProof gadget always expects, regardless of how
// many leaves are actually in use — but do so in O(used + MerkleDepth)
// time and memory, not O(TreeSize): only the leaves actually inserted are
// ever stored or hashed. Everything beyond them is represented by
// zeroHashes, the precomputed root of an all-zero subtree at each level,
// so a whole empty region collapses to one already-known value instead of
// being hashed leaf by leaf. This is what makes a real production-sized
// MerkleDepth (see that constant's own doc) tractable at all: the
// original implementation materialized and rehashed all TreeSize leaves
// on every single Root()/Prove() call, which is fine at 16 leaves but
// allocates and hashes on the order of TreeSize*segmentSize bytes at any
// depth large enough to matter — completely infeasible once TreeSize
// reaches the billions.
type Tree struct {
	leaves [][segmentSize]byte // only the `used` real leaf pre-images, in insertion order
	used   int
}

// NewTree returns an empty tree.
func NewTree() *Tree {
	return &Tree{}
}

// hasher returns a fresh MiMC/BN254 hash.Hash — the exact primitive
// gnark-crypto's merkletree package uses internally (leafSum/nodeSum in
// its tree.go: Hash(data) for a leaf, Hash(a||b) for an internal node, no
// domain-separation prefix despite that package's own RFC 6962 doc
// comment — the actual code has the prefix commented out). zeroHashes and
// every level fold below replicate that exact, unprefixed convention so a
// Tree built here produces byte-identical roots/proofs to what the old,
// dense implementation produced (verified by this package's existing
// round-trip tests, which exercise the real TransferCircuit end to end).
func hasher() hash.Hash { return gchash.MIMC_BN254.New() }

func leafHash(h hash.Hash, data []byte) [segmentSize]byte {
	h.Reset()
	h.Write(data) //nolint:errcheck // hash.Hash.Write never errors, per its own interface contract
	return to32(h.Sum(nil))
}

func nodeHash(h hash.Hash, a, b [segmentSize]byte) [segmentSize]byte {
	h.Reset()
	h.Write(a[:]) //nolint:errcheck
	h.Write(b[:]) //nolint:errcheck
	return to32(h.Sum(nil))
}

func to32(b []byte) [segmentSize]byte {
	var out [segmentSize]byte
	copy(out[:], b)
	return out
}

// zeroHashes[0] is the leaf hash of an all-zero leaf pre-image; zeroHashes[k]
// (k=1..MerkleDepth) is the Merkle root of a fully-zero subtree of exactly
// 2^k leaves — computed once, lazily, and shared by every Tree, since it
// depends on nothing but MerkleDepth.
var (
	zeroHashesOnce sync.Once
	zeroHashes     [MerkleDepth + 1][segmentSize]byte
)

func computeZeroHashes() {
	h := hasher()
	var zeroLeaf [segmentSize]byte
	zeroHashes[0] = leafHash(h, zeroLeaf[:])
	for k := 1; k <= MerkleDepth; k++ {
		zeroHashes[k] = nodeHash(h, zeroHashes[k-1], zeroHashes[k-1])
	}
}

// Insert places commitment at the next free slot and returns its index.
func (t *Tree) Insert(commitment FieldElement) (int, error) {
	if t.used >= TreeSize {
		return 0, ErrTreeFull
	}
	t.leaves = append(t.leaves, commitment.Bytes())
	idx := t.used
	t.used++
	return idx, nil
}

// Remaining reports how many more commitments this Tree can accept
// before ErrTreeFull. A real caller that must insert several commitments
// as one atomic unit (pkg/tx's pipeline commits every output of one
// transaction together) checks this first, so a transaction whose
// outputs wouldn't all fit is rejected outright — before any of its
// outputs are inserted — rather than partially applied.
func (t *Tree) Remaining() int {
	return TreeSize - t.used
}

// AdvanceUsedForTest claims n additional capacity slots as zero-content
// leaves, without storing them individually, and exists solely so a test
// can exercise Remaining()/ErrTreeFull's near-capacity boundary. A real
// caller never needs this — Insert is the only real way to add leaf
// content — and it exists because MerkleDepth's real production capacity
// (see that constant's own doc) makes literally filling a Tree via
// TreeSize real Insert calls economically infeasible in a test, the way
// it trivially wasn't at the old 16-leaf placeholder depth.
//
// This is sound, not a shortcut that could let a real proof cheat: Root()
// and Prove() are computed purely from the leaves actually stored (see
// fold), and a zero-content leaf slot is indistinguishable from an
// unclaimed one either way — advancing `used` past `len(t.leaves)`
// changes what Remaining() reports, never what any real, already-inserted
// leaf's root or membership path computes to.
func (t *Tree) AdvanceUsedForTest(n int) error {
	if n < 0 || t.used+n > TreeSize {
		return fmt.Errorf("zk: cannot advance used by %d (used=%d, capacity=%d)", n, t.used, TreeSize)
	}
	t.used += n
	return nil
}

// fold walks every real, stored leaf up to the root — never the logical
// used-count AdvanceUsedForTest can inflate past len(t.leaves) — one
// level at a time, optionally tracking the sibling of trackIndex at each
// level (track<0 skips that bookkeeping entirely, for a plain Root()
// call). A level with n active nodes folds to ceil(n/2) parent nodes; a
// level's missing right child (n odd) is exactly zeroHashes[level], never
// rehashed by hand. Both callers guard the len(t.leaves)==0 case before
// ever reaching here, so n is always >= 1 at level 0 and stays >= 1 at
// every level above it.
func (t *Tree) fold(track int) (root [segmentSize]byte, siblings [MerkleDepth][segmentSize]byte) {
	zeroHashesOnce.Do(computeZeroHashes)
	h := hasher()

	level := make([][segmentSize]byte, len(t.leaves))
	for i, l := range t.leaves {
		level[i] = leafHash(h, l[:])
	}

	for depth := 0; depth < MerkleDepth; depth++ {
		n := len(level)
		if track >= 0 {
			var sib [segmentSize]byte
			if track%2 == 0 {
				if track+1 < n {
					sib = level[track+1]
				} else {
					sib = zeroHashes[depth]
				}
			} else {
				sib = level[track-1]
			}
			siblings[depth] = sib
			track /= 2
		}

		next := make([][segmentSize]byte, (n+1)/2)
		for i := range next {
			left := level[2*i]
			right := zeroHashes[depth]
			if 2*i+1 < n {
				right = level[2*i+1]
			}
			next[i] = nodeHash(h, left, right)
		}
		level = next
	}
	root = level[0]
	return
}

// Root returns the current Merkle root as a field element.
func (t *Tree) Root() (FieldElement, error) {
	if len(t.leaves) == 0 {
		zeroHashesOnce.Do(computeZeroHashes)
		var root FieldElement
		root.SetBytes(zeroHashes[MerkleDepth][:])
		return root, nil
	}
	rootBytes, _ := t.fold(-1)
	var root FieldElement
	root.SetBytes(rootBytes[:])
	return root, nil
}

// Proof is a fixed-length (MerkleDepth+1) authentication path, in the exact
// shape gnark's std/accumulator/merkle.MerkleProof expects: Path[0] is the
// leaf pre-image, Path[1:] are sibling hashes bottom-up.
type Proof struct {
	Root  FieldElement
	Path  [MerkleDepth + 1]FieldElement
	Index int
}

// Prove builds the authentication path for the leaf at index. index must
// be a real, previously-Inserted leaf — a slot only ever claimed via
// AdvanceUsedForTest has no real pre-image to prove.
func (t *Tree) Prove(index int) (Proof, error) {
	if index < 0 || index >= len(t.leaves) {
		return Proof{}, fmt.Errorf("zk: index %d out of range [0,%d)", index, len(t.leaves))
	}
	rootBytes, siblings := t.fold(index)

	var p Proof
	p.Root.SetBytes(rootBytes[:])
	p.Index = index
	p.Path[0].SetBytes(t.leaves[index][:])
	for i, sib := range siblings {
		p.Path[i+1].SetBytes(sib[:])
	}
	return p, nil
}
