package tx_test

import (
	"encoding/json"
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

// TestMempoolRemoveDropsMatchingEntriesOnly proves Remove drops exactly
// the pending entries whose TxID matches, leaving everything else — the
// real fix for a node that only ever votes (never proposes) keeping a
// stale local copy of a tx that became committed via someone else's
// proposal, found via real multi-node testing under sustained traffic.
func TestMempoolRemoveDropsMatchingEntriesOnly(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	keep := types.ShieldedTx{TxID: types.Hash{1}}
	drop1 := types.ShieldedTx{TxID: types.Hash{2}}
	drop2 := types.ShieldedTx{TxID: types.Hash{3}}
	for _, e := range []types.ShieldedTx{keep, drop1, drop2} {
		if err := m.Submit(e, now); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}

	m.Remove([]types.Hash{drop1.TxID, drop2.TxID})

	if m.Len() != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", m.Len())
	}
	batch := m.DrainBatch(0)
	if len(batch) != 1 || batch[0].Tx.TxID != keep.TxID {
		t.Fatalf("expected only the untouched entry to remain, got %+v", batch)
	}
}

// TestMempoolRemoveDoesNotClearDedupRecord proves Remove leaves seen
// alone: a late-arriving duplicate gossip echo for an already-committed
// TxID must still be recognized and rejected, not silently re-admitted as
// if it were new.
func TestMempoolRemoveDoesNotClearDedupRecord(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	tr := types.ShieldedTx{TxID: types.Hash{9}}
	if err := m.Submit(tr, now); err != nil {
		t.Fatalf("submit: %v", err)
	}
	m.Remove([]types.Hash{tr.TxID})
	if err := m.Submit(tr, now.Add(time.Second)); err != tx.ErrDuplicateTx {
		t.Fatalf("expected a late duplicate to still be rejected after Remove, got %v", err)
	}
}

// TestMempoolRemoveEmptyIsNoop proves Remove(nil) doesn't touch pending —
// the common case where a batch this node itself proposed (and therefore
// already drained) is pruned again defensively.
func TestMempoolRemoveEmptyIsNoop(t *testing.T) {
	m := tx.NewMempool()
	if err := m.Submit(types.ShieldedTx{TxID: types.Hash{1}}, time.Now()); err != nil {
		t.Fatalf("submit: %v", err)
	}
	m.Remove(nil)
	if m.Len() != 1 {
		t.Fatalf("expected Remove(nil) to be a no-op, len=%d", m.Len())
	}
}

func TestMempoolContainsReflectsPendingState(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	tr := types.ShieldedTx{TxID: types.Hash{4}}

	if m.Contains(tr.TxID) {
		t.Fatalf("expected Contains to be false before Submit")
	}
	if err := m.Submit(tr, now); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !m.Contains(tr.TxID) {
		t.Fatalf("expected Contains to be true once pending")
	}

	m.Remove([]types.Hash{tr.TxID})
	if m.Contains(tr.TxID) {
		t.Fatalf("expected Contains to be false after Remove, even though seen still remembers it")
	}
}

// sizedTx builds a real, distinctly-identified ShieldedTx padded with a
// Memo of paddingLen bytes, so its JSON-marshaled size is
// deterministic and controllable — used to test DrainBatchBytes' actual
// size-based behavior without depending on ShieldedTx's exact zero-value
// JSON overhead.
func sizedTx(id byte, paddingLen int) types.ShieldedTx {
	return types.ShieldedTx{TxID: types.Hash{id}, Memo: make([]byte, paddingLen)}
}

func marshaledSize(t *testing.T, tx types.ShieldedTx) int {
	t.Helper()
	b, err := json.Marshal(tx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return len(b)
}

// TestMempoolDrainBatchBytesStopsAtSizeLimit proves the drain is bounded
// by real cumulative serialized size, not just entry count — the actual
// defense this build needed: a fixed count can't safely bound total size
// when per-tx size varies (this is exactly what let a count-bounded batch
// of ordinary transactions blow past Badger's 1MB per-value limit for
// real, once real Dilithium3 signature/pubkey overhead was accounted for).
func TestMempoolDrainBatchBytesStopsAtSizeLimit(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	one := sizedTx(0, 1000)
	sizeOne := marshaledSize(t, one)

	for i := byte(0); i < 5; i++ {
		if err := m.Submit(sizedTx(i, 1000), now); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	batch := m.DrainBatchBytes(0, sizeOne*2)
	if len(batch) != 2 {
		t.Fatalf("expected exactly 2 entries within a %d-byte budget (each ~%d bytes), got %d", sizeOne*2, sizeOne, len(batch))
	}
	if m.Len() != 3 {
		t.Fatalf("expected 3 entries left pending, got %d", m.Len())
	}
}

// TestMempoolDrainBatchBytesAlwaysDrainsAtLeastOne proves a single
// transaction that alone exceeds the byte budget still gets drained
// (Stage 2's MaxTxSize check is what actually rejects a tx too large to
// ever fit) rather than wedging the queue forever.
func TestMempoolDrainBatchBytesAlwaysDrainsAtLeastOne(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	big := sizedTx(0, 10_000)
	if err := m.Submit(big, now); err != nil {
		t.Fatalf("submit: %v", err)
	}
	batch := m.DrainBatchBytes(0, 10) // budget far smaller than the entry itself
	if len(batch) != 1 {
		t.Fatalf("expected the single oversized entry to still be drained, got %d", len(batch))
	}
}

// TestMempoolDrainBatchBytesRespectsCountLimit proves maxCount still
// applies even when the byte budget alone wouldn't have stopped earlier.
func TestMempoolDrainBatchBytesRespectsCountLimit(t *testing.T) {
	m := tx.NewMempool()
	now := time.Now()
	for i := byte(0); i < 5; i++ {
		if err := m.Submit(sizedTx(i, 10), now); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	batch := m.DrainBatchBytes(2, 10_000_000) // huge byte budget, tight count
	if len(batch) != 2 {
		t.Fatalf("expected maxCount=2 to cap the batch regardless of byte budget, got %d", len(batch))
	}
}
