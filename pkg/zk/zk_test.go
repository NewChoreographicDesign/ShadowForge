package zk_test

import (
	"errors"
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

// TestProofFromBytesRejectsHugeCommitmentsClaim is a real, independent
// pentest finding, found by native Go fuzzing (FuzzProofFromBytesNeverPanics)
// rather than static review: gnark-crypto v0.21.0's slice decoder (used
// internally by gnark v0.16.3's groth16.Proof.ReadFrom for the Commitments
// field) reads a raw, attacker-controlled uint32 length prefix and
// immediately allocates a slice of that length with no bound check —
// fuzzing found a real 160-byte input that crashed the whole process with
// "fatal error: runtime: out of memory", not a recoverable panic. Since
// Proof bytes arrive over the network as part of an untrusted
// types.ShieldedTx.Proof field, this was a genuine, remote,
// pre-authentication denial-of-service against any validator processing
// such a transaction. This proves ProofFromBytes now rejects the exact
// class of input that caused it — a real, well-formed proof prefix
// (Ar/Bs/Krs) followed by a Commitments-length claim far beyond anything
// a real proof would ever carry — with a clean error instead of crashing.
func TestProofFromBytesRejectsHugeCommitmentsClaim(t *testing.T) {
	sys, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	input, _ := buildInput(t)
	proof, err := sys.Prove(input)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	realBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	// realBytes is Ar(32) || Bs(64) || Krs(32) || commitments-length(4) ||
	// ... — splice in a huge claimed length right where the real (zero)
	// one is, keeping Ar/Bs/Krs genuinely well-formed so this actually
	// exercises the vulnerable field, not an earlier parse failure.
	const prefixLen = 32 + 64 + 32
	if len(realBytes) < prefixLen+4 {
		t.Fatalf("real proof too short (%d bytes) for this test's assumptions", len(realBytes))
	}
	tampered := append([]byte{}, realBytes...)
	tampered[prefixLen] = 0xFF
	tampered[prefixLen+1] = 0xFF
	tampered[prefixLen+2] = 0xFF
	tampered[prefixLen+3] = 0xFF

	if _, err := zk.ProofFromBytes(tampered); !errors.Is(err, zk.ErrProofClaimsTooManyCommitments) {
		t.Fatalf("expected ErrProofClaimsTooManyCommitments, got %v", err)
	}

	// A real, honestly-generated proof (zero commitments) must still work.
	if _, err := zk.ProofFromBytes(realBytes); err != nil {
		t.Fatalf("expected a real proof to still deserialize cleanly, got %v", err)
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
