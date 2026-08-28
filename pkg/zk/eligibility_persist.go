package zk

import (
	"fmt"
	"io"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
)

// WriteTo serializes s's real Groth16 proving and verifying keys — see
// System.WriteTo's own doc for why this must be shared, not
// independently regenerated, by every real process that needs to Prove
// or Verify an eligibility proof. The compiled circuit (ccs) is not
// serialized: ReadEligibilitySystem recompiles it locally,
// deterministically, via the same compiledEligibilityCircuit SetupEligibility
// itself uses.
func (s *EligibilitySystem) WriteTo(w io.Writer) (int64, error) {
	var total int64
	n, err := s.pk.WriteTo(w)
	total += n
	if err != nil {
		return total, fmt.Errorf("zk: write eligibility proving key: %w", err)
	}
	n, err = s.vk.WriteTo(w)
	total += n
	if err != nil {
		return total, fmt.Errorf("zk: write eligibility verifying key: %w", err)
	}
	return total, nil
}

// ReadEligibilitySystem reconstructs an EligibilitySystem from a stream
// WriteTo produced.
func ReadEligibilitySystem(r io.Reader) (*EligibilitySystem, error) {
	ccs, err := compiledEligibilityCircuit()
	if err != nil {
		return nil, err
	}
	pk := groth16.NewProvingKey(ecc.BN254)
	if _, err := pk.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("zk: read eligibility proving key: %w", err)
	}
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("zk: read eligibility verifying key: %w", err)
	}
	return &EligibilitySystem{ccs: ccs, pk: pk, vk: vk}, nil
}
