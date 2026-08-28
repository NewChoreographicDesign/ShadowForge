package nft_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// attestor is a real Dilithium keypair standing in for a trusted
// proof-of-humanity attestor service, built once per test so tests can
// both sign genuine attestations and configure it as (or deliberately
// leave it out of) MintParams.TrustedAttestors.
func attestor(t *testing.T) (crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey) {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate attestor key: %v", err)
	}
	return pk, sk
}

func TestMintRequiresProofOfHumanity(t *testing.T) {
	apk, _ := attestor(t)
	// A zero-value attestation: unsigned, no attestor — must be rejected,
	// not silently accepted the way a bare bool defaulting to false used
	// to require an explicit ProofOfHumanityPassed: false.
	_, err := nft.Mint(nft.MintParams{Owner: types.Address{1}, TrustedAttestors: []crypto.DilithiumPublicKey{apk}})
	if err == nil {
		t.Fatalf("expected an empty/unsigned attestation to be rejected")
	}
}

func TestMintOnePerWallet(t *testing.T) {
	apk, ask := attestor(t)
	owner := types.Address{1}
	now := time.Now()
	att, err := nft.SignPoHAttestation(apk, ask, owner, 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	_, err = nft.Mint(nft.MintParams{
		Owner: owner, AlreadyHasNFT: true, Attestation: att, Nonce: 1, Now: now,
		TrustedAttestors: []crypto.DilithiumPublicKey{apk},
	})
	if err != nft.ErrAlreadyMinted {
		t.Fatalf("expected ErrAlreadyMinted, got %v", err)
	}
}

func TestMintSucceedsAndIsSoulbound(t *testing.T) {
	apk, ask := attestor(t)
	owner := types.Address{1}
	now := time.Now()
	att, err := nft.SignPoHAttestation(apk, ask, owner, 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	got, err := nft.Mint(nft.MintParams{
		Owner: owner, Attestation: att, Nonce: 1, Now: now,
		TrustedAttestors: []crypto.DilithiumPublicKey{apk},
	})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if got.TP != 0 || got.Slashed {
		t.Fatalf("unexpected initial nft state: %+v", got)
	}
	if nft.CanTransfer(got) {
		t.Fatalf("freshly minted NFT must not be transferable")
	}
	nft.UnlockTransfer(&got)
	if !nft.CanTransfer(got) {
		t.Fatalf("expected transfer to be unlocked after UnlockTransfer")
	}
}

func TestMintRejectsUntrustedAttestor(t *testing.T) {
	realPK, realSK := attestor(t) // genuinely signs the attestation
	trustedPK, _ := attestor(t)   // but Mint is only told to trust a different key
	owner := types.Address{1}
	now := time.Now()
	att, err := nft.SignPoHAttestation(realPK, realSK, owner, 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	_, err = nft.Mint(nft.MintParams{
		Owner: owner, Attestation: att, Nonce: 1, Now: now,
		TrustedAttestors: []crypto.DilithiumPublicKey{trustedPK},
	})
	if err != nft.ErrUntrustedAttestor {
		t.Fatalf("expected ErrUntrustedAttestor, got %v", err)
	}
}

func TestMintRejectsAttestationForDifferentOwner(t *testing.T) {
	apk, ask := attestor(t)
	signedFor := types.Address{1}
	presentedFor := types.Address{2}
	now := time.Now()
	att, err := nft.SignPoHAttestation(apk, ask, signedFor, 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	_, err = nft.Mint(nft.MintParams{
		Owner: presentedFor, Attestation: att, Nonce: 1, Now: now,
		TrustedAttestors: []crypto.DilithiumPublicKey{apk},
	})
	if err != nft.ErrAttestationMismatch {
		t.Fatalf("expected ErrAttestationMismatch for a different owner, got %v", err)
	}
}

func TestMintRejectsAttestationForDifferentNonce(t *testing.T) {
	apk, ask := attestor(t)
	owner := types.Address{1}
	now := time.Now()
	att, err := nft.SignPoHAttestation(apk, ask, owner, 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	_, err = nft.Mint(nft.MintParams{
		Owner: owner, Attestation: att, Nonce: 2, Now: now, // replaying attestation for nonce 1 against a mint claiming nonce 2
		TrustedAttestors: []crypto.DilithiumPublicKey{apk},
	})
	if err != nft.ErrAttestationMismatch {
		t.Fatalf("expected ErrAttestationMismatch for a different nonce, got %v", err)
	}
}

func TestMintRejectsTamperedAttestationSignature(t *testing.T) {
	apk, ask := attestor(t)
	owner := types.Address{1}
	now := time.Now()
	att, err := nft.SignPoHAttestation(apk, ask, owner, 1, now.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	// A genuinely signed attestation, but for a different nonce than what
	// it was signed over is caught by the mismatch check above; here we
	// prove tampering with the signature bytes themselves (still matching
	// owner/nonce/attestor) is caught by DilithiumVerify, not accepted
	// just because the metadata lines up.
	tampered := att
	tamperedSig := append([]byte{}, att.Sig...)
	tamperedSig[0] ^= 0xFF
	tampered.Sig = tamperedSig
	_, err = nft.Mint(nft.MintParams{
		Owner: owner, Attestation: tampered, Nonce: 1, Now: now,
		TrustedAttestors: []crypto.DilithiumPublicKey{apk},
	})
	if err != nft.ErrProofOfHumanityRequired {
		t.Fatalf("expected ErrProofOfHumanityRequired for a tampered signature, got %v", err)
	}
}

func TestMintRejectsExpiredAttestation(t *testing.T) {
	apk, ask := attestor(t)
	owner := types.Address{1}
	issuedAt := time.Now().Add(-nft.PoHAttestationTTL - time.Minute)
	att, err := nft.SignPoHAttestation(apk, ask, owner, 1, issuedAt.UnixMilli())
	if err != nil {
		t.Fatalf("sign attestation: %v", err)
	}
	_, err = nft.Mint(nft.MintParams{
		Owner: owner, Attestation: att, Nonce: 1, Now: time.Now(),
		TrustedAttestors: []crypto.DilithiumPublicKey{apk},
	})
	if err != nft.ErrAttestationExpired {
		t.Fatalf("expected ErrAttestationExpired, got %v", err)
	}
}

func TestTrustPointsAccrual(t *testing.T) {
	v := types.ValidatorNFT{}
	nft.AwardStageSuccess(&v)
	nft.AwardStageSuccess(&v)
	nft.AwardUptimeSlice(&v, false)
	if v.TP != 3 {
		t.Fatalf("expected TP=3, got %d", v.TP)
	}
}

func TestAwardUptimeSliceGreenEnergyBonus(t *testing.T) {
	plain := types.ValidatorNFT{}
	nft.AwardUptimeSlice(&plain, false)
	if plain.TP != nft.TPPerUptimeSlice {
		t.Fatalf("expected TP=%d without green attestation, got %d", nft.TPPerUptimeSlice, plain.TP)
	}

	green := types.ValidatorNFT{}
	nft.AwardUptimeSlice(&green, true)
	want := uint64(nft.TPPerUptimeSlice + nft.TPGreenEnergyBonus)
	if green.TP != want {
		t.Fatalf("expected TP=%d with a verified green attestation, got %d", want, green.TP)
	}
}

func TestApplyTraitUpdateSetAndIncrement(t *testing.T) {
	v := types.ValidatorNFT{}
	if err := nft.ApplyTraitUpdate(&v, "balance", "=", decimal.MustFromString("1000")); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := nft.ApplyTraitUpdate(&v, "balance", "+=", decimal.MustFromString("500")); err != nil {
		t.Fatalf("+=: %v", err)
	}
	if v.Traits["balance"] != "1500" {
		t.Fatalf("balance = %s, want 1500", v.Traits["balance"])
	}
	if err := nft.ApplyTraitUpdate(&v, "balance", "-=", decimal.MustFromString("200")); err != nil {
		t.Fatalf("-=: %v", err)
	}
	if v.Traits["balance"] != "1300" {
		t.Fatalf("balance = %s, want 1300", v.Traits["balance"])
	}
}

func TestApplySlashFreezeVsBurn(t *testing.T) {
	frozen := types.ValidatorNFT{TP: 50}
	nft.ApplySlash(&frozen, nft.SlashFreeze)
	if !frozen.Slashed || frozen.TP != 50 {
		t.Fatalf("expected freeze to keep TP, got %+v", frozen)
	}

	burned := types.ValidatorNFT{TP: 50}
	nft.ApplySlash(&burned, nft.SlashBurn)
	if !burned.Slashed || burned.TP != 0 {
		t.Fatalf("expected burn to zero TP, got %+v", burned)
	}
}

func TestTransferOwnershipRequiresUnlock(t *testing.T) {
	locked := types.ValidatorNFT{ID: types.NFTID{0x1}, Owner: types.Address{0x1}}
	_, err := nft.TransferOwnership(nft.TransferParams{NFT: locked, NewOwner: types.Address{0x2}})
	if err != nft.ErrTransferDisabled {
		t.Fatalf("expected ErrTransferDisabled for a locked (never-unlocked) NFT, got %v", err)
	}
}

func TestTransferOwnershipRejectsSlashed(t *testing.T) {
	target := types.ValidatorNFT{ID: types.NFTID{0x1}, Owner: types.Address{0x1}, Slashed: true}
	nft.UnlockTransfer(&target)
	_, err := nft.TransferOwnership(nft.TransferParams{NFT: target, NewOwner: types.Address{0x2}})
	if err != nft.ErrNFTSlashed {
		t.Fatalf("expected ErrNFTSlashed for a slashed NFT even if unlocked, got %v", err)
	}
}

func TestTransferOwnershipRejectsWhenNewOwnerAlreadyHasNFT(t *testing.T) {
	target := types.ValidatorNFT{ID: types.NFTID{0x1}, Owner: types.Address{0x1}}
	nft.UnlockTransfer(&target)
	_, err := nft.TransferOwnership(nft.TransferParams{NFT: target, NewOwner: types.Address{0x2}, NewOwnerAlreadyHasNFT: true})
	if err != nft.ErrAlreadyMinted {
		t.Fatalf("expected ErrAlreadyMinted when the receiving wallet already holds a different NFT, got %v", err)
	}
}

func TestTransferOwnershipSucceedsOnceUnlocked(t *testing.T) {
	target := types.ValidatorNFT{ID: types.NFTID{0x1}, Owner: types.Address{0x1}, TP: 7}
	nft.UnlockTransfer(&target)
	newOwner := types.Address{0x2}
	updated, err := nft.TransferOwnership(nft.TransferParams{NFT: target, NewOwner: newOwner})
	if err != nil {
		t.Fatalf("expected a real unlocked transfer to succeed: %v", err)
	}
	if updated.Owner != newOwner {
		t.Fatalf("expected Owner to be reassigned to %s, got %s", newOwner, updated.Owner)
	}
	if updated.ID != target.ID || updated.TP != target.TP {
		t.Fatalf("expected the rest of the record (ID, TP) to be preserved, got %+v", updated)
	}
}
