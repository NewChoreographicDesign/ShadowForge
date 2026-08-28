package txbuilder

import "github.com/shadowforge/shadowforge-l1/pkg/types"

// Mint builds a real, correctly-signed TxMint transaction — well-formed
// enough that pkg/tx's pipeline accepts it (Stage 2/4 require nothing
// beyond a valid signature for this kind; there is no MintPublicInputs
// structure in this build). types.TxMint's own doc explains why this
// has no effect and is not the real spec-17.4 epoch-mint mechanism:
// that now lives entirely in TxVote (see this package's ProposeMint and
// types.VotePublicInputs.MintAmount's own doc). Submitting a Mint
// transaction today is real and will be accepted and committed, but
// never has been, and still isn't, how SFG actually gets minted.
func (b *Builder) Mint() (types.ShieldedTx, error) {
	nullifier, err := randomHash()
	if err != nil {
		return types.ShieldedTx{}, err
	}
	t := types.ShieldedTx{
		Kind:      types.TxMint,
		Nullifier: nullifier,
	}
	return b.finalize(t)
}
