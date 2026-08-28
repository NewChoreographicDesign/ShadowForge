package zk

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
)

// MintCircuit proves spec 17.4's epoch mint is real: that a claimed
// output note commitment (MintOutCommit) really is a well-formed note of
// exactly the publicly-voted Amount, without revealing who it belongs
// to. Unlike TransferCircuit, there is no spent-note side to this
// circuit at all — a passed mint proposal is itself the governance
// authorization for this specific amount of new value to exist, the
// same way a Bank deposit's oracle-verified buffer is the authorization
// for a BankDeposit's issuance (see pkg/tx's Stage 4 TxVote case for how
// Amount itself is bound to what governance actually voted on, and
// checked against pkg/vault's fee split).
//
//  1. OutCommit is a well-formed binding of Amount to a claimed secret
//     opening (OwnerPK, Rho) — the same commitment formula
//     TransferCircuit's own OutCommits property 5 uses (MiMC(value,
//     ownerPK, rho)), so a note this circuit originates is spendable
//     later by the exact same TransferCircuit logic that verifies any
//     other note's opening.
//  2. Amount is range-constrained to this codebase's uint64 domain
//     (valueBits, shared with TransferCircuit) for the identical reason
//     documented there: an unconstrained Amount could wrap the ~2^254
//     BN254 field modulus and let a later Transfer spending this note
//     fabricate value the mint proposal never actually authorized.
type MintCircuit struct {
	// Public inputs.
	Amount    frontend.Variable `gnark:",public"`
	OutCommit frontend.Variable `gnark:",public"`

	// Private witness.
	OwnerPK frontend.Variable
	Rho     frontend.Variable
}

// Define encodes the two properties described in the package doc above.
func (c *MintCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}

	api.ToBinary(c.Amount, valueBits)

	h.Reset()
	h.Write(c.Amount, c.OwnerPK, c.Rho)
	commitment := h.Sum()
	api.AssertIsEqual(commitment, c.OutCommit)

	return nil
}
