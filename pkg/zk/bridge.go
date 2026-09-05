package zk

import (
	"bytes"
	"encoding/binary"
	"errors"
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

// ErrProofClaimsTooManyCommitments is returned by ProofFromBytes when the
// encoded proof's own Commitments-slice length prefix exceeds
// maxProofCommitments — see that constant's own doc for why this check
// exists at all.
var ErrProofClaimsTooManyCommitments = errors.New("zk: proof claims an unreasonable number of commitments")

// maxProofCommitments generously bounds how many BN254 G1 points a real
// Groth16 proof's Commitments field can carry from any circuit in this
// codebase (all of which use zero) — real, independent pentest finding:
// gnark-crypto v0.21.0's slice decoder (ecc/bn254/marshal.go, used by
// gnark v0.16.3's groth16.Proof.ReadFrom for the Commitments field) reads
// a raw, attacker-controlled uint32 length prefix and immediately does
// make([]bn254.G1Affine, claimedLen) with no bound check against that
// value or the actual remaining input size. Native Go fuzzing
// (FuzzProofFromBytesNeverPanics) found a real, 160-byte crafted input
// that crashes the process outright with "fatal error: runtime: out of
// memory" — not a panic, and NOT recoverable via recover(), since Proof
// bytes arrive over the real network as part of an untrusted
// types.ShieldedTx.Proof field, this is a genuine, remote,
// pre-authentication denial-of-service against any validator that
// receives such a transaction. This is a bug in a vendored third-party
// dependency this codebase cannot patch in place (gnark-crypto is
// already at its latest released version); validateProofPrefix below
// closes it from this side of the boundary by rejecting any claimed
// commitment count larger than any real proof would ever need, before
// gnark's own decoder ever sees it.
const maxProofCommitments = 64

// validateProofPrefix walks just far enough into data to find the
// Commitments-slice length prefix (immediately after the three
// fixed-shape curve points Ar, Bs, Krs — see groth16/bn254/marshal.go's
// Proof.WriteTo/ReadFrom for the exact field order) and rejects it
// outright if the claimed count exceeds maxProofCommitments, before ever
// handing data to gnark's own ReadFrom. It deliberately does not
// validate anything else about the encoding — gnark's own decoder
// already does that correctly and safely for every fixed-size field;
// this only guards the one field proven exploitable.
//
// pointSize determines each point's real encoded length from its own
// leading byte: gnark-crypto's wire format reserves the top two bits of
// a point's first byte to select compressed (the size ProofToBytes
// always writes) vs uncompressed encoding (ecc/bn254/marshal.go's
// mMask/mUncompressed) — top bits 0b00 means uncompressed (double the
// compressed size), anything else means compressed. Both are small,
// fixed sizes either way (at most 128 bytes for a G2 point), so unlike
// the Commitments slice, no field here can ever cause an unbounded
// allocation — only the offset where the vulnerable length prefix sits
// depends on this.
func validateProofPrefix(data []byte) error {
	const (
		sizeG1Compressed = 32
		sizeG2Compressed = 64
	)
	pointSize := func(data []byte, compressed int) (int, error) {
		if len(data) < 1 {
			return 0, fmt.Errorf("zk: proof too short to contain a point header")
		}
		if data[0]&0xC0 == 0x00 { // top two bits 0b00 => uncompressed
			return compressed * 2, nil
		}
		return compressed, nil
	}

	offset := 0
	for _, compressed := range []int{sizeG1Compressed, sizeG2Compressed, sizeG1Compressed} { // Ar, Bs, Krs
		if offset >= len(data) {
			return fmt.Errorf("zk: proof too short to contain Ar/Bs/Krs")
		}
		n, err := pointSize(data[offset:], compressed)
		if err != nil {
			return err
		}
		offset += n
	}

	if offset+4 > len(data) {
		return fmt.Errorf("zk: proof too short to contain a commitments-length prefix")
	}
	claimed := binary.BigEndian.Uint32(data[offset : offset+4])
	if claimed > maxProofCommitments {
		return ErrProofClaimsTooManyCommitments
	}
	return nil
}

// ProofFromBytes deserializes a proof previously produced by ProofToBytes.
func ProofFromBytes(data []byte) (groth16.Proof, error) {
	if err := validateProofPrefix(data); err != nil {
		return nil, fmt.Errorf("zk: deserialize proof: %w", err)
	}
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
