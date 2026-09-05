// Package tx implements the ShadowForge transaction layer: the mempool,
// 1-second batcher, and five-stage validation pipeline (spec section 5.3,
// 3.1).
package tx

import (
	"encoding/json"
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

// MaxTxSize caps one transaction's serialized size (checked at pipeline
// Stage 2), well under Badger's 1MB per-value limit so a single admitted
// transaction can never alone risk that limit regardless of batching.
const MaxTxSize = 256 * 1024

// ErrMempoolFull is returned by Submit once MaxSize entries are pending.
var ErrMempoolFull = errors.New("tx: mempool is full")

// ErrTxTooLarge is returned by Submit for a transaction whose serialized
// size exceeds MaxTxSize.
var ErrTxTooLarge = errors.New("tx: transaction exceeds MaxTxSize")

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
// pending, ErrDuplicateTx without re-admitting a TxID already seen within
// seenTTL (whether still pending, already drained, or resubmitted via
// gossip echo), and ErrTxTooLarge without admitting a transaction whose
// serialized size exceeds MaxTxSize.
//
// Real, independent pentest finding: MaxTxSize's own doc says it "caps
// one transaction's serialized size" and is "checked at pipeline Stage 2"
// — true, but Stage 2 only runs once a transaction is later drained into
// a batch a node happens to be proposing or verifying. Before this fix,
// nothing enforced MaxTxSize here at admission, so a transaction could
// sit in the mempool, fully counted against MaxSize, at up to
// pkg/net.MaxEnvelopeSize (4 MiB — 16x MaxTxSize) for as long as it took
// to be drained and finally rejected. A remote, entirely unauthenticated
// peer (no signature or proof is checked before Submit — that's Stage
// 2's own job) sending TxOffer messages with an oversized field (e.g. a
// large Memo) could exhaust real memory well beyond what MaxTxSize was
// ever meant to bound (worst case MaxSize * MaxEnvelopeSize, not MaxSize
// * MaxTxSize) — a real, remote, pre-authentication memory-exhaustion DoS.
// Enforcing the same bound here, at the only real admission point every
// external transaction passes through, closes it directly.
func (m *Mempool) Submit(t types.ShieldedTx, now time.Time) error {
	if size, err := jsonSize(t); err == nil && size > MaxTxSize {
		return ErrTxTooLarge
	}

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

// DrainBatchBytes removes and returns up to maxCount pending entries,
// stopping early once their cumulative JSON-marshaled size would exceed
// maxBytes — a real, size-based bound, not just an entry count. A fixed
// count can't safely bound total serialized size when per-tx size varies
// (a real post-quantum Dilithium3 signature+pubkey alone is several KB;
// a Memo or ZK proof adds more on top), and this build hit exactly that
// for real: a count-bounded batch of otherwise-ordinary Vote/VoteReveal
// transactions serialized past Badger's 1MB per-value limit, and the
// resulting chain.Append failure (with the whole batch reinserted, per
// tryFinalizeLocked) formed a livelock — the identical oversized batch
// got rebuilt and re-rejected every round, forever. maxCount<=0 means no
// count limit (size alone bounds it); maxBytes<=0 means no size limit
// (count alone bounds it, i.e. behaves like DrainBatch). At least one
// entry is always drained if any are pending, even if it alone exceeds
// maxBytes, so a single oversized transaction can't wedge the queue —
// pipeline Stage 2 (stage2TxOffer) is what rejects a single tx that's
// unreasonably large on its own.
func (m *Mempool) DrainBatchBytes(maxCount, maxBytes int) []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepSeenLocked(time.Now())

	limit := len(m.pending)
	if maxCount > 0 && maxCount < limit {
		limit = maxCount
	}

	n := 0
	total := 0
	for n < limit {
		size, err := jsonSize(m.pending[n].Tx)
		if err != nil {
			// Unmarshalable content shouldn't be possible for a
			// well-formed ShieldedTx, but if it ever happens, don't let
			// it silently wedge the batch — include it (Stage 2 will
			// reject it for real reasons) and keep going.
			n++
			continue
		}
		if maxBytes > 0 && n > 0 && total+size > maxBytes {
			break
		}
		total += size
		n++
	}

	batch := m.pending[:n]
	m.pending = m.pending[n:]
	out := make([]Entry, len(batch))
	copy(out, batch)
	return out
}

func jsonSize(t types.ShieldedTx) (int, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return 0, err
	}
	return len(b), nil
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

// Contains reports whether id is currently waiting in this node's pending
// queue — a real "still pending" answer for pkg/query, distinct from
// "already committed" (pkg/state.GetTxHeight) and "never seen at all".
// It does not consult seen: an entry that was submitted, then already
// drained/committed and Remove'd, is no longer pending even though seen
// still remembers its TxID for dedup purposes.
func (m *Mempool) Contains(id types.Hash) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.pending {
		if e.Tx.TxID == id {
			return true
		}
	}
	return false
}

// Remove drops any pending entries matching the given TxIDs — for a
// transaction that became committed (or is now being voted on) via a
// batch this mempool's own node did not itself drain via DrainBatch/
// DrainBatchBytes, e.g. a proposal built by a different committee member,
// or a block adopted via BlockAnnounce. Without this, real multi-node
// testing under sustained traffic showed exactly the failure mode this
// closes: a node that only ever votes (never proposes) for several
// rounds keeps a stale local copy of every gossiped tx even after it's
// durably committed elsewhere, then later drags it into its own
// proposal once it *is* the proposer — Stage 4 rejects that tx (its
// effect is already applied), and the whole-batch-atomicity rule (spec
// 5.3) discards every other, individually-valid tx bundled alongside it
// too, repeating every round a new otherwise-fine batch happens to
// include the stale entry.
//
// Does not touch seen: a late-arriving duplicate gossip echo for an
// already-committed TxID should still be recognized and rejected by
// Submit's existing dedup, not silently re-admitted as if new.
func (m *Mempool) Remove(ids []types.Hash) {
	if len(ids) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	drop := make(map[types.Hash]bool, len(ids))
	for _, id := range ids {
		drop[id] = true
	}
	kept := m.pending[:0]
	for _, e := range m.pending {
		if !drop[e.Tx.TxID] {
			kept = append(kept, e)
		}
	}
	m.pending = kept
}
