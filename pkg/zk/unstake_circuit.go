package zk

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/accumulator/merkle"
	"github.com/consensys/gnark/std/hash/mimc"
)

// UnstakeCircuit proves a real, previously-staked position (StakeCircuit)
// is being honestly converted into a real, ordinary spendable note,
// exactly once. It is EligibilityCircuit's membership-plus-nullifier
// shape (see that circuit's own doc) combined with MintCircuit's
// output-commitment shape, proving three properties:
//
//  1. The claimed (Principal, StartEpoch) really was committed by some
//     real StakeCircuit proof that is a member of the tree at MerkleRoot,
//     for a claimed secret opening (OwnerSK, Rho) this prover actually
//     knows — ownerPK is derived in-circuit from OwnerSK (mirroring
//     TransferCircuit's InOwnerSK -> ownerPK derivation, not
//     StakeCircuit's own create-side OwnerPK-as-witness shortcut),
//     binding "knows the position's real spend key" to "knows the
//     position's real opening", the same way a Transfer's spend side
//     does for an ordinary note.
//  2. Nullifier is correctly derived from Rho and OwnerSK — MiMC(Rho,
//     OwnerSK), the identical formula NoteSecret.Nullifier() already uses
//     for ordinary notes (this package deliberately reuses it via
//     StakeSecret.Nullifier(), rather than inventing a second formula) —
//     so the same position can never be unstaked twice: a second attempt
//     reproduces the identical Nullifier, which pkg/tx's pipeline checks
//     against the same nullifier-spent set an ordinary Transfer's note
//     nullifiers already share (types.ShieldedTx.Nullifier's own doc).
//  3. OutCommit is a well-formed binding of the claimed FinalAmount to a
//     fresh secret opening (OutOwnerPK, OutRho) — MintCircuit's own
//     property 1, reused here for the position's real proceeds — so the
//     resulting note is spendable later by the exact same TransferCircuit
//     logic that verifies any other note.
//
// FinalAmount itself (principal plus real accrued yield) is NOT computed
// in-circuit: like MintNetAmount for the direct mint path, that
// arithmetic is exact-integer Go (pkg/staking.FinalAmount, using the real
// wall-clock-aware yield formula — see that package's own doc) that
// pkg/tx's pipeline recomputes and checks the claimed FinalAmount against
// before ever trusting this proof's public inputs; the circuit only
// proves FinalAmount was honestly bound into a real note, never what
// value it "should" be. This keeps the circuit free of in-field division,
// exactly like MintCircuit's own design already avoids it.
type UnstakeCircuit struct {
	// Public inputs.
	MerkleRoot  frontend.Variable `gnark:",public"`
	Nullifier   frontend.Variable `gnark:",public"`
	Principal   frontend.Variable `gnark:",public"`
	StartEpoch  frontend.Variable `gnark:",public"`
	FinalAmount frontend.Variable `gnark:",public"`
	OutCommit   frontend.Variable `gnark:",public"`

	// Private witness, the locked position being spent.
	OwnerSK   frontend.Variable
	Rho       frontend.Variable
	LeafIndex frontend.Variable
	Path      [MerkleDepth + 1]frontend.Variable

	// Private witness, the new note being created.
	OutOwnerPK frontend.Variable
	OutRho     frontend.Variable
}

// Define encodes the three properties described in the package doc above.
func (c *UnstakeCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}

	api.ToBinary(c.Principal, valueBits)
	api.ToBinary(c.StartEpoch, epochBits)
	api.ToBinary(c.FinalAmount, valueBits)

	// Property 1: the position's real commitment exists in the tree at a
	// private path, for a claimed opening this prover knows the spend key
	// for.
	h.Reset()
	h.Write(c.OwnerSK)
	ownerPK := h.Sum()

	h.Reset()
	h.Write(c.Principal, c.StartEpoch, ownerPK, c.Rho)
	positionCommit := h.Sum()
	api.AssertIsEqual(positionCommit, c.Path[0])
	mp := merkle.MerkleProof{RootHash: c.MerkleRoot, Path: c.Path[:]}
	mp.VerifyProof(api, &h, c.LeafIndex)

	// Property 2: nullifier correctly derived from Rho and OwnerSK.
	h.Reset()
	h.Write(c.Rho, c.OwnerSK)
	nullifier := h.Sum()
	api.AssertIsEqual(nullifier, c.Nullifier)

	// Property 3: the new note's commitment is a well-formed binding of
	// FinalAmount to a claimed opening.
	h.Reset()
	h.Write(c.FinalAmount, c.OutOwnerPK, c.OutRho)
	outCommit := h.Sum()
	api.AssertIsEqual(outCommit, c.OutCommit)

	return nil
}
