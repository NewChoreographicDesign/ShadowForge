// Package txbuilder is Tier B priority #3: real, correct-by-construction
// transaction builders for the six ShieldedTx kinds pkg/tx's pipeline
// accepts without a zero-knowledge proof — TxVote, TxVoteReveal,
// TxBankDeposit, TxBankWithdraw, TxMint, TxNFTTrait. (Kind Transfer needs
// a real Groth16 proof over shielded notes and is deliberately out of
// scope here — see pkg/zk and pkg/query's own doc for that boundary.)
//
// Every function in this package builds a types.ShieldedTx that matches
// pkg/tx's pipeline checks exactly — the same TxID/signature scheme
// cmd/walletsim already proves works, extended to every no-proof kind and
// tested directly against the real Pipeline (pkg/tx), not a mock of it.
// A Builder wraps one real, already-unlocked identity (the crypto.
// DilithiumPublicKey/PrivateKey pair pkg/walletkey.Keystore.Unlock
// returns) and signs everything it constructs with it.
package txbuilder

import (
	"crypto/rand"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Builder constructs and signs real ShieldedTx transactions for one
// identity. It never touches the network or persistent storage — every
// method is a pure, synchronous function from inputs to a signed
// transaction, so it's trivially testable and safe to use from any
// context (a CLI, a future wallet UI, or directly in tests) without
// pulling in networking.
type Builder struct {
	pk crypto.DilithiumPublicKey
	sk crypto.DilithiumPrivateKey
	// classicalPK/classicalSK are spec 8.5's optional dual-sign migration
	// aid — nil unless WithClassicalKey is used. When set, finalize
	// co-signs every transaction with a real ed25519 signature over the
	// same TxID, alongside (never instead of) the Dilithium signature
	// pk/sk above always produce — see types.ShieldedTx.ClassicalSig's
	// own doc.
	classicalPK crypto.ClassicalPublicKey
	classicalSK crypto.ClassicalPrivateKey
}

// New wraps an already-unlocked real Dilithium identity. Callers
// typically get pk/sk from pkg/walletkey.Keystore.Unlock.
func New(pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey) *Builder {
	return &Builder{pk: pk, sk: sk}
}

// WithClassicalKey attaches a real ed25519 keypair to b, so every
// transaction it builds from here on also carries a real classical
// co-signature — spec 8.5's dual-sign migration aid, an addition to the
// Dilithium signature b already always produces, never a substitute for
// it. Rejected by pkg/tx's Stage 2 once governance has retired dual-sign
// (types.VotePublicInputs.UnwindDualSign's own doc); a caller has no way
// to know that from here, so it should stop calling WithClassicalKey (or
// building fresh identities without it) once it observes a real,
// tallied, passed ProposalUpgradeUnwind.
func (b *Builder) WithClassicalKey(pk crypto.ClassicalPublicKey, sk crypto.ClassicalPrivateKey) *Builder {
	b.classicalPK = pk
	b.classicalSK = sk
	return b
}

// Identity is this builder's consensus-style identity — the same
// NFTID(SumHash(publicKey)) convention pkg/validator, cmd/walletsim, and
// pkg/walletkey all use, so a transaction this package signs is
// addressed to on-chain state (a voter's ballot, an NFTTrait target's
// owner check) the same way every other part of the system already does.
func (b *Builder) Identity() types.NFTID {
	return types.NFTID(types.SumHash(b.pk))
}

// PublicKey is this builder's real Dilithium public key — needed
// wherever a caller must derive something other than Identity()'s own
// hash from it, e.g. types.AddressFromPubkey for an NFTMint's Owner
// field (see NFTMint's own doc).
func (b *Builder) PublicKey() crypto.DilithiumPublicKey {
	return b.pk
}

// randomHash draws a fresh, unpredictable Hash from a real CSPRNG — used
// wherever a transaction just needs a unique value (a nullifier for a
// kind Stage 1 never double-spend-checks) rather than one that must be
// independently reproducible later.
func randomHash() (types.Hash, error) {
	var h types.Hash
	if _, err := rand.Read(h[:]); err != nil {
		return types.Hash{}, fmt.Errorf("txbuilder: read random bytes: %w", err)
	}
	return h, nil
}

// finalize computes t's TxID (spec 4.1: Hash(proof || commitments ||
// nullifier)) and signs it with this builder's real private key — the
// exact well-formedness Stage 2 checks, applied identically regardless of
// kind. Every constructor in this package ends by calling this.
func (b *Builder) finalize(t types.ShieldedTx) (types.ShieldedTx, error) {
	t.TxID = types.ComputeTxID(t.Proof, t.Commitments, t.Nullifier)
	sig, err := crypto.DilithiumSign(b.sk, t.TxID[:])
	if err != nil {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: sign: %w", err)
	}
	t.Sig = types.DilithiumSig(sig)
	t.SignerPubKey = []byte(b.pk)
	if len(b.classicalSK) > 0 {
		classicalSig, err := crypto.ClassicalSign(b.classicalSK, t.TxID[:])
		if err != nil {
			return types.ShieldedTx{}, fmt.Errorf("txbuilder: classical co-sign: %w", err)
		}
		t.ClassicalSig = []byte(classicalSig)
		t.ClassicalPubKey = []byte(b.classicalPK)
	}
	return t, nil
}
