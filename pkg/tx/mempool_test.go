package tx_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestMempoolSubmitAndDrain(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{1}}, now); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{2}}, now); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if m.Len() != 2 {
		t.Fatalf("expected len 2, got %d", m.Len())
	}
	batch := m.DrainBatch(1)
	if len(batch) != 1 || batch[0].Tx.TxID != (types.Hash{1}) {
		t.Fatalf("expected to drain the first entry, got %+v", batch)
	}
	if m.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", m.Len())
	}
	rest := m.DrainBatch(0) // 0 = drain everything
	if len(rest) != 1 || m.Len() != 0 {
		t.Fatalf("expected full drain, got %d remaining (rest=%+v)", m.Len(), rest)
	}
}

func TestMempoolRejectsWhenFull(t *testing.T) {
	m := tx.NewMempool()
	m.MaxSize = 3
	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := m.Submit(types.ShieldedTx{TxID: types.Hash{byte(i)}}, now); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{99}}, now); err != tx.ErrMempoolFull {
		t.Fatalf("expected ErrMempoolFull, got %v", err)
	}
	if m.Len() != 3 {
		t.Fatalf("rejected submission must not grow the mempool, len=%d", m.Len())
	}
}

func TestMempoolDefaultMaxSize(t *testing.T) {
	m := tx.NewMempool()
	if m.MaxSize != 0 {
		t.Fatalf("expected zero-value MaxSize to mean 'use the default'")
	}
	// A cheap smoke check that the default is a real, large cap rather
	// than accidentally zero (which would reject everything).
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{1}}, time.Now()); err != nil {
		t.Fatalf("expected submit to succeed under the default cap: %v", err)
	}
}

// TestMempoolSubmitRejectsDuplicateTxID proves Submit is idempotent for a
// TxID it has already admitted — the property pkg/validator's TxOffer
// gossip forwarding relies on to avoid two peers relaying the same
// transaction back and forth forever.
func TestMempoolSubmitRejectsDuplicateTxID(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{7}}, now); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{7}}, now.Add(time.Second)); err != tx.ErrDuplicateTx {
		t.Fatalf("expected ErrDuplicateTx for a resubmitted TxID, got %v", err)
	}
	if m.Len() != 1 {
		t.Fatalf("a rejected duplicate must not grow the mempool, len=%d", m.Len())
	}
}

// TestMempoolSubmitAllowsResubmissionAfterSeenTTLExpires proves the dedup
// window is bounded, not permanent: once enough time (seenTTL) has passed
// since a TxID was first seen, resubmitting it is treated as new again
// rather than a duplicate forever.
func TestMempoolSubmitAllowsResubmissionAfterSeenTTLExpires(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{9}}, now); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	m.DrainBatch(0) // empties pending, but the seen record survives the drain

	long := now.Add(3 * tx.TxTTL) // past seenTTL (2*TxTTL)
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{9}}, long); err != nil {
		t.Fatalf("expected resubmission to succeed once the TxID's seen record has aged past seenTTL, got %v", err)
	}
	if m.Len() != 1 {
		t.Fatalf("expected the resubmitted tx to be pending again, len=%d", m.Len())
	}
}

// TestMempoolReinsertBypassesDuplicateCheck proves Reinsert (used by
// pkg/validator's sweepTimeouts to return a rolled-back round's
// transactions to the mempool) succeeds even though Submit would reject
// the same TxID as a duplicate — this is the mempool's own entry coming
// back, not an external resubmission.
func TestMempoolReinsertBypassesDuplicateCheck(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	tr := types.ShieldedTx{TxID: types.Hash{3}}
	if err := m.Submit(tr, now); err != nil {
		t.Fatalf("submit: %v", err)
	}
	m.DrainBatch(0) // simulates a round draining it for a proposal

	if err := m.Submit(tr, now); err != tx.ErrDuplicateTx {
		t.Fatalf("expected Submit to still reject this TxID as a duplicate, got %v", err)
	}
	if err := m.Reinsert(tr, now); err != nil {
		t.Fatalf("expected Reinsert to succeed despite the duplicate record, got %v", err)
	}
	if m.Len() != 1 {
		t.Fatalf("expected the reinserted tx to be pending again, len=%d", m.Len())
	}
}
