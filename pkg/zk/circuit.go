// Package zk implements the shielded-transfer zero-knowledge circuit (spec
// 3.3: "gnark (Go); Native to the Go node; Groth16 / Plonk style circuits
// for shielded transfers") and its off-circuit witness/tree helpers.
//
// The circuit proves, without revealing values or addresses, exactly the
// five properties spec 8.1 lists:
//
//  1. Each spent note commitment exists in the Merkle tree at a private path.
//  2. The spender knows the opening (value, owner key, rho).
//  3. The nullifier is correctly derived from rho (and the owner's spend
//     key, in this implementation — see Nullifier below) so the same note
//     cannot be spent twice.
//  4. Sum(spent values) = sum(new note values) + fee — every value and
//     the fee are also range-constrained to this codebase's uint64
//     domain (see valueBits below), since the sum check alone is
//     arithmetic modulo the BN254 scalar field: an unconstrained value
//     could otherwise wrap around that modulus and let a prover claim
//     conservation while actually creating value.
//  5. New commitments are well-formed bindings of the claimed secret openings.
//
// Circuit size: spec 23's own risk register says the Year-1 mitigation for
// "gnark / circuit bugs" is "tiny circuits, recursive later, external audit
// ... of the prover/verifier pair." MerkleDepth is now a real, production
// capacity (see its own doc) rather than a 16-leaf placeholder, but the
// circuit itself stays genuinely small in the sense spec 23 actually
// cares about — total constraint count, and therefore setup/proving time
// and audit surface: a real Setup()+Prove()+Verify() cycle at the current
// MerkleDepth still completes in low single-digit seconds (see
// Tree's own doc for why Root()/Prove() don't pay for the full leaf
// capacity either). pkg/zk's tree is a separate, circuit-native (MiMC over
// the BN254 scalar field) accumulator from pkg/state's SHA256 block-level
// Merkle tree — the two serve different audiences (an in-circuit
// membership proof vs. a general-purpose public state root a light client
// checks with plain hashing) and are bridged explicitly by pkg/tx rather
// than being silently assumed to be the same structure.
package zk

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/accumulator/merkle"
	"github.com/consensys/gnark/std/hash/mimc"
)

const (
	// NumInputs / NumOutputs fix the shape of one shielded transfer: up to
	// two spent notes (e.g. a balance note plus a smaller one to make
	// exact change) producing up to two new notes (payment + change).
	NumInputs  = 2
	NumOutputs = 2

	// MerkleDepth is a real, production commitment-tree capacity: depth 32
	// (2^32 = 4,294,967,296 leaves) matches the depth production shielded
	// pools have actually shipped at (e.g. Zcash's Orchard commitment
	// tree) — enough headroom that a live network exhausting it is not a
	// realistic operational concern. This build previously ran at depth 4
	// (16 leaves) as a placeholder — raising the constant alone would have
	// been correct in principle but was not, in practice, a "one-constant
	// change": Tree's original implementation eagerly materialized and
	// rehashed all TreeSize leaves on every Root()/Prove() call, which
	// allocates and hashes on the order of TreeSize*32 bytes — completely
	// infeasible once TreeSize reaches the billions (roughly 137GB just to
	// hold the leaf array at depth 32). Tree's own doc explains the real
	// fix: Root()/Prove() now cost O(used + MerkleDepth), not O(TreeSize),
	// via precomputed all-zero-subtree hashes for everything beyond the
	// leaves actually inserted.
	MerkleDepth = 32
)

// TransferCircuit is the gnark circuit definition for a shielded transfer.
type TransferCircuit struct {
	// Public inputs.
	MerkleRoot frontend.Variable             `gnark:",public"`
	Nullifiers [NumInputs]frontend.Variable  `gnark:",public"`
	OutCommits [NumOutputs]frontend.Variable `gnark:",public"`
	Fee        frontend.Variable             `gnark:",public"`

	// Private witness, per spent note.
	InValue     [NumInputs]frontend.Variable
	InOwnerSK   [NumInputs]frontend.Variable
	InRho       [NumInputs]frontend.Variable
	InLeafIndex [NumInputs]frontend.Variable
	InPath      [NumInputs][MerkleDepth + 1]frontend.Variable

	// Private witness, per new note.
	OutValue   [NumOutputs]frontend.Variable
	OutOwnerPK [NumOutputs]frontend.Variable
	OutRho     [NumOutputs]frontend.Variable
}

// valueBits bounds every note value and the fee to this codebase's own
// uint64 domain (zk.NoteSecret.Value, ValueElement's parameter — every
// value this package's own tooling ever constructs is already in this
// range). Property 4's sum check below is arithmetic modulo the BN254
// scalar field (~2^254), not modulo 2^64: without this bound, a prover
// could choose an out-of-range OutValue that wraps around the field
// modulus so inSum == outSum+Fee holds in-circuit while one output note
// is actually worth far more than the inputs ever were — a real value-
// creation exploit, not merely a theoretical one, since nothing else in
// the pipeline (which never sees plaintext values) would catch it. Two
// inputs plus two outputs plus a fee, each under 2^64, sums to well
// under 2^67 — nowhere near large enough to wrap the ~2^254 field, so
// the addition itself is safe once every term is individually bounded.
const valueBits = 64

// Define encodes the five spec-8.1 constraints described in the package
// doc, plus the range constraints valueBits documents above.
func (c *TransferCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}

	api.ToBinary(c.Fee, valueBits)

	inSum := frontend.Variable(0)
	for i := 0; i < NumInputs; i++ {
		api.ToBinary(c.InValue[i], valueBits)
		// Property 2: the spender knows the opening. ownerPK is derived
		// in-circuit from the claimed spend key, binding "knows the
		// secret key" to "knows the note opening" below.
		h.Reset()
		h.Write(c.InOwnerSK[i])
		ownerPK := h.Sum()

		h.Reset()
		h.Write(c.InValue[i], ownerPK, c.InRho[i])
		commitment := h.Sum()

		// Property 1: the commitment exists in the tree at a private path.
		// InPath[i][0] is the claimed leaf pre-image; binding it to the
		// freshly computed commitment ties the Merkle witness to this
		// specific note rather than an arbitrary tree member.
		api.AssertIsEqual(commitment, c.InPath[i][0])
		mp := merkle.MerkleProof{RootHash: c.MerkleRoot, Path: c.InPath[i][:]}
		mp.VerifyProof(api, &h, c.InLeafIndex[i])

		// Property 3: nullifier correctly derived from rho (spec 8.1),
		// additionally bound to the owner's spend key so a party who only
		// learns rho (e.g. from a leaked memo) cannot forge a valid
		// nullifier without also knowing the spend key.
		h.Reset()
		h.Write(c.InRho[i], c.InOwnerSK[i])
		nullifier := h.Sum()
		api.AssertIsEqual(nullifier, c.Nullifiers[i])

		inSum = api.Add(inSum, c.InValue[i])
	}

	outSum := frontend.Variable(0)
	for j := 0; j < NumOutputs; j++ {
		api.ToBinary(c.OutValue[j], valueBits)

		// Property 5: new commitments are well-formed bindings of the
		// claimed secret openings.
		h.Reset()
		h.Write(c.OutValue[j], c.OutOwnerPK[j], c.OutRho[j])
		commitment := h.Sum()
		api.AssertIsEqual(commitment, c.OutCommits[j])

		outSum = api.Add(outSum, c.OutValue[j])
	}

	// Property 4: value conservation.
	api.AssertIsEqual(inSum, api.Add(outSum, c.Fee))
	return nil
}
