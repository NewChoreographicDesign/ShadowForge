package state_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestTxnCommitMakesWritesVisible(t *testing.T) {
	s := openTestStore(t)
	txn := s.BeginTxn()
	defer txn.Discard()

	null := types.Hash{1, 2, 3}
	if err := txn.MarkNullifierSpent(null); err != nil {
		t.Fatalf("mark within txn: %v", err)
	}

	// Not yet visible outside the open txn.
	spent, err := s.IsNullifierSpent(null)
	if err != nil {
		t.Fatalf("outside check: %v", err)
	}
	if spent {
		t.Fatalf("uncommitted write must not be visible via the Store")
	}

	if err := txn.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	spent, err = s.IsNullifierSpent(null)
	if err != nil || !spent {
		t.Fatalf("expected committed write to be visible: spent=%v err=%v", spent, err)
	}
}

func TestTxnDiscardRollsBackWrites(t *testing.T) {
	s := openTestStore(t)
	txn := s.BeginTxn()

	null := types.Hash{4, 5, 6}
	if err := txn.MarkNullifierSpent(null); err != nil {
		t.Fatalf("mark within txn: %v", err)
	}
	txn.Discard()

	spent, err := s.IsNullifierSpent(null)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if spent {
		t.Fatalf("discarded write must never become visible")
	}
}

func TestTxnReadsItsOwnUncommittedWrites(t *testing.T) {
	s := openTestStore(t)
	txn := s.BeginTxn()
	defer txn.Discard()

	nft := types.ValidatorNFT{ID: types.NFTID{9}, TP: 7}
	if err := txn.PutNFT(nft); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, found, err := txn.GetNFT(nft.ID)
	if err != nil || !found {
		t.Fatalf("expected to read back own uncommitted write: found=%v err=%v", found, err)
	}
	if got.TP != 7 {
		t.Fatalf("unexpected TP: %d", got.TP)
	}

	byOwner, found, err := txn.GetNFTByOwner(nft.Owner)
	if err != nil || !found {
		t.Fatalf("expected to read back own uncommitted owner-index write: found=%v err=%v", found, err)
	}
	if byOwner.ID != nft.ID {
		t.Fatalf("expected owner-index lookup to find the same nft, got %+v", byOwner)
	}
}

func TestTxnDoubleSpendRejectedWithinSameTxn(t *testing.T) {
	s := openTestStore(t)
	txn := s.BeginTxn()
	defer txn.Discard()

	null := types.Hash{7}
	if err := txn.MarkNullifierSpent(null); err != nil {
		t.Fatalf("first mark: %v", err)
	}
	if err := txn.MarkNullifierSpent(null); err == nil {
		t.Fatalf("expected a second mark of the same nullifier, within the same open txn, to fail")
	}
}

func TestTxnAuthorizedAssetCommitMakesWritesVisible(t *testing.T) {
	s := openTestStore(t)
	txn := s.BeginTxn()
	defer txn.Discard()

	authorized, err := txn.IsAssetAuthorized(types.AssetBTC)
	if err != nil {
		t.Fatalf("check within txn: %v", err)
	}
	if authorized {
		t.Fatalf("expected BTC to start out unauthorized")
	}
	if err := txn.PutAuthorizedAsset(types.AssetBTC); err != nil {
		t.Fatalf("authorize within txn: %v", err)
	}
	authorized, err = txn.IsAssetAuthorized(types.AssetBTC)
	if err != nil || !authorized {
		t.Fatalf("expected to read back own uncommitted write: authorized=%v err=%v", authorized, err)
	}

	// Not yet visible outside the open txn.
	authorized, err = s.IsAssetAuthorized(types.AssetBTC)
	if err != nil {
		t.Fatalf("outside check: %v", err)
	}
	if authorized {
		t.Fatalf("uncommitted write must not be visible via the Store")
	}

	if err := txn.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	authorized, err = s.IsAssetAuthorized(types.AssetBTC)
	if err != nil || !authorized {
		t.Fatalf("expected committed write to be visible: authorized=%v err=%v", authorized, err)
	}
}

func TestTxnSatisfiesAccessorInterface(t *testing.T) {
	s := openTestStore(t)
	var _ state.Accessor = s
	txn := s.BeginTxn()
	defer txn.Discard()
	var _ state.Accessor = txn
}

func TestBlockPersistenceRoundTrip(t *testing.T) {
	s := openTestStore(t)
	if _, found, err := s.GetHead(); err != nil || found {
		t.Fatalf("expected no head yet: found=%v err=%v", found, err)
	}

	b := types.Block{Height: 0, StateRoot: types.Hash{1}}
	if err := s.PutBlock(b); err != nil {
		t.Fatalf("put block: %v", err)
	}
	if err := s.SetHead(0); err != nil {
		t.Fatalf("set head: %v", err)
	}

	got, found, err := s.GetBlock(0)
	if err != nil || !found {
		t.Fatalf("get block: found=%v err=%v", found, err)
	}
	if got.StateRoot != b.StateRoot {
		t.Fatalf("unexpected state root: %v", got.StateRoot)
	}

	height, found, err := s.GetHead()
	if err != nil || !found || height != 0 {
		t.Fatalf("unexpected head: height=%d found=%v err=%v", height, found, err)
	}
}

func TestTxIndexRoundTrip(t *testing.T) {
	s := openTestStore(t)
	txid := types.Hash{0x42}

	if _, found, err := s.GetTxHeight(txid); err != nil || found {
		t.Fatalf("expected unindexed tx to be not found: found=%v err=%v", found, err)
	}

	if err := s.IndexTx(txid, 7); err != nil {
		t.Fatalf("index tx: %v", err)
	}

	height, found, err := s.GetTxHeight(txid)
	if err != nil || !found {
		t.Fatalf("expected indexed tx to be found: found=%v err=%v", found, err)
	}
	if height != 7 {
		t.Fatalf("expected height 7, got %d", height)
	}

	if _, found, err := s.GetTxHeight(types.Hash{0x99}); err != nil || found {
		t.Fatalf("expected a different tx id to remain unindexed: found=%v err=%v", found, err)
	}
}
