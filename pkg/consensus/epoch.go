// Package consensus implements the ShadowForge L1 consensus layer: the
// epoch clock, revolver queue, five-stage BFT vote rule, sentinel
// activation, and outage/megabatch recovery (spec section 5).
package consensus

import (
	"math/big"
	"time"
)

// EpochGrowthNumerator/Denominator encode the 1.1x-per-epoch growth factor
// as an exact rational (11/10), so repeated multiplication never
// accumulates floating-point error — spec 5.2's "use ... a decimal or
// rational 11/10 multiplier so floating error cannot skip an epoch"
// requirement, applied literally.
const (
	EpochGrowthNumerator   = 11
	EpochGrowthDenominator = 10
)

// EpochCap is the maximum epoch duration: one non-leap year (spec 5.2).
const EpochCap = 365 * 24 * time.Hour

// EpochCapMillis is EpochCap expressed in integer milliseconds.
const EpochCapMillis = int64(EpochCap / time.Millisecond)

var hourMillis = big.NewInt(int64(time.Hour / time.Millisecond))

// durationMillis returns duration(n) = 1 hour * (11/10)^n, in exact integer
// milliseconds (rounded to nearest), before the 1-year cap is applied. Spec
// 5.2 is explicit that epoch math must use "integer milliseconds and a
// decimal or rational 11/10 multiplier so floating error cannot skip an
// epoch" — every epoch computation in this file, including the wall-clock
// comparison in CurrentEpoch, is built on this single millisecond-exact
// function so no two code paths can disagree by a sub-millisecond rounding
// difference.
func durationMillis(n uint64) int64 {
	num := new(big.Int).Exp(big.NewInt(EpochGrowthNumerator), new(big.Int).SetUint64(n), nil)
	den := new(big.Int).Exp(big.NewInt(EpochGrowthDenominator), new(big.Int).SetUint64(n), nil)
	r := new(big.Rat).SetFrac(num, den)
	r.Mul(r, new(big.Rat).SetInt(hourMillis))

	// Round to nearest millisecond.
	rNum := new(big.Int).Mul(r.Num(), big.NewInt(2))
	den2 := new(big.Int).Mul(r.Denom(), big.NewInt(2))
	rNum.Add(rNum, r.Denom())
	q := new(big.Int).Quo(rNum, den2)

	if !q.IsInt64() || q.Int64() > EpochCapMillis {
		return EpochCapMillis
	}
	return q.Int64()
}

// EpochDuration implements spec 19.1:
//
//	duration(epoch) = min(1 hour * 1.1^epoch_number, 1 year)
//
// using exact rational, millisecond-precision arithmetic (see
// durationMillis) instead of math.Pow(1.1, n), so no epoch boundary can be
// skipped by floating-point drift over a long-lived chain (spec 5.2).
func EpochDuration(n uint64) time.Duration {
	return time.Duration(durationMillis(n)) * time.Millisecond
}

// GenesisTime is the chain's genesis timestamp, in unix milliseconds
// (spec 5.2: "store GenesisTime").
type GenesisTime int64

// CurrentEpoch returns the largest n such that
// sum(duration(i) for i in 0..n-1) <= now - GenesisTime, per spec 5.2's
// "Implementation" paragraph. It walks epochs one at a time until the
// 1-year cap is reached (about 96 epochs for the 1.1x growth factor — well
// under a millisecond of work), then switches to O(1) arithmetic for every
// epoch after the cap so a long-lived chain never pays for a long loop.
func CurrentEpoch(genesis GenesisTime, now time.Time) uint64 {
	elapsedMillis := now.UnixMilli() - int64(genesis)
	if elapsedMillis < 0 {
		return 0
	}

	var n uint64
	for {
		d := durationMillis(n)
		if d >= EpochCapMillis {
			break
		}
		if elapsedMillis < d {
			return n
		}
		elapsedMillis -= d
		n++
	}
	// Every epoch from here on lasts exactly EpochCapMillis.
	extra := uint64(elapsedMillis / EpochCapMillis)
	return n + extra
}
