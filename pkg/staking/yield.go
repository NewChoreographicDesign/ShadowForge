// Package staking implements spec 17.4's other epoch-mint proposer path —
// "staked 2 percent yield" — as a real, exact-integer yield formula.
//
// The spec names this path in one sentence (13.1: "mint escrow (10 percent
// fee to take tokens immediately, or 2 percent USDT-equivalent stake
// yield)"; 17.4: "direct with 10 percent fee, or staked 2 percent yield
// path") and defines neither an accrual period, a funding source, nor
// withdrawal mechanics — the same kind of genuinely underspecified detail
// spec 22's genesis-defaults table leaves for MintFeeNumerator/
// MintFeeDenominator (see types.MintNetAmount's own doc). This package
// makes the same kind of real, disclosed implementation decision rather
// than leaving the path unbuilt:
//
//  1. "2 percent" is a real annual rate (APY), matching this repo's only
//     other yield figure with an explicit unit (the metrics document's
//     "bank yields 2-5% APY on buffers") — the natural reading of a bare
//     "2 percent yield" elsewhere in the same spec.
//  2. Yield accrues pro-rata over the position's real held duration, in
//     wall-clock milliseconds (pkg/consensus.ElapsedMillis), not a naive
//     epoch count — spec 5.2's epochs grow 1.1x per epoch and are capped
//     at one year (pkg/consensus.EpochDuration), so two positions held for
//     "10 epochs" at different points in a chain's life cover wildly
//     different real durations; pricing yield off epoch count alone would
//     badly over- or under-pay depending purely on when the stake happened
//     to occur.
//  3. The yield itself is newly-minted SFG, exactly like the direct path's
//     principal is — both are epoch-mint proposer paths, and spec 9.1's
//     inflation ("2-5 percent per year ... unlocked only against
//     activity") already names real, ongoing SFG issuance as this
//     protocol's normal mechanism, not an exception this path invents.
//     Unlike the direct path, no Vault fee is taken on the staked path
//     (there is nothing to take a cut of at proposal time — the position's
//     principal is locked in full, and the "cost" to the protocol is
//     realized later, gradually, as yield); this is a real, deliberate,
//     disclosed asymmetry between the two paths, not an oversight.
//
// All arithmetic here is exact-integer (math/big), matching this
// codebase's consistent refusal to use floating point for money anywhere
// (pkg/decimal, pkg/consensus's own durationMillis) — see YieldAmount's
// own doc for the exact floor-division rule.
package staking

import (
	"math/big"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// YieldNumerator/YieldDenominator encode spec 17.4's "2 percent" staked
// yield path as an exact integer fraction, read as an annual rate (see
// package doc point 1) — a fixed protocol constant, not a
// governance.Params field, for the identical reason MintFeeNumerator/
// MintFeeDenominator are (types.MintNetAmount's own doc): spec 22's
// genesis-defaults table lists no such governance-adjustable parameter.
const (
	YieldNumerator   = 2
	YieldDenominator = 100
)

// millisPerYear is the fixed 365-day year pkg/consensus.EpochCap itself
// already uses (spec 5.2's own epoch-duration cap), reused here as the
// same real "one year" YieldNumerator/YieldDenominator is denominated
// against.
const millisPerYear = int64(365 * 24 * 60 * 60 * 1000)

// YieldAmount returns the real SFG yield a staked position of principal,
// created at startEpoch, has earned by endEpoch (typically the pipeline's
// own current epoch, Deps.Epoch, at the moment an Unstake transaction is
// processed) — exact-integer floor division of
// principal * YieldNumerator * elapsedMillis / (YieldDenominator *
// millisPerYear), so a duration that doesn't divide evenly rounds the
// yield down rather than fabricating a fractional SFG amount; the
// position holder never receives more than the formula's exact integer
// result. endEpoch <= startEpoch (including the impossible case of a
// claimed startEpoch after the real current epoch) yields exactly zero —
// no accrual, not an error — closing off any incentive to
// unstake-then-immediately-restake for a windfall, since zero elapsed
// time can only ever produce zero yield.
func YieldAmount(principal uint64, startEpoch, endEpoch types.EpochNumber) uint64 {
	if principal == 0 || endEpoch <= startEpoch {
		return 0
	}
	elapsedMillis := consensus.ElapsedMillis(uint64(startEpoch), uint64(endEpoch))
	if elapsedMillis <= 0 {
		return 0
	}

	num := new(big.Int).SetUint64(principal)
	num.Mul(num, big.NewInt(YieldNumerator))
	num.Mul(num, big.NewInt(elapsedMillis))
	den := new(big.Int).Mul(big.NewInt(YieldDenominator), big.NewInt(millisPerYear))
	q := new(big.Int).Quo(num, den) // both operands positive: Quo truncates toward zero, i.e. floors here

	if !q.IsUint64() {
		// Unreachable for any real deployment (would require a principal
		// and/or duration far beyond this codebase's own uint64 value
		// domain elsewhere), but a formula that silently wrapped instead
		// of failing loudly would be a real value-fabrication bug, not a
		// theoretical one — see zk.TransferCircuit's valueBits doc for
		// the identical reasoning applied to in-circuit arithmetic.
		return ^uint64(0)
	}
	return q.Uint64()
}

// FinalAmount is principal plus its own real YieldAmount — the exact
// value a real Unstake transaction's new output note must carry (see
// types.UnstakePublicInputs.FinalAmount's own doc).
func FinalAmount(principal uint64, startEpoch, endEpoch types.EpochNumber) uint64 {
	return principal + YieldAmount(principal, startEpoch, endEpoch)
}
