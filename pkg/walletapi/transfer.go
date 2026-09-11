package walletapi

import (
	"context"
	"crypto/ecdh"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/txclient"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// connectTimeout bounds dialing the bootstrap peer — long enough for a
// real libp2p handshake on a slow network, short enough that an
// unreachable bootstrap address fails fast instead of hanging a UI.
const connectTimeout = 15 * time.Second

// TransferResult is what a real, submitted Transfer produced.
type TransferResult struct {
	TxID      string
	Confirmed bool
	Height    uint64
}

// loadZKSystem loads real, previously-generated Groth16 Transfer
// parameters from path. Never generates its own — an independent setup
// could never verify against a live network's own validators; see
// pkg/zk's own doc and cmd/wallet's identical loadZKSystem.
func loadZKSystem(path string) (*zk.System, error) {
	if path == "" {
		return nil, fmt.Errorf("walletapi: zkParamsPath is required — a wallet must prove against the exact same Groth16 parameters the network's validators verify against")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("walletapi: open zk params file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	sys, err := zk.ReadSystem(f)
	if err != nil {
		return nil, fmt.Errorf("walletapi: load zk params from %s: %w", path, err)
	}
	return sys, nil
}

// BuildAndSubmitTransfer syncs keystorePath's wallet against queryURL,
// builds a real, Groth16-proven Kind Transfer sending amount (plus fee)
// to the receiver's real X25519 shielded public key, and submits it over
// a freshly dialed libp2p connection to bootstrapAddr — the same real
// path 'wallet transfer' drives, minus the terminal prompt. Keys are
// unlocked in-process for the duration of this call only; nothing but
// the finished, signed, proven transaction ever leaves this function.
//
// listenAddr may be empty (defaults to an ephemeral local port).
// confirmTimeoutSeconds <= 0 submits without waiting for confirmation.
func BuildAndSubmitTransfer(keystorePath, passphrase, queryURL, listenAddr, bootstrapAddr, zkParamsPath, toX25519PubHex string, amount, fee uint64, confirmTimeoutSeconds int64) (TransferResult, error) {
	if bootstrapAddr == "" {
		return TransferResult{}, fmt.Errorf("walletapi: bootstrapAddr is required — submitting a transaction needs at least one connected peer to broadcast to")
	}
	if amount == 0 {
		return TransferResult{}, fmt.Errorf("walletapi: amount must be greater than 0")
	}
	toBytes, err := hex.DecodeString(toX25519PubHex)
	if err != nil {
		return TransferResult{}, fmt.Errorf("walletapi: toX25519PubHex: %w", err)
	}
	receiverPub, err := ecdh.X25519().NewPublicKey(toBytes)
	if err != nil {
		return TransferResult{}, fmt.Errorf("walletapi: toX25519PubHex: invalid X25519 public key: %w", err)
	}

	_, w, err := unlockShieldedWallet(keystorePath, passphrase, queryURL)
	if err != nil {
		return TransferResult{}, err
	}
	syncCtx, cancelSync := context.WithTimeout(context.Background(), defaultSyncTimeout)
	defer cancelSync()
	if err := w.Sync(syncCtx); err != nil {
		return TransferResult{}, fmt.Errorf("walletapi: sync: %w", err)
	}
	if w.KnownNoteCount() < zk.NumInputs {
		return TransferResult{}, fmt.Errorf("walletapi: this wallet knows %d spendable note(s); a transfer needs %d — a wallet must first receive real transfers before it can send one (see pkg/shieldedwallet's own doc)", w.KnownNoteCount(), zk.NumInputs)
	}

	sys, err := loadZKSystem(zkParamsPath)
	if err != nil {
		return TransferResult{}, err
	}
	txn, err := w.BuildTransfer(sys, receiverPub, amount, fee)
	if err != nil {
		return TransferResult{}, fmt.Errorf("walletapi: build transfer: %w", err)
	}

	if listenAddr == "" {
		listenAddr = "/ip4/0.0.0.0/tcp/0"
	}
	h, err := shadownet.NewHost(listenAddr)
	if err != nil {
		return TransferResult{}, fmt.Errorf("walletapi: create libp2p host: %w", err)
	}
	defer func() { _ = h.Close() }()

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), connectTimeout)
	defer cancelConnect()
	if err := shadownet.Connect(connectCtx, h, bootstrapAddr); err != nil {
		return TransferResult{}, fmt.Errorf("walletapi: connect to bootstrap %s: %w", bootstrapAddr, err)
	}

	node := shadownet.NewNode(h, nil, nil)
	client, err := txclient.New(txclient.Config{Net: node, QueryURLs: []string{queryURL}})
	if err != nil {
		return TransferResult{}, fmt.Errorf("walletapi: create tx client: %w", err)
	}

	if confirmTimeoutSeconds <= 0 {
		if err := client.Submit(context.Background(), txn); err != nil {
			return TransferResult{}, fmt.Errorf("walletapi: submit: %w", err)
		}
		return TransferResult{TxID: txn.TxID.String()}, nil
	}
	st, err := client.SubmitAndConfirm(context.Background(), txn, time.Duration(confirmTimeoutSeconds)*time.Second)
	if err != nil {
		return TransferResult{TxID: txn.TxID.String()}, err
	}
	result := TransferResult{TxID: txn.TxID.String(), Confirmed: st.State == txclient.StatusCommitted}
	if st.Height != nil {
		result.Height = *st.Height
	}
	return result, nil
}
