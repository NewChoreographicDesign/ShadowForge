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
//  4. Sum(spent values) = sum(new note values) + fee.
//  5. New commitments are well-formed bindings of the claimed secret openings.
//
// Circuit size: spec 23's own risk register says the Year-1 mitigation for
// "gnark / circuit bugs" is "tiny circuits, recursive later, external audit
// ... of the prover/verifier pair." MerkleDepth is deliberately small for
// that reason; pkg/zk's tree is a separate, circuit-native (MiMC over the
// BN254 scalar field) accumulator from pkg/state's SHA256 block-level
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

	// MerkleDepth is intentionally small (16 leaves) per spec 23's "tiny
	// circuits" mitigation for Year-1; raising it for a larger commitment
	// set is a circuit (and trusted-setup) parameter change, not a
	// structural one.
	MerkleDepth = 4
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

// Define encodes the five spec-8.1 constraints described in the package doc.
func (c *TransferCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}

	inSum := frontend.Variable(0)
	for i := 0; i < NumInputs; i++ {
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
