package walletapi

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
)

// IdentitySummary is a keystore's public identity — every field here is
// safe to show in a UI without ever asking for a passphrase, since none
// of it is secret.
type IdentitySummary struct {
	IdentityHex          string
	AddressHex           string
	PublicKeyHex         string
	ShieldedPublicKeyHex string
}

func summarize(ks *walletkey.Keystore) IdentitySummary {
	addr := types.AddressFromPubkey(ks.PublicKey())
	return IdentitySummary{
		IdentityHex:          ks.Identity().String(),
		AddressHex:           addr.String(),
		PublicKeyHex:         hex.EncodeToString(ks.PublicKey()),
		ShieldedPublicKeyHex: hex.EncodeToString(ks.ShieldedPublicKey().Bytes()),
	}
}

// CreateIdentity generates a fresh, real Dilithium + X25519 keystore
// (pkg/walletkey.Generate) sealed under passphrase and saves it to
// keystorePath — the same real on-disk format 'wallet identity' already
// produces. Refuses to overwrite an existing file, mirroring every
// *-zk-setup subcommand's own refusal to silently clobber existing key
// material.
//
// keystorePath is deliberately a filesystem path, not an in-memory blob:
// on-device, this is a path inside the app's own sandboxed storage, with
// the *passphrase* (not the keystore bytes) held behind iOS Keychain /
// Android Keystore biometric gating — see the Wallet Blueprint's §2.3.
func CreateIdentity(keystorePath, passphrase string) (IdentitySummary, error) {
	if _, err := os.Stat(keystorePath); err == nil {
		return IdentitySummary{}, fmt.Errorf("walletapi: %s already exists — refusing to overwrite an existing keystore", keystorePath)
	}
	ks, err := walletkey.Generate(passphrase)
	if err != nil {
		return IdentitySummary{}, fmt.Errorf("walletapi: generate identity: %w", err)
	}
	if err := ks.Save(keystorePath); err != nil {
		return IdentitySummary{}, fmt.Errorf("walletapi: save keystore: %w", err)
	}
	return summarize(ks), nil
}

// LoadIdentitySummary reads keystorePath's public identity — no
// passphrase needed, since nothing here is secret (mirrors 'wallet
// identity').
func LoadIdentitySummary(keystorePath string) (IdentitySummary, error) {
	ks, err := walletkey.Load(keystorePath)
	if err != nil {
		return IdentitySummary{}, err
	}
	return summarize(ks), nil
}
