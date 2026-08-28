package zk

import (
	"fmt"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// UnstakeSystem holds the compiled UnstakeCircuit and its Groth16
// proving/verifying keys — a distinct circuit and distinct keys from
// StakeSystem (creating a position vs. spending one are different
// claims), mirroring how Transfer's spend side and Mint's create side
// are already separate systems despite sharing a commitment formula.
type UnstakeSystem struct {
	ccs constraint.ConstraintSystem
	pk  groth16.ProvingKey
	vk  groth16.VerifyingKey
}

func compiledUnstakeCircuit() (constraint.ConstraintSystem, error) {
	var circuit UnstakeCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("zk: compile unstake circuit: %w", err)
	}
	return ccs, nil
}

// SetupUnstake compiles UnstakeCircuit and runs a Groth16 setup,
// returning an UnstakeSystem ready to Prove/Verify. Like Setup, this is a
// per-process development setup, not a ceremony.
func SetupUnstake() (*UnstakeSystem, error) {
	ccs, err := compiledUnstakeCircuit()
	if err != nil {
		return nil, err
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return nil, fmt.Errorf("zk: unstake groth16 setup: %w", err)
	}
	return &UnstakeSystem{ccs: ccs, pk: pk, vk: vk}, nil
}

// UnstakeInput is the Go-friendly form of everything a real unstake proof
// needs: the position being spent (and its real membership witness in
// the stake-commitment tree), plus the new note it becomes.
type UnstakeInput struct {
	MerkleRoot FieldElement
	Position   StakeSecret
	Proof      Proof // membership witness for Position.Commitment()
	Out        NoteSecret
}

// Nullifier returns the public nullifier this input implies.
func (in UnstakeInput) Nullifier() FieldElement { return in.Position.Nullifier() }

// OutCommitment returns the public new-note commitment this input implies.
func (in UnstakeInput) OutCommitment() FieldElement { return in.Out.Commitment() }

// UnstakePublic is the public-input projection of an UnstakeCircuit
// instance — exactly what a verifier sees. Building one never requires
// the position's secret opening or the new note's own opening.
type UnstakePublic struct {
	MerkleRoot  FieldElement
	Nullifier   FieldElement
	Principal   uint64
	StartEpoch  uint64
	FinalAmount uint64
	OutCommit   FieldElement
}

// Public projects a full (prover-side) UnstakeInput down to what the
// verifier is given.
func (in UnstakeInput) Public() UnstakePublic {
	return UnstakePublic{
		MerkleRoot:  in.MerkleRoot,
		Nullifier:   in.Nullifier(),
		Principal:   in.Position.Principal,
		StartEpoch:  in.Position.StartEpoch,
		FinalAmount: in.Out.Value,
		OutCommit:   in.OutCommitment(),
	}
}

func buildUnstakeAssignment(in UnstakeInput) *UnstakeCircuit {
	c := &UnstakeCircuit{
		MerkleRoot:  in.MerkleRoot,
		Nullifier:   in.Nullifier(),
		Principal:   ValueElement(in.Position.Principal),
		StartEpoch:  ValueElement(in.Position.StartEpoch),
		FinalAmount: ValueElement(in.Out.Value),
		OutCommit:   in.OutCommitment(),
		OwnerSK:     in.Position.OwnerSK,
		Rho:         in.Position.Rho,
		LeafIndex:   ValueElement(uint64(in.Proof.Index)),
		OutOwnerPK:  in.Out.OwnerPK(),
		OutRho:      in.Out.Rho,
	}
	for k, pe := range in.Proof.Path {
		c.Path[k] = pe
	}
	return c
}

// Prove builds a real Groth16 proof for in.
func (s *UnstakeSystem) Prove(in UnstakeInput) (groth16.Proof, error) {
	assignment := buildUnstakeAssignment(in)
	fullWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("zk: build unstake witness: %w", err)
	}
	proof, err := groth16.Prove(s.ccs, s.pk, fullWitness)
	if err != nil {
		return nil, fmt.Errorf("zk: prove unstake: %w", err)
	}
	return proof, nil
}

// UnstakePublicWitnessFromPublic builds the public-input witness from
// exactly the values a real verifier has: no position or note secret
// required. This is what pkg/tx's pipeline uses to check a submitted
// unstake proof.
func UnstakePublicWitnessFromPublic(pub UnstakePublic) (witness.Witness, error) {
	c := &UnstakeCircuit{
		MerkleRoot:  pub.MerkleRoot,
		Nullifier:   pub.Nullifier,
		Principal:   ValueElement(pub.Principal),
		StartEpoch:  ValueElement(pub.StartEpoch),
		FinalAmount: ValueElement(pub.FinalAmount),
		OutCommit:   pub.OutCommit,
	}
	w, err := frontend.NewWitness(c, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return nil, fmt.Errorf("zk: build public unstake witness: %w", err)
	}
	return w, nil
}

// Verify checks proof against the public witness.
func (s *UnstakeSystem) Verify(proof groth16.Proof, publicWitness witness.Witness) error {
	if err := groth16.Verify(proof, s.vk, publicWitness); err != nil {
		return fmt.Errorf("zk: unstake verify: %w", err)
	}
	return nil
}

// VerifyPublicProofBytes deserializes proof and verifies it against pub
// — the call pkg/tx's pipeline actually uses.
func (s *UnstakeSystem) VerifyPublicProofBytes(proofBytes []byte, pub UnstakePublic) error {
	proof, err := ProofFromBytes(proofBytes)
	if err != nil {
		return err
	}
	pubWitness, err := UnstakePublicWitnessFromPublic(pub)
	if err != nil {
		return err
	}
	return s.Verify(proof, pubWitness)
}
