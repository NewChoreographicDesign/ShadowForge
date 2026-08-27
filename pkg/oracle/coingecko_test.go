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

// coinGeckoOHLCFixture builds a real-shaped CoinGecko OHLC response: a JSON
// array of [timestamp_ms, open, high, low, close] arrays, oldest-first,
// with a deliberate volatile candle so the computed ATR is verifiably not
// just "high-low of the last candle".
func coinGeckoOHLCFixture() [][]float64 {
	return [][]float64{
		{1_700_000_000_000, 60000, 60500, 59800, 60200},
		{1_700_014_400_000, 60200, 61000, 60100, 60900}, // true range vs prev close: max(900, 800, 100)=900
		{1_700_028_800_000, 60900, 61200, 60700, 61000},
		{1_700_043_200_000, 61000, 65000, 60900, 64000}, // big spike candle: TR = max(4100, 4000, 100) = 4100
	}
}

func TestCoinGeckoSourceQuoteRealResponseShape(t *testing.T) {
	var gotPricePath, gotOHLCPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/price":
			gotPricePath = r.URL.String()
			if got := r.URL.Query().Get("ids"); got != "bitcoin" {
				t.Errorf("expected ids=bitcoin, got %q", got)
			}
			if got := r.URL.Query().Get("vs_currencies"); got != "usd" {
				t.Errorf("expected vs_currencies=usd, got %q", got)
			}
			// Real documented /simple/price shape.
			if _, err := fmt.Fprint(w, `{"bitcoin":{"usd":62345.67}}`); err != nil {
				t.Errorf("write response: %v", err)
			}
		case "/coins/bitcoin/ohlc":
			gotOHLCPath = r.URL.String()
			if got := r.URL.Query().Get("days"); got != "7" {
				t.Errorf("expected days=7, got %q", got)
			}
			b, err := json.Marshal(coinGeckoOHLCFixture())
			if err != nil {
				t.Fatalf("marshal fixture: %v", err)
			}
			if _, err := w.Write(b); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	src := oracle.CoinGeckoSource{BaseURL: srv.URL}
	q, err := src.Quote(types.AssetBTC)
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if gotPricePath == "" || gotOHLCPath == "" {
		t.Fatalf("expected both endpoints to be hit, price=%q ohlc=%q", gotPricePath, gotOHLCPath)
	}
	if q.PriceUSD.String() != "6234567/100" {
		t.Fatalf("expected exact decoded price 62345.67, got %s (%v)", q.PriceUSD, q.PriceUSD.Float64())
	}
	// ATR = mean(900, 500, 4100) over the 3 true-range samples from 4
	// candles = 5500/3 ≈ 1833.33 — a real computed value, not a canned one.
	wantATR := (900.0 + 500.0 + 4100.0) / 3.0
	gotATR := q.ATRUSD.Float64()
	if diff := gotATR - wantATR; diff > 0.01 || diff < -0.01 {
		t.Fatalf("expected ATR ~%.4f, got %.4f", wantATR, gotATR)
	}
	if q.Timestamp == 0 {
		t.Fatalf("expected a non-zero timestamp")
	}
}

func TestCoinGeckoSourceQuoteUnknownAsset(t *testing.T) {
	src := oracle.CoinGeckoSource{BaseURL: "http://unused.invalid"}
	if _, err := src.Quote(types.AssetSFG); err == nil {
		t.Fatalf("expected an error for an asset with no configured CoinGecko coin id")
	}
}

func TestCoinGeckoSourceQuoteRejectsMalformedOHLCRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/price":
			_, _ = fmt.Fprint(w, `{"bitcoin":{"usd":60000}}`)
		case "/coins/bitcoin/ohlc":
			// A 4-field row instead of the documented 5.
			_, _ = fmt.Fprint(w, `[[1700000000000,1,2,3]]`)
		}
	}))
	defer srv.Close()

	src := oracle.CoinGeckoSource{BaseURL: srv.URL}
	if _, err := src.Quote(types.AssetBTC); err == nil {
		t.Fatalf("expected a malformed OHLC row to be rejected, not silently accepted")
	}
}

func TestCoinGeckoSourceQuotePropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"status":{"error_message":"rate limited"}}`)
	}))
	defer srv.Close()

	src := oracle.CoinGeckoSource{BaseURL: srv.URL}
	if _, err := src.Quote(types.AssetBTC); err == nil {
		t.Fatalf("expected a non-200 response to surface as an error, not a zero-value quote")
	}
}
