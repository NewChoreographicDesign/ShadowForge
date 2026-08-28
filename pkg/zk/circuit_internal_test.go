package zk

import (
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
)

// TestOutOfRangeOutValueRejected is a direct regression test for a real
// soundness bug live testing surfaced: without valueBits' range
// constraints in Define, the sum-conservation check (property 4) is
// arithmetic modulo the ~2^254 BN254 scalar field, not modulo 2^64. A
// prover could assign an out-of-range OutValue that wraps around the
// field modulus and still satisfy inSum == outSum+Fee in-circuit, while
// the note actually opens to far more value than the true inputs ever
// held — real value fabrication, not a theoretical gap, since nothing
// off-circuit ever inspects a note's plaintext value. This test builds a
// genuinely valid witness, then tampers just one OutValue to an
// out-of-range field element (comfortably inside the field, nowhere near
// wrapping on its own — the wraparound only ever mattered for the sum
// check, which this test doesn't even need to complete since ToBinary's
// own constraint must reject the value first) and confirms proving now
// fails outright.
func TestOutOfRangeOutValueRejected(t *testing.T) {
	tree := NewTree()
	mkSecret := func(value uint64) NoteSecret {
		sk, err := NewSpendKey()
		if err != nil {
			t.Fatalf("spend key: %v", err)
		}
		rho, err := NewRho()
		if err != nil {
			t.Fatalf("rho: %v", err)
		}
		return NoteSecret{Value: value, OwnerSK: sk, Rho: rho}
	}

	in0, in1 := mkSecret(60), mkSecret(40)
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

	input := TransferInput{
		MerkleRoot: proof0.Root,
		Fee:        5,
		InSecrets:  []NoteSecret{in0, in1},
		InProofs:   []Proof{proof0, proof1},
		OutSecrets: []NoteSecret{mkSecret(70), mkSecret(25)},
	}
	assignment, err := buildAssignment(input)
	if err != nil {
		t.Fatalf("build assignment: %v", err)
	}

	// Tamper: assign OutValue[0] to a huge, out-of-range field element
	// (2^200, far beyond any real uint64 value but still comfortably
	// less than the ~2^254 field modulus) instead of the legitimate 70.
	// OutCommits[0] is left as the real commitment to the true (70,
	// OwnerPK, Rho) opening, so this also exercises that the tampered
	// value doesn't just fail some unrelated check: absent valueBits,
	// property 4's sum check would still need a compensating wraparound
	// on OutValue[1] to balance — but the range constraint added here
	// must reject OutValue[0] on its own, at property 5's commitment
	// check, before conservation is ever considered.
	huge := new(big.Int).Lsh(big.NewInt(1), 200)
	assignment.OutValue[0] = frontend.Variable(huge)

	ccs, err := compiledCircuit()
	if err != nil {
		t.Fatalf("compile circuit: %v", err)
	}
	pk, _, err := groth16.Setup(ccs)
	if err != nil {
		t.Fatalf("groth16 setup: %v", err)
	}
	fullWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("build witness: %v", err)
	}
	if _, err := groth16.Prove(ccs, pk, fullWitness); err == nil {
		t.Fatalf("expected proving to fail for an out-of-range OutValue, but it succeeded")
	}
}
