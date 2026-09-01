package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/txbuilder"
)

// classicalKeyFile is the on-disk shape of a real spec-8.5 dual-sign
// migration-aid keypair. Deliberately unencrypted at rest, unlike
// pkg/walletkey's passphrase-protected keystore — a real, disclosed
// trade-off: this key only ever adds a second signature alongside the
// wallet's own always-required Dilithium one (types.ShieldedTx.
// ClassicalSig's own doc), so its compromise alone can never let an
// attacker move funds or forge governance ballots on its own; it is not
// this wallet's real identity. A production deployment that wants this
// key encrypted at rest can layer that on separately.
type classicalKeyFile struct {
	PublicKey  string `json:"public_key"`  // hex, ed25519
	PrivateKey string `json:"private_key"` // hex, ed25519
}

// runClassicalKeygen generates a real ed25519 keypair for the dual-sign
// migration path and saves it to -out.
func runClassicalKeygen(args []string) error {
	fs := flag.NewFlagSet("classical-keygen", flag.ExitOnError)
	out := fs.String("out", "classical-key.json", "where to write the real ed25519 keypair")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite an existing key", *out)
	}
	pk, sk, err := crypto.GenerateClassicalKey()
	if err != nil {
		return fmt.Errorf("generate classical key: %w", err)
	}
	b, err := json.MarshalIndent(classicalKeyFile{
		PublicKey:  hex.EncodeToString(pk),
		PrivateKey: hex.EncodeToString(sk),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal classical key: %w", err)
	}
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	fmt.Printf("wrote %s — pass it to a signing command's -classical-key flag to dual-sign (real spec-8.5 migration aid, never a substitute for your real Dilithium identity)\n", *out)
	return nil
}

// loadClassicalKey reads a real ed25519 keypair saved by
// runClassicalKeygen.
func loadClassicalKey(path string) (crypto.ClassicalPublicKey, crypto.ClassicalPrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f classicalKeyFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	pk, err := hex.DecodeString(f.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: decode public key: %w", path, err)
	}
	sk, err := hex.DecodeString(f.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: decode private key: %w", path, err)
	}
	return crypto.ClassicalPublicKey(pk), crypto.ClassicalPrivateKey(sk), nil
}

// withOptionalClassicalKey attaches the real ed25519 keypair at path to
// b (Builder.WithClassicalKey) when path is non-empty, so every
// transaction b builds from here on also carries a real classical
// co-signature — a no-op, returning b unchanged, when path is empty
// (dual-sign stays exactly as optional at the CLI layer as it is in the
// pipeline itself).
func withOptionalClassicalKey(b *txbuilder.Builder, path string) (*txbuilder.Builder, error) {
	if path == "" {
		return b, nil
	}
	pk, sk, err := loadClassicalKey(path)
	if err != nil {
		return nil, err
	}
	return b.WithClassicalKey(pk, sk), nil
}
