package zk

import (
	"fmt"
	"io"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
)

// WriteTo serializes s's real Groth16 proving and verifying keys — the
// actual "development setup" trusted parameters System's own doc says a
// real deployment must eventually replace with an audited ceremony.
// Persisting them here is what lets every real, independent process that
// needs to Prove or Verify agree on the same ones: a validator node and
// a wallet CLI each calling Setup() on their own would get their own
// randomized, mutually incompatible keys, and a proof built under one
// process's keys could never verify under another's — a real
// interoperability gap this closes. The compiled circuit (ccs) is not
// serialized: ReadSystem recompiles it locally, deterministically, via
// the same compiledCircuit Setup itself uses.
func (s *System) WriteTo(w io.Writer) (int64, error) {
	var total int64
	n, err := s.pk.WriteTo(w)
	total += n
	if err != nil {
		return total, fmt.Errorf("zk: write proving key: %w", err)
	}
	n, err = s.vk.WriteTo(w)
	total += n
	if err != nil {
		return total, fmt.Errorf("zk: write verifying key: %w", err)
	}
	return total, nil
}

// ReadSystem reconstructs a System from a stream WriteTo produced — real,
// previously-generated proving/verifying keys, not a fresh random setup.
func ReadSystem(r io.Reader) (*System, error) {
	ccs, err := compiledCircuit()
	if err != nil {
		return nil, err
	}
	pk := groth16.NewProvingKey(ecc.BN254)
	if _, err := pk.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("zk: read proving key: %w", err)
	}
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("zk: read verifying key: %w", err)
	}
	return &System{ccs: ccs, pk: pk, vk: vk}, nil
}
