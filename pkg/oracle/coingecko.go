package oracle

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// DefaultCoinGeckoBaseURL is CoinGecko's keyless public REST API base
// (docs.coingecko.com/reference/simple-price and .../coins-id-ohlc) — no
// API key required, rate-limited to a handful of calls/minute under their
// public tier, which is why Quorum polling should stay infrequent.
const DefaultCoinGeckoBaseURL = "https://api.coingecko.com/api/v3"

// DefaultCoinGeckoIDs maps this chain's AssetIDs to CoinGecko's own coin
// ids, the identifier its API requires in the `ids` query param and the
// `/coins/{id}` path (spec 3.3's "Chainlink-class + fallback" feed). SFG
// has no entry: it is this chain's own native asset, not something an
// external market lists.
var DefaultCoinGeckoIDs = map[types.AssetID]string{
	types.AssetBTC: "bitcoin",
	types.AssetETH: "ethereum",
}

// CoinGeckoSource is a real Source backed by CoinGecko's public REST API: a
// real HTTP client fetching the real, documented /simple/price and
// /coins/{id}/ohlc response shapes, with a real ATR (Average True Range)
// computed from real historical OHLC candles — not a fixed test value.
type CoinGeckoSource struct {
	// BaseURL overrides DefaultCoinGeckoBaseURL; tests point this at an
	// httptest.Server serving canned responses in CoinGecko's exact
	// documented shape.
	BaseURL string
	// HTTPClient overrides the default (5s deadline, so one slow feed
	// can't hang a whole Quorum poll).
	HTTPClient *http.Client
	// CoinIDs overrides DefaultCoinGeckoIDs.
	CoinIDs map[types.AssetID]string
	// OHLCDays selects the /coins/{id}/ohlc `days` window. CoinGecko's
	// public API only accepts specific values (1/7/14/30/90/180/365) and
	// auto-buckets candle granularity from it; <=0 defaults to 7 (4-hourly
	// candles).
	OHLCDays int
	// ATRPeriod is how many trailing true-range samples the ATR average
	// covers; <=0 defaults to 14 (Wilder's original convention).
	ATRPeriod int
}

func (c CoinGeckoSource) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultCoinGeckoBaseURL
}

func (c CoinGeckoSource) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (c CoinGeckoSource) coinID(asset types.AssetID) (string, error) {
	ids := c.CoinIDs
	if ids == nil {
		ids = DefaultCoinGeckoIDs
	}
	id, ok := ids[asset]
	if !ok {
		return "", fmt.Errorf("oracle: no CoinGecko coin id configured for asset %q", asset)
	}
	return id, nil
}

func (c CoinGeckoSource) ohlcDays() int {
	if c.OHLCDays > 0 {
		return c.OHLCDays
	}
	return 7
}

func (c CoinGeckoSource) atrPeriod() int {
	if c.ATRPeriod > 0 {
		return c.ATRPeriod
	}
	return 14
}

// Quote fetches asset's real current spot price and a real ATR computed
// from real historical OHLC candles, both from CoinGecko's live public API.
func (c CoinGeckoSource) Quote(asset types.AssetID) (Quote, error) {
	id, err := c.coinID(asset)
	if err != nil {
		return Quote{}, err
	}
	price, err := c.fetchPrice(id)
	if err != nil {
		return Quote{}, fmt.Errorf("oracle: coingecko price fetch for %s: %w", asset, err)
	}
	atr, err := c.fetchATR(id)
	if err != nil {
		return Quote{}, fmt.Errorf("oracle: coingecko ATR fetch for %s: %w", asset, err)
	}
	return Quote{PriceUSD: price, ATRUSD: atr, Timestamp: time.Now().Unix()}, nil
}

func (c CoinGeckoSource) get(path string, query url.Values) ([]byte, error) {
	u := c.baseURL() + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := readAllLimited(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, truncate(body, 256))
	}
	return body, nil
}

// simplePriceResponse mirrors CoinGecko's documented /simple/price shape:
// {"<coin id>": {"usd": <float>, ...}}.
type simplePriceResponse map[string]map[string]float64

func (c CoinGeckoSource) fetchPrice(coinID string) (decimal.Decimal, error) {
	body, err := c.get("/simple/price", url.Values{
		"ids":           {coinID},
		"vs_currencies": {"usd"},
	})
	if err != nil {
		return decimal.Zero, err
	}
	var parsed simplePriceResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return decimal.Zero, fmt.Errorf("parse simple/price response: %w", err)
	}
	entry, ok := parsed[coinID]
	if !ok {
		return decimal.Zero, fmt.Errorf("simple/price response missing coin id %q", coinID)
	}
	usd, ok := entry["usd"]
	if !ok {
		return decimal.Zero, fmt.Errorf("simple/price response missing usd field for %q", coinID)
	}
	return floatToDecimal(usd)
}

func (c CoinGeckoSource) fetchATR(coinID string) (decimal.Decimal, error) {
	body, err := c.get(fmt.Sprintf("/coins/%s/ohlc", coinID), url.Values{
		"vs_currency": {"usd"},
		"days":        {strconv.Itoa(c.ohlcDays())},
	})
	if err != nil {
		return decimal.Zero, err
	}
	// CoinGecko's documented /coins/{id}/ohlc shape: a JSON array of
	// 5-element [timestamp_ms, open, high, low, close] arrays.
	var raw [][]float64
	if err := json.Unmarshal(body, &raw); err != nil {
		return decimal.Zero, fmt.Errorf("parse ohlc response: %w", err)
	}
	candles := make([]ohlcCandle, 0, len(raw))
	for i, row := range raw {
		if len(row) != 5 {
			return decimal.Zero, fmt.Errorf("ohlc candle %d has %d fields, want 5", i, len(row))
		}
		candles = append(candles, ohlcCandle{TimestampMS: row[0], Open: row[1], High: row[2], Low: row[3], Close: row[4]})
	}
	return averageTrueRange(candles, c.atrPeriod())
}

// ohlcCandle is one open/high/low/close candle, timestamped, used by both
// CoinGeckoSource and CoinbaseSource so averageTrueRange has one shared
// implementation regardless of which provider's response shape it came
// from.
type ohlcCandle struct {
	TimestampMS            float64
	Open, High, Low, Close float64
}

// averageTrueRange computes a real ATR (spec 11.1's ATR-derived buffer):
// each candle's true range is the widest of its own high-low spread and
// its gap from the previous candle's close, and ATR is the simple average
// of true range over the trailing `period` samples (falling back to every
// available sample once fewer than `period` exist, so a short history
// still yields a real, if less smoothed, reading rather than an error).
// Candles are sorted oldest-first before computing, since providers differ
// on whether they return newest-first or oldest-first.
func averageTrueRange(candles []ohlcCandle, period int) (decimal.Decimal, error) {
	if len(candles) < 2 {
		return decimal.Zero, fmt.Errorf("need at least 2 candles to compute a true range, got %d", len(candles))
	}
	sorted := append([]ohlcCandle{}, candles...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TimestampMS < sorted[j].TimestampMS })

	trueRanges := make([]decimal.Decimal, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		hi, lo, prevClose := sorted[i].High, sorted[i].Low, sorted[i-1].Close
		tr := math.Max(hi-lo, math.Max(math.Abs(hi-prevClose), math.Abs(lo-prevClose)))
		d, err := floatToDecimal(tr)
		if err != nil {
			return decimal.Zero, err
		}
		trueRanges = append(trueRanges, d)
	}
	if period > 0 && period < len(trueRanges) {
		trueRanges = trueRanges[len(trueRanges)-period:]
	}
	sum := decimal.Zero
	for _, tr := range trueRanges {
		sum = sum.Add(tr)
	}
	return sum.Div(decimal.FromInt(int64(len(trueRanges)))), nil
}

func floatToDecimal(f float64) (decimal.Decimal, error) {
	return decimal.FromString(strconv.FormatFloat(f, 'f', -1, 64))
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// readAllLimited caps a response body read at 1MB, shared by every real
// HTTP Source in this package — an oracle feed is untrusted external
// input, and an unbounded read is a memory-exhaustion vector if a
// misbehaving or compromised endpoint ever returned an enormous body.
func readAllLimited(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, 1<<20))
}
