// Package decimal is the exact-arithmetic type spec section 19 pseudocode
// calls Decimal: `buffer := Decimal(2.5).Mul(currentATR_USD)`,
// `net.Sub(fee)`, `net.Sign()`. It wraps math/big.Rat so Bank and Vault
// math never accumulates floating-point error, matching spec 5.2's
// "Use integer milliseconds and a decimal or rational ... multiplier so
// floating error cannot skip an epoch" instruction applied consistently
// across all money math in the system.
package decimal

import (
	"fmt"
	"math/big"
)

// Decimal is an exact rational number.
type Decimal struct {
	r *big.Rat
}

// Zero is the additive identity.
var Zero = Decimal{r: new(big.Rat)}

// New builds a Decimal from an int64 numerator over an int64 denominator.
func New(num, den int64) Decimal {
	return Decimal{r: big.NewRat(num, den)}
}

// FromInt builds a Decimal from a whole number.
func FromInt(n int64) Decimal {
	return Decimal{r: big.NewRat(n, 1)}
}

// FromUint64 builds a Decimal from an unsigned whole number without ever
// round-tripping through int64. Phase 2 independent audit finding: several
// real, ZK-proof-bound on-chain quantities (e.g.
// TransferPublicInputs.FeeAmount) are uint64, range-constrained only to
// this codebase's own 64-bit domain (pkg/zk/circuit.go's valueBits), so
// they can legitimately exceed math.MaxInt64. Converting such a value with
// a bare int64(n) cast (the bug this constructor replaces at its one real
// call site, pkg/tx/pipeline.go's stage5PlaceFinal) reinterprets the top
// bit as a sign bit for any n >= 2^63, silently producing a large
// *negative* Decimal — every downstream Vault pool it's added to
// (EpochBonusPool, AuditPool, RemainderPool, BurnedTotal) would then be
// decremented by a real, attacker-reachable amount instead of credited,
// permanently corrupting the chain's shared accounting. Routing every
// uint64 that can plausibly exceed 2^63 through this constructor instead
// keeps the value's true, non-negative magnitude intact.
func FromUint64(n uint64) Decimal {
	return Decimal{r: new(big.Rat).SetInt(new(big.Int).SetUint64(n))}
}

// FromString parses a decimal or rational literal, e.g. "2.5", "0.001", "5/2".
func FromString(s string) (Decimal, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return Decimal{}, fmt.Errorf("decimal: invalid literal %q", s)
	}
	return Decimal{r: r}, nil
}

// MustFromString is FromString, panicking on error; only for compile-time
// constants inside this codebase (e.g. governance parameter defaults).
func MustFromString(s string) Decimal {
	d, err := FromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Decimal) rat() *big.Rat {
	if d.r == nil {
		return new(big.Rat)
	}
	return d.r
}

func (d Decimal) Add(o Decimal) Decimal { return Decimal{r: new(big.Rat).Add(d.rat(), o.rat())} }
func (d Decimal) Sub(o Decimal) Decimal { return Decimal{r: new(big.Rat).Sub(d.rat(), o.rat())} }
func (d Decimal) Mul(o Decimal) Decimal { return Decimal{r: new(big.Rat).Mul(d.rat(), o.rat())} }

// Div panics on division by zero — callers must check the divisor with
// Sign() first, matching how the spec's pseudocode always guards its own
// divisions (e.g. "if net.Sign() <= 0 { reject }" before any Div).
func (d Decimal) Div(o Decimal) Decimal {
	if o.Sign() == 0 {
		panic("decimal: division by zero")
	}
	return Decimal{r: new(big.Rat).Quo(d.rat(), o.rat())}
}

func (d Decimal) Sign() int { return d.rat().Sign() }

func (d Decimal) Cmp(o Decimal) int { return d.rat().Cmp(o.rat()) }

func (d Decimal) IsNeg() bool { return d.Sign() < 0 }

// Max0 returns max(0, d), matching the spec's repeated `max(0, ·)` rule
// (refund and retention formulas, 11.2 / 19.4).
func (d Decimal) Max0() Decimal {
	if d.Sign() < 0 {
		return Zero
	}
	return d
}

func Max(a, b Decimal) Decimal {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}

func Min(a, b Decimal) Decimal {
	if a.Cmp(b) <= 0 {
		return a
	}
	return b
}

func (d Decimal) String() string { return d.rat().RatString() }

// MarshalJSON encodes the exact rational as its RatString (e.g. "5/2"),
// never as a lossy float, so persisted Bank/Vault amounts round-trip
// exactly through the state store.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.rat().RatString() + `"`), nil
}

func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("decimal: invalid JSON string %q", s)
	}
	parsed, err := FromString(s[1 : len(s)-1])
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Float64 is for display/logging only — never for consensus-relevant math.
func (d Decimal) Float64() float64 {
	f, _ := d.rat().Float64()
	return f
}

// Uint64 truncates toward zero. Used to convert a whole-token amount (e.g.
// SFGIssued) after all fractional math is done.
func (d Decimal) Uint64() uint64 {
	r := d.rat()
	q := new(big.Int).Quo(r.Num(), r.Denom())
	return q.Uint64()
}
