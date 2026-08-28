package txclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// ErrConfirmTimeout is wrapped into Confirm's returned error once timeout
// elapses without ever observing StatusCommitted — check for it with
// errors.Is to distinguish "never confirmed in time" from a harder
// failure (every query endpoint unreachable, a malformed response).
var ErrConfirmTimeout = errors.New("txclient: transaction not confirmed within timeout")

// The three states pkg/query's /v1/tx/{txid} endpoint reports — mirrored
// here as constants rather than re-declared string literals at every call
// site.
const (
	StatusCommitted = "committed"
	StatusPending   = "pending"
	StatusUnknown   = "unknown"
)

// Status is one transaction's confirmation state, aggregated across
// however many query endpoints this Client is configured with.
type Status struct {
	State  string
	Height *uint64
}

type txStatusResponse struct {
	Status string  `json:"status"`
	Height *uint64 `json:"height"`
}

// QueryStatus asks every configured query endpoint for txn's status and
// aggregates the answers. An endpoint that fails to respond at all is
// skipped (its failure doesn't sink the whole call as long as at least
// one endpoint answers) — but if two or more endpoints that DID answer
// disagree about a committed height for the same transaction, that's
// treated as a real anomaly, not resolved by silently trusting whichever
// happened to be checked first: it's returned as an explicit error, since
// it means either a bug, a fork, or a lying node, and a caller should
// decide how to handle that rather than have it hidden from them.
func (c *Client) QueryStatus(ctx context.Context, txid types.Hash) (Status, error) {
	if len(c.queryURLs) == 0 {
		return Status{}, fmt.Errorf("txclient: no query endpoints configured")
	}

	var results []Status
	var lastErr error
	for _, base := range c.queryURLs {
		st, err := c.queryOne(ctx, base, txid)
		if err != nil {
			lastErr = err
			continue
		}
		results = append(results, st)
	}
	if len(results) == 0 {
		return Status{}, fmt.Errorf("txclient: all %d query endpoint(s) failed to respond, last error: %w", len(c.queryURLs), lastErr)
	}
	return aggregate(results)
}

func (c *Client) queryOne(ctx context.Context, base string, txid types.Hash) (Status, error) {
	url := base + "/v1/tx/" + txid.String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Status{}, fmt.Errorf("txclient: build request for %s: %w", base, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("txclient: query %s: %w", base, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Status{}, fmt.Errorf("txclient: %s returned HTTP %d: %s", base, resp.StatusCode, body)
	}
	var parsed txStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Status{}, fmt.Errorf("txclient: decode response from %s: %w", base, err)
	}
	return Status{State: parsed.Status, Height: parsed.Height}, nil
}

// aggregate combines multiple endpoints' answers for the same
// transaction: committed beats pending beats unknown, and two differing
// committed heights is a hard error rather than a silent pick.
func aggregate(results []Status) (Status, error) {
	best := Status{State: StatusUnknown}
	var committedHeight *uint64
	for _, r := range results {
		switch r.State {
		case StatusCommitted:
			if committedHeight != nil && r.Height != nil && *committedHeight != *r.Height {
				return Status{}, fmt.Errorf("txclient: query endpoints disagree on committed height for the same transaction (%d vs %d)", *committedHeight, *r.Height)
			}
			if r.Height != nil {
				committedHeight = r.Height
			}
			best = r
		case StatusPending:
			if best.State == StatusUnknown {
				best = r
			}
		}
	}
	return best, nil
}

// Confirm polls QueryStatus on Client's configured interval until txid
// reaches StatusCommitted, ctx is cancelled, or timeout elapses —
// whichever comes first. A timeout is reported as a distinct, named
// error (ErrConfirmTimeout, via errors.Is) rather than folded into a
// generic failure, so a caller can tell "never got a definitive answer in
// time" apart from "a query endpoint is actually broken".
//
// A real, disclosed limitation: pkg/query's tri-state is a node-local
// view (see its own doc), and nothing in this wire protocol carries a
// rejection reason back to a submitter — a transaction that fails Stage 2
// -5 of the pipeline simply never appears anywhere, indistinguishable
// from one that's merely still propagating. Confirm can prove a
// transaction landed; it cannot prove why one never does, only that it
// didn't within the time given.
func (c *Client) Confirm(ctx context.Context, txid types.Hash, timeout time.Duration) (Status, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	// Check immediately rather than waiting a full interval first — a
	// transaction that already committed by the time Confirm is called
	// (e.g. Submit followed by a slow caller) shouldn't pay for a poll
	// tick it doesn't need.
	for {
		st, err := c.QueryStatus(ctx, txid)
		if err == nil && st.State == StatusCommitted {
			return st, nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return Status{}, fmt.Errorf("%w: last query error: %v", ErrConfirmTimeout, err)
			}
			return st, fmt.Errorf("%w: last observed status %q", ErrConfirmTimeout, st.State)
		}
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// SubmitAndConfirm is the real submit-and-confirm loop this package
// exists for: Submit, then Confirm, end to end.
func (c *Client) SubmitAndConfirm(ctx context.Context, txn types.ShieldedTx, timeout time.Duration) (Status, error) {
	if err := c.Submit(ctx, txn); err != nil {
		return Status{}, fmt.Errorf("txclient: submit: %w", err)
	}
	txid := txn.TxID
	st, err := c.Confirm(ctx, txid, timeout)
	if err != nil {
		return st, fmt.Errorf("txclient: confirm: %w", err)
	}
	return st, nil
}
