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
// A keystore holds two real keypairs: a Dilithium signing pair (spec 8.5
// — every transaction, vote, and block signs with this) and an X25519
// key-agreement pair (Tier B priority #5's shielded-transfer note
// delivery — see pkg/shieldedwallet's doc for why a signature-only key
// can't do that job and this second key exists). Both public keys are
// stored in the clear (neither is secret); both private keys are sealed
// together under one passphrase-derived key via Argon2id (kdf.go) and
// ChaCha20-Poly1305 AEAD (pkg/crypto, the same primitive this codebase
// already uses for encrypted note storage), with both public keys bound
// into the AEAD's associated data — a keystore file tampered with in
// either declared public key or the ciphertext fails to decrypt rather
// than silently returning a mismatched pair.
package walletkey

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// keystoreVersion is bumped if this file's on-disk shape ever changes in
// a way older code can't read. Bumped to 2 when the X25519 shielded key
// was added — a v1 file (Dilithium only) is not silently upgraded; Load
// rejects it with a clear, specific error rather than guessing.
const keystoreVersion = 2

const kdfArgon2id = "argon2id"

// selfTestMessage is signed and verified once after every successful
// decrypt, as a real, cheap correctness proof that the returned key pair
// actually works for genuine Dilithium operations — not just that the
// AEAD tag happened to check out.
var selfTestMessage = []byte("shadowforge-walletkey-self-test-v1")

// x25519Curve is the one key-agreement curve this package uses.
func x25519Curve() ecdh.Curve { return ecdh.X25519() }

// keystoreFile is the on-disk JSON shape. Every field name is stable API
// once written — a real person's saved file must keep opening after this
// package changes.
type keystoreFile struct {
	Version         int          `json:"version"`
	PublicKey       string       `json:"public_key"`        // hex, Dilithium
	X25519PublicKey string       `json:"x25519_public_key"` // hex
	KDF             string       `json:"kdf"`
	KDFParams       argon2Params `json:"kdf_params"`
	Salt            string       `json:"salt"`       // hex
	Ciphertext      string       `json:"ciphertext"` // hex; crypto.Encrypt's nonce||ciphertext||tag
}

// sealedSecrets is the plaintext payload encrypted into keystoreFile.
// Ciphertext — both private keys together, so a single passphrase check
// unlocks both, and ChangePassphrase re-seals both atomically as one
// operation.
type sealedSecrets struct {
	DilithiumSK []byte `json:"dilithium_sk"`
	X25519SK    []byte `json:"x25519_sk"`
}

// Keystore is one loaded (or freshly generated, unsaved) keystore. No
// private key material lives on this struct — only Unlock/UnlockShielded
// ever decrypt it, and only for as long as the caller holds the returned
// values.
type Keystore struct {
	file keystoreFile
}

// ShieldedIdentity holds both real keypairs an unlocked wallet identity
// has: Dilithium for signing (every transaction, vote, and block in this
// system) and X25519 for shielded note receipt (pkg/shieldedwallet's
// ECIES-style memo encryption/decryption).
type ShieldedIdentity struct {
	PublicKey   crypto.DilithiumPublicKey
	PrivateKey  crypto.DilithiumPrivateKey
	ShieldedPub *ecdh.PublicKey
	ShieldedKey *ecdh.PrivateKey
}

// Generate creates a fresh, real Dilithium keypair and a fresh, real
// X25519 keypair, and seals both under passphrase, ready to Save.
// passphrase must be non-empty — an empty passphrase would mean an
// unencrypted keystore wearing the appearance of an encrypted one, which
// is worse than refusing outright.
func Generate(passphrase string) (*Keystore, error) {
	if passphrase == "" {
		return nil, errors.New("walletkey: passphrase must not be empty")
	}
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		return nil, fmt.Errorf("walletkey: generate identity: %w", err)
	}
	xsk, err := x25519Curve().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("walletkey: generate shielded key: %w", err)
	}
	return seal(pk, sk, xsk, passphrase)
}

// seal encrypts sk/xsk together under passphrase with a fresh salt,
// producing a Keystore ready to Save. Shared by Generate and
// ChangePassphrase.
func seal(pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, xsk *ecdh.PrivateKey, passphrase string) (*Keystore, error) {
	params := defaultArgon2Params
	salt, err := newSalt(params.SaltLength)
	if err != nil {
		return nil, err
	}
	key := deriveKey(passphrase, salt, params)
	defer zero(key[:])

	plain, err := json.Marshal(sealedSecrets{DilithiumSK: sk, X25519SK: xsk.Bytes()})
	if err != nil {
		return nil, fmt.Errorf("walletkey: marshal secrets: %w", err)
	}
	defer zero(plain)

	xpk := xsk.PublicKey()
	aad := append(append([]byte{}, pk...), xpk.Bytes()...)
	ciphertext, err := crypto.Encrypt(key, plain, aad)
	if err != nil {
		return nil, fmt.Errorf("walletkey: seal secrets: %w", err)
	}

	return &Keystore{file: keystoreFile{
		Version:         keystoreVersion,
		PublicKey:       hex.EncodeToString(pk),
		X25519PublicKey: hex.EncodeToString(xpk.Bytes()),
		KDF:             kdfArgon2id,
		KDFParams:       params,
		Salt:            hex.EncodeToString(salt),
		Ciphertext:      hex.EncodeToString(ciphertext),
	}}, nil
}

// Load reads a keystore file from disk. Private key material stays
// encrypted — Load never needs, and never asks for, a passphrase.
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
	xpkBytes, err := hex.DecodeString(f.X25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("walletkey: %s has a malformed shielded public key: %w", path, err)
	}
	if _, err := x25519Curve().NewPublicKey(xpkBytes); err != nil {
		return nil, fmt.Errorf("walletkey: %s has an invalid shielded public key: %w", path, err)
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

// ShieldedPublicKey returns this keystore's real X25519 public key — what
// someone else needs to encrypt a shielded note to this wallet (pkg/
// shieldedwallet). Always available without a passphrase; it was never
// secret.
func (k *Keystore) ShieldedPublicKey() *ecdh.PublicKey {
	b, _ := hex.DecodeString(k.file.X25519PublicKey) // validated at Load/Generate time
	pub, _ := x25519Curve().NewPublicKey(b)          // validated at Load/Generate time
	return pub
}

// Identity is this keystore's consensus-style identity: the same
// NFTID(SumHash(publicKey)) derivation pkg/validator and cmd/walletsim
// already use, so a wallet identity and a validator identity are
// addressed the same way throughout the system.
func (k *Keystore) Identity() types.NFTID {
	return types.NFTID(types.SumHash(k.PublicKey()))
}

// unlockSecrets is the shared decrypt path Unlock and UnlockShielded both
// build on: derive the passphrase key, decrypt, and unmarshal — the one
// place that touches the AEAD and KDF, so the two public methods can't
// drift into checking the passphrase two different ways.
func (k *Keystore) unlockSecrets(passphrase string) (sealedSecrets, error) {
	if passphrase == "" {
		return sealedSecrets{}, errors.New("walletkey: passphrase must not be empty")
	}
	salt, err := hex.DecodeString(k.file.Salt)
	if err != nil {
		return sealedSecrets{}, fmt.Errorf("walletkey: malformed salt: %w", err)
	}
	ciphertext, err := hex.DecodeString(k.file.Ciphertext)
	if err != nil {
		return sealedSecrets{}, fmt.Errorf("walletkey: malformed ciphertext: %w", err)
	}
	xpkBytes, err := hex.DecodeString(k.file.X25519PublicKey)
	if err != nil {
		return sealedSecrets{}, fmt.Errorf("walletkey: malformed shielded public key: %w", err)
	}
	aad := append(append([]byte{}, k.PublicKey()...), xpkBytes...)

	key := deriveKey(passphrase, salt, k.file.KDFParams)
	defer zero(key[:])

	plain, err := crypto.Decrypt(key, ciphertext, aad)
	if err != nil {
		return sealedSecrets{}, errors.New("walletkey: wrong passphrase or corrupted keystore")
	}
	defer zero(plain)

	var secrets sealedSecrets
	if err := json.Unmarshal(plain, &secrets); err != nil {
		return sealedSecrets{}, fmt.Errorf("walletkey: decrypted keystore is corrupt: %w", err)
	}
	return secrets, nil
}

// Unlock derives the passphrase-based key, decrypts the signing private
// key, and proves the resulting pair is genuinely usable by signing and
// verifying a fixed self-test message with it before returning — a wrong
// passphrase fails at Decrypt's AEAD authentication (a clear, specific
// error), and any other internal inconsistency fails the self-test
// rather than silently handing back unusable key material.
func (k *Keystore) Unlock(passphrase string) (crypto.DilithiumPublicKey, crypto.DilithiumPrivateKey, error) {
	secrets, err := k.unlockSecrets(passphrase)
	if err != nil {
		return nil, nil, err
	}
	pk := k.PublicKey()
	sk := crypto.DilithiumPrivateKey(secrets.DilithiumSK)

	sig, err := crypto.DilithiumSign(sk, selfTestMessage)
	if err != nil {
		zero(sk)
		return nil, nil, fmt.Errorf("walletkey: decrypted key failed self-test signing: %w", err)
	}
	ok, err := crypto.DilithiumVerify(pk, selfTestMessage, sig)
	if err != nil || !ok {
		zero(sk)
		return nil, nil, errors.New("walletkey: decrypted key failed self-test verification — keystore is inconsistent")
	}
	return pk, sk, nil
}

// UnlockShielded is Unlock's counterpart returning both real keypairs —
// Dilithium (self-tested exactly as Unlock does) and X25519 (reconstructed
// via ecdh.Curve.NewPrivateKey, which itself validates the scalar is
// well-formed, a real correctness check on its own).
func (k *Keystore) UnlockShielded(passphrase string) (ShieldedIdentity, error) {
	secrets, err := k.unlockSecrets(passphrase)
	if err != nil {
		return ShieldedIdentity{}, err
	}
	pk := k.PublicKey()
	sk := crypto.DilithiumPrivateKey(secrets.DilithiumSK)

	sig, err := crypto.DilithiumSign(sk, selfTestMessage)
	if err != nil {
		zero(sk)
		zero(secrets.X25519SK)
		return ShieldedIdentity{}, fmt.Errorf("walletkey: decrypted key failed self-test signing: %w", err)
	}
	ok, err := crypto.DilithiumVerify(pk, selfTestMessage, sig)
	if err != nil || !ok {
		zero(sk)
		zero(secrets.X25519SK)
		return ShieldedIdentity{}, errors.New("walletkey: decrypted key failed self-test verification — keystore is inconsistent")
	}

	xsk, err := x25519Curve().NewPrivateKey(secrets.X25519SK)
	if err != nil {
		zero(sk)
		zero(secrets.X25519SK)
		return ShieldedIdentity{}, fmt.Errorf("walletkey: decrypted shielded key is invalid: %w", err)
	}

	return ShieldedIdentity{
		PublicKey:   pk,
		PrivateKey:  sk,
		ShieldedPub: xsk.PublicKey(),
		ShieldedKey: xsk,
	}, nil
}

// ChangePassphrase re-encrypts both private keys under newPassphrase with
// a freshly generated salt (never reusing the old one), after confirming
// oldPassphrase genuinely unlocks the current ciphertext. The receiver is
// only mutated once the new ciphertext is fully computed — a failure
// partway through never leaves the in-memory keystore (or, since callers
// are expected to Save only after this returns, the on-disk file) in a
// half-changed state.
func (k *Keystore) ChangePassphrase(oldPassphrase, newPassphrase string) error {
	id, err := k.UnlockShielded(oldPassphrase)
	if err != nil {
		return err
	}
	defer zero(id.PrivateKey)

	resealed, err := seal(id.PublicKey, id.PrivateKey, id.ShieldedKey, newPassphrase)
	if err != nil {
		return err
	}
	k.file = resealed.file
	return nil
}
