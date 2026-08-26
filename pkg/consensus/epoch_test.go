package consensus_test

import (
	"math"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
)

func closeEnough(t *testing.T, got, want time.Duration, tolerance time.Duration) {
	t.Helper()
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("got %v, want %v +/- %v (diff %v)", got, want, tolerance, diff)
	}
}

// TestEpochDurationSpecExamples checks the exact worked examples in spec
// section 19.1 / 5.2.
func TestEpochDurationSpecExamples(t *testing.T) {
	hourF := float64(time.Hour)
	expect := func(n float64) time.Duration { return time.Duration(math.Pow(1.1, n) * hourF) }
	closeEnough(t, consensus.EpochDuration(0), time.Hour, time.Millisecond)
	closeEnough(t, consensus.EpochDuration(1), expect(1), time.Millisecond)
	closeEnough(t, consensus.EpochDuration(10), expect(10), time.Millisecond)
	closeEnough(t, consensus.EpochDuration(50), expect(50), 100*time.Millisecond)
}

func TestEpochDurationYearCap(t *testing.T) {
	if got := consensus.EpochDuration(1000); got != consensus.EpochCap {
		t.Fatalf("epoch 1000 should be capped at 1 year, got %v", got)
	}
	// Find the first epoch that hits the cap and confirm epochs before it
	// are strictly shorter (monotonic growth, no premature capping).
	var capped uint64
	for n := uint64(0); n < 200; n++ {
		if consensus.EpochDuration(n) >= consensus.EpochCap {
			capped = n
			break
		}
	}
	if capped == 0 {
		t.Fatalf("expected some epoch > 0 to be the first capped epoch")
	}
	if consensus.EpochDuration(capped-1) >= consensus.EpochCap {
		t.Fatalf("epoch %d should be shorter than the cap", capped-1)
	}
}

// TestCurrentEpochMatchesWallClock is the Testing Matrix section 20 "Epoch"
// row: "Sum of durations matches wall clock across 0..N including the year
// cap."
func TestCurrentEpochMatchesWallClock(t *testing.T) {
	genesis := consensus.GenesisTime(0)
	var cumulative time.Duration
	for n := uint64(0); n < 120; n++ {
		// Just before this epoch's cumulative boundary, CurrentEpoch must
		// still report n.
		probe := time.UnixMilli(0).Add(cumulative + consensus.EpochDuration(n) - time.Millisecond)
		if got := consensus.CurrentEpoch(genesis, probe); got != n {
			t.Fatalf("epoch %d: at t=%v expected CurrentEpoch=%d, got %d", n, cumulative, n, got)
		}
		cumulative += consensus.EpochDuration(n)
		// Exactly at the boundary, CurrentEpoch must report n+1.
		atBoundary := time.UnixMilli(0).Add(cumulative)
		if got := consensus.CurrentEpoch(genesis, atBoundary); got != n+1 {
			t.Fatalf("epoch boundary %d: expected CurrentEpoch=%d, got %d", n, n+1, got)
		}
	}
}

func TestCurrentEpochBeforeGenesisIsZero(t *testing.T) {
	genesis := consensus.GenesisTime(time.Now().Add(time.Hour).UnixMilli())
	if got := consensus.CurrentEpoch(genesis, time.Now()); got != 0 {
		t.Fatalf("expected epoch 0 before genesis, got %d", got)
	}
}

func TestCurrentEpochLongAfterCap(t *testing.T) {
	genesis := consensus.GenesisTime(0)
	// 200 years after genesis: well past the ~11-year point where epochs
	// start lasting exactly 1 year each (kept under time.Duration's ~292
	// year int64-nanosecond range).
	now := time.UnixMilli(0).Add(time.Duration(200) * consensus.EpochCap)
	got := consensus.CurrentEpoch(genesis, now)
	if got == 0 {
		t.Fatalf("expected a large epoch number 200 years after genesis")
	}
}
