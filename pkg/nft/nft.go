// Package nft implements the ShadowForge validator/department NFT
// lifecycle: free one-per-wallet mint, soulbinding, Trust Points, trait
// updates, and governance-gated slashing (spec section 10, spec 4.5).
package nft

import (
	"errors"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

var (
	ErrAlreadyMinted           = errors.New("nft: wallet already holds a soulbound NFT (one per wallet)")
	ErrProofOfHumanityRequired = errors.New("nft: proof-of-humanity / CAPTCHA challenge not passed")
	ErrTransferDisabled        = errors.New("nft: soulbound transfer is disabled until governance unlocks it")
	ErrNFTSlashed              = errors.New("nft: NFT is slashed and cannot act as a validator")
)

// MintParams are the inputs to a free mint (spec 10.1: "User requests a
// micro-drop of SFG... Mint UI presents CAPTCHA and a proof-of-humanity
// challenge... Contract enforces one NFT per wallet").
type MintParams struct {
	Owner                 types.Address
	AlreadyHasNFT         bool
	ProofOfHumanityPassed bool
	MintedAt              types.BlockHeight
	Nonce                 uint64 // caller-supplied uniqueness salt (e.g. tx counter)
}

// Mint enforces the one-per-wallet and proof-of-humanity gates and returns
// a fresh soulbound NFT with an empty trait map and zero Trust Points.
func Mint(p MintParams) (types.ValidatorNFT, error) {
	if p.AlreadyHasNFT {
		return types.ValidatorNFT{}, ErrAlreadyMinted
	}
	if !p.ProofOfHumanityPassed {
		return types.ValidatorNFT{}, ErrProofOfHumanityRequired
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

// --- Trust Points (spec 5.4.2, 10.3) ---

const (
	TPPerSuccessfulStage = 1
	TPPerUptimeSlice     = 1
)

// AwardStageSuccess increments TP for a successful stage-work turn (spec
// 5.4.2: "TP: increment on successful stage work").
func AwardStageSuccess(nft *types.ValidatorNFT) {
	nft.TP += TPPerSuccessfulStage
}

// AwardUptimeSlice increments TP for a continuous-uptime slice (spec 5.4.2).
func AwardUptimeSlice(nft *types.ValidatorNFT) {
	nft.TP += TPPerUptimeSlice
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
