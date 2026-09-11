package walletapi

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/shieldedwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// defaultSyncTimeout bounds how long Balance/BuildAndSubmitTransfer will
// wait for a real Sync (replaying every committed block since genesis)
// against a live node — generous enough for the four-node quickstart
// network, short enough that a wallet UI calling this from a button tap
// doesn't hang indefinitely against an unreachable node.
const defaultSyncTimeout = 30 * time.Second

// queryTimeout bounds a single, one-shot read-only query call.
const queryTimeout = 10 * time.Second

// BalanceResult is a synced wallet's real, current spendable state — the
// same values 'wallet balance' prints, structured for a caller.
type BalanceResult struct {
	Balance    uint64
	KnownNotes int
	RootHex    string
	Identity   IdentitySummary
}

// unlockShieldedWallet unlocks keystorePath's full shielded identity and
// wraps it in a real shieldedwallet.Wallet bound to queryURL — the same
// real unlock-then-wrap sequence cmd/wallet's own loadShieldedWallet
// uses, without the terminal-prompting.
func unlockShieldedWallet(keystorePath, passphrase, queryURL string) (*walletkey.Keystore, *shieldedwallet.Wallet, error) {
	if queryURL == "" {
		return nil, nil, fmt.Errorf("walletapi: queryURL is required")
	}
	ks, err := walletkey.Load(keystorePath)
	if err != nil {
		return nil, nil, err
	}
	id, err := ks.UnlockShielded(passphrase)
	if err != nil {
		return nil, nil, err
	}
	w, err := shieldedwallet.New(id.PublicKey, id.PrivateKey, id.ShieldedPub, id.ShieldedKey, shieldedwallet.Config{QueryBase: queryURL})
	if err != nil {
		return nil, nil, err
	}
	return ks, w, nil
}

// Balance unlocks keystorePath, syncs it against a real running node at
// queryURL — replaying every committed Transfer since genesis, exactly
// as pkg/shieldedwallet.Wallet.Sync always does, never a cached or
// estimated figure — and reports the real, current spendable total.
func Balance(keystorePath, passphrase, queryURL string) (BalanceResult, error) {
	ks, w, err := unlockShieldedWallet(keystorePath, passphrase, queryURL)
	if err != nil {
		return BalanceResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultSyncTimeout)
	defer cancel()
	if err := w.Sync(ctx); err != nil {
		return BalanceResult{}, fmt.Errorf("walletapi: sync: %w", err)
	}
	result := BalanceResult{
		Balance:    w.Balance(),
		KnownNotes: w.KnownNoteCount(),
		Identity:   summarize(ks),
	}
	if root, err := w.CurrentRoot(); err == nil {
		rootBytes := zk.ToBytes32(root)
		result.RootHex = hex.EncodeToString(rootBytes[:])
	}
	return result, nil
}
