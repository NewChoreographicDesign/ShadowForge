package zk

import (
	"crypto/sha256"

	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/accumulator/merkle"
	"github.com/consensys/gnark/std/hash/mimc"
)

// EligibilityCircuit proves spec 9.1's "one NFT, one vote" governance
// eligibility anonymously: unlike the plaintext SignerPubKey -> owner ->
// NFT lookup this replaces (pkg/tx's former requireEligibleVoter), a
// verifier who checks this proof learns only that *some* real, minted
// NFT is voting — never which one.
//
//  1. The prover knows VoterSK such that VoterCommitment (MiMC(VoterSK))
//     — the same value a real Kind NFTMint inserted into the eligibility
//     tree at mint time, see NFTMintPublicInputs.VoterCommitment — exists
//     in the tree at a private path.
//  2. Nullifier = MiMC(VoterSK, ProposalScope) is correctly derived,
//     scoping double-vote prevention to one specific proposal
//     (ProposalScope binds one governance proposal, see
//     types.VoteEligibilityScope): the same NFT can still vote on a
//     different proposal — a different ProposalScope yields an
//     unlinkable nullifier — but cannot cast two ballots on this one,
//     since a second attempt reproduces the identical Nullifier.
//
// A real, disclosed limitation: because a valid proof never reveals
// which leaf it opens, this circuit cannot re-check whether that
// specific NFT has since been slashed (the old plaintext lookup did).
// Revoking an anonymous credential requires either a slashed-leaf
// accumulator or epoch-scoped re-registration, neither of which this
// build implements — see pkg/tx's own doc at the call site for the full
// disclosure. Circuit size mirrors TransferCircuit's own MerkleDepth
// (spec 23's "tiny circuits" Year-1 mitigation).
type EligibilityCircuit struct {
	// Public inputs.
	MerkleRoot    frontend.Variable `gnark:",public"`
	Nullifier     frontend.Variable `gnark:",public"`
	ProposalScope frontend.Variable `gnark:",public"`

	// Private witness.
	VoterSK   frontend.Variable
	LeafIndex frontend.Variable
	Path      [MerkleDepth + 1]frontend.Variable
}

// Define encodes the two properties described in the package doc above.
func (c *EligibilityCircuit) Define(api frontend.API) error {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return err
	}

	h.Reset()
	h.Write(c.VoterSK)
	commitment := h.Sum()

	// Property 1: the commitment exists in the tree at a private path.
	// Path[0] is the claimed leaf pre-image; binding it to the freshly
	// computed commitment ties the Merkle witness to this specific
	// secret rather than an arbitrary tree member.
	api.AssertIsEqual(commitment, c.Path[0])
	mp := merkle.MerkleProof{RootHash: c.MerkleRoot, Path: c.Path[:]}
	mp.VerifyProof(api, &h, c.LeafIndex)

	// Property 2: nullifier correctly derived from VoterSK and the
	// public per-proposal scope.
	h.Reset()
	h.Write(c.VoterSK, c.ProposalScope)
	nullifier := h.Sum()
	api.AssertIsEqual(nullifier, c.Nullifier)

	return nil
}

// voterSKDomain separates DeriveVoterSK's hash input from any other use
// of a wallet's raw secret key bytes elsewhere in this codebase, so the
// same key material can never accidentally produce the same output for
// two different purposes.
var voterSKDomain = []byte("shadowforge-vote-eligibility-voter-sk-v1")

// DeriveVoterSK deterministically derives a wallet's persistent
// eligibility-proof secret from its real signing key material — the same
// deliberate design choice pkg/txbuilder's own voteNonce already makes
// for sealed-ballot nonces (see that function's doc): a wallet that
// stores nothing extra between minting its NFT and later proving
// eligibility with it can still reprove correctly, because this always
// recomputes the identical secret from the same key. It intentionally
// takes a raw []byte rather than crypto.DilithiumPrivateKey so pkg/zk
// doesn't need to import pkg/crypto — every real caller (pkg/txbuilder,
// pkg/govwallet) already holds that key as a []byte-backed type.
func DeriveVoterSK(secretKeyBytes []byte) FieldElement {
	sum := sha256.Sum256(append(append([]byte{}, voterSKDomain...), secretKeyBytes...))
	return FieldElementFromBytes32(sum)
}

// VoterCommitment computes the real eligibility-tree leaf value a Kind
// NFTMint transaction commits (NFTMintPublicInputs.VoterCommitment) and
// a later anonymous vote proves membership against — MiMC(VoterSK),
// matching EligibilityCircuit's in-circuit computation exactly.
func VoterCommitment(voterSK FieldElement) FieldElement {
	return mimcHash(voterSK)
}
