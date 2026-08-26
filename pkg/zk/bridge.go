package zk

import (
	"bytes"
	"fmt"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
)

// ProofToBytes serializes a proof for storage in types.ShieldedTx.Proof
// (spec 4.2: "Proof []byte // gnark proof bytes").
func ProofToBytes(p groth16.Proof) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := p.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("zk: serialize proof: %w", err)
	}
	return buf.Bytes(), nil
}

// ProofFromBytes deserializes a proof previously produced by ProofToBytes.
func ProofFromBytes(data []byte) (groth16.Proof, error) {
	p := groth16.NewProof(ecc.BN254)
	if _, err := p.ReadFrom(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("zk: deserialize proof: %w", err)
	}
	return p, nil
}

// ToBytes32 returns e's canonical 32-byte big-endian representation.
// Because FieldElement and types.Hash are both plain [32]byte-shaped
// values, callers can convert directly (types.Hash(zk.ToBytes32(elem)))
// without pkg/zk needing to import pkg/types. (A free function, not a
// method: FieldElement is an alias for gnark-crypto's fr.Element, and Go
// forbids defining new methods on an aliased external type.)
func ToBytes32(e FieldElement) [32]byte { return e.Bytes() }

// FieldElementFromBytes32 is the inverse of ToBytes32.
func FieldElementFromBytes32(b [32]byte) FieldElement {
	var e FieldElement
	e.SetBytes(b[:])
	return e
}

// VerifyProofBytes deserializes proof and verifies it against in's implied
// public inputs. Prover-side convenience — see PublicWitness's doc.
func (s *System) VerifyProofBytes(proofBytes []byte, in TransferInput) error {
	return s.VerifyPublicProofBytes(proofBytes, in.Public())
}

// VerifyPublicProofBytes deserializes proof and verifies it against pub —
// the call pkg/tx's Stage 1 actually uses, since a validator only ever has
// the public inputs, never the spender's secrets.
func (s *System) VerifyPublicProofBytes(proofBytes []byte, pub TransferPublic) error {
	proof, err := ProofFromBytes(proofBytes)
	if err != nil {
		return err
	}
	pubWitness, err := PublicWitnessFromPublic(pub)
	if err != nil {
		return err
	}
	return s.Verify(proof, pubWitness)
}
