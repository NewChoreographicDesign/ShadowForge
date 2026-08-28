package shieldedwallet_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/shieldedwallet"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

func genX25519(t *testing.T) *ecdh.PrivateKey {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate x25519 key: %v", err)
	}
	return priv
}

func realNote(t *testing.T, value uint64) zk.NoteSecret {
	t.Helper()
	sk, err := zk.NewSpendKey()
	if err != nil {
		t.Fatal(err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatal(err)
	}
	return zk.NoteSecret{Value: value, OwnerSK: sk, Rho: rho}
}

func TestEncryptDecryptMemoRoundTrip(t *testing.T) {
	receiver := genX25519(t)
	secret := realNote(t, 12345)

	memo, err := shieldedwallet.EncryptMemo(receiver.PublicKey(), secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	got, err := shieldedwallet.DecryptMemo(receiver, memo)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got.Value != secret.Value {
		t.Fatalf("value mismatch: got %d want %d", got.Value, secret.Value)
	}
	if got.Commitment() != secret.Commitment() {
		t.Fatalf("expected the decrypted note to reconstruct the identical real commitment")
	}
	if got.Nullifier() != secret.Nullifier() {
		t.Fatalf("expected the decrypted note to reconstruct the identical real nullifier")
	}
}

func TestDecryptMemoRejectsWrongReceiver(t *testing.T) {
	receiver := genX25519(t)
	stranger := genX25519(t)
	secret := realNote(t, 500)

	memo, err := shieldedwallet.EncryptMemo(receiver.PublicKey(), secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := shieldedwallet.DecryptMemo(stranger, memo); err == nil {
		t.Fatalf("expected a memo addressed to someone else to fail to decrypt")
	}
}

func TestDecryptMemoRejectsTamperedCiphertext(t *testing.T) {
	receiver := genX25519(t)
	secret := realNote(t, 42)

	memo, err := shieldedwallet.EncryptMemo(receiver.PublicKey(), secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	tampered := append([]byte{}, memo...)
	tampered[len(tampered)-1] ^= 0xFF

	if _, err := shieldedwallet.DecryptMemo(receiver, tampered); err == nil {
		t.Fatalf("expected a tampered memo to fail authentication")
	}
}

func TestDecryptMemoRejectsTooShortMemo(t *testing.T) {
	receiver := genX25519(t)
	if _, err := shieldedwallet.DecryptMemo(receiver, []byte("short")); err == nil {
		t.Fatalf("expected a too-short memo to be rejected")
	}
}

func TestEncryptMemoProducesDistinctCiphertextsForSameNote(t *testing.T) {
	receiver := genX25519(t)
	secret := realNote(t, 999)

	memoA, err := shieldedwallet.EncryptMemo(receiver.PublicKey(), secret)
	if err != nil {
		t.Fatalf("encrypt a: %v", err)
	}
	memoB, err := shieldedwallet.EncryptMemo(receiver.PublicKey(), secret)
	if err != nil {
		t.Fatalf("encrypt b: %v", err)
	}
	if string(memoA) == string(memoB) {
		t.Fatalf("expected two encryptions of the same note to differ (fresh ephemeral key + nonce each time)")
	}

	// Both must still decrypt to the same real note.
	gotA, err := shieldedwallet.DecryptMemo(receiver, memoA)
	if err != nil {
		t.Fatalf("decrypt a: %v", err)
	}
	gotB, err := shieldedwallet.DecryptMemo(receiver, memoB)
	if err != nil {
		t.Fatalf("decrypt b: %v", err)
	}
	if gotA.Commitment() != gotB.Commitment() || gotA.Commitment() != secret.Commitment() {
		t.Fatalf("expected both decryptions to reconstruct the identical real note")
	}
}
