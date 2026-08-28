// Package txclient is Tier B priority #4: a real submit-and-confirm loop
// wiring pkg/query (priority #1, "did my transaction land") to real
// transaction submission over the actual libp2p wire protocol (the same
// TxOffer broadcast cmd/walletsim, pkg/walletkey's live-network proof,
// and pkg/txbuilder's live-network proof all already use). Before this
// package, submitting a transaction and confirming it required hand-
// wiring those two pieces yourself, as every prior verification script in
// this codebase's history did — this is that wiring, done once, tested,
// and reusable.
//
// Client works with any real types.ShieldedTx, not only ones pkg/
// txbuilder produced — it has no dependency on that package.
package txclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// DefaultPollInterval is how often Confirm re-checks status while
// waiting. Comfortably under pkg/query's default rate limit (10/s
// sustained, burst 20) even against a single query endpoint, and far
// enough under it that querying several endpoints per poll (multi-
// endpoint Confirm) still never approaches the limit.
const DefaultPollInterval = 750 * time.Millisecond

// DefaultHTTPTimeout bounds a single query request — long enough for a
// slow node, short enough that one unreachable endpoint can't stall an
// entire poll cycle when other endpoints are configured.
const DefaultHTTPTimeout = 5 * time.Second

// Client submits real transactions over an already-connected libp2p Node
// and confirms them against one or more pkg/query API endpoints.
type Client struct {
	net          *shadownet.Node
	queryURLs    []string
	http         *http.Client
	pollInterval time.Duration
}

// Config configures a Client.
type Config struct {
	// Net is an already-constructed, already-connected libp2p node — this
	// package never creates or manages host/connection lifecycle itself,
	// the same dependency-injection pattern pkg/query and pkg/tx.Pipeline
	// already use. Required.
	Net *shadownet.Node
	// QueryURLs are one or more pkg/query base URLs (e.g.
	// "http://127.0.0.1:8081", no trailing slash needed). Required for
	// QueryStatus/Confirm/SubmitAndConfirm; Submit alone doesn't need it.
	// Configuring more than one gives Confirm a real safety property: if
	// two configured endpoints ever disagree about a transaction's
	// committed height, that's treated as a serious anomaly and returned
	// as an explicit error rather than silently trusting whichever
	// answered first.
	QueryURLs []string
	// PollInterval overrides DefaultPollInterval.
	PollInterval time.Duration
	// HTTPClient overrides the default HTTP client (DefaultHTTPTimeout).
	HTTPClient *http.Client
}

// New builds a Client. Returns an error if Net is nil — every method
// needs it, and failing fast here beats a confusing nil-pointer panic
// deep inside Submit.
func New(cfg Config) (*Client, error) {
	if cfg.Net == nil {
		return nil, fmt.Errorf("txclient: Config.Net must not be nil")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	// Copy QueryURLs so a caller mutating their own slice afterward can't
	// reach into this Client's configuration.
	urls := append([]string(nil), cfg.QueryURLs...)
	return &Client{net: cfg.Net, queryURLs: urls, http: httpClient, pollInterval: pollInterval}, nil
}

// Submit broadcasts txn as a real TxOffer to every currently connected
// peer. It fails loudly rather than reporting a silent success in the two
// ways that would otherwise look identical to a caller: zero peers
// connected (nothing to broadcast to at all — Node.Broadcast's own return
// value can't distinguish "sent to nobody" from "sent to everybody",
// since it only ever reports failures) and every connected peer's send
// failing.
func (c *Client) Submit(ctx context.Context, txn types.ShieldedTx) error {
	if len(c.net.Host.Network().Peers()) == 0 {
		return fmt.Errorf("txclient: no connected peers — nothing to submit to")
	}

	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: txn})
	if err != nil {
		return fmt.Errorf("txclient: build envelope: %w", err)
	}

	errs := c.net.Broadcast(ctx, env)
	// Broadcast only ever records failures (a peer's absence from errs
	// means its send succeeded), so re-checking the peer count right
	// after it returns and finding every one of them present in errs
	// means nobody succeeded — report one representative error rather
	// than silently returning nil while nothing actually got through.
	attempted := len(c.net.Host.Network().Peers())
	if len(errs) > 0 && len(errs) >= attempted {
		for _, e := range errs {
			return fmt.Errorf("txclient: send failed to all %d connected peer(s), e.g.: %w", attempted, e)
		}
	}
	return nil
}
