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
