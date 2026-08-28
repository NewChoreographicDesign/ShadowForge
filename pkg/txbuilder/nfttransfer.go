package txbuilder

import (
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// NFTTransfer builds a real Kind NFTTransfer transaction moving target to
// newOwner — see types.TxNFTTransfer's own doc for the full real-check
// list pkg/tx's pipeline enforces (target must actually be unlocked via a
// passed governance vote, this builder's own identity must be target's
// current owner, and newOwner must not already hold a different NFT).
// This builder signs with b's own real key exactly like NFTTrait does:
// unlike Vote/ProposeMint's throwaway-signer pattern, a transfer's real
// authorization IS the signature — Stage 4 checks it resolves to the
// NFT's current owner, so there is no anonymity property to preserve
// here.
func (b *Builder) NFTTransfer(target types.NFTID, newOwner types.Address) (types.ShieldedTx, error) {
	if target.IsZero() {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: transfer target must not be empty")
	}
	nullifier, err := randomHash()
	if err != nil {
		return types.ShieldedTx{}, err
	}
	t := types.ShieldedTx{
		Kind:        types.TxNFTTransfer,
		Commitments: []types.Hash{types.Hash(target)},
		NFTTransferPublicInputs: &types.NFTTransferPublicInputs{
			Target:   target,
			NewOwner: newOwner,
		},
		Nullifier: nullifier,
	}
	return b.finalize(t)
}
