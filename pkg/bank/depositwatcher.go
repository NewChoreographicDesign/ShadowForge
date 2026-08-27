package bank

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// DefaultMinConfirmations is how many confirmations a claimed deposit's
// on-chain transaction must have before DepositWatcher-gated Deposit calls
// trust it — 6 is Bitcoin's own long-standing conventional "safe" depth
// (the point past which a reorg deep enough to reverse it is considered
// impractical), used here rather than an arbitrary smaller number.
const DefaultMinConfirmations = 6

// ErrDepositVerificationFailed wraps any error from a DepositWatcher's
// VerifyDeposit call (network failure, malformed response, etc.) — the
// underlying error is preserved via errors.Unwrap.
var ErrDepositVerificationFailed = errors.New("bank: deposit verification failed")

// ErrDepositNotConfirmed means a DepositWatcher found the claimed
// transaction but it does not yet have enough confirmations.
var ErrDepositNotConfirmed = errors.New("bank: deposit transaction does not have enough confirmations yet")

// ErrDepositAmountMismatch means a DepositWatcher's real on-chain
// observation pays less than DepositParams.ExternalAmount claims — the
// exact gap this file closes: Deposit used to trust ExternalAmount as a
// bare caller-supplied number with no independent verification at all.
var ErrDepositAmountMismatch = errors.New("bank: claimed deposit amount exceeds what is actually confirmed on-chain")

// DepositObservation is what a DepositWatcher found for one claimed
// deposit transaction.
type DepositObservation struct {
	// ConfirmedAmount is the real amount (in the asset's natural whole
	// unit, e.g. whole BTC — matching DepositParams.ExternalAmount's own
	// unit) the transaction actually paid to the custody address,
	// regardless of confirmation depth.
	ConfirmedAmount decimal.Decimal
	// Confirmations is how many blocks deep the transaction is; 0 means
	// unconfirmed (still in the mempool).
	Confirmations int
}

// DepositWatcher independently verifies that a claimed external-asset
// deposit really arrived on-chain, before Deposit's math ever trusts
// ExternalAmount. This interface only ever reads/verifies — nothing behind
// it holds private key material or can move funds; see
// EsploraDepositWatcher's own doc for why moving real cryptocurrency is a
// deliberate, hard boundary this build does not cross regardless of
// technical feasibility.
type DepositWatcher interface {
	// VerifyDeposit looks up txID and reports what it actually paid to
	// custodyAddress and how many confirmations it has. A txID that
	// doesn't exist, or doesn't pay custodyAddress at all, is not an
	// error — it comes back as a zero ConfirmedAmount, letting the caller
	// apply its own ErrDepositAmountMismatch policy uniformly.
	VerifyDeposit(asset types.AssetID, custodyAddress, txID string) (DepositObservation, error)
}

// DefaultEsploraBaseURL is Blockstream's public Esplora REST API for the
// Bitcoin mainnet (github.com/Blockstream/esplora API.md) — no API key
// required.
const DefaultEsploraBaseURL = "https://blockstream.info/api"

// EsploraDepositWatcher is a real DepositWatcher backed by a real Esplora
// block-explorer API: a real HTTP client fetching the real, documented
// GET /tx/{txid} and GET /blocks/tip/height response shapes.
//
// Deliberate boundary: this type verifies that a Bitcoin payment happened;
// it never holds a private key, never constructs or signs a Bitcoin
// transaction, and never has any code path that could move real BTC.
// Actual custody (generating/holding deposit addresses' private keys,
// sweeping received funds) is out of scope for this build — moving real
// cryptocurrency carries catastrophic, irreversible financial risk and
// requires infrastructure (HSM-backed key management, a funded hot/cold
// wallet, withdrawal review process) this reference implementation
// deliberately does not attempt to provide. Wiring a real custody backend
// behind the same DepositWatcher-verified Deposit flow is a separate,
// much larger undertaking a production deployment must do explicitly.
type EsploraDepositWatcher struct {
	// BaseURL overrides DefaultEsploraBaseURL; tests point this at an
	// httptest.Server serving canned responses in Esplora's exact
	// documented shape.
	BaseURL    string
	HTTPClient *http.Client
}

func (w EsploraDepositWatcher) baseURL() string {
	if w.BaseURL != "" {
		return w.BaseURL
	}
	return DefaultEsploraBaseURL
}

func (w EsploraDepositWatcher) httpClient() *http.Client {
	if w.HTTPClient != nil {
		return w.HTTPClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (w EsploraDepositWatcher) get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, w.baseURL()+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := w.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		s := string(body)
		if len(s) > 256 {
			s = s[:256] + "..."
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, s)
	}
	return body, nil
}

// esploraTx mirrors Esplora's documented GET /tx/{txid} shape (the same
// shape as one entry of GET /address/{address}/txs): amounts are always in
// satoshis.
type esploraTx struct {
	TxID   string        `json:"txid"`
	Vout   []esploraVout `json:"vout"`
	Status struct {
		Confirmed   bool  `json:"confirmed"`
		BlockHeight int64 `json:"block_height"`
	} `json:"status"`
}

type esploraVout struct {
	ScriptPubKeyAddress string `json:"scriptpubkey_address"`
	Value               uint64 `json:"value"`
}

// satsPerBTC converts Esplora's satoshi-denominated amounts to the whole
// BTC decimal unit DepositParams.ExternalAmount itself already uses (spec
// 19.3's worked examples use whole coins, e.g. "Q=1 BTC").
const satsPerBTC = 100_000_000

func satsToBTC(sats uint64) decimal.Decimal {
	return decimal.New(int64(sats), satsPerBTC)
}

// fetchTipHeight fetches the chain's current tip height. Esplora's
// documented GET /blocks/tip/height response is plain text (not JSON): a
// bare decimal integer.
func (w EsploraDepositWatcher) fetchTipHeight() (int64, error) {
	body, err := w.get("/blocks/tip/height")
	if err != nil {
		return 0, fmt.Errorf("fetch tip height: %w", err)
	}
	height, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse tip height %q: %w", string(body), err)
	}
	return height, nil
}

// VerifyDeposit fetches txID's real on-chain state from a live Esplora
// instance and reports what it actually paid to custodyAddress and its
// real confirmation depth.
func (w EsploraDepositWatcher) VerifyDeposit(asset types.AssetID, custodyAddress, txID string) (DepositObservation, error) {
	if asset != types.AssetBTC {
		return DepositObservation{}, fmt.Errorf("bank: EsploraDepositWatcher only supports BTC, got %q", asset)
	}
	if custodyAddress == "" || txID == "" {
		return DepositObservation{}, errors.New("bank: custodyAddress and txID are required")
	}
	body, err := w.get("/tx/" + txID)
	if err != nil {
		return DepositObservation{}, fmt.Errorf("fetch tx %s: %w", txID, err)
	}
	var tx esploraTx
	if err := json.Unmarshal(body, &tx); err != nil {
		return DepositObservation{}, fmt.Errorf("parse tx response: %w", err)
	}

	var sats uint64
	for _, out := range tx.Vout {
		if out.ScriptPubKeyAddress == custodyAddress {
			sats += out.Value
		}
	}

	confirmations := 0
	if tx.Status.Confirmed {
		tip, err := w.fetchTipHeight()
		if err != nil {
			return DepositObservation{}, err
		}
		confirmations = int(tip-tx.Status.BlockHeight) + 1
		if confirmations < 0 {
			confirmations = 0
		}
	}

	return DepositObservation{ConfirmedAmount: satsToBTC(sats), Confirmations: confirmations}, nil
}
