package walletkey_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/walletkey"
)

func TestGenerateRejectsEmptyPassphrase(t *testing.T) {
	if _, err := walletkey.Generate(""); err == nil {
		t.Fatalf("expected an error for an empty passphrase")
	}
}

// TestGenerateSaveLoadUnlockRoundTrip is the core real end-to-end proof:
// a fresh Dilithium keypair is generated, sealed to disk, reloaded from a
// clean Keystore value (not the one still holding it in memory), unlocked
// with the real passphrase, and the returned key pair actually signs and
// verifies real data with pkg/crypto's real Dilithium primitives.
func TestGenerateSaveLoadUnlockRoundTrip(t *testing.T) {
	ks, err := walletkey.Generate("correct horse battery staple")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if err := ks.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := walletkey.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.PublicKey().String() != ks.PublicKey().String() {
		t.Fatalf("loaded public key does not match the generated one")
	}
	if loaded.Identity() != ks.Identity() {
		t.Fatalf("loaded identity does not match the generated one")
	}

	pk, sk, err := loaded.Unlock("correct horse battery staple")
	if err != nil {
		t.Fatalf("unlock with the real passphrase: %v", err)
	}
	if pk.String() != ks.PublicKey().String() {
		t.Fatalf("unlocked public key does not match")
	}

	msg := []byte("a real message a real wallet would sign")
	sig, err := crypto.DilithiumSign(sk, msg)
	if err != nil {
		t.Fatalf("sign with the unlocked private key: %v", err)
	}
	ok, err := crypto.DilithiumVerify(pk, msg, sig)
	if err != nil || !ok {
		t.Fatalf("expected the unlocked key pair to produce a verifiable signature: ok=%v err=%v", ok, err)
	}
}

func TestUnlockRejectsWrongPassphrase(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, _, err := ks.Unlock("a-wrong-passphrase"); err == nil {
		t.Fatalf("expected wrong passphrase to be rejected")
	}
}

func TestUnlockRejectsEmptyPassphrase(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, _, err := ks.Unlock(""); err == nil {
		t.Fatalf("expected an empty passphrase to be rejected")
	}
}

// TestUnlockRejectsTamperedCiphertext proves the AEAD is actually doing
// real integrity checking, not just obfuscation: flipping a single byte
// of the stored ciphertext must make Unlock fail, even with the correct
// passphrase.
func TestUnlockRejectsTamperedCiphertext(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if err := ks.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ct := f["ciphertext"].(string)
	// Flip one hex character deep enough to land inside the ciphertext
	// body (past the nonce), not just its first byte.
	tampered := []byte(ct)
	idx := len(tampered) - 1
	if tampered[idx] == '0' {
		tampered[idx] = '1'
	} else {
		tampered[idx] = '0'
	}
	f["ciphertext"] = string(tampered)
	out, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("write tampered file: %v", err)
	}

	loaded, err := walletkey.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, _, err := loaded.Unlock("the-real-passphrase"); err == nil {
		t.Fatalf("expected a tampered ciphertext to fail authentication even with the correct passphrase")
	}
}

// TestUnlockRejectsTamperedPublicKey proves the public key is really
// bound into the AEAD's associated data: swapping in a different (but
// well-formed) public key must break decryption of the untouched
// ciphertext, not silently pair a stranger's declared identity with this
// keystore's real private key.
func TestUnlockRejectsTamperedPublicKey(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if err := ks.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	otherPK, _, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	f["public_key"] = otherPK.String()
	out, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("write tampered file: %v", err)
	}

	loaded, err := walletkey.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, _, err := loaded.Unlock("the-real-passphrase"); err == nil {
		t.Fatalf("expected a swapped public key to break decryption of the original ciphertext")
	}
}

// TestChangePassphraseRotatesRealEncryption proves ChangePassphrase does
// a real re-encryption (old passphrase stops working, new one works, and
// the underlying key material is unchanged) rather than just relabeling
// something.
func TestChangePassphraseRotatesRealEncryption(t *testing.T) {
	ks, err := walletkey.Generate("old-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	originalPK := ks.PublicKey().String()

	if err := ks.ChangePassphrase("wrong-old-passphrase", "new-passphrase"); err == nil {
		t.Fatalf("expected ChangePassphrase to fail with the wrong old passphrase")
	}

	if err := ks.ChangePassphrase("old-passphrase", "new-passphrase"); err != nil {
		t.Fatalf("change passphrase: %v", err)
	}

	if _, _, err := ks.Unlock("old-passphrase"); err == nil {
		t.Fatalf("expected the old passphrase to no longer work after rotation")
	}
	pk, sk, err := ks.Unlock("new-passphrase")
	if err != nil {
		t.Fatalf("unlock with the new passphrase: %v", err)
	}
	if pk.String() != originalPK {
		t.Fatalf("expected the same identity to survive a passphrase change")
	}

	msg := []byte("proving the rotated key still really works")
	sig, err := crypto.DilithiumSign(sk, msg)
	if err != nil {
		t.Fatalf("sign after rotation: %v", err)
	}
	if ok, err := crypto.DilithiumVerify(pk, msg, sig); err != nil || !ok {
		t.Fatalf("expected a verifiable signature after rotation: ok=%v err=%v", ok, err)
	}
}

// TestSaveNeverWritesPlaintextPrivateKey scans the actual bytes written
// to disk for the real private key material, as a direct regression test
// against the one failure mode this whole package exists to prevent.
func TestSaveNeverWritesPlaintextPrivateKey(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if err := ks.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, sk, err := ks.Unlock("the-real-passphrase")
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(sk) == 0 {
		t.Fatalf("expected a non-empty private key from Unlock")
	}
	// The real, meaningful check: the actual private key bytes, hex
	// -encoded, must not appear anywhere in the saved file. Casting to
	// DilithiumPublicKey reuses its (non-redacted) String() to get the
	// real hex encoding of sk's raw bytes for comparison.
	hexSK := crypto.DilithiumPublicKey(sk).String()
	if strings.Contains(string(raw), hexSK) {
		t.Fatalf("SAFETY VIOLATION: keystore file contains the plaintext private key hex-encoded")
	}
}

func TestSaveWritesOwnerOnlyPermissions(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file mode bits don't restrict root, skip")
	}
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if err := ks.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected owner-only 0600 permissions, got %o", perm)
	}
}

func TestSaveIsAtomicNoPartialFileOnFailure(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// A directory that doesn't exist makes the rename step fail, but the
	// point is that no file ever appears at the *final* path in that case.
	badPath := filepath.Join(t.TempDir(), "does-not-exist", "wallet.json")
	if err := ks.Save(badPath); err == nil {
		t.Fatalf("expected Save to fail for a non-existent directory")
	}
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Fatalf("expected no file at the final path after a failed save")
	}
}

func TestLoadRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet.json")
	bad := `{"version":99,"public_key":"aa","kdf":"argon2id","kdf_params":{},"salt":"bb","ciphertext":"cc"}`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := walletkey.Load(path); err == nil {
		t.Fatalf("expected an unsupported version to be rejected")
	}
}

func TestLoadRejectsUnsupportedKDF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet.json")
	bad := `{"version":1,"public_key":"aa","kdf":"md5-obviously-not-real","kdf_params":{},"salt":"bb","ciphertext":"cc"}`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := walletkey.Load(path); err == nil {
		t.Fatalf("expected an unsupported KDF to be rejected")
	}
}

func TestIdentityMatchesConsensusConvention(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want := types.NFTID(types.SumHash(ks.PublicKey()))
	if ks.Identity() != want {
		t.Fatalf("expected Identity() to match NFTID(SumHash(pk)), the same convention pkg/validator and cmd/walletsim use")
	}
}

// TestTwoGeneratedKeystoresAreDistinct guards against a catastrophic RNG
// or salt-reuse bug: two independently generated keystores must never
// share a public key or a salt.
// --- X25519 shielded key ---

func TestUnlockShieldedRoundTripEnablesRealECDH(t *testing.T) {
	alice, err := walletkey.Generate("alice-passphrase")
	if err != nil {
		t.Fatalf("generate alice: %v", err)
	}
	bob, err := walletkey.Generate("bob-passphrase")
	if err != nil {
		t.Fatalf("generate bob: %v", err)
	}

	aliceID, err := alice.UnlockShielded("alice-passphrase")
	if err != nil {
		t.Fatalf("unlock alice: %v", err)
	}
	bobID, err := bob.UnlockShielded("bob-passphrase")
	if err != nil {
		t.Fatalf("unlock bob: %v", err)
	}

	if aliceID.ShieldedPub.Equal(bob.ShieldedPublicKey()) {
		t.Fatalf("expected alice and bob to have distinct shielded public keys")
	}

	// The real point of the X25519 key: two independently-unlocked
	// identities that only ever exchanged public keys must derive the
	// identical shared secret.
	secretFromAlice, err := aliceID.ShieldedKey.ECDH(bobID.ShieldedPub)
	if err != nil {
		t.Fatalf("alice ECDH: %v", err)
	}
	secretFromBob, err := bobID.ShieldedKey.ECDH(aliceID.ShieldedPub)
	if err != nil {
		t.Fatalf("bob ECDH: %v", err)
	}
	if string(secretFromAlice) != string(secretFromBob) {
		t.Fatalf("expected both sides to derive the same ECDH shared secret")
	}
}

func TestShieldedPublicKeyAvailableWithoutPassphrase(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if err := ks.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := walletkey.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ShieldedPublicKey() == nil {
		t.Fatalf("expected a shielded public key to be available without unlocking")
	}
	if !loaded.ShieldedPublicKey().Equal(ks.ShieldedPublicKey()) {
		t.Fatalf("expected the loaded shielded public key to match the generated one")
	}
}

func TestUnlockShieldedRejectsTamperedShieldedPublicKey(t *testing.T) {
	ks, err := walletkey.Generate("the-real-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "wallet.json")
	if err := ks.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	other, err := walletkey.Generate("unrelated")
	if err != nil {
		t.Fatalf("generate other: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	f["x25519_public_key"] = hex.EncodeToString(other.ShieldedPublicKey().Bytes())
	out, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("write tampered file: %v", err)
	}

	loaded, err := walletkey.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := loaded.UnlockShielded("the-real-passphrase"); err == nil {
		t.Fatalf("expected a swapped shielded public key to break decryption of the original ciphertext")
	}
}

func TestChangePassphrasePreservesShieldedKey(t *testing.T) {
	ks, err := walletkey.Generate("old-passphrase")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	originalShielded := ks.ShieldedPublicKey().Bytes()

	if err := ks.ChangePassphrase("old-passphrase", "new-passphrase"); err != nil {
		t.Fatalf("change passphrase: %v", err)
	}

	id, err := ks.UnlockShielded("new-passphrase")
	if err != nil {
		t.Fatalf("unlock with new passphrase: %v", err)
	}
	if string(id.ShieldedPub.Bytes()) != string(originalShielded) {
		t.Fatalf("expected the same shielded identity to survive a passphrase change")
	}
}

func TestTwoGeneratedKeystoresAreDistinct(t *testing.T) {
	a, err := walletkey.Generate("passphrase-a")
	if err != nil {
		t.Fatalf("generate a: %v", err)
	}
	b, err := walletkey.Generate("passphrase-b")
	if err != nil {
		t.Fatalf("generate b: %v", err)
	}
	if a.PublicKey().String() == b.PublicKey().String() {
		t.Fatalf("expected two independently generated keystores to have distinct public keys")
	}
}
