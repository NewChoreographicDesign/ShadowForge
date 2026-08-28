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

// ElapsedMillis returns the real, exact wall-clock milliseconds separating
// the start of fromEpoch and the start of toEpoch — sum(duration(i) for i
// in [fromEpoch, toEpoch)) — using the same exact-integer durationMillis
// every other epoch computation in this file is built on. toEpoch <=
// fromEpoch returns 0 rather than a negative value: pkg/staking's real
// yield formula (spec 17.4's "staked 2 percent yield path") needs this to
// convert a stake position's real held duration into a fraction of a
// year, and epochs grow 1.1x per epoch (durationMillis above) rather than
// being fixed-length, so counting elapsed *epochs* alone — ignoring how
// long each one actually lasted — would badly misprice yield for a
// position held across epoch-duration growth. Once durationMillis(n)
// reaches EpochCapMillis (spec 5.2's one-year cap, after roughly 96
// epochs), every further epoch is worth exactly EpochCapMillis, so the
// tail of a very long span is summed in O(1) rather than one epoch at a
// time — the same capped-tail strategy CurrentEpoch already uses, for the
// same reason (a long-lived chain must never pay for an ever-growing
// loop).
func ElapsedMillis(fromEpoch, toEpoch uint64) int64 {
	if toEpoch <= fromEpoch {
		return 0
	}
	var total int64
	n := fromEpoch
	for n < toEpoch {
		d := durationMillis(n)
		if d >= EpochCapMillis {
			remaining := toEpoch - n
			// remaining epochs, each capped at EpochCapMillis: bounded by
			// big.Int first so a pathological (attacker-unreachable, since
			// toEpoch is always this node's own real current epoch, never
			// attacker-supplied) huge span clamps rather than silently
			// wrapping an int64 multiplication.
			capped := new(big.Int).Mul(big.NewInt(int64(remaining)), big.NewInt(EpochCapMillis))
			capped.Add(capped, big.NewInt(total))
			if capped.IsInt64() {
				return capped.Int64()
			}
			return int64(^uint64(0) >> 1) // math.MaxInt64, avoiding an import solely for one constant
		}
		total += d
		n++
	}
	return total
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
