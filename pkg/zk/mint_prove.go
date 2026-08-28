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

// MintSystem holds the compiled MintCircuit and its Groth16
// proving/verifying keys — the real epoch-mint counterpart of System
// (see that type's own doc: same development-setup caveat applies).
type MintSystem struct {
	ccs constraint.ConstraintSystem
	pk  groth16.ProvingKey
	vk  groth16.VerifyingKey
}

func compiledMintCircuit() (constraint.ConstraintSystem, error) {
	var circuit MintCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("zk: compile mint circuit: %w", err)
	}
	return ccs, nil
}

// SetupMint compiles MintCircuit and runs a Groth16 setup, returning a
// MintSystem ready to Prove/Verify. Like Setup, this is a per-process
// development setup, not a ceremony.
func SetupMint() (*MintSystem, error) {
	ccs, err := compiledMintCircuit()
	if err != nil {
		return nil, err
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return nil, fmt.Errorf("zk: mint groth16 setup: %w", err)
	}
	return &MintSystem{ccs: ccs, pk: pk, vk: vk}, nil
}

// MintPublic is the public-input projection of a MintCircuit instance —
// exactly what a verifier sees. Building one never requires the note's
// secret opening.
type MintPublic struct {
	Amount    uint64
	OutCommit FieldElement
}

func buildMintAssignment(secret NoteSecret) *MintCircuit {
	return &MintCircuit{
		Amount:    ValueElement(secret.Value),
		OutCommit: secret.Commitment(),
		OwnerPK:   secret.OwnerPK(),
		Rho:       secret.Rho,
	}
}

// Prove builds a real Groth16 proof that secret's real commitment
// (secret.Commitment(), the same formula every other note in this
// codebase already uses) opens to exactly secret.Value — a real minted
// note is just a NoteSecret like any other, spendable later by the
// ordinary TransferCircuit path once its commitment is part of the
// canonical tree.
func (s *MintSystem) Prove(secret NoteSecret) (groth16.Proof, error) {
	assignment := buildMintAssignment(secret)
	fullWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("zk: build mint witness: %w", err)
	}
	proof, err := groth16.Prove(s.ccs, s.pk, fullWitness)
	if err != nil {
		return nil, fmt.Errorf("zk: prove mint: %w", err)
	}
	return proof, nil
}

// MintPublicWitnessFromPublic builds the public-input witness from
// exactly the values a real verifier has: no note secret required. This
// is what pkg/tx's pipeline uses to check a submitted mint proof.
func MintPublicWitnessFromPublic(pub MintPublic) (witness.Witness, error) {
	c := &MintCircuit{Amount: ValueElement(pub.Amount), OutCommit: pub.OutCommit}
	w, err := frontend.NewWitness(c, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return nil, fmt.Errorf("zk: build public mint witness: %w", err)
	}
	return w, nil
}

// Verify checks proof against the public witness.
func (s *MintSystem) Verify(proof groth16.Proof, publicWitness witness.Witness) error {
	if err := groth16.Verify(proof, s.vk, publicWitness); err != nil {
		return fmt.Errorf("zk: mint verify: %w", err)
	}
	return nil
}

// VerifyPublicProofBytes deserializes proof and verifies it against pub
// — the call pkg/tx's pipeline actually uses.
func (s *MintSystem) VerifyPublicProofBytes(proofBytes []byte, pub MintPublic) error {
	proof, err := ProofFromBytes(proofBytes)
	if err != nil {
		return err
	}
	pubWitness, err := MintPublicWitnessFromPublic(pub)
	if err != nil {
		return err
	}
	return s.Verify(proof, pubWitness)
}
