// Package silent implements spec 15.4's silent-transaction / DoS defense:
// sentinels and the Vault inject irregular (Poisson) null ZK padding
// transactions to keep circuits warm and absorb burst load, and a
// per-wallet rate monitor flags a spike (more than 20% above baseline)
// for a governance-adjustable hold-and-fee response.
//
// Timing jitter here uses math/rand, not crypto/rand: the Poisson
// inter-arrival draw only shapes traffic patterns for cover/warmth, never
// derives a key, nonce, or anything else where predictability would be a
// security weakness (those all live in pkg/crypto and pkg/zk, and use
// crypto/rand exclusively).
package silent

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// PoissonInterval draws one inter-arrival duration from an exponential
// distribution with the given mean, which is the standard construction for
// Poisson-process arrival times (spec 15.4: "irregular (Poisson) null ZK
// transactions"). rng may be shared across calls; callers needing
// concurrency-safety should guard it themselves (rand.Rand is not
// goroutine-safe).
func PoissonInterval(rng *rand.Rand, mean time.Duration) time.Duration {
	if mean <= 0 {
		return 0
	}
	// Inverse-CDF sampling: -ln(U) * mean, U uniform in (0,1].
	u := rng.Float64()
	for u == 0 {
		u = rng.Float64()
	}
	return time.Duration(-math.Log(u) * float64(mean))
}

// SpikeThreshold, HoldDuration, and VaultFeeRate are the spec 15.4 / 22
// defaults: "+20%, 7 days, 10% fee."
var (
	SpikeThreshold = decimal.MustFromString("0.20")
	HoldDuration   = 7 * 24 * time.Hour
	VaultFeeRate   = decimal.MustFromString("0.10")
)

// walletStats tracks one wallet's recent transaction timestamps and its
// established baseline rate.
type walletStats struct {
	baselinePerMinute decimal.Decimal
	recentTimestamps  []time.Time
	whitelisted       bool
}

// windowSize is how far back RateMonitor looks when computing a wallet's
// current rate.
const windowSize = time.Minute

// RateMonitor tracks per-wallet transaction rates (including silent-padded
// traffic) and flags a spike per spec 15.4.
type RateMonitor struct {
	mu      sync.Mutex
	wallets map[types.Address]*walletStats
}

func NewRateMonitor() *RateMonitor {
	return &RateMonitor{wallets: map[types.Address]*walletStats{}}
}

func (m *RateMonitor) stats(w types.Address) *walletStats {
	s, ok := m.wallets[w]
	if !ok {
		s = &walletStats{}
		m.wallets[w] = s
	}
	return s
}

// SetBaseline establishes a wallet's normal transactions-per-minute rate,
// against which future activity is compared.
func (m *RateMonitor) SetBaseline(w types.Address, perMinute decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats(w).baselinePerMinute = perMinute
}

// Whitelist exempts a wallet from spike holds — spec 15.4: "Business
// containers are whitelistable so payroll bursts are not treated as
// attacks."
func (m *RateMonitor) Whitelist(w types.Address) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats(w).whitelisted = true
}

// RecordTx logs one transaction from w at time now, for rate computation.
func (m *RateMonitor) RecordTx(w types.Address, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.stats(w)
	s.recentTimestamps = append(s.recentTimestamps, now)
	s.recentTimestamps = pruneOlderThan(s.recentTimestamps, now.Add(-windowSize))
}

func pruneOlderThan(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	return ts[i:]
}

// CurrentRatePerMinute returns w's observed rate over the trailing window.
func (m *RateMonitor) CurrentRatePerMinute(w types.Address, now time.Time) decimal.Decimal {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.stats(w)
	s.recentTimestamps = pruneOlderThan(s.recentTimestamps, now.Add(-windowSize))
	return decimal.FromInt(int64(len(s.recentTimestamps)))
}

// IsSpiking reports whether w's current rate exceeds its baseline by more
// than SpikeThreshold, and is not whitelisted. A wallet with no baseline
// set yet (baseline 0) never spikes — there is nothing to compare against.
func (m *RateMonitor) IsSpiking(w types.Address, now time.Time) bool {
	m.mu.Lock()
	whitelisted := m.stats(w).whitelisted
	baseline := m.stats(w).baselinePerMinute
	m.mu.Unlock()

	if whitelisted || baseline.Sign() <= 0 {
		return false
	}
	current := m.CurrentRatePerMinute(w, now)
	// current > baseline * (1 + threshold)
	limit := baseline.Mul(decimal.FromInt(1).Add(SpikeThreshold))
	return current.Cmp(limit) > 0
}

// SpikeAction is the spec 15.4 response to a detected spike: "the protocol
// can place a 7-day hold, take a 10 percent Vault fee, and open a
// burn-or-transfer vote." The vote itself is a governance.Proposal the
// caller opens; this package only computes the hold/fee terms.
type SpikeAction struct {
	HoldUntil time.Time
	VaultFee  decimal.Decimal // fraction of the flagged wallet's held value
}

// EvaluateSpike returns the SpikeAction for w if it is currently spiking,
// and whether it should be applied. "Appeals are staked TP votes" (spec
// 15.4) — outside this package's scope, same as the burn-or-transfer vote.
func EvaluateSpike(m *RateMonitor, w types.Address, now time.Time) (SpikeAction, bool) {
	if !m.IsSpiking(w, now) {
		return SpikeAction{}, false
	}
	return SpikeAction{HoldUntil: now.Add(HoldDuration), VaultFee: VaultFeeRate}, true
}

// RunPadGenerator calls emit() at Poisson-distributed intervals with mean
// meanInterval until ctx is done — the sentinel/Vault-side half of spec
// 15.4: "Sentinels and the Vault inject irregular (Poisson) null ZK
// transactions. They keep circuits from going cold, absorb burst junk, and
// give the monitor a baseline." emit is expected to build and send a
// SilentPad message (pkg/net.MsgSilentPad); this package stays independent
// of pkg/net so it has no networking dependency of its own.
func RunPadGenerator(ctx context.Context, rng *rand.Rand, meanInterval time.Duration, emit func()) {
	for {
		wait := PoissonInterval(rng, meanInterval)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			emit()
		}
	}
}
