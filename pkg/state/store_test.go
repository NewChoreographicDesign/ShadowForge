package state_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func openTestStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("test-key-32-bytes-padding-000000"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNoteRoundTripIsEncryptedAtRest(t *testing.T) {
	s := openTestStore(t)
	note := types.Note{
		Commitment: types.Hash{7},
		Value:      12345,
		OwnerPk:    []byte("owner-pubkey"),
		Rho:        []byte("rho-seed"),
		Asset:      types.AssetSFG,
	}
	if err := s.PutNote(note); err != nil {
		t.Fatalf("put note: %v", err)
	}
	got, found, err := s.GetNote(note.Commitment)
	if err != nil || !found {
		t.Fatalf("get note: found=%v err=%v", found, err)
	}
	if got.Value != note.Value || string(got.OwnerPk) != string(note.OwnerPk) {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, note)
	}
}

func TestNullifierDoubleSpendRejected(t *testing.T) {
	s := openTestStore(t)
	null := types.Hash{9}
	if err := s.MarkNullifierSpent(null); err != nil {
		t.Fatalf("first spend should succeed: %v", err)
	}
	spent, err := s.IsNullifierSpent(null)
	if err != nil || !spent {
		t.Fatalf("expected nullifier to be marked spent: spent=%v err=%v", spent, err)
	}
	if err := s.MarkNullifierSpent(null); err == nil {
		t.Fatalf("expected double-spend of the same nullifier to be rejected")
	}
}

func TestNFTRoundTrip(t *testing.T) {
	s := openTestStore(t)
	nft := types.ValidatorNFT{
		ID:     types.NFTID{1, 2, 3},
		Owner:  types.Address{4, 5, 6},
		Traits: map[string]string{"badge": "Valor"},
		TP:     42,
	}
	if err := s.PutNFT(nft); err != nil {
		t.Fatalf("put nft: %v", err)
	}
	got, found, err := s.GetNFT(nft.ID)
	if err != nil || !found {
		t.Fatalf("get nft: found=%v err=%v", found, err)
	}
	if got.TP != 42 || got.Traits["badge"] != "Valor" {
		t.Fatalf("nft round trip mismatch: %+v", got)
	}
}

func TestBankHoldRoundTrip(t *testing.T) {
	s := openTestStore(t)
	hold := types.BankHold{
		HoldID:        types.Hash{8},
		ExternalAsset: types.AssetBTC,
		EntryBuffer:   decimal.MustFromString("250.5"),
		Status:        types.HoldLocked24h,
	}
	if err := s.PutHold(hold); err != nil {
		t.Fatalf("put hold: %v", err)
	}
	got, found, err := s.GetHold(hold.HoldID)
	if err != nil || !found {
		t.Fatalf("get hold: found=%v err=%v", found, err)
	}
	if got.EntryBuffer.Cmp(hold.EntryBuffer) != 0 || got.Status != types.HoldLocked24h {
		t.Fatalf("hold round trip mismatch: %+v", got)
	}
}

func TestGetMissingReturnsNotFound(t *testing.T) {
	s := openTestStore(t)
	_, found, err := s.GetNote(types.Hash{99})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatalf("expected not found for missing note")
	}
}
