package staking_test

import (
	"math/big"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/staking"
)

func TestYieldAmountZeroWhenNoElapsedEpochs(t *testing.T) {
	if got := staking.YieldAmount(1_000_000, 5, 5); got != 0 {
		t.Fatalf("YieldAmount with equal start/end epoch = %d, want 0", got)
	}
	if got := staking.YieldAmount(1_000_000, 5, 3); got != 0 {
		t.Fatalf("YieldAmount with endEpoch before startEpoch = %d, want 0", got)
	}
}

func TestYieldAmountZeroForZeroPrincipal(t *testing.T) {
	if got := staking.YieldAmount(0, 0, 100); got != 0 {
		t.Fatalf("YieldAmount(0, ...) = %d, want 0", got)
	}
}

// wantYield reimplements the exact formula staking.YieldAmount's own doc
// describes — principal * 2 * elapsedMillis / (100 * millisPerYear),
// floored — independently, against the real elapsed-milliseconds figure
// pkg/consensus.ElapsedMillis computes, so this test is a genuine
// regression guard against the formula's own constants changing
// silently, not a tautology.
func wantYield(principal uint64, startEpoch, endEpoch uint64) uint64 {
	elapsed := consensus.ElapsedMillis(startEpoch, endEpoch)
	num := new(big.Int).SetUint64(principal)
	num.Mul(num, big.NewInt(2))
	num.Mul(num, big.NewInt(elapsed))
	den := new(big.Int).Mul(big.NewInt(100), big.NewInt(int64(365*24*60*60*1000)))
	q := new(big.Int).Quo(num, den)
	return q.Uint64()
}

func TestYieldAmountMatchesExactFormulaAcrossRealEpochRanges(t *testing.T) {
	cases := []struct {
		principal            uint64
		startEpoch, endEpoch uint64
	}{
		{1_000_000, 0, 1},     // one short (1 hour) epoch
		{1_000_000, 0, 10},    // several early, still-growing epochs
		{500_000_000, 0, 96},  // spans the 1.1x growth curve up to the ~1-year cap
		{500_000_000, 96, 97}, // one full capped (1-year) epoch: real ~2% yield
		{123456789, 3, 50},
	}
	for _, c := range cases {
		want := wantYield(c.principal, c.startEpoch, c.endEpoch)
		got := staking.YieldAmount(c.principal, c.startEpoch, c.endEpoch)
		if got != want {
			t.Fatalf("YieldAmount(%d, %d, %d) = %d, want %d", c.principal, c.startEpoch, c.endEpoch, got, want)
		}
	}
}

// TestYieldAmountOneCappedYearIsRealTwoPercent proves the headline
// figure directly: staking principal for exactly one full, capped
// (post-96th-epoch) real epoch — which pkg/consensus.EpochCap fixes at
// exactly one year — yields exactly 2% of principal, floored.
func TestYieldAmountOneCappedYearIsRealTwoPercent(t *testing.T) {
	const principal = 1_000_000_000
	got := staking.YieldAmount(principal, 96, 97)
	want := uint64(principal * 2 / 100)
	if got != want {
		t.Fatalf("one capped year's yield = %d, want exactly 2%% of principal (%d)", got, want)
	}
}

func TestFinalAmountIsPrincipalPlusYield(t *testing.T) {
	const principal = 7_777_777
	final := staking.FinalAmount(principal, 0, 50)
	yield := staking.YieldAmount(principal, 0, 50)
	if final != principal+yield {
		t.Fatalf("FinalAmount = %d, want principal + yield = %d", final, principal+yield)
	}
	if final <= principal {
		t.Fatalf("expected some real positive yield over 50 epochs, FinalAmount %d <= principal %d", final, principal)
	}
}
