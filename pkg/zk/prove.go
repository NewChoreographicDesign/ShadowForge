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

// System holds the compiled circuit and the Groth16 proving/verifying keys
// produced by a (development) trusted setup. Spec 23 flags "gnark / circuit
// bugs" as a Year-1 risk whose mitigation is "tiny circuits ... external
// audit, fuzz of the prover/verifier pair" — Setup below is a per-process
// development setup (gnark's unsafe/local Groth16 Setup), not a ceremony;
// production deployment must replace it with an audited multi-party
// ceremony before mainnet, per spec 23 / 18.6's external-audit gate.
type System struct {
	ccs constraint.ConstraintSystem
	pk  groth16.ProvingKey
	vk  groth16.VerifyingKey
}

// compiledCircuit is the shared, uncompiled circuit shape.
func compiledCircuit() (constraint.ConstraintSystem, error) {
	var circuit TransferCircuit
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, fmt.Errorf("zk: compile circuit: %w", err)
	}
	return ccs, nil
}

// Setup compiles TransferCircuit and runs a Groth16 setup, returning a
// System ready to Prove/Verify.
func Setup() (*System, error) {
	ccs, err := compiledCircuit()
	if err != nil {
		return nil, err
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return nil, fmt.Errorf("zk: groth16 setup: %w", err)
	}
	return &System{ccs: ccs, pk: pk, vk: vk}, nil
}

// TransferInput is the Go-friendly form of everything a shielded transfer
// proof needs, built from NoteSecret values and Tree proofs.
type TransferInput struct {
	MerkleRoot FieldElement
	Fee        uint64

	InSecrets []NoteSecret // len must be NumInputs
	InProofs  []Proof      // len must be NumInputs, matching InSecrets order

	OutSecrets []NoteSecret // len must be NumOutputs
}

// Nullifiers returns the public nullifier list this input implies.
func (in TransferInput) Nullifiers() []FieldElement {
	out := make([]FieldElement, len(in.InSecrets))
	for i, s := range in.InSecrets {
		out[i] = s.Nullifier()
	}
	return out
}

// OutCommitments returns the public new-note commitment list this input implies.
func (in TransferInput) OutCommitments() []FieldElement {
	out := make([]FieldElement, len(in.OutSecrets))
	for i, s := range in.OutSecrets {
		out[i] = s.Commitment()
	}
	return out
}

// TransferPublic is the public-input projection of a TransferCircuit
// instance: exactly what a verifier sees (spec 8.1: "Explorers ... do not
// display sender, receiver, or amount"). Unlike TransferInput, building a
// TransferPublic never requires any note secret — a validator running
// Stage 1 (spec 5.3) only ever has these values plus the proof bytes, never
// the spender's witness.
type TransferPublic struct {
	MerkleRoot FieldElement
	Nullifiers []FieldElement
	OutCommits []FieldElement
	Fee        uint64
}

// Public projects a full (prover-side) TransferInput down to what the
// verifier is given. Provers use this to package the public inputs
// alongside a proof (e.g. into types.TransferPublicInputs) without leaking
// the TransferInput type itself past the proving boundary.
func (in TransferInput) Public() TransferPublic {
	return TransferPublic{
		MerkleRoot: in.MerkleRoot,
		Nullifiers: in.Nullifiers(),
		OutCommits: in.OutCommitments(),
		Fee:        in.Fee,
	}
}

func buildAssignment(in TransferInput) (*TransferCircuit, error) {
	if len(in.InSecrets) != NumInputs || len(in.InProofs) != NumInputs {
		return nil, fmt.Errorf("zk: expected %d input notes, got %d secrets / %d proofs", NumInputs, len(in.InSecrets), len(in.InProofs))
	}
	if len(in.OutSecrets) != NumOutputs {
		return nil, fmt.Errorf("zk: expected %d output notes, got %d", NumOutputs, len(in.OutSecrets))
	}

	c := &TransferCircuit{
		MerkleRoot: in.MerkleRoot,
		Fee:        ValueElement(in.Fee),
	}
	for i, s := range in.InSecrets {
		c.Nullifiers[i] = s.Nullifier()
		c.InValue[i] = ValueElement(s.Value)
		c.InOwnerSK[i] = s.OwnerSK
		c.InRho[i] = s.Rho
		c.InLeafIndex[i] = ValueElement(uint64(in.InProofs[i].Index))
		for k, pe := range in.InProofs[i].Path {
			c.InPath[i][k] = pe
		}
	}
	for j, s := range in.OutSecrets {
		c.OutCommits[j] = s.Commitment()
		c.OutValue[j] = ValueElement(s.Value)
		c.OutOwnerPK[j] = s.OwnerPK()
		c.OutRho[j] = s.Rho
	}
	return c, nil
}

// Prove builds a Groth16 proof for in. It also returns the exact public
// inputs (in circuit field order) so the caller can persist them alongside
// the proof for later verification (spec 4.2's ShieldedTx.Proof).
func (s *System) Prove(in TransferInput) (groth16.Proof, error) {
	assignment, err := buildAssignment(in)
	if err != nil {
		return nil, err
	}
	fullWitness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("zk: build witness: %w", err)
	}
	proof, err := groth16.Prove(s.ccs, s.pk, fullWitness)
	if err != nil {
		return nil, fmt.Errorf("zk: prove: %w", err)
	}
	return proof, nil
}

// PublicWitness builds just the public-input witness for in. It is a
// prover-side convenience (in already holds the secrets, so deriving the
// public projection from it is harmless); a real verifier — anyone who
// only has proof bytes plus the public values, never the secrets — must
// use PublicWitnessFromPublic instead.
func PublicWitness(in TransferInput) (witness.Witness, error) {
	return PublicWitnessFromPublic(in.Public())
}

// PublicWitnessFromPublic builds the public-input witness from exactly the
// values a verifier has: no note secrets required. This is what Stage 1
// (spec 5.3) uses to check a submitted proof.
func PublicWitnessFromPublic(pub TransferPublic) (witness.Witness, error) {
	if len(pub.Nullifiers) != NumInputs {
		return nil, fmt.Errorf("zk: expected %d nullifiers, got %d", NumInputs, len(pub.Nullifiers))
	}
	if len(pub.OutCommits) != NumOutputs {
		return nil, fmt.Errorf("zk: expected %d output commitments, got %d", NumOutputs, len(pub.OutCommits))
	}
	c := &TransferCircuit{MerkleRoot: pub.MerkleRoot, Fee: ValueElement(pub.Fee)}
	for i, n := range pub.Nullifiers {
		c.Nullifiers[i] = n
	}
	for i, oc := range pub.OutCommits {
		c.OutCommits[i] = oc
	}
	w, err := frontend.NewWitness(c, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		return nil, fmt.Errorf("zk: build public witness: %w", err)
	}
	return w, nil
}

// Verify checks proof against the public witness.
func (s *System) Verify(proof groth16.Proof, publicWitness witness.Witness) error {
	if err := groth16.Verify(proof, s.vk, publicWitness); err != nil {
		return fmt.Errorf("zk: verify: %w", err)
	}
	return nil
}
