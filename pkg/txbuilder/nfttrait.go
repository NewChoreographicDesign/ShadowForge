package txbuilder

import (
	"encoding/binary"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// traitCommitmentDomain separates NFTTrait delta commitments from any
// other use of SumHash in this package.
var traitCommitmentDomain = []byte("shadowforge-txbuilder-trait-delta-v1")

// ComputeTraitDeltaCommitment is this package's own, documented
// commitment scheme for an NFTTrait's shielded delta — the pipeline
// (pkg/tx, Stage 4) only ever stores this commitment opaquely; nothing in
// this build currently decrypts or verifies what it opens to (see that
// stage's own doc: "decrypting and applying the actual numeric delta
// requires the receiver's viewing key and happens client-side"), the same
// disclosed boundary types.ComputeVoteCommitment's doc describes for
// votes. salt must be kept by whoever needs to later prove what this
// commitment opens to — this package never stores it for you.
func ComputeTraitDeltaCommitment(key string, delta int64, salt []byte) types.Hash {
	var deltaBytes [8]byte
	binary.BigEndian.PutUint64(deltaBytes[:], uint64(delta))
	return types.SumHash(traitCommitmentDomain, []byte(key), deltaBytes[:], salt)
}

// NFTTrait builds a real TxNFTTrait transaction updating target's trait
// key by a shielded delta. target must already be a minted NFT — Stage 4
// looks it up and rejects the transaction outright if it doesn't exist,
// so this isn't a way to create a new NFT record, only to record a
// committed update against one that's real. salt should be freshly random
// per call (crypto/rand, not reused) unless the caller has a specific
// reason to derive it deterministically; this package never generates one
// on the caller's behalf, since only the caller knows whether it needs to
// be recoverable later.
//
// A real, disclosed limitation this shares with Mint (see that
// constructor's own doc): nothing on this build's live path ever calls
// state.Store.PutNFT to create a fresh ValidatorNFT record, so on a
// freshly deployed network there is currently no target this can ever
// successfully reference.
func (b *Builder) NFTTrait(target types.NFTID, key string, delta int64, salt []byte) (types.ShieldedTx, error) {
	if key == "" {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: trait key must not be empty")
	}
	if len(salt) == 0 {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: salt must not be empty")
	}
	nullifier, err := randomHash()
	if err != nil {
		return types.ShieldedTx{}, err
	}
	t := types.ShieldedTx{
		Kind:        types.TxNFTTrait,
		Commitments: []types.Hash{types.Hash(target)},
		TraitPublicInputs: &types.TraitPublicInputs{
			Key:             key,
			DeltaCommitment: ComputeTraitDeltaCommitment(key, delta, salt),
		},
		Nullifier: nullifier,
	}
	return b.finalize(t)
}
