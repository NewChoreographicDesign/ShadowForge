package nft_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/nft"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestMintRequiresProofOfHumanity(t *testing.T) {
	_, err := nft.Mint(nft.MintParams{Owner: types.Address{1}, ProofOfHumanityPassed: false})
	if err != nft.ErrProofOfHumanityRequired {
		t.Fatalf("expected ErrProofOfHumanityRequired, got %v", err)
	}
}

func TestMintOnePerWallet(t *testing.T) {
	_, err := nft.Mint(nft.MintParams{Owner: types.Address{1}, ProofOfHumanityPassed: true, AlreadyHasNFT: true})
	if err != nft.ErrAlreadyMinted {
		t.Fatalf("expected ErrAlreadyMinted, got %v", err)
	}
}

func TestMintSucceedsAndIsSoulbound(t *testing.T) {
	got, err := nft.Mint(nft.MintParams{Owner: types.Address{1}, ProofOfHumanityPassed: true})
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

func TestTrustPointsAccrual(t *testing.T) {
	v := types.ValidatorNFT{}
	nft.AwardStageSuccess(&v)
	nft.AwardStageSuccess(&v)
	nft.AwardUptimeSlice(&v)
	if v.TP != 3 {
		t.Fatalf("expected TP=3, got %d", v.TP)
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
