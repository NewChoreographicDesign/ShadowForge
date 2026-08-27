package oracle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// DefaultCoinbaseBaseURL is Coinbase's public, unauthenticated "Data API"
// used for exchange-rates lookups (api.coinbase.com/v2/exchange-rates).
const DefaultCoinbaseBaseURL = "https://api.coinbase.com/v2"

// DefaultCoinbaseExchangeBaseURL is Coinbase's public Exchange REST API
// used for historical candles (api.exchange.coinbase.com), a separate host
// and product from the v2 Data API above.
const DefaultCoinbaseExchangeBaseURL = "https://api.exchange.coinbase.com"

// DefaultCoinbaseProducts maps this chain's AssetIDs to Coinbase Exchange
// product ids (the path segment its candles endpoint requires).
var DefaultCoinbaseProducts = map[types.AssetID]string{
	types.AssetBTC: "BTC-USD",
	types.AssetETH: "ETH-USD",
}

// CoinbaseSource is a second, independent real Source — deliberately a
// different provider with a different response shape from CoinGeckoSource,
// so a configured Quorum has genuine redundancy: spec 11.3's "if oracles
// disagree beyond a bound, freeze new deposits" only means anything when
// more than one real, independently-operated feed is actually in the
// quorum.
type CoinbaseSource struct {
	// BaseURL overrides DefaultCoinbaseBaseURL (the v2 exchange-rates
	// host); tests point this at an httptest.Server.
	BaseURL string
	// ExchangeBaseURL overrides DefaultCoinbaseExchangeBaseURL (the
	// candles host); tests point this at an httptest.Server.
	ExchangeBaseURL string
	HTTPClient      *http.Client
	Products        map[types.AssetID]string
	// Granularity is the candles endpoint's bucket size in seconds; must
	// be one of Coinbase's accepted values (60/300/900/3600/21600/86400).
	// <=0 defaults to 3600 (1 hour).
	Granularity int
	// ATRPeriod is how many trailing true-range samples the ATR average
	// covers; <=0 defaults to 14.
	ATRPeriod int
}

func (c CoinbaseSource) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return DefaultCoinbaseBaseURL
}

func (c CoinbaseSource) exchangeBaseURL() string {
	if c.ExchangeBaseURL != "" {
		return c.ExchangeBaseURL
	}
	return DefaultCoinbaseExchangeBaseURL
}

func (c CoinbaseSource) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (c CoinbaseSource) product(asset types.AssetID) (string, error) {
	products := c.Products
	if products == nil {
		products = DefaultCoinbaseProducts
	}
	p, ok := products[asset]
	if !ok {
		return "", fmt.Errorf("oracle: no Coinbase product configured for asset %q", asset)
	}
	return p, nil
}

func (c CoinbaseSource) granularity() int {
	if c.Granularity > 0 {
		return c.Granularity
	}
	return 3600
}

func (c CoinbaseSource) atrPeriod() int {
	if c.ATRPeriod > 0 {
		return c.ATRPeriod
	}
	return 14
}

func (c CoinbaseSource) get(base, path string, query url.Values) ([]byte, error) {
	u := base + path
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

// exchangeRatesResponse mirrors Coinbase's documented v2/exchange-rates
// shape: {"data": {"currency": "BTC", "rates": {"USD": "76975.42", ...}}}.
// Rates are quoted as decimal strings, not JSON numbers.
type exchangeRatesResponse struct {
	Data struct {
		Currency string            `json:"currency"`
		Rates    map[string]string `json:"rates"`
	} `json:"data"`
}

// currencyCode maps a chain AssetID to the ISO-style code Coinbase's
// exchange-rates endpoint expects as the `currency` query param.
func currencyCode(asset types.AssetID) (string, error) {
	switch asset {
	case types.AssetBTC:
		return "BTC", nil
	case types.AssetETH:
		return "ETH", nil
	default:
		return "", fmt.Errorf("oracle: no Coinbase currency code configured for asset %q", asset)
	}
}

func (c CoinbaseSource) fetchPrice(asset types.AssetID) (decimal.Decimal, error) {
	code, err := currencyCode(asset)
	if err != nil {
		return decimal.Zero, err
	}
	body, err := c.get(c.baseURL(), "/exchange-rates", url.Values{"currency": {code}})
	if err != nil {
		return decimal.Zero, err
	}
	var parsed exchangeRatesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return decimal.Zero, fmt.Errorf("parse exchange-rates response: %w", err)
	}
	usd, ok := parsed.Data.Rates["USD"]
	if !ok {
		return decimal.Zero, fmt.Errorf("exchange-rates response missing USD rate for %q", code)
	}
	return decimal.FromString(usd)
}

func (c CoinbaseSource) fetchATR(productID string) (decimal.Decimal, error) {
	body, err := c.get(c.exchangeBaseURL(), fmt.Sprintf("/products/%s/candles", productID), url.Values{
		"granularity": {strconv.Itoa(c.granularity())},
	})
	if err != nil {
		return decimal.Zero, err
	}
	// Coinbase Exchange's documented /products/{id}/candles shape: a JSON
	// array of [time, low, high, open, close, volume] arrays (numeric
	// fields; time in Unix seconds).
	var raw [][]float64
	if err := json.Unmarshal(body, &raw); err != nil {
		return decimal.Zero, fmt.Errorf("parse candles response: %w", err)
	}
	candles := make([]ohlcCandle, 0, len(raw))
	for i, row := range raw {
		if len(row) != 6 {
			return decimal.Zero, fmt.Errorf("candle %d has %d fields, want 6", i, len(row))
		}
		candles = append(candles, ohlcCandle{TimestampMS: row[0] * 1000, Low: row[1], High: row[2], Open: row[3], Close: row[4]})
	}
	return averageTrueRange(candles, c.atrPeriod())
}

// Quote fetches asset's real current spot price (v2 exchange-rates) and a
// real ATR computed from real historical candles (Exchange product
// candles), both from Coinbase's live public APIs.
func (c CoinbaseSource) Quote(asset types.AssetID) (Quote, error) {
	product, err := c.product(asset)
	if err != nil {
		return Quote{}, err
	}
	price, err := c.fetchPrice(asset)
	if err != nil {
		return Quote{}, fmt.Errorf("oracle: coinbase price fetch for %s: %w", asset, err)
	}
	atr, err := c.fetchATR(product)
	if err != nil {
		return Quote{}, fmt.Errorf("oracle: coinbase ATR fetch for %s: %w", asset, err)
	}
	return Quote{PriceUSD: price, ATRUSD: atr, Timestamp: time.Now().Unix()}, nil
}
