// Package oracle provides the external price/ATR feed abstraction spec
// section 3.3 calls for ("Redundant price + ATR feeds (Chainlink-class +
// fallback)") and the quorum-with-freeze-on-disagreement policy from spec
// 11.3: "Oracle quorum; if oracles disagree beyond a bound, freeze new
// deposits and use last-good snapshots for open holds."
package oracle

import (
	"errors"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Quote is one oracle's reading for an asset: spot price and current ATR,
// both in USD (spec 11.1: "ATR is in USD").
type Quote struct {
	PriceUSD  decimal.Decimal
	ATRUSD    decimal.Decimal
	Timestamp int64
}

// Source is a single price/ATR feed.
type Source interface {
	Quote(asset types.AssetID) (Quote, error)
}

// ErrDisagreement is returned when the configured sources' quotes spread
// beyond MaxDisagreement.
var ErrDisagreement = errors.New("oracle: sources disagree beyond the allowed bound")

// Quorum aggregates multiple Sources and enforces the disagreement bound.
// It reports the median quote when sources agree, and remembers the last
// quote it successfully produced per asset so callers can still price
// existing open holds during a disagreement freeze (spec 11.3).
type Quorum struct {
	sources         []Source
	maxDisagreement decimal.Decimal // fractional, e.g. 0.02 == 2%
	lastGood        map[types.AssetID]Quote
}

// NewQuorum builds a Quorum. maxDisagreement is a fraction of the median
// price/ATR (e.g. decimal.MustFromString("0.02") for 2%).
func NewQuorum(maxDisagreement decimal.Decimal, sources ...Source) *Quorum {
	return &Quorum{sources: sources, maxDisagreement: maxDisagreement, lastGood: map[types.AssetID]Quote{}}
}

// Quote polls every source and returns the median if all readings agree
// within maxDisagreement; otherwise it returns ErrDisagreement.
func (q *Quorum) Quote(asset types.AssetID) (Quote, error) {
	if len(q.sources) == 0 {
		return Quote{}, errors.New("oracle: no sources configured")
	}
	prices := make([]decimal.Decimal, 0, len(q.sources))
	atrs := make([]decimal.Decimal, 0, len(q.sources))
	var latest Quote
	for _, s := range q.sources {
		quote, err := s.Quote(asset)
		if err != nil {
			return Quote{}, fmt.Errorf("oracle: source failed: %w", err)
		}
		prices = append(prices, quote.PriceUSD)
		atrs = append(atrs, quote.ATRUSD)
		if quote.Timestamp >= latest.Timestamp {
			latest = quote
		}
	}
	if !withinBound(prices, q.maxDisagreement) || !withinBound(atrs, q.maxDisagreement) {
		return Quote{}, ErrDisagreement
	}
	result := Quote{PriceUSD: median(prices), ATRUSD: median(atrs), Timestamp: latest.Timestamp}
	q.lastGood[asset] = result
	return result, nil
}

// LastGood returns the most recent successfully-agreed quote for asset, for
// pricing open holds while new deposits are frozen (spec 11.3).
func (q *Quorum) LastGood(asset types.AssetID) (Quote, bool) {
	v, ok := q.lastGood[asset]
	return v, ok
}

func withinBound(vals []decimal.Decimal, bound decimal.Decimal) bool {
	if len(vals) < 2 {
		return true
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals[1:] {
		if v.Cmp(lo) < 0 {
			lo = v
		}
		if v.Cmp(hi) > 0 {
			hi = v
		}
	}
	if lo.Sign() <= 0 {
		return hi.Sign() == 0
	}
	spread := hi.Sub(lo).Div(lo)
	return spread.Cmp(bound) <= 0
}

// median is a simple O(n^2) median for the small (single-digit) source
// counts a quorum realistically has; clarity over micro-optimization.
func median(vals []decimal.Decimal) decimal.Decimal {
	sorted := append([]decimal.Decimal{}, vals...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Cmp(sorted[j-1]) < 0; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return sorted[n/2-1].Add(sorted[n/2]).Div(decimal.FromInt(2))
}

// StaticSource is a fixed-value Source, used by tests, the ShadeLang
// sandbox, and local development networks.
type StaticSource struct {
	Value Quote
	Err   error
}

func (s StaticSource) Quote(types.AssetID) (Quote, error) { return s.Value, s.Err }
