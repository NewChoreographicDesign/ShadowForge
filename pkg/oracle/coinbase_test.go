package oracle_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// coinbaseCandlesFixtureNewestFirst mirrors Coinbase Exchange's documented
// /products/{id}/candles shape ([time, low, high, open, close, volume]),
// deliberately returned newest-first (as the live API does) to prove
// averageTrueRange's own oldest-first sort, not fixture ordering, is what
// makes the result correct.
func coinbaseCandlesFixtureNewestFirst() [][]float64 {
	return [][]float64{
		{1700043200, 60900, 65000, 61000, 64000, 12.5}, // newest
		{1700028800, 60700, 61200, 60900, 61000, 8.1},
		{1700014400, 60100, 61000, 60200, 60900, 9.4},
		{1700000000, 59800, 60500, 60000, 60200, 7.2}, // oldest
	}
}

func TestCoinbaseSourceQuoteRealResponseShape(t *testing.T) {
	var ratesSrv, exchangeSrv *httptest.Server
	ratesSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exchange-rates" {
			t.Fatalf("unexpected path %q on rates server", r.URL.Path)
		}
		if got := r.URL.Query().Get("currency"); got != "BTC" {
			t.Errorf("expected currency=BTC, got %q", got)
		}
		// Real documented v2/exchange-rates shape: rates are decimal
		// strings, not JSON numbers.
		_, _ = fmt.Fprint(w, `{"data":{"currency":"BTC","rates":{"USD":"62345.67","EUR":"57000.00"}}}`)
	}))
	defer ratesSrv.Close()

	exchangeSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/BTC-USD/candles" {
			t.Fatalf("unexpected path %q on exchange server", r.URL.Path)
		}
		if got := r.URL.Query().Get("granularity"); got != "3600" {
			t.Errorf("expected granularity=3600, got %q", got)
		}
		_, _ = fmt.Fprint(w, toJSON(t, coinbaseCandlesFixtureNewestFirst()))
	}))
	defer exchangeSrv.Close()

	src := oracle.CoinbaseSource{BaseURL: ratesSrv.URL, ExchangeBaseURL: exchangeSrv.URL}
	q, err := src.Quote(types.AssetBTC)
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if q.PriceUSD.String() != "6234567/100" {
		t.Fatalf("expected exact decoded price 62345.67, got %s", q.PriceUSD)
	}
	// Same candle shape/values as the CoinGecko fixture (just relabeled
	// low/high/open/close field order and newest-first), so the same true
	// ranges (900, 500, 4100) and ATR (~1833.33) should come out — proving
	// the oldest-first sort actually ran rather than silently trusting
	// arrival order.
	wantATR := (900.0 + 500.0 + 4100.0) / 3.0
	gotATR := q.ATRUSD.Float64()
	if diff := gotATR - wantATR; diff > 0.01 || diff < -0.01 {
		t.Fatalf("expected ATR ~%.4f, got %.4f", wantATR, gotATR)
	}
}

func TestCoinbaseSourceQuoteUnknownAsset(t *testing.T) {
	src := oracle.CoinbaseSource{BaseURL: "http://unused.invalid", ExchangeBaseURL: "http://unused.invalid"}
	if _, err := src.Quote(types.AssetSFG); err == nil {
		t.Fatalf("expected an error for an asset with no configured Coinbase product/currency")
	}
}

func TestCoinbaseSourceQuoteRejectsMalformedCandleRow(t *testing.T) {
	ratesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":{"currency":"BTC","rates":{"USD":"60000"}}}`)
	}))
	defer ratesSrv.Close()
	exchangeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A 5-field row instead of the documented 6.
		_, _ = fmt.Fprint(w, `[[1700000000,1,2,3,4]]`)
	}))
	defer exchangeSrv.Close()

	src := oracle.CoinbaseSource{BaseURL: ratesSrv.URL, ExchangeBaseURL: exchangeSrv.URL}
	if _, err := src.Quote(types.AssetBTC); err == nil {
		t.Fatalf("expected a malformed candle row to be rejected, not silently accepted")
	}
}

func toJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
