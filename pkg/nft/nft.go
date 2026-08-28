// Package nft implements the ShadowForge validator/department NFT
// lifecycle: free one-per-wallet mint, soulbinding, Trust Points, trait
// updates, and governance-gated slashing (spec section 10, spec 4.5).
package nft

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

var (
	ErrAlreadyMinted           = errors.New("nft: wallet already holds a soulbound NFT (one per wallet)")
	ErrProofOfHumanityRequired = errors.New("nft: proof-of-humanity / CAPTCHA challenge not passed")
	ErrTransferDisabled        = errors.New("nft: soulbound transfer is disabled until governance unlocks it")
	ErrNFTSlashed              = errors.New("nft: NFT is slashed and cannot act as a validator")

	// ErrAttestationMismatch means a PoHAttestation was presented for a
	// different owner or a different mint attempt (Nonce) than the one it
	// was bound to at signing time — never a generic "not passed" failure,
	// since the attestation itself may be a perfectly genuine one, just
	// for the wrong mint.
	ErrAttestationMismatch = errors.New("nft: proof-of-humanity attestation does not match this mint (owner/nonce mismatch)")
	// ErrUntrustedAttestor means the attestation's signer is not in the
	// caller-supplied trusted attestor set — a real signature that simply
	// isn't from anyone this chain currently recognizes.
	ErrUntrustedAttestor = errors.New("nft: proof-of-humanity attestation signed by an untrusted attestor")
	// ErrAttestationExpired means the attestation's IssuedAtMs falls
	// outside PoHAttestationTTL of Now — a genuine, correctly-signed
	// attestation that has simply aged out, so it can't be replayed
	// indefinitely against a later mint attempt.
	ErrAttestationExpired = errors.New("nft: proof-of-humanity attestation has expired")
)

// PoHAttestationTTL bounds how long after issuance a PoHAttestation may
// still be used for a mint — long enough for the App/wallet-UI's
// CAPTCHA-and-challenge flow (spec 10.1) to hand off to an on-chain mint
// call, short enough that a captured attestation can't be replayed
// indefinitely against a different mint attempt later on.
const PoHAttestationTTL = 15 * time.Minute

// PoHAttestation is a real, signed claim from a trusted attestor that Owner
// passed a proof-of-humanity/CAPTCHA challenge (spec 10.1: "Mint UI
// presents CAPTCHA and a proof-of-humanity challenge"). Running the actual
// challenge is the App/wallet-UI layer's job, explicitly out of this L1
// core's scope per spec; this package's job is to verify a real
// cryptographic signature over that claim — matching the codebase's
// universal "never trust a bare flag, verify a real Dilithium signature"
// pattern (pkg/crypto.DilithiumVerify, used identically for tx signatures,
// stage votes, and block proposals) — rather than trusting a caller-supplied
// bool the way this package used to.
type PoHAttestation struct {
	Owner      types.Address
	Nonce      uint64 // must equal MintParams.Nonce: binds this attestation to one specific mint attempt
	IssuedAtMs int64
	Attestor   crypto.DilithiumPublicKey
	Sig        crypto.DilithiumSignature
}

// poHAttestationMessage is the exact hash a PoHAttestation.Sig must cover —
// Owner and Nonce bind it to one mint attempt (so a captured attestation
// can't be replayed against a different wallet or a different mint attempt
// by the same wallet), IssuedAtMs anchors PoHAttestationTTL.
func poHAttestationMessage(owner types.Address, nonce uint64, issuedAtMs int64) types.Hash {
	var nonceBytes, tsBytes [8]byte
	binary.LittleEndian.PutUint64(nonceBytes[:], nonce)
	binary.LittleEndian.PutUint64(tsBytes[:], uint64(issuedAtMs))
	return types.SumHash(owner[:], nonceBytes[:], tsBytes[:])
}

// SignPoHAttestation builds and signs a real PoHAttestation with the
// attestor's Dilithium keypair — the App/wallet-UI-layer attestor service's
// side of the protocol, used directly by tests and by any real attestor
// implementation.
func SignPoHAttestation(attestorPK crypto.DilithiumPublicKey, attestorSK crypto.DilithiumPrivateKey, owner types.Address, nonce uint64, issuedAtMs int64) (PoHAttestation, error) {
	msg := poHAttestationMessage(owner, nonce, issuedAtMs)
	sig, err := crypto.DilithiumSign(attestorSK, msg[:])
	if err != nil {
		return PoHAttestation{}, fmt.Errorf("nft: sign proof-of-humanity attestation: %w", err)
	}
	return PoHAttestation{Owner: owner, Nonce: nonce, IssuedAtMs: issuedAtMs, Attestor: attestorPK, Sig: sig}, nil
}

// verify checks att is a genuine, fresh, trusted-attestor signature bound
// to (owner, nonce) — the real check that replaces a bare
// ProofOfHumanityPassed bool.
func (att PoHAttestation) verify(owner types.Address, nonce uint64, now time.Time, trustedAttestors []crypto.DilithiumPublicKey) error {
	if att.Owner != owner || att.Nonce != nonce {
		return ErrAttestationMismatch
	}
	trusted := false
	for _, a := range trustedAttestors {
		if string(a) == string(att.Attestor) {
			trusted = true
			break
		}
	}
	if !trusted {
		return ErrUntrustedAttestor
	}
	msg := poHAttestationMessage(att.Owner, att.Nonce, att.IssuedAtMs)
	ok, err := crypto.DilithiumVerify(att.Attestor, msg[:], att.Sig)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProofOfHumanityRequired, err)
	}
	if !ok {
		return ErrProofOfHumanityRequired
	}
	issuedAt := time.UnixMilli(att.IssuedAtMs)
	age := now.Sub(issuedAt)
	if age < 0 || age > PoHAttestationTTL {
		return ErrAttestationExpired
	}
	return nil
}

// MintParams are the inputs to a free mint (spec 10.1: "User requests a
// micro-drop of SFG... Mint UI presents CAPTCHA and a proof-of-humanity
// challenge... Contract enforces one NFT per wallet").
type MintParams struct {
	Owner         types.Address
	AlreadyHasNFT bool
	// Attestation is the real, signed proof-of-humanity claim (replacing
	// the previous bare ProofOfHumanityPassed bool). It must be signed by
	// one of the caller-supplied TrustedAttestors, bound to this exact
	// Owner/Nonce, and fresh (within PoHAttestationTTL of Now).
	Attestation PoHAttestation
	// TrustedAttestors is the currently-recognized attestor public key
	// set — a governance-configured value the caller supplies, mirroring
	// how pkg/chain.PubKeyLookup and pkg/oracle.Quorum's sources are
	// caller-supplied rather than hardcoded here.
	TrustedAttestors []crypto.DilithiumPublicKey
	MintedAt         types.BlockHeight
	Now              time.Time
	Nonce            uint64 // caller-supplied uniqueness salt (e.g. tx counter); must match Attestation.Nonce
}

// Mint enforces the one-per-wallet gate and a real, verified
// proof-of-humanity attestation (spec 10.1), and returns a fresh soulbound
// NFT with an empty trait map and zero Trust Points.
func Mint(p MintParams) (types.ValidatorNFT, error) {
	if p.AlreadyHasNFT {
		return types.ValidatorNFT{}, ErrAlreadyMinted
	}
	if err := p.Attestation.verify(p.Owner, p.Nonce, p.Now, p.TrustedAttestors); err != nil {
		return types.ValidatorNFT{}, err
	}
	var nonceBytes [8]byte
	for i := 0; i < 8; i++ {
		nonceBytes[i] = byte(p.Nonce >> (8 * i))
	}
	id := types.SumHash(p.Owner[:], nonceBytes[:])
	return types.ValidatorNFT{
		ID:       types.NFTID(id),
		Owner:    p.Owner,
		MintedAt: p.MintedAt,
		Traits:   map[string]string{},
		TP:       0,
	}, nil
}

// CanTransfer reports whether trading is unlocked for this NFT. Soulbound
// NFTs cannot transfer until a governance vote sets the "transferable"
// trait (spec 4.5, 10.1: "Trading stays disabled until a governance vote
// unlocks a transfer trait").
func CanTransfer(nft types.ValidatorNFT) bool {
	return nft.Traits["transferable"] == "true"
}

// UnlockTransfer sets the transferable trait; only a governance vote should
// ever call this (spec 10.1).
func UnlockTransfer(nft *types.ValidatorNFT) {
	if nft.Traits == nil {
		nft.Traits = map[string]string{}
	}
	nft.Traits["transferable"] = "true"
}

// TransferParams are the inputs to a real spec-10.1 soulbound-unlock NFT
// ownership transfer — the actual use ErrTransferDisabled/CanTransfer/
// UnlockTransfer above exist for: a governance-unlocked NFT that could
// never, before this, actually change hands.
type TransferParams struct {
	// NFT is the current, real record being transferred.
	NFT      types.ValidatorNFT
	NewOwner types.Address
	// NewOwnerAlreadyHasNFT mirrors MintParams.AlreadyHasNFT: a real,
	// caller-supplied check (pkg/tx's pipeline, via state.Store.
	// GetNFTByOwner) that spec 10.1's "one per wallet" invariant holds
	// for the receiving wallet too, not just at mint time — a transfer
	// must never let one wallet accumulate two NFTs, since that would
	// undermine the "one NFT, one vote" Sybil resistance every other
	// real governance/eligibility check in this codebase depends on.
	NewOwnerAlreadyHasNFT bool
}

// TransferOwnership enforces spec 10.1's real constraints on a transfer
// and returns the updated record (Owner reassigned), ready for the
// caller to persist. It never touches storage itself, mirroring Mint's
// own pure-validation shape — pkg/tx's pipeline does the real I/O
// (including the real secondary-index bookkeeping a changed Owner
// requires, which this package has no access to).
func TransferOwnership(p TransferParams) (types.ValidatorNFT, error) {
	if p.NFT.Slashed {
		return types.ValidatorNFT{}, ErrNFTSlashed
	}
	if !CanTransfer(p.NFT) {
		return types.ValidatorNFT{}, ErrTransferDisabled
	}
	if p.NewOwnerAlreadyHasNFT {
		return types.ValidatorNFT{}, ErrAlreadyMinted
	}
	updated := p.NFT
	updated.Owner = p.NewOwner
	return updated, nil
}

// --- Trust Points (spec 5.4.2, 10.3) ---

const (
	TPPerSuccessfulStage = 1
	TPPerUptimeSlice     = 1

	// TPGreenEnergyBonus is the extra TP awarded per uptime slice to a
	// validator with a verified renewable-energy attestation (spec 9.3:
	// "Verified green hardware / renewable energy oracle: extra Trust
	// Points").
	TPGreenEnergyBonus = 1
)

// AwardStageSuccess increments TP for a successful stage-work turn (spec
// 5.4.2: "TP: increment on successful stage work").
func AwardStageSuccess(nft *types.ValidatorNFT) {
	nft.TP += TPPerSuccessfulStage
}

// AwardUptimeSlice increments TP for a continuous-uptime slice (spec
// 5.4.2), plus TPGreenEnergyBonus if greenEnergyVerified is true — the
// caller is expected to have already checked that attestation against a
// green-energy oracle (spec 3.3: "Oracles ... green-energy attestations"),
// mirroring how pkg/oracle.Source feeds Bank price/ATR checks.
func AwardUptimeSlice(nft *types.ValidatorNFT, greenEnergyVerified bool) {
	nft.TP += TPPerUptimeSlice
	if greenEnergyVerified {
		nft.TP += TPGreenEnergyBonus
	}
}

// FreezeOnCooldown leaves TP unchanged, implementing the "freeze" half of
// spec 5.4.2's "decrement or freeze during cooldown" — a validator that
// goes offline keeps the TP it already earned but earns no more until it
// rejoins successfully.
func FreezeOnCooldown(nft *types.ValidatorNFT) {
	// intentionally a no-op; documents the policy choice explicitly.
}

// --- Trait updates (spec 4.5, 16.3: department NFTs, applied through the
// five-stage pipeline as Kind NFTTrait transactions) ---

// ApplyTraitUpdate applies `update_trait target key op value` (spec 14.3)
// to a numeric trait, matching the ShadowRust UpdateTraitStatement
// semantics (=, +=, -=). Non-numeric traits (e.g. "badge", "dept") must use
// op "=".
func ApplyTraitUpdate(nft *types.ValidatorNFT, key, op string, value decimal.Decimal) error {
	if nft.Traits == nil {
		nft.Traits = map[string]string{}
	}
	switch op {
	case "=":
		nft.Traits[key] = value.String()
		return nil
	case "+=", "-=":
		cur := decimal.Zero
		if s, ok := nft.Traits[key]; ok {
			parsed, err := decimal.FromString(s)
			if err != nil {
				return fmt.Errorf("nft: existing trait %q is not numeric, cannot apply %s: %w", key, op, err)
			}
			cur = parsed
		}
		if op == "+=" {
			cur = cur.Add(value)
		} else {
			cur = cur.Sub(value)
		}
		nft.Traits[key] = cur.String()
		return nil
	default:
		return fmt.Errorf("nft: unknown trait op %q", op)
	}
}

// --- Slashing (spec 10.3: governance-only) ---

// SlashAction is the outcome of a passed slash proposal.
type SlashAction uint8

const (
	SlashFreeze SlashAction = iota // Slashed=true, NFT record kept (frozen)
	SlashBurn                      // NFT record removed entirely
)

// ApplySlash marks the NFT slashed. Burning (removing the record outright)
// is the caller's responsibility once this returns SlashBurn — pkg/nft
// never deletes state itself, since only the node's state layer owns that.
// This function must only be invoked after a governance vote has passed
// (spec 10.3: "Slash execution is a governance vote... Automatic silent
// burns are not in spec").
func ApplySlash(nft *types.ValidatorNFT, action SlashAction) {
	nft.Slashed = true
	if action == SlashBurn {
		nft.TP = 0
	}
}
