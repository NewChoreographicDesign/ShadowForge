package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/txclient"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// networkFlags are the flags every transaction-submitting subcommand
// shares: how to reach a real libp2p peer to broadcast to, and how to
// confirm the result against one or more real pkg/query endpoints.
type networkFlags struct {
	listen         string
	bootstrap      string
	bootstrapFile  string
	query          string
	confirmTimeout time.Duration
}

func (f *networkFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.listen, "listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen multiaddr")
	fs.StringVar(&f.bootstrap, "bootstrap", "", "bootstrap peer multiaddr (must include /p2p/<peerID>)")
	fs.StringVar(&f.bootstrapFile, "bootstrap-file", "", "read a bootstrap multiaddr from this path, waiting for it to appear (alternative to -bootstrap)")
	fs.StringVar(&f.query, "query", "", "comma-separated pkg/query base URL(s) (e.g. http://127.0.0.1:8081) used to confirm the submission")
	fs.DurationVar(&f.confirmTimeout, "confirm-timeout", 30*time.Second, "how long to wait for the transaction to be confirmed committed; 0 submits without waiting")
}

func (f *networkFlags) queryURLs() []string {
	var out []string
	for _, p := range strings.Split(f.query, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// firstQueryURL is the single query endpoint a read-only lookup needs —
// networkFlags.query is comma-separated for txclient's multi-endpoint
// confirmation, but a plain read only ever talks to one node at a time.
func (f *networkFlags) firstQueryURL() (string, error) {
	urls := f.queryURLs()
	if len(urls) == 0 {
		return "", fmt.Errorf("-query is required")
	}
	return urls[0], nil
}

// submitTx connects a fresh libp2p host to the configured bootstrap
// peer, broadcasts txn as a real TxOffer, and — unless -confirm-timeout
// is 0 — waits for it to be confirmed committed via the configured query
// endpoint(s). This is the one real submission path every transaction-
// producing subcommand in this binary shares.
func submitTx(ctx context.Context, f *networkFlags, txn types.ShieldedTx) error {
	if f.bootstrap == "" && f.bootstrapFile == "" {
		return fmt.Errorf("-bootstrap or -bootstrap-file is required: submitting a transaction needs at least one connected peer to broadcast to")
	}
	if f.confirmTimeout > 0 && len(f.queryURLs()) == 0 {
		return fmt.Errorf("-confirm-timeout > 0 needs at least one -query endpoint to confirm against (pass -confirm-timeout 0 to submit without waiting)")
	}

	h, err := shadownet.NewHost(f.listen)
	if err != nil {
		return fmt.Errorf("create libp2p host: %w", err)
	}
	defer func() { _ = h.Close() }()

	bootstrapAddr := f.bootstrap
	if bootstrapAddr == "" {
		addr, err := waitForAddrFile(ctx, f.bootstrapFile)
		if err != nil {
			return fmt.Errorf("waiting for bootstrap file %s: %w", f.bootstrapFile, err)
		}
		bootstrapAddr = addr
	}
	if err := shadownet.Connect(ctx, h, bootstrapAddr); err != nil {
		return fmt.Errorf("connect to bootstrap %s: %w", bootstrapAddr, err)
	}

	node := shadownet.NewNode(h, nil, nil)
	client, err := txclient.New(txclient.Config{Net: node, QueryURLs: f.queryURLs()})
	if err != nil {
		return fmt.Errorf("create tx client: %w", err)
	}

	fmt.Printf("txid: %s\n", txn.TxID)
	if f.confirmTimeout <= 0 {
		if err := client.Submit(ctx, txn); err != nil {
			return fmt.Errorf("submit: %w", err)
		}
		fmt.Println("submitted (not waiting for confirmation — pass -confirm-timeout > 0 to wait)")
		return nil
	}

	st, err := client.SubmitAndConfirm(ctx, txn, f.confirmTimeout)
	if err != nil {
		return err
	}
	height := "?"
	if st.Height != nil {
		height = fmt.Sprintf("%d", *st.Height)
	}
	fmt.Printf("confirmed: status=%s height=%s\n", st.State, height)
	return nil
}

// waitForAddrFile polls for path to appear, mirroring cmd/walletsim's
// helper of the same purpose (kept duplicated rather than shared — see
// passphrase.go's own note on this codebase's per-binary convention).
func waitForAddrFile(ctx context.Context, path string) (string, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b)), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out waiting for %s", path)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}
