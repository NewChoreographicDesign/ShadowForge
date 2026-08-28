package txbuilder

import (
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// NFTMint builds a real TxNFTMint transaction claiming this wallet's
// free, one-per-wallet soulbound validator NFT (spec 10.1: "User
// requests a micro-drop of SFG... Mint UI presents CAPTCHA and a
// proof-of-humanity challenge... Contract enforces one NFT per wallet").
//
// This package never verifies or fabricates the proof-of-humanity claim
// itself — attestor/attestationIssuedAtMs/attestationSig are a real,
// signed PoHAttestation (pkg/nft.SignPoHAttestation) obtained separately
// from a trusted attestor, mirroring how BankDeposit's oracle quote is
// supplied by its caller rather than invented here. nonce must match the
// exact value the attestation was signed for — pkg/tx's pipeline
// rejects any mismatch, and also rejects an attestor this node doesn't
// currently trust or an attestation older than pkg/nft.PoHAttestationTTL.
//
// This also registers b's real anonymous voter-eligibility commitment
// (NFTMintPublicInputs.VoterCommitment = zk.VoterCommitment(zk.
// DeriveVoterSK(b.sk))) — the same deterministic secret
// pkg/govwallet.New(sk, ...) rederives from this identical b.sk when
// later building a real VoteEligibilityProof, so nothing needs to be
// saved alongside b's own keystore for that to work.
func (b *Builder) NFTMint(nonce uint64, attestationIssuedAtMs int64, attestor crypto.DilithiumPublicKey, attestationSig crypto.DilithiumSignature) (types.ShieldedTx, error) {
	if len(attestor) == 0 || len(attestationSig) == 0 {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: attestor public key and attestation signature are required")
	}
	nullifier, err := randomHash()
	if err != nil {
		return types.ShieldedTx{}, err
	}
	voterSK := zk.DeriveVoterSK(b.sk)
	t := types.ShieldedTx{
		Kind: types.TxNFTMint,
		NFTMintPublicInputs: &types.NFTMintPublicInputs{
			Owner:                 types.AddressFromPubkey(b.pk),
			Nonce:                 nonce,
			AttestationIssuedAtMs: attestationIssuedAtMs,
			Attestor:              []byte(attestor),
			AttestationSig:        types.DilithiumSig(attestationSig),
			VoterCommitment:       types.Hash(zk.ToBytes32(zk.VoterCommitment(voterSK))),
		},
		Nullifier: nullifier,
	}
	return b.finalize(t)
}
