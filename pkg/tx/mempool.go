// Package tx implements the ShadowForge transaction layer: the mempool,
// 1-second batcher, and five-stage validation pipeline (spec section 5.3,
// 3.1).
package tx

import (
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Entry wraps a submitted transaction with bookkeeping the pipeline needs
// but which is not part of the spec-4.2 ShieldedTx shape itself (e.g.
// submission time, for Stage 2's "not expired" check).
type Entry struct {
	Tx          types.ShieldedTx
	SubmittedAt time.Time
}

// TxTTL is how long a mempool entry may sit before Stage 2 rejects it as
// expired.
const TxTTL = 60 * time.Second

// Mempool holds admitted-but-not-yet-batched transactions (spec 3.1:
// "mempool, 1-second batcher").
type Mempool struct {
	mu      sync.Mutex
	pending []Entry
}

func NewMempool() *Mempool { return &Mempool{} }

// Submit admits tx to the mempool, timestamped now.
func (m *Mempool) Submit(t types.ShieldedTx, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = append(m.pending, Entry{Tx: t, SubmittedAt: now})
}

// DrainBatch removes and returns up to max pending entries, implementing
// the 1-second batcher's pop (spec 22: BatchInterval default 1 second).
// max<=0 drains everything.
func (m *Mempool) DrainBatch(max int) []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if max <= 0 || max > len(m.pending) {
		max = len(m.pending)
	}
	batch := m.pending[:max]
	m.pending = m.pending[max:]
	out := make([]Entry, len(batch))
	copy(out, batch)
	return out
}

func (m *Mempool) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}
