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

// ErrDuplicateTx is returned by Submit for a TxID this mempool has already
// admitted (whether still pending, already drained into a round, or seen
// recently enough not to have expired from the dedup window). Callers that
// gossip transactions onward (pkg/validator's TxOffer handler) use this to
// distinguish "genuinely new, forward it" from "an echo of something
// already circulating" — without it, two peers that both forward every
// TxOffer they receive to each other would flood the network with the
// same transaction forever.
var ErrDuplicateTx = errors.New("tx: duplicate transaction")

// seenTTL bounds how long a TxID is remembered for duplicate detection
// after being submitted — long enough to outlast normal gossip fan-out and
// a round's RoundTimeout, short enough that the dedup set doesn't grow
// without bound over a long-running node's lifetime.
const seenTTL = 2 * TxTTL

// Mempool holds admitted-but-not-yet-batched transactions (spec 3.1:
// "mempool, 1-second batcher").
type Mempool struct {
	// MaxSize caps Len(); zero means DefaultMaxMempoolSize. Exported so a
	// node can raise or lower it (e.g. lower for a resource-constrained
	// enterprise container per spec 15.3).
	MaxSize int

	mu      sync.Mutex
	pending []Entry
	seen    map[types.Hash]time.Time
}

func NewMempool() *Mempool { return &Mempool{seen: map[types.Hash]time.Time{}} }

func (m *Mempool) maxSize() int {
	if m.MaxSize <= 0 {
		return DefaultMaxMempoolSize
	}
	return m.MaxSize
}

// Submit admits tx to the mempool, timestamped now. It returns
// ErrMempoolFull without admitting tx once MaxSize entries are already
// pending, and ErrDuplicateTx without re-admitting a TxID already seen
// within seenTTL (whether still pending, already drained, or resubmitted
// via gossip echo).
func (m *Mempool) Submit(t types.ShieldedTx, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seenAt, dup := m.seen[t.TxID]; dup && now.Sub(seenAt) < seenTTL {
		return ErrDuplicateTx
	}
	if len(m.pending) >= m.maxSize() {
		return ErrMempoolFull
	}
	m.seen[t.TxID] = now
	m.pending = append(m.pending, Entry{Tx: t, SubmittedAt: now})
	return nil
}

// DrainBatch removes and returns up to max pending entries, implementing
// the 1-second batcher's pop (spec 22: BatchInterval default 1 second).
// max<=0 drains everything. Draining does not clear an entry's seen
// record — a tx being finalized in a round must still be rejected as a
// duplicate if it arrives again via gossip while that round is in flight.
func (m *Mempool) DrainBatch(max int) []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepSeenLocked(time.Now())
	if max <= 0 || max > len(m.pending) {
		max = len(m.pending)
	}
	batch := m.pending[:max]
	m.pending = m.pending[max:]
	out := make([]Entry, len(batch))
	copy(out, batch)
	return out
}

func (m *Mempool) sweepSeenLocked(now time.Time) {
	for id, seenAt := range m.seen {
		if now.Sub(seenAt) >= seenTTL {
			delete(m.seen, id)
		}
	}
}

// Reinsert returns a transaction this mempool itself drained earlier (e.g.
// via DrainBatch, into a round that then got rolled back — see
// pkg/validator's sweepTimeouts) back onto the pending queue, bypassing
// Submit's duplicate check. This is the mempool's own entry coming back,
// not an external resubmission gossip forwarding needs defending against;
// calling Submit here instead would make it indistinguishable from a
// duplicate and silently drop it for the rest of the dedup window.
func (m *Mempool) Reinsert(t types.ShieldedTx, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pending) >= m.maxSize() {
		return ErrMempoolFull
	}
	m.seen[t.TxID] = now
	m.pending = append(m.pending, Entry{Tx: t, SubmittedAt: now})
	return nil
}

func (m *Mempool) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}
