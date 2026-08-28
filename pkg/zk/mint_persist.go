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
// or Verify a mint proof. The compiled circuit (ccs) is not serialized:
// ReadMintSystem recompiles it locally, deterministically, via the same
// compiledMintCircuit SetupMint itself uses.
func (s *MintSystem) WriteTo(w io.Writer) (int64, error) {
	var total int64
	n, err := s.pk.WriteTo(w)
	total += n
	if err != nil {
		return total, fmt.Errorf("zk: write mint proving key: %w", err)
	}
	n, err = s.vk.WriteTo(w)
	total += n
	if err != nil {
		return total, fmt.Errorf("zk: write mint verifying key: %w", err)
	}
	return total, nil
}

// ReadMintSystem reconstructs a MintSystem from a stream WriteTo produced.
func ReadMintSystem(r io.Reader) (*MintSystem, error) {
	ccs, err := compiledMintCircuit()
	if err != nil {
		return nil, err
	}
	pk := groth16.NewProvingKey(ecc.BN254)
	if _, err := pk.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("zk: read mint proving key: %w", err)
	}
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("zk: read mint verifying key: %w", err)
	}
	return &MintSystem{ccs: ccs, pk: pk, vk: vk}, nil
}
