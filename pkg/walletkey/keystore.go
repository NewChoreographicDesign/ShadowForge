// Package walletkey is a real, passphrase-protected local keystore for an
// end-user identity — Tier B priority #2 from the Horizon/Ignition
// roadmap. pkg/crypto already generates real Dilithium keypairs; what was
// missing is anywhere to keep one at rest without either leaving the
// private key in plaintext on disk or reusing cmd/node's own -key-file
// mechanism, which is deliberately unencrypted for a different reason: a
// validator process needs to restart unattended (e.g. under systemd)
// without a human present to type a passphrase. A wallet identity has the
// opposite threat model — a human is present, the key sits on a personal
// machine, and it should never be usable by anyone who doesn't know the
// passphrase — so this is a separate, real mechanism, not a shortcut on
// top of the validator one.
//
// A keystore file holds the real Dilithium public key in the clear (it
// isn't secret) and the real private key sealed under a passphrase-derived
// key via Argon2id (kdf.go) and ChaCha20-Poly1305 AEAD (pkg/crypto, the
// same primitive this codebase already uses for encrypted note storage).
// The public key is bound into the AEAD's associated data, so a keystore
// file tampered with in either its declared public key or its ciphertext
// fails to decrypt rather than silently returning a mismatched pair.
package walletkey

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// keystoreVersion is bumped if this file's on-disk shape ever changes in
// a way older code can't read.
const keystoreVersion = 1

const kdfArgon2id = "argon2id"

// selfTestMessage is signed and verified once after every successful
// decrypt, as a real, cheap correctness proof that the returned key pair
// actually works for genuine Dilithium operations — not just that the
// AEAD tag happened to check out.
var selfTestMessage = []byte("shadowforge-walletkey-self-test-v1")

// keystoreFile is the on-disk JSON shape. Every field name is stable API
// once written — a real person's saved file must keep opening after this
// package changes.
type keystoreFile struct {
	Version    int          `json:"version"`
	PublicKey  string       `json:"public_key"` // hex
	KDF        string       `json:"kdf"`
	KDFParams  argon2Params `json:"kdf_params"`
	Salt       string       `json:"salt"`       // hex
	Ciphertext string       `json:"ciphertext"` // hex; crypto.Encrypt's nonce||ciphertext||tag
}

// Keystore is one loaded (or freshly generated, unsaved) keystore. The
// private key never lives on this struct — only Unlock ever decrypts it,
// and only for as long as the caller holds the returned slice.
type Keystore struct {
	file keystoreFile
}

// Generate creates a fresh, real Dilithium keypair and seals it under
// passphrase, ready to Save. passphrase must be non-empty — an empty
// passphrase would mean an unencrypted keystore wearing the appearance of
// an encrypted one, which is worse than refusing outright.
func Generate(passphrase string) (*Keystore, error) {
	if passphrase == "" {
		return nil, errors.New("walletkey: passphrase must not be empty")
	}
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		return nil, fmt.Errorf("walletkey: generate identity: %w", err)
	}
	return seal(pk, sk, passphrase)
}

// seal encrypts sk under passphrase with a fresh salt, producing a
// Keystore ready to Save. Shared by Generate and ChangePassphrase.
func seal(pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, passphrase string) (*Keystore, error) {
	params := defaultArgon2Params
	salt, err := newSalt(params.SaltLength)
	if err != nil {
		return nil, err
	}
	key := deriveKey(passphrase, salt, params)
	defer zero(key[:])

	ciphertext, err := crypto.Encrypt(key, []byte(sk), pk)
	if err != nil {
		return nil, fmt.Errorf("walletkey: seal private key: %w", err)
	}

	return &Keystore{file: keystoreFile{
		Version:    keystoreVersion,
		PublicKey:  hex.EncodeToString(pk),
		KDF:        kdfArgon2id,
		KDFParams:  params,
		Salt:       hex.EncodeToString(salt),
		Ciphertext: hex.EncodeToString(ciphertext),
	}}, nil
}

// Load reads a keystore file from disk. The private key stays encrypted
// — Load never needs, and never asks for, a passphrase.
func Load(path string) (*Keystore, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("walletkey: read %s: %w", path, err)
	}
	var f keystoreFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("walletkey: parse %s: %w", path, err)
	}
	if f.Version != keystoreVersion {
		return nil, fmt.Errorf("walletkey: %s has unsupported version %d (expected %d)", path, f.Version, keystoreVersion)
	}
	if f.KDF != kdfArgon2id {
		return nil, fmt.Errorf("walletkey: %s uses unsupported kdf %q", path, f.KDF)
	}
	if _, err := hex.DecodeString(f.PublicKey); err != nil {
		return nil, fmt.Errorf("walletkey: %s has a malformed public key: %w", path, err)
	}
	return &Keystore{file: f}, nil
}

// Save writes the keystore to path, atomically (write-to-temp then
// rename, so a crash or power loss mid-write can never leave a
// half-written file at path) and with owner-only permissions — the same
// pattern and mode cmd/node's own identity file already uses.
func (k *Keystore) Save(path string) error {
	b, err := json.MarshalIndent(k.file, "", "  ")
	if err != nil {
		return fmt.Errorf("walletkey: marshal keystore: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("walletkey: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("walletkey: rename %s to %s: %w", tmp, path, err)
	}
	return nil
}

// PublicKey returns this keystore's real Dilithium public key. Always
// available without a passphrase — it was never secret.
func (k *Keystore) PublicKey() crypto.DilithiumPublicKey {
	b, _ := hex.DecodeString(k.file.PublicKey) // validated at Load/Generate time
	return crypto.DilithiumPublicKey(b)
}

// Identity is this keystore's consensus-style identity: the same
// NFTID(SumHash(publicKey)) derivation pkg/validator and cmd/walletsim
// already use, so a wallet identity and a validator identity are
// addressed the same way throughout the system.
func (k *Keystore) Identity() types.NFTID {
	return types.NFTID(types.SumHash(k.PublicKey()))
}

// Unlock derives the passphrase-based key, decrypts the private key, and
// proves the resulting pair is genuinely usable by signing and verifying
// a fixed self-test message with it before returning — a wrong
// passphrase fails at Decrypt's AEAD authentication (a clear, specific
// error), and any other internal inconsistency fails the self-test
// rather than silently handing back unusable key material.
func (k *Keystore) Unlock(passphrase string) (crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey, error) {
	if passphrase == "" {
		return nil, nil, errors.New("walletkey: passphrase must not be empty")
	}
	salt, err := hex.DecodeString(k.file.Salt)
	if err != nil {
		return nil, nil, fmt.Errorf("walletkey: malformed salt: %w", err)
	}
	ciphertext, err := hex.DecodeString(k.file.Ciphertext)
	if err != nil {
		return nil, nil, fmt.Errorf("walletkey: malformed ciphertext: %w", err)
	}
	pk := k.PublicKey()

	key := deriveKey(passphrase, salt, k.file.KDFParams)
	defer zero(key[:])

	plain, err := crypto.Decrypt(key, ciphertext, pk)
	if err != nil {
		return nil, nil, errors.New("walletkey: wrong passphrase or corrupted keystore")
	}
	sk := crypto.DilithiumPrivateKey(plain)

	sig, err := crypto.DilithiumSign(sk, selfTestMessage)
	if err != nil {
		zero(plain)
		return nil, nil, fmt.Errorf("walletkey: decrypted key failed self-test signing: %w", err)
	}
	ok, err := crypto.DilithiumVerify(pk, selfTestMessage, sig)
	if err != nil || !ok {
		zero(plain)
		return nil, nil, errors.New("walletkey: decrypted key failed self-test verification — keystore is inconsistent")
	}

	return pk, sk, nil
}

// ChangePassphrase re-encrypts the private key under newPassphrase with a
// freshly generated salt (never reusing the old one), after confirming
// oldPassphrase genuinely unlocks the current ciphertext. The receiver is
// only mutated once the new ciphertext is fully computed — a failure
// partway through never leaves the in-memory keystore (or, since callers
// are expected to Save only after this returns, the on-disk file) in a
// half-changed state.
func (k *Keystore) ChangePassphrase(oldPassphrase, newPassphrase string) error {
	pk, sk, err := k.Unlock(oldPassphrase)
	if err != nil {
		return err
	}
	defer zero(sk)

	resealed, err := seal(pk, sk, newPassphrase)
	if err != nil {
		return err
	}
	k.file = resealed.file
	return nil
}
