package silent_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/silent"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestPoissonIntervalIsPositiveAndVaries(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seen := map[time.Duration]bool{}
	for i := 0; i < 20; i++ {
		d := silent.PoissonInterval(rng, time.Second)
		if d < 0 {
			t.Fatalf("expected a non-negative interval, got %v", d)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected varying intervals from a Poisson draw, got only %d distinct values", len(seen))
	}
}

func TestPoissonIntervalZeroMean(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if got := silent.PoissonInterval(rng, 0); got != 0 {
		t.Fatalf("expected 0 for a zero mean, got %v", got)
	}
}

func addr(b byte) types.Address {
	var a types.Address
	a[0] = b
	return a
}

func TestRateMonitorFlagsSpike(t *testing.T) {
	m := silent.NewRateMonitor()
	w := addr(1)
	now := time.Now()
	m.SetBaseline(w, decimal.FromInt(10)) // 10 tx/min normal

	// 10 tx/min: at baseline, not a spike.
	for i := 0; i < 10; i++ {
		m.RecordTx(w, now)
	}
	if m.IsSpiking(w, now) {
		t.Fatalf("at-baseline traffic should not spike")
	}

	// Push well past baseline*1.2 = 12.
	for i := 0; i < 10; i++ {
		m.RecordTx(w, now)
	}
	if !m.IsSpiking(w, now) {
		t.Fatalf("expected a spike at 2x baseline")
	}
}

func TestRateMonitorNoBaselineNeverSpikes(t *testing.T) {
	m := silent.NewRateMonitor()
	w := addr(2)
	now := time.Now()
	for i := 0; i < 1000; i++ {
		m.RecordTx(w, now)
	}
	if m.IsSpiking(w, now) {
		t.Fatalf("a wallet with no established baseline should never be flagged")
	}
}

func TestRateMonitorWhitelistExempt(t *testing.T) {
	m := silent.NewRateMonitor()
	w := addr(3)
	now := time.Now()
	m.SetBaseline(w, decimal.FromInt(1))
	m.Whitelist(w)
	for i := 0; i < 100; i++ {
		m.RecordTx(w, now)
	}
	if m.IsSpiking(w, now) {
		t.Fatalf("a whitelisted wallet (e.g. a business container payroll burst) must never spike")
	}
}

func TestRateMonitorWindowExpires(t *testing.T) {
	m := silent.NewRateMonitor()
	w := addr(4)
	base := time.Now()
	m.SetBaseline(w, decimal.FromInt(5))
	for i := 0; i < 50; i++ {
		m.RecordTx(w, base)
	}
	// Far enough in the future that the recorded burst has aged out of
	// the trailing window.
	later := base.Add(5 * time.Minute)
	if m.IsSpiking(w, later) {
		t.Fatalf("old activity outside the trailing window must not count toward a spike")
	}
}

func TestEvaluateSpikeReturnsHoldAndFee(t *testing.T) {
	m := silent.NewRateMonitor()
	w := addr(5)
	now := time.Now()
	m.SetBaseline(w, decimal.FromInt(1))
	for i := 0; i < 10; i++ {
		m.RecordTx(w, now)
	}
	action, flagged := silent.EvaluateSpike(m, w, now)
	if !flagged {
		t.Fatalf("expected the spike to be flagged")
	}
	if action.VaultFee.Cmp(silent.VaultFeeRate) != 0 {
		t.Fatalf("expected vault fee %s, got %s", silent.VaultFeeRate, action.VaultFee)
	}
	if !action.HoldUntil.After(now) {
		t.Fatalf("expected HoldUntil to be in the future")
	}
	wantHold := now.Add(silent.HoldDuration)
	if action.HoldUntil.Sub(wantHold) > time.Second {
		t.Fatalf("HoldUntil %v not close to expected %v", action.HoldUntil, wantHold)
	}
}

func TestEvaluateSpikeNotFlaggedForNormalTraffic(t *testing.T) {
	m := silent.NewRateMonitor()
	w := addr(6)
	now := time.Now()
	_, flagged := silent.EvaluateSpike(m, w, now)
	if flagged {
		t.Fatalf("expected no spike action for a wallet with no recorded traffic")
	}
}

func TestRunPadGeneratorEmitsAndStopsOnCancel(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	ctx, cancel := context.WithCancel(context.Background())
	emitted := make(chan struct{}, 100)

	done := make(chan struct{})
	go func() {
		silent.RunPadGenerator(ctx, rng, time.Millisecond, func() {
			select {
			case emitted <- struct{}{}:
			default:
			}
		})
		close(done)
	}()

	select {
	case <-emitted:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected at least one emit before timeout")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected RunPadGenerator to return promptly after cancel")
	}
}
