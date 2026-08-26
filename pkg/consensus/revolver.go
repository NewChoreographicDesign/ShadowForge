package consensus

import (
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Heartbeat / cooldown constants (spec 5.4.2, section 22 governance
// defaults).
const (
	HeartbeatInterval  = 10 * time.Second
	MissedBeatsOffline = 3                                                     // treat offline after 3 missed beats
	OfflineWindow      = time.Duration(MissedBeatsOffline) * HeartbeatInterval // ~30 seconds
	CooldownDuration   = time.Hour
	SentinelThreshold  = 10 // strictly fewer than this activates sentinels
)

// LowTPThreshold and LowTPDelay implement the "Sybil brake": low Trust
// Point holders may have extra delay before the first insert (spec 5.4.1).
const (
	LowTPThreshold = 50
	LowTPDelay     = 30 * time.Second
)

// Revolver is the global deque of online, Validate-toggled, non-cooldown
// NFT holders (spec 5.4). All mutating operations are taken under a single
// mutex, which is the "under a mutex or with a single owner goroutine"
// concurrency contract spec 14.4 requires generated `queue insert` code to
// respect.
type Revolver struct {
	mu    sync.Mutex
	items []types.QueueItem
	// pendingLowTP holds addresses whose Sybil-brake delay has not yet
	// elapsed; InsertValidator is a no-op for them until ReleasePending is
	// called (normally driven by a timer in the node's scheduler).
	pendingLowTP map[types.NFTID]time.Time
}

func NewRevolver() *Revolver {
	return &Revolver{pendingLowTP: map[types.NFTID]time.Time{}}
}

// Len returns the number of items currently queued. Safe for concurrent use.
func (r *Revolver) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

// Snapshot returns a copy of the current queue order, for tests and
// diagnostics.
func (r *Revolver) Snapshot() []types.QueueItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]types.QueueItem, len(r.items))
	copy(out, r.items)
	return out
}

func (r *Revolver) contains(id types.NFTID) bool {
	for _, it := range r.items {
		if it.NFT == id {
			return true
		}
	}
	return false
}

// unfairPositions is the exact position list from spec 5.4.1 / 19.2:
// `positions := []int{ queue.Len(), 4, 10, 2, 7 }`. The first entry means
// "append"; the rest scatter the joiner through the queue.
func unfairPositions(queueLen int) []int {
	return []int{queueLen, 4, 10, 2, 7}
}

// InsertValidator runs the unfair-position algorithm from spec 5.4.1 /
// 19.2: it inserts item at each of the five computed positions in turn,
// scattering a fresh joiner through the queue instead of only appending it,
// so — per spec section 1's plain-language framing — "New joiners are
// inserted at several 'unfair' positions so no one parks at the front
// forever" and "a fresh node cannot monopolize the next five stages."
//
// A literal reading of 19.2's pseudocode (`if !queue.Contains(addr) {
// queue.Insert(idx, addr) }` re-checked before every position) would make
// positions 2-5 permanently dead code, since the very first insert makes
// Contains true for the rest of the same call — silently defeating the
// mechanism the surrounding prose describes. Per the Phase 1 "internal
// review week: run the whitepaper queue-insert pseudocode through the
// toolchain, update this spec if it must change" directive (spec 18.3),
// this implementation instead applies the existence check once, at the
// start of the call: a validator already anywhere in the queue is a
// duplicate join request and is a no-op (spec 20 Testing Matrix: "duplicate
// ignored"); a genuinely new joiner is scattered into all five positions.
//
// If item's TP is below LowTPThreshold and it has not yet cleared the
// Sybil-brake delay, the insert is deferred — see RequestJoin.
func (r *Revolver) InsertValidator(item types.QueueItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.insertLocked(item)
}

func (r *Revolver) insertLocked(item types.QueueItem) {
	if r.contains(item.NFT) {
		return
	}
	for _, pos := range unfairPositions(len(r.items)) {
		idx := pos % (len(r.items) + 1)
		r.items = insertAt(r.items, idx, item)
	}
}

func insertAt(s []types.QueueItem, idx int, v types.QueueItem) []types.QueueItem {
	if idx < 0 {
		idx = 0
	}
	if idx > len(s) {
		idx = len(s)
	}
	s = append(s, types.QueueItem{})
	copy(s[idx+1:], s[idx:])
	s[idx] = v
	return s
}

// RequestJoin is the entry point a newly-online validator calls. It applies
// the Sybil brake: an NFT with TP below LowTPThreshold is held in a pending
// set for LowTPDelay before its unfair-position insert actually runs.
func (r *Revolver) RequestJoin(item types.QueueItem, now time.Time) {
	if item.TP < LowTPThreshold {
		r.mu.Lock()
		r.pendingLowTP[item.NFT] = now.Add(LowTPDelay)
		r.mu.Unlock()
		return
	}
	r.InsertValidator(item)
}

// ReleaseEligiblePending inserts any pending low-TP joiners whose brake
// delay has elapsed as of now. Callers (the node's scheduler) invoke this
// periodically; it does not need to be called at the exact millisecond the
// delay elapses.
func (r *Revolver) ReleaseEligiblePending(now time.Time, resolve func(types.NFTID) (types.QueueItem, bool)) {
	r.mu.Lock()
	var ready []types.NFTID
	for id, at := range r.pendingLowTP {
		if !now.Before(at) {
			ready = append(ready, id)
		}
	}
	for _, id := range ready {
		delete(r.pendingLowTP, id)
	}
	r.mu.Unlock()

	for _, id := range ready {
		if item, ok := resolve(id); ok {
			r.InsertValidator(item)
		}
	}
}

// PopFront removes and returns the item at the front of the queue (the
// validator assigned to the next pipeline stage). ok is false if the queue
// is empty.
func (r *Revolver) PopFront() (types.QueueItem, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.items) == 0 {
		return types.QueueItem{}, false
	}
	item := r.items[0]
	r.items = r.items[1:]
	return item, true
}

// PushBack re-queues a validator that finished a stage successfully (spec
// 5.3.1: "A validator who finishes a stage successfully is pushed to the
// back of the revolver").
func (r *Revolver) PushBack(item types.QueueItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, item)
}

// Remove drops an NFT from the queue entirely (used when it goes into
// cooldown — spec 5.3.1: a validator who fails a stage "is not pushed to
// the back until a later successful turn").
func (r *Revolver) Remove(id types.NFTID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.items[:0]
	for _, it := range r.items {
		if it.NFT != id {
			out = append(out, it)
		}
	}
	r.items = out
}

// Online counts entries not currently in cooldown, as of now. This is the
// count the sentinel-activation and adaptive-stage-width rules use.
func (r *Revolver) Online(now time.Time) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, it := range r.items {
		if it.CooldownUntil == 0 || now.UnixMilli() >= it.CooldownUntil {
			n++
		}
	}
	return n
}

// IsOffline reports whether a validator should be considered offline given
// its LastBeat, per spec 5.4.2 ("treat offline after 3 missed beats, about
// 30 seconds").
func IsOffline(lastBeat time.Time, now time.Time) bool {
	return now.Sub(lastBeat) >= time.Duration(MissedBeatsOffline)*HeartbeatInterval
}

// ApplyCooldown sets an item's CooldownUntil to now+1h and removes it from
// the active queue (spec 5.4.2: "Offline penalty: 1 hour cooldown before
// the NFT may re-enter the revolver").
func (r *Revolver) ApplyCooldown(id types.NFTID, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.items[:0]
	for _, it := range r.items {
		if it.NFT == id {
			continue // cooldown'd validators leave the active queue outright
		}
		out = append(out, it)
	}
	r.items = out
}

// CooldownExpiry returns the unix-millisecond time at which a cooldown
// started now expires.
func CooldownExpiry(now time.Time) int64 {
	return now.Add(CooldownDuration).UnixMilli()
}

// ValidatorsPerStage implements spec 19.5 / 5.3.1: adaptive stage width
// based on how many validators are online.
func ValidatorsPerStage(online int) int {
	switch {
	case online > 300:
		return 3
	case online > 200:
		return 2
	default:
		return 1
	}
}

// SentinelsNeeded implements spec 19.6: "if revolver.Online() < 10,
// activateSentinels(10)".
func SentinelsNeeded(online int) bool {
	return online < SentinelThreshold
}
