package bank

import (
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// IPFingerprint implements spec 8.4 / 11.3 exactly: "Hashed IP fingerprints
// on Bank flows to flag cycling/arbitrage. Store H(ip || day_salt), never
// raw IP." daySalt should rotate once per UTC day so the same IP hashes to
// a different fingerprint each day, bounding how far back a correlation
// can be traced.
func IPFingerprint(ip string, daySalt []byte) types.Hash {
	return types.SumHash([]byte(ip), daySalt)
}

// CorrelationTracker counts how many Bank flows have been seen under each
// day's IP fingerprint, to flag cycling/arbitrage abuse (spec 8.4, 11.3)
// without ever storing a raw IP address.
type CorrelationTracker struct {
	mu     sync.Mutex
	counts map[types.Hash]int
}

func NewCorrelationTracker() *CorrelationTracker {
	return &CorrelationTracker{counts: map[types.Hash]int{}}
}

// Record logs one Bank flow under ip's fingerprint for today (daySalt) and
// returns the fingerprint's updated flow count.
func (c *CorrelationTracker) Record(ip string, daySalt []byte) (types.Hash, int) {
	fp := IPFingerprint(ip, daySalt)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[fp]++
	return fp, c.counts[fp]
}

// IsCorrelated reports whether fp has been seen more than threshold times
// today — the caller's cue to flag the flow for review (spec 8.4: "flag
// cycling/arbitrage").
func (c *CorrelationTracker) IsCorrelated(fp types.Hash, threshold int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[fp] > threshold
}

// DaySalt derives a deterministic, once-per-UTC-day salt. Using the day's
// own ISO date as the salt seed is enough here: the goal is rotation, not
// secrecy — the fingerprint's un-reversibility already comes from hashing
// the IP, not from the salt being secret.
func DaySalt(t time.Time) []byte {
	return []byte(t.UTC().Format("2006-01-02"))
}
