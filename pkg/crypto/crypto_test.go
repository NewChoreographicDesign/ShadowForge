package crypto_test

import (
	"bytes"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
)

func TestDilithiumSignVerifyRoundTrip(t *testing.T) {
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	msg := []byte("stage-5 vote: commit state root abc123")
	sig, err := crypto.DilithiumSign(sk, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ok, err := crypto.DilithiumVerify(pk, msg, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatalf("expected valid signature to verify")
	}
}

func TestDilithiumVerifyRejectsTamperedMessage(t *testing.T) {
	pk, sk, _ := crypto.GenerateDilithiumKey()
	sig, _ := crypto.DilithiumSign(sk, []byte("original"))
	ok, err := crypto.DilithiumVerify(pk, []byte("tampered"), sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatalf("expected tampered message to fail verification")
	}
}

func TestDilithiumVerifyRejectsWrongKey(t *testing.T) {
	_, sk, _ := crypto.GenerateDilithiumKey()
	otherPK, _, _ := crypto.GenerateDilithiumKey()
	msg := []byte("msg")
	sig, _ := crypto.DilithiumSign(sk, msg)
	ok, err := crypto.DilithiumVerify(otherPK, msg, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatalf("expected verification against the wrong public key to fail")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	var key crypto.EncryptionKey
	copy(key[:], bytes.Repeat([]byte{0x42}, len(key)))
	plaintext := []byte("shielded note: value=100 owner=alice")
	aad := []byte("commitment:abc")

	blob, err := crypto.Encrypt(key, plaintext, aad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(blob, plaintext) {
		t.Fatalf("ciphertext must not contain plaintext")
	}
	got, err := crypto.Decrypt(key, blob, aad)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
	}
}

func TestDecryptRejectsWrongAAD(t *testing.T) {
	var key crypto.EncryptionKey
	blob, _ := crypto.Encrypt(key, []byte("secret"), []byte("aad-1"))
	if _, err := crypto.Decrypt(key, blob, []byte("aad-2")); err == nil {
		t.Fatalf("expected decrypt to fail with mismatched AAD")
	}
}
