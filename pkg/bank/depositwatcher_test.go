package bank_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/bank"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

const testTxID = "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"
const testAddress = "bc1qexamplecustodyaddress0000000000000000"

// esploraTxFixture builds a real-shaped Esplora GET /tx/{txid} response: a
// confirmed transaction paying 150,000,000 sats (1.5 BTC) to the custody
// address, plus an unrelated change output back to some other address —
// proving VerifyDeposit sums only outputs that actually pay the custody
// address, not the transaction's total output value.
func esploraTxFixture(confirmed bool, blockHeight int64) string {
	status := `{"confirmed":false,"block_height":null,"block_hash":null,"block_time":null}`
	if confirmed {
		status = fmt.Sprintf(`{"confirmed":true,"block_height":%d,"block_hash":"00000000000000000001","block_time":1700000000}`, blockHeight)
	}
	return fmt.Sprintf(`{
		"txid": %q,
		"version": 2,
		"locktime": 0,
		"vin": [],
		"vout": [
			{"scriptpubkey":"a", "scriptpubkey_address": %q, "value": 150000000},
			{"scriptpubkey":"b", "scriptpubkey_address": "bc1qsomeoneelsechangeaddress0000000000", "value": 25000}
		],
		"status": %s
	}`, testTxID, testAddress, status)
}

func newEsploraServer(t *testing.T, confirmed bool, blockHeight, tipHeight int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tx/" + testTxID:
			_, _ = fmt.Fprint(w, esploraTxFixture(confirmed, blockHeight))
		case "/blocks/tip/height":
			_, _ = fmt.Fprintf(w, "%d", tipHeight)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
}

func TestEsploraDepositWatcherVerifyDepositRealResponseShape(t *testing.T) {
	// blockHeight 800000, tip 800005: 6 confirmations (800005-800000+1).
	srv := newEsploraServer(t, true, 800000, 800005)
	defer srv.Close()

	w := bank.EsploraDepositWatcher{BaseURL: srv.URL}
	obs, err := w.VerifyDeposit(types.AssetBTC, testAddress, testTxID)
	if err != nil {
		t.Fatalf("VerifyDeposit: %v", err)
	}
	if obs.ConfirmedAmount.String() != "3/2" { // 150,000,000 sats / 100,000,000 = 1.5 BTC = 3/2 exactly
		t.Fatalf("expected exact 1.5 BTC (3/2), got %s (%v)", obs.ConfirmedAmount, obs.ConfirmedAmount.Float64())
	}
	if obs.Confirmations != 6 {
		t.Fatalf("expected 6 confirmations, got %d", obs.Confirmations)
	}
}

func TestEsploraDepositWatcherUnconfirmedTxHasZeroConfirmations(t *testing.T) {
	srv := newEsploraServer(t, false, 0, 800005)
	defer srv.Close()

	w := bank.EsploraDepositWatcher{BaseURL: srv.URL}
	obs, err := w.VerifyDeposit(types.AssetBTC, testAddress, testTxID)
	if err != nil {
		t.Fatalf("VerifyDeposit: %v", err)
	}
	if obs.Confirmations != 0 {
		t.Fatalf("expected 0 confirmations for an unconfirmed tx, got %d", obs.Confirmations)
	}
	// The claimed payment amount is real and observable even before
	// confirmation — only the confirmation *count* differs.
	if obs.ConfirmedAmount.String() != "3/2" {
		t.Fatalf("expected the paid amount to still be reported, got %s", obs.ConfirmedAmount)
	}
}

func TestEsploraDepositWatcherIgnoresPaymentsToOtherAddresses(t *testing.T) {
	srv := newEsploraServer(t, true, 800000, 800000)
	defer srv.Close()

	w := bank.EsploraDepositWatcher{BaseURL: srv.URL}
	obs, err := w.VerifyDeposit(types.AssetBTC, "bc1qNOT-the-custody-address", testTxID)
	if err != nil {
		t.Fatalf("VerifyDeposit: %v", err)
	}
	if obs.ConfirmedAmount.Sign() != 0 {
		t.Fatalf("expected zero confirmed amount for a custody address this tx never paid, got %s", obs.ConfirmedAmount)
	}
}

func TestEsploraDepositWatcherRejectsNonBTCAsset(t *testing.T) {
	w := bank.EsploraDepositWatcher{BaseURL: "http://unused.invalid"}
	if _, err := w.VerifyDeposit(types.AssetETH, testAddress, testTxID); err == nil {
		t.Fatalf("expected an error for an unsupported asset")
	}
}

func TestEsploraDepositWatcherPropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"message":"Transaction not found"}`)
	}))
	defer srv.Close()

	w := bank.EsploraDepositWatcher{BaseURL: srv.URL}
	if _, err := w.VerifyDeposit(types.AssetBTC, testAddress, testTxID); err == nil {
		t.Fatalf("expected a 404 to surface as an error")
	}
}

// stubWatcher lets Deposit-level tests below drive DepositWatcher's
// contract directly, without a real HTTP round trip — proving Deposit's
// own wiring (confirmation threshold, amount-mismatch rejection) rather
// than re-testing EsploraDepositWatcher's HTTP parsing.
type stubWatcher struct {
	obs bank.DepositObservation
	err error
}

func (s stubWatcher) VerifyDeposit(types.AssetID, string, string) (bank.DepositObservation, error) {
	return s.obs, s.err
}

func TestDepositAcceptsWhenWatcherConfirmsClaimedAmount(t *testing.T) {
	hold, err := bank.Deposit(bank.DepositParams{
		Asset:            types.AssetBTC,
		ExternalAmount:   dec("1"),
		PriceUSD:         dec("60000"),
		ATRUSD:           dec("2000"),
		SFGUSDPrice:      dec("5"),
		Now:              1000,
		CustodyAddress:   testAddress,
		TxID:             testTxID,
		Watcher:          stubWatcher{obs: bank.DepositObservation{ConfirmedAmount: dec("1"), Confirmations: 6}},
		MinConfirmations: 6,
	})
	if err != nil {
		t.Fatalf("expected a fully-confirmed, amount-matching deposit to be accepted, got %v", err)
	}
	if hold.SFGIssued == 0 {
		t.Fatalf("expected a real hold to be issued")
	}
}

func TestDepositRejectsWhenWatcherReportsInsufficientConfirmations(t *testing.T) {
	_, err := bank.Deposit(bank.DepositParams{
		Asset: types.AssetBTC, ExternalAmount: dec("1"), PriceUSD: dec("60000"),
		ATRUSD: dec("2000"), SFGUSDPrice: dec("5"), Now: 1000,
		CustodyAddress:   testAddress,
		TxID:             testTxID,
		Watcher:          stubWatcher{obs: bank.DepositObservation{ConfirmedAmount: dec("1"), Confirmations: 1}},
		MinConfirmations: 6,
	})
	if err != bank.ErrDepositNotConfirmed {
		t.Fatalf("expected ErrDepositNotConfirmed, got %v", err)
	}
}

func TestDepositRejectsWhenClaimedAmountExceedsRealOnChainAmount(t *testing.T) {
	// The exploit this closes: claiming a deposit far larger than what
	// actually arrived on-chain. ExternalAmount was previously a bare,
	// unverified caller-supplied number.
	_, err := bank.Deposit(bank.DepositParams{
		Asset: types.AssetBTC, ExternalAmount: dec("100"), PriceUSD: dec("60000"),
		ATRUSD: dec("2000"), SFGUSDPrice: dec("5"), Now: 1000,
		CustodyAddress:   testAddress,
		TxID:             testTxID,
		Watcher:          stubWatcher{obs: bank.DepositObservation{ConfirmedAmount: dec("1"), Confirmations: 6}},
		MinConfirmations: 6,
	})
	if err != bank.ErrDepositAmountMismatch {
		t.Fatalf("expected ErrDepositAmountMismatch, got %v", err)
	}
}

func TestDepositPropagatesWatcherFailure(t *testing.T) {
	_, err := bank.Deposit(bank.DepositParams{
		Asset: types.AssetBTC, ExternalAmount: dec("1"), PriceUSD: dec("60000"),
		ATRUSD: dec("2000"), SFGUSDPrice: dec("5"), Now: 1000,
		CustodyAddress: testAddress,
		TxID:           testTxID,
		Watcher:        stubWatcher{err: fmt.Errorf("network unreachable")},
	})
	if err == nil {
		t.Fatalf("expected a watcher failure to reject the deposit rather than silently accept it")
	}
}
