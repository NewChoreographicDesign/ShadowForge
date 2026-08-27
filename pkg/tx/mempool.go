// Package tx implements the ShadowForge transaction layer: the mempool,
// 1-second batcher, and five-stage validation pipeline (spec section 5.3,
// 3.1).
package tx

import (
	"errors"
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

// DefaultMaxMempoolSize bounds how many entries Submit will accept before
// refusing new ones. Per-peer rate limiting (pkg/net.RateLimiter) slows a
// single flooding peer; this is the second, independent brake against a
// Sybil attacker submitting from many peer identities at once — without a
// cap, unbounded Submit calls are a memory-exhaustion DoS vector.
const DefaultMaxMempoolSize = 100_000

// ErrMempoolFull is returned by Submit once MaxSize entries are pending.
var ErrMempoolFull = errors.New("tx: mempool is full")

// Mempool holds admitted-but-not-yet-batched transactions (spec 3.1:
// "mempool, 1-second batcher").
type Mempool struct {
	// MaxSize caps Len(); zero means DefaultMaxMempoolSize. Exported so a
	// node can raise or lower it (e.g. lower for a resource-constrained
	// enterprise container per spec 15.3).
	MaxSize int

	mu      sync.Mutex
	pending []Entry
}

func NewMempool() *Mempool { return &Mempool{} }

func (m *Mempool) maxSize() int {
	if m.MaxSize <= 0 {
		return DefaultMaxMempoolSize
	}
	return m.MaxSize
}

// Submit admits tx to the mempool, timestamped now. It returns
// ErrMempoolFull without admitting tx once MaxSize entries are already
// pending.
func (m *Mempool) Submit(t types.ShieldedTx, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pending) >= m.maxSize() {
		return ErrMempoolFull
	}
	m.pending = append(m.pending, Entry{Tx: t, SubmittedAt: now})
	return nil
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
