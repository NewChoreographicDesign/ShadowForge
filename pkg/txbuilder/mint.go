package txbuilder

import "github.com/shadowforge/shadowforge-l1/pkg/types"

// Mint builds a real, correctly-signed TxMint transaction — well-formed
// enough that pkg/tx's pipeline accepts it (Stage 2/4 require nothing
// beyond a valid signature for this kind; there is no MintPublicInputs
// structure in this build). Documented honestly, matching this
// codebase's own existing disclosure: actual SFG issuance is an
// epoch-boundary governance decision this L1-core build doesn't execute
// yet (see pkg/tx.Pipeline.TallyDueProposals' own doc and the project
// README's Scope section) — submitting a Mint transaction today is real
// and will be accepted and committed, but has no minting effect on its
// own.
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
