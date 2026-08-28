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

// StakeSystem holds the compiled StakeCircuit and its Groth16
// proving/verifying keys — the staked-yield-position-creation counterpart
// of MintSystem (see that type's own doc: same development-setup caveat
// applies).
type StakeSystem struct {
	ccs constraint.ConstraintSystem
	pk  groth16.ProvingKey
	vk  groth16.VerifyingKey
}

func compiledStakeCircuit() (constraint.ConstraintSystem, error) {
	var circuit StakeCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("zk: compile stake circuit: %w", err)
	}
	return ccs, nil
}

// SetupStake compiles StakeCircuit and runs a Groth16 setup, returning a
// StakeSystem ready to Prove/Verify. Like Setup, this is a per-process
// development setup, not a ceremony.
func SetupStake() (*StakeSystem, error) {
	ccs, err := compiledStakeCircuit()
	if err != nil {
		return nil, err
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return nil, fmt.Errorf("zk: stake groth16 setup: %w", err)
	}
	return &StakeSystem{ccs: ccs, pk: pk, vk: vk}, nil
}

// StakePublic is the public-input projection of a StakeCircuit instance —
// exactly what a verifier sees. Building one never requires the
// position's secret opening.
type StakePublic struct {
	Principal      uint64
	StartEpoch     uint64
	PositionCommit FieldElement
}

func buildStakeAssignment(secret StakeSecret) *StakeCircuit {
	return &StakeCircuit{
		Principal:      ValueElement(secret.Principal),
		StartEpoch:     ValueElement(secret.StartEpoch),
		PositionCommit: secret.Commitment(),
		OwnerPK:        secret.OwnerPK(),
		Rho:            secret.Rho,
	}
}

// Prove builds a real Groth16 proof that secret's real commitment
// (secret.Commitment()) opens to exactly secret.Principal, locked from
// exactly secret.StartEpoch — a real staked position is just a
// StakeSecret, later spent by the exact same UnstakeCircuit logic that
// verifies any other position's opening.
func (s *StakeSystem) Prove(secret StakeSecret) (groth16.Proof, error) {
	assignment := buildStakeAssignment(secret)
	fullWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("zk: build stake witness: %w", err)
	}
	proof, err := groth16.Prove(s.ccs, s.pk, fullWitness)
	if err != nil {
		return nil, fmt.Errorf("zk: prove stake: %w", err)
	}
	return proof, nil
}

// StakePublicWitnessFromPublic builds the public-input witness from
// exactly the values a real verifier has: no position secret required.
// This is what pkg/tx's pipeline uses to check a submitted stake proof.
func StakePublicWitnessFromPublic(pub StakePublic) (witness.Witness, error) {
	c := &StakeCircuit{
		Principal:      ValueElement(pub.Principal),
		StartEpoch:     ValueElement(pub.StartEpoch),
		PositionCommit: pub.PositionCommit,
	}
	w, err := frontend.NewWitness(c, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return nil, fmt.Errorf("zk: build public stake witness: %w", err)
	}
	return w, nil
}

// Verify checks proof against the public witness.
func (s *StakeSystem) Verify(proof groth16.Proof, publicWitness witness.Witness) error {
	if err := groth16.Verify(proof, s.vk, publicWitness); err != nil {
		return fmt.Errorf("zk: stake verify: %w", err)
	}
	return nil
}

// VerifyPublicProofBytes deserializes proof and verifies it against pub —
// the call pkg/tx's pipeline actually uses.
func (s *StakeSystem) VerifyPublicProofBytes(proofBytes []byte, pub StakePublic) error {
	proof, err := ProofFromBytes(proofBytes)
	if err != nil {
		return err
	}
	pubWitness, err := StakePublicWitnessFromPublic(pub)
	if err != nil {
		return err
	}
	return s.Verify(proof, pubWitness)
}
