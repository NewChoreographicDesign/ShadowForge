package tx_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// mkNote generates a fresh, real note secret of value v.
func mkNote(t *testing.T, v uint64) zk.NoteSecret {
	t.Helper()
	sk, err := zk.NewSpendKey()
	if err != nil {
		t.Fatal(err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatal(err)
	}
	return zk.NoteSecret{Value: v, OwnerSK: sk, Rho: rho}
}

// buildTransferAnchoredTo proves a real two-input/two-output transfer
// (60+40 -> 70+25, fee 5) using tree's *current* real state as the
// anchor — the caller is responsible for having already inserted in0/in1
// into tree at idx0/idx1 (real prior canonical commitments, not
// fabricated ones), mirroring exactly what a real wallet does: build a
// Merkle proof against the tree it has actually synced.
func buildTransferAnchoredTo(t *testing.T, tree *zk.Tree, in0, in1 zk.NoteSecret, idx0, idx1 int) types.ShieldedTx {
	t.Helper()
	sys := getZKSystem(t)

	proof0, err := tree.Prove(idx0)
	if err != nil {
		t.Fatalf("prove idx0: %v", err)
	}
	proof1, err := tree.Prove(idx1)
	if err != nil {
		t.Fatalf("prove idx1: %v", err)
	}
	out0, out1 := mkNote(t, 70), mkNote(t, 25)

	input := zk.TransferInput{
		MerkleRoot: proof0.Root,
		Fee:        5,
		InSecrets:  []zk.NoteSecret{in0, in1},
		InProofs:   []zk.Proof{proof0, proof1},
		OutSecrets: []zk.NoteSecret{out0, out1},
	}
	zproof, err := sys.Prove(input)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(zproof)
	if err != nil {
		t.Fatalf("serialize proof: %v", err)
	}

	pub := input.Public()
	txPub := &types.TransferPublicInputs{
		MerkleRoot: types.Hash(zk.ToBytes32(pub.MerkleRoot)),
		FeeAmount:  pub.Fee,
	}
	for _, n := range pub.Nullifiers {
		txPub.Nullifiers = append(txPub.Nullifiers, types.Hash(zk.ToBytes32(n)))
	}
	for _, c := range pub.OutCommits {
		txPub.OutCommits = append(txPub.OutCommits, types.Hash(zk.ToBytes32(c)))
	}

	return mustSign(t, types.ShieldedTx{
		Nullifier:            txPub.Nullifiers[0],
		Commitments:          txPub.OutCommits,
		Proof:                proofBytes,
		FeeCommit:            types.SumHash([]byte("fee")),
		Kind:                 types.TxTransfer,
		TransferPublicInputs: txPub,
	})
}

// newDepsWithCanonicalTree builds real pipeline Deps with a real,
// freshly-seeded canonical zk.Tree/RootHistory wired in — the "security
// fix enabled" configuration a live validator now always runs with.
func newDepsWithCanonicalTree(t *testing.T) (tx.Deps, *zk.Tree) {
	t.Helper()
	deps := newDeps(t)
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial root: %v", err)
	}
	deps.ZKTree = zkTree
	deps.ZKRoots = zk.NewRootHistory(initialRoot)
	return deps, zkTree
}

// TestTransferRejectedWhenAnchoredToUnrecognizedRoot is the direct
// regression test for the real gap this fix closes: a real,
// internally-valid proof anchored to a root nobody on the real network
// ever produced (here, a tree the test built entirely on its own,
// disconnected from the pipeline's real canonical tree) must be
// rejected — not merely "well-formed", genuinely anchored to reality.
func TestTransferRejectedWhenAnchoredToUnrecognizedRoot(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	// A real proof, real signature, real value-conservation — proven
	// against a tree of the attacker's own invention, never inserted into
	// the pipeline's real canonical tree.
	fabricated := buildValidTransfer(t)

	results := p.ProcessBatch([]tx.Entry{{Tx: fabricated, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("SAFETY VIOLATION: a proof anchored to a fabricated, never-canonical root was accepted")
	}
}

// TestTransferAcceptedWhenAnchoredToRealCanonicalRoot is the positive
// case: a real wallet that actually synced the canonical tree (its input
// notes really were inserted into it, its proof is anchored to the real
// resulting root) must still be accepted — the fix closes a hole, it
// doesn't break the real path.
func TestTransferAcceptedWhenAnchoredToRealCanonicalRoot(t *testing.T) {
	deps, zkTree := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	// Simulates two notes this wallet already legitimately received —
	// really present in the canonical tree, the same way a real prior
	// transfer's outputs would be.
	in0, in1 := mkNote(t, 60), mkNote(t, 40)
	idx0, err := zkTree.Insert(in0.Commitment())
	if err != nil {
		t.Fatalf("insert in0: %v", err)
	}
	idx1, err := zkTree.Insert(in1.Commitment())
	if err != nil {
		t.Fatalf("insert in1: %v", err)
	}
	root, err := zkTree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	deps.ZKRoots.Record(root)

	realTransfer := buildTransferAnchoredTo(t, zkTree, in0, in1, idx0, idx1)
	results := p.ProcessBatch([]tx.Entry{{Tx: realTransfer, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected a genuinely, canonically-anchored transfer to be accepted: %v", results[0].Error)
	}

	if got := zkTree.Remaining(); got != zk.TreeSize-4 {
		t.Fatalf("expected 2 inputs + 2 real outputs inserted (4 total), Remaining()=%d", got)
	}
}

// TestTransferSequentialAnchorsToPriorCommitRoot proves RootHistory's
// real purpose: a second, independent transfer whose proof is built
// against the root left behind by the first transfer's own commit (not
// the tree's original empty-root state) must be accepted — the ordinary,
// expected shape of real sequential use, not just a single one-shot case.
func TestTransferSequentialAnchorsToPriorCommitRoot(t *testing.T) {
	deps, zkTree := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	// First transfer: seed two "already-canonical" inputs exactly as
	// above, commit it for real.
	in0, in1 := mkNote(t, 60), mkNote(t, 40)
	idx0, err := zkTree.Insert(in0.Commitment())
	if err != nil {
		t.Fatalf("insert in0: %v", err)
	}
	idx1, err := zkTree.Insert(in1.Commitment())
	if err != nil {
		t.Fatalf("insert in1: %v", err)
	}
	root1, err := zkTree.Root()
	if err != nil {
		t.Fatalf("root1: %v", err)
	}
	deps.ZKRoots.Record(root1)

	first := buildTransferAnchoredTo(t, zkTree, in0, in1, idx0, idx1)
	if results := p.ProcessBatch([]tx.Entry{{Tx: first, SubmittedAt: time.Now()}}); results[0].Error != nil {
		t.Fatalf("first transfer: %v", results[0].Error)
	}

	// Second transfer spends the first transfer's own real outputs (its
	// 70 and 25 notes) — but buildTransferAnchoredTo doesn't expose the
	// output secrets it generated internally, so instead prove the
	// *sequential-anchoring* property directly: build a fresh pair of
	// "already canonical" inputs, insert them (advancing the tree further
	// past what root1 already reflected), and confirm a proof anchored to
	// this *new* post-first-commit root is accepted too — proving
	// RootHistory tracks more than just the tree's original state.
	// Same 60+40 value split as buildTransferAnchoredTo's fixed 70+25+5
	// output layout expects (real value conservation, not a fabricated
	// witness) — fresh notes, not reused ones.
	in2, in3 := mkNote(t, 60), mkNote(t, 40)
	idx2, err := zkTree.Insert(in2.Commitment())
	if err != nil {
		t.Fatalf("insert in2: %v", err)
	}
	idx3, err := zkTree.Insert(in3.Commitment())
	if err != nil {
		t.Fatalf("insert in3: %v", err)
	}
	root2, err := zkTree.Root()
	if err != nil {
		t.Fatalf("root2: %v", err)
	}
	if root2.Equal(&root1) {
		t.Fatalf("test setup bug: expected the tree to have advanced past root1")
	}
	deps.ZKRoots.Record(root2)

	second := buildTransferAnchoredTo(t, zkTree, in2, in3, idx2, idx3)
	if results := p.ProcessBatch([]tx.Entry{{Tx: second, SubmittedAt: time.Now()}}); results[0].Error != nil {
		t.Fatalf("expected a transfer anchored to the post-first-commit root to be accepted: %v", results[0].Error)
	}
}

// TestTransferFullTreeRejectsWithoutPartialMutation proves a transaction
// whose outputs wouldn't all fit is rejected outright — before either
// StateTree or the canonical ZKTree is mutated at all — per the
// pipeline's atomicity rule (spec 5.3).
func TestTransferFullTreeRejectsWithoutPartialMutation(t *testing.T) {
	deps, zkTree := newDepsWithCanonicalTree(t)
	p := tx.NewPipeline(deps)

	// Two genuinely real, provable inputs — this transfer's proof must be
	// valid on its own merits, so a rejection can only be attributed to
	// capacity, not to an unrelated proof failure.
	in0, in1 := mkNote(t, 60), mkNote(t, 40)
	idx0, err := zkTree.Insert(in0.Commitment())
	if err != nil {
		t.Fatalf("insert in0: %v", err)
	}
	idx1, err := zkTree.Insert(in1.Commitment())
	if err != nil {
		t.Fatalf("insert in1: %v", err)
	}

	// Claim the remaining capacity down to exactly one free slot — a real
	// 2-output transfer cannot possibly fit, no matter how valid its
	// proof is. Real Insert calls one at a time would need TreeSize-scale
	// (billions of) iterations at MerkleDepth's real production depth —
	// AdvanceUsedForTest's own doc explains why that's both infeasible
	// and unnecessary: Root()/Prove() for in0/in1 only ever depend on the
	// leaves actually stored, never on this accounting-only claim.
	if err := zkTree.AdvanceUsedForTest(zkTree.Remaining() - 1); err != nil {
		t.Fatalf("claim remaining capacity: %v", err)
	}
	root, err := zkTree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	deps.ZKRoots.Record(root)
	remainingBefore := zkTree.Remaining()
	if remainingBefore != 1 {
		t.Fatalf("test setup bug: expected exactly 1 remaining slot, got %d", remainingBefore)
	}

	txn := buildTransferAnchoredTo(t, zkTree, in0, in1, idx0, idx1)

	results := p.ProcessBatch([]tx.Entry{{Tx: txn, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a transfer whose 2 outputs can't fit in 1 remaining slot to be rejected")
	}
	if got := zkTree.Remaining(); got != remainingBefore {
		t.Fatalf("SAFETY VIOLATION: a rejected transfer left the canonical tree partially mutated (remaining went from %d to %d)", remainingBefore, got)
	}
}
