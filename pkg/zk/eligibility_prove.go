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

// EligibilitySystem holds the compiled EligibilityCircuit and its Groth16
// proving/verifying keys — the anonymous-voter-eligibility counterpart of
// System (see that type's own doc: same development-setup caveat
// applies).
type EligibilitySystem struct {
	ccs constraint.ConstraintSystem
	pk  groth16.ProvingKey
	vk  groth16.VerifyingKey
}

func compiledEligibilityCircuit() (constraint.ConstraintSystem, error) {
	var circuit EligibilityCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("zk: compile eligibility circuit: %w", err)
	}
	return ccs, nil
}

// SetupEligibility compiles EligibilityCircuit and runs a Groth16 setup,
// returning an EligibilitySystem ready to Prove/Verify. Like Setup, this
// is a per-process development setup, not a ceremony.
func SetupEligibility() (*EligibilitySystem, error) {
	ccs, err := compiledEligibilityCircuit()
	if err != nil {
		return nil, err
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return nil, fmt.Errorf("zk: eligibility groth16 setup: %w", err)
	}
	return &EligibilitySystem{ccs: ccs, pk: pk, vk: vk}, nil
}

// EligibilityInput is the Go-friendly form of everything a real
// anonymous-eligibility proof needs.
type EligibilityInput struct {
	MerkleRoot    FieldElement
	ProposalScope FieldElement
	VoterSK       FieldElement
	Proof         Proof // membership witness for VoterCommitment(VoterSK)
}

// Nullifier returns the public per-proposal nullifier this input implies.
func (in EligibilityInput) Nullifier() FieldElement {
	return mimcHash(in.VoterSK, in.ProposalScope)
}

// EligibilityPublic is the public-input projection of an
// EligibilityCircuit instance — exactly what a verifier sees. Building
// one never requires VoterSK.
type EligibilityPublic struct {
	MerkleRoot    FieldElement
	Nullifier     FieldElement
	ProposalScope FieldElement
}

// Public projects a full (prover-side) EligibilityInput down to what the
// verifier is given.
func (in EligibilityInput) Public() EligibilityPublic {
	return EligibilityPublic{
		MerkleRoot:    in.MerkleRoot,
		Nullifier:     in.Nullifier(),
		ProposalScope: in.ProposalScope,
	}
}

func buildEligibilityAssignment(in EligibilityInput) *EligibilityCircuit {
	c := &EligibilityCircuit{
		MerkleRoot:    in.MerkleRoot,
		Nullifier:     in.Nullifier(),
		ProposalScope: in.ProposalScope,
		VoterSK:       in.VoterSK,
		LeafIndex:     ValueElement(uint64(in.Proof.Index)),
	}
	for k, pe := range in.Proof.Path {
		c.Path[k] = pe
	}
	return c
}

// Prove builds a Groth16 proof for in.
func (s *EligibilitySystem) Prove(in EligibilityInput) (groth16.Proof, error) {
	assignment := buildEligibilityAssignment(in)
	fullWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("zk: build eligibility witness: %w", err)
	}
	proof, err := groth16.Prove(s.ccs, s.pk, fullWitness)
	if err != nil {
		return nil, fmt.Errorf("zk: prove eligibility: %w", err)
	}
	return proof, nil
}

// EligibilityPublicWitnessFromPublic builds the public-input witness from
// exactly the values a real verifier has: no VoterSK required. This is
// what pkg/tx's pipeline uses to check a submitted eligibility proof.
func EligibilityPublicWitnessFromPublic(pub EligibilityPublic) (witness.Witness, error) {
	c := &EligibilityCircuit{
		MerkleRoot:    pub.MerkleRoot,
		Nullifier:     pub.Nullifier,
		ProposalScope: pub.ProposalScope,
	}
	w, err := frontend.NewWitness(c, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return nil, fmt.Errorf("zk: build public eligibility witness: %w", err)
	}
	return w, nil
}

// Verify checks proof against the public witness.
func (s *EligibilitySystem) Verify(proof groth16.Proof, publicWitness witness.Witness) error {
	if err := groth16.Verify(proof, s.vk, publicWitness); err != nil {
		return fmt.Errorf("zk: eligibility verify: %w", err)
	}
	return nil
}

// VerifyPublicProofBytes deserializes proof and verifies it against pub —
// the call pkg/tx's pipeline actually uses.
func (s *EligibilitySystem) VerifyPublicProofBytes(proofBytes []byte, pub EligibilityPublic) error {
	proof, err := ProofFromBytes(proofBytes)
	if err != nil {
		return err
	}
	pubWitness, err := EligibilityPublicWitnessFromPublic(pub)
	if err != nil {
		return err
	}
	return s.Verify(proof, pubWitness)
}
