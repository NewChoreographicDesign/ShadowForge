package zk

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/hash/mimc"
)

// epochBits bounds StartEpoch (here and in UnstakeCircuit) to a 32-bit
// domain: spec 5.2's epoch numbers grow far more slowly than uint64's
// full range could ever matter (roughly 96 epochs to reach the one-year
// cap, then one more per year — 2^32 years is not a real concern), and a
// bound is required for the identical reason valueBits documents for
// note values: an unconstrained field element could otherwise wrap the
// ~2^254 BN254 modulus.
const epochBits = 32

// StakeCircuit proves spec 17.4's "staked 2 percent yield" mint proposer
// path creates a real, well-formed locked position: PositionCommit is a
// binding commitment to exactly the publicly-voted Principal, created at
// exactly the publicly-claimed StartEpoch, for a claimed secret opening
// (OwnerPK, Rho) — the create-side counterpart of MintCircuit, with one
// addition: StartEpoch is baked directly into the commitment, not tracked
// in any separate server-side record. That is deliberate: because
// UnstakeCircuit (below) proves membership of this same commitment
// anonymously — without ever revealing which leaf it opens — the epoch a
// position started earning yield from cannot be looked up out-of-band by
// leaf identity the way an ordinary database record would allow; it must
// instead be provable purely from knowledge of the (hidden) leaf's own
// preimage, exactly the way Principal itself already is. See
// UnstakeCircuit's own doc for why this closes rather than opens a
// soundness gap.
//
// Structurally identical to MintCircuit's single property (a well-formed
// commitment binding), plus the extra StartEpoch operand — kept as its
// own circuit and its own Groth16 keys anyway, per this codebase's
// standing rule that every distinct real-world claim gets its own proof
// system (see MintCircuit's own doc for why it, in turn, is not simply
// TransferCircuit reused, despite sharing that circuit's OutCommits
// formula).
type StakeCircuit struct {
	// Public inputs.
	Principal      frontend.Variable `gnark:",public"`
	StartEpoch     frontend.Variable `gnark:",public"`
	PositionCommit frontend.Variable `gnark:",public"`

	// Private witness.
	OwnerPK frontend.Variable
	Rho     frontend.Variable
}

// Define encodes the single property described in the package doc above.
func (c *StakeCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}

	api.ToBinary(c.Principal, valueBits)
	api.ToBinary(c.StartEpoch, epochBits)

	h.Reset()
	h.Write(c.Principal, c.StartEpoch, c.OwnerPK, c.Rho)
	commitment := h.Sum()
	api.AssertIsEqual(commitment, c.PositionCommit)

	return nil
}
