package main

import (
	"context"
	"crypto/ecdh"
	"encoding/hex"
	"flag"
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/shieldedwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// loadShieldedWallet unlocks path's full shielded identity (both real
// keypairs — Dilithium and X25519) and builds a real shieldedwallet.Wallet
// bound to queryBase, then Syncs it: a fresh replay of every committed
// Transfer since genesis, decrypting whatever memos this identity's real
// X25519 key can open. This is deliberately a full resync on every
// invocation rather than persisted local state — see shieldedwallet.
// Wallet's own doc on the fixed, tiny (16-leaf) circuit scope this build
// runs at, which keeps a from-genesis resync cheap even done every time.
func loadShieldedWallet(ctx context.Context, path string, fromStdin bool, queryBase string) (*shieldedwallet.Wallet, error) {
	if queryBase == "" {
		return nil, fmt.Errorf("-query is required")
	}
	ks, err := walletkey.Load(path)
	if err != nil {
		return nil, err
	}
	passphrase, err := readExistingPassphrase(fromStdin)
	if err != nil {
		return nil, err
	}
	id, err := ks.UnlockShielded(passphrase)
	if err != nil {
		return nil, err
	}
	w, err := shieldedwallet.New(id.PublicKey, id.PrivateKey, id.ShieldedPub, id.ShieldedKey, shieldedwallet.Config{QueryBase: queryBase})
	if err != nil {
		return nil, err
	}
	if err := w.Sync(ctx); err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	return w, nil
}

func runBalance(args []string) error {
	fs := flag.NewFlagSet("balance", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	queryURL := fs.String("query", "", "pkg/query base URL, e.g. http://127.0.0.1:8081")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	w, err := loadShieldedWallet(ctx, *path, *fromStdin, *queryURL)
	if err != nil {
		return err
	}
	fmt.Printf("balance: %d\n", w.Balance())
	fmt.Printf("known notes: %d\n", w.KnownNoteCount())
	root, err := w.CurrentRoot()
	if err == nil {
		rootBytes := zk.ToBytes32(root)
		fmt.Printf("synced canonical root: %s\n", hex.EncodeToString(rootBytes[:]))
	}
	return nil
}

func runTransfer(args []string) error {
	fs := flag.NewFlagSet("transfer", flag.ExitOnError)
	path := fs.String("keystore", "walletkey.json", "keystore to unlock")
	fromStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from stdin instead of prompting the terminal")
	toHex := fs.String("to", "", "receiver's shielded (X25519) public key, hex — see 'wallet identity'")
	amount := fs.Uint64("amount", 0, "amount to send")
	fee := fs.Uint64("fee", 0, "fee")
	zkParams := fs.String("zk-params", "", "path to the real, shared Groth16 params file every validator loads (see 'wallet zk-setup' and cmd/node's -zk-params) — required")
	var nf networkFlags
	nf.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *toHex == "" {
		return fmt.Errorf("-to is required")
	}
	if *amount == 0 {
		return fmt.Errorf("-amount must be greater than 0")
	}
	toBytes, err := hex.DecodeString(*toHex)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}
	receiverPub, err := ecdh.X25519().NewPublicKey(toBytes)
	if err != nil {
		return fmt.Errorf("-to: invalid X25519 public key: %w", err)
	}

	ctx := context.Background()
	queryURL, err := nf.firstQueryURL()
	if err != nil {
		return err
	}
	w, err := loadShieldedWallet(ctx, *path, *fromStdin, queryURL)
	if err != nil {
		return err
	}
	if w.KnownNoteCount() < zk.NumInputs {
		return fmt.Errorf("this wallet knows %d spendable note(s); a transfer needs %d — a real, disclosed limitation: this build has no on-chain mechanism to originate a wallet's first shielded note (see pkg/shieldedwallet's own doc), so a wallet must first receive real transfers from one that already holds spendable notes",
			w.KnownNoteCount(), zk.NumInputs)
	}

	sys, err := loadZKSystem(*zkParams)
	if err != nil {
		return err
	}

	txn, err := w.BuildTransfer(sys, receiverPub, *amount, *fee)
	if err != nil {
		return fmt.Errorf("build transfer: %w", err)
	}
	fmt.Println("built and proved a real Transfer — submitting")
	return submitTx(ctx, &nf, txn)
}
