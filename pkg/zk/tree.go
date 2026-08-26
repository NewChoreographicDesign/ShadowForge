package zk

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/consensys/gnark-crypto/accumulator/merkletree"
	gchash "github.com/consensys/gnark-crypto/hash"

	_ "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc" // registers hash.MIMC_BN254
)

// TreeSize is the fixed leaf capacity implied by MerkleDepth.
const TreeSize = 1 << MerkleDepth

const segmentSize = 32 // bn254 fr.Element byte width

// ErrTreeFull is returned by Insert once TreeSize leaves are occupied.
// MerkleDepth is intentionally small (spec 23 "tiny circuits" Year-1
// mitigation); pkg/tx is expected to run one Tree per batch/epoch window
// rather than growing a single tree unboundedly.
var ErrTreeFull = errors.New("zk: commitment tree is full for this MerkleDepth")

// Tree is the ZK-circuit-native (MiMC/BN254) note-commitment accumulator
// the TransferCircuit proves membership against. It is intentionally
// separate from pkg/state's general-purpose SHA256 Merkle tree — see the
// pkg/zk package doc.
type Tree struct {
	leaves [][segmentSize]byte
	used   int
}

// NewTree returns an empty tree, pre-padded to TreeSize with zero leaves so
// every Proof has a fixed-length, MerkleDepth-consistent path.
func NewTree() *Tree {
	return &Tree{leaves: make([][segmentSize]byte, TreeSize)}
}

// Insert places commitment at the next free slot and returns its index.
func (t *Tree) Insert(commitment FieldElement) (int, error) {
	if t.used >= TreeSize {
		return 0, ErrTreeFull
	}
	idx := t.used
	t.leaves[idx] = commitment.Bytes()
	t.used++
	return idx, nil
}

func (t *Tree) reader() *bytes.Buffer {
	var buf bytes.Buffer
	for _, l := range t.leaves {
		buf.Write(l[:])
	}
	return &buf
}

// Root returns the current Merkle root as a field element.
func (t *Tree) Root() (FieldElement, error) {
	rootBytes, err := merkletree.ReaderRoot(t.reader(), gchash.MIMC_BN254.New(), segmentSize)
	if err != nil {
		return FieldElement{}, fmt.Errorf("zk: tree root: %w", err)
	}
	var root FieldElement
	root.SetBytes(rootBytes)
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

// Prove builds the authentication path for the leaf at index.
func (t *Tree) Prove(index int) (Proof, error) {
	if index < 0 || index >= t.used {
		return Proof{}, fmt.Errorf("zk: index %d out of range [0,%d)", index, t.used)
	}
	rootBytes, proofSet, _, err := merkletree.BuildReaderProof(t.reader(), gchash.MIMC_BN254.New(), segmentSize, uint64(index))
	if err != nil {
		return Proof{}, fmt.Errorf("zk: build proof: %w", err)
	}
	if len(proofSet) != MerkleDepth+1 {
		return Proof{}, fmt.Errorf("zk: proof path length %d, want %d (tree not fully padded to TreeSize?)", len(proofSet), MerkleDepth+1)
	}
	var p Proof
	p.Root.SetBytes(rootBytes)
	p.Index = index
	for i, seg := range proofSet {
		p.Path[i].SetBytes(seg)
	}
	return p, nil
}
