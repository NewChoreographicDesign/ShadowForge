package zk_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// fatalHelper is the minimal subset of *testing.T that buildInput needs.
// *testing.F implements it too (with the identical signatures), so fuzz
// seed-corpus setup can call buildInput directly with the real *testing.F
// it's handed, instead of needing a fake or nil *testing.T.
type fatalHelper interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

// buildInput assembles a valid two-input, two-output shielded transfer:
// two spent notes worth 60 + 40 = 100, split into a 70 payment note and a
// 25 change note, with a fee of 5 (70+25+5 = 100). This exercises all five
// spec-8.1 properties end to end: Merkle membership, opening knowledge,
// nullifier derivation, value conservation, and well-formed new commitments.
func buildInput(t fatalHelper) (zk.TransferInput, *zk.Tree) {
	t.Helper()
	tree := zk.NewTree()

	mkSecret := func(value uint64) zk.NoteSecret {
		sk, err := zk.NewSpendKey()
		if err != nil {
			t.Fatalf("spend key: %v", err)
		}
		rho, err := zk.NewRho()
		if err != nil {
			t.Fatalf("rho: %v", err)
		}
		return zk.NoteSecret{Value: value, OwnerSK: sk, Rho: rho}
	}

	in0 := mkSecret(60)
	in1 := mkSecret(40)
	idx0, err := tree.Insert(in0.Commitment())
	if err != nil {
		t.Fatalf("insert in0: %v", err)
	}
	idx1, err := tree.Insert(in1.Commitment())
	if err != nil {
		t.Fatalf("insert in1: %v", err)
	}

	proof0, err := tree.Prove(idx0)
	if err != nil {
		t.Fatalf("prove in0: %v", err)
	}
	proof1, err := tree.Prove(idx1)
	if err != nil {
		t.Fatalf("prove in1: %v", err)
	}
	if proof0.Root != proof1.Root {
		t.Fatalf("both proofs must share the same root")
	}

	out0 := mkSecret(70)
	out1 := mkSecret(25)

	return zk.TransferInput{
		MerkleRoot: proof0.Root,
		Fee:        5,
		InSecrets:  []zk.NoteSecret{in0, in1},
		InProofs:   []zk.Proof{proof0, proof1},
		OutSecrets: []zk.NoteSecret{out0, out1},
	}, tree
}

func TestShieldedTransferProofRoundTrip(t *testing.T) {
	sys, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	input, _ := buildInput(t)

	proof, err := sys.Prove(input)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	pubWitness, err := zk.PublicWitness(input)
	if err != nil {
		t.Fatalf("public witness: %v", err)
	}
	if err := sys.Verify(proof, pubWitness); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestProofRejectsBrokenValueConservation(t *testing.T) {
	sys, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	input, _ := buildInput(t)
	// Break conservation: bump one output's value without changing its
	// commitment's declared value consistently — the prover must fail
	// because the circuit's own commitment/value binding will fail before
	// value conservation is even checked, which is exactly the point: no
	// malformed witness can produce a passing proof.
	input.OutSecrets[0].Value += 1000

	if _, err := sys.Prove(input); err == nil {
		t.Fatalf("expected proving to fail for a witness that breaks value conservation")
	}
}

func TestProofRejectsWrongMerkleRoot(t *testing.T) {
	sys, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	input, _ := buildInput(t)
	var bogus [32]byte
	bogus[0] = 0xFF
	input.MerkleRoot.SetBytes(bogus[:])

	if _, err := sys.Prove(input); err == nil {
		t.Fatalf("expected proving to fail against a wrong Merkle root")
	}
}

func TestVerifyRejectsTamperedPublicInput(t *testing.T) {
	sys, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	input, _ := buildInput(t)
	proof, err := sys.Prove(input)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}

	// Verifying against a witness for a different fee must fail: the
	// public input the verifier checks no longer matches what was proved.
	tampered := input
	tampered.Fee = 6
	pubWitness, err := zk.PublicWitness(tampered)
	if err != nil {
		t.Fatalf("public witness: %v", err)
	}
	if err := sys.Verify(proof, pubWitness); err == nil {
		t.Fatalf("expected verification to fail against a tampered public fee")
	}
}

func TestNullifierDeterministicFromRho(t *testing.T) {
	sk, _ := zk.NewSpendKey()
	rho, _ := zk.NewRho()
	a := zk.NoteSecret{Value: 10, OwnerSK: sk, Rho: rho}
	b := zk.NoteSecret{Value: 10, OwnerSK: sk, Rho: rho}
	if a.Nullifier() != b.Nullifier() {
		t.Fatalf("same (rho, ownerSK) must yield the same nullifier")
	}
	rho2, _ := zk.NewRho()
	c := zk.NoteSecret{Value: 10, OwnerSK: sk, Rho: rho2}
	if a.Nullifier() == c.Nullifier() {
		t.Fatalf("different rho must yield a different nullifier")
	}
}
