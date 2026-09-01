package crypto_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
)

func TestClassicalSignVerifyRoundTrip(t *testing.T) {
	pk, sk, err := crypto.GenerateClassicalKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	msg := []byte("dual-sign migration aid: TxID abc123")
	sig, err := crypto.ClassicalSign(sk, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ok, err := crypto.ClassicalVerify(pk, msg, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatalf("expected valid signature to verify")
	}
}

func TestClassicalVerifyRejectsTamperedMessage(t *testing.T) {
	pk, sk, _ := crypto.GenerateClassicalKey()
	sig, _ := crypto.ClassicalSign(sk, []byte("original"))
	ok, err := crypto.ClassicalVerify(pk, []byte("tampered"), sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatalf("expected tampered message to fail verification")
	}
}

func TestClassicalVerifyRejectsWrongKey(t *testing.T) {
	_, sk, _ := crypto.GenerateClassicalKey()
	otherPK, _, _ := crypto.GenerateClassicalKey()
	msg := []byte("msg")
	sig, _ := crypto.ClassicalSign(sk, msg)
	ok, err := crypto.ClassicalVerify(otherPK, msg, sig)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ok {
		t.Fatalf("expected verification against the wrong public key to fail")
	}
}

func TestClassicalVerifyRejectsWrongLengthKeyOrSig(t *testing.T) {
	pk, sk, _ := crypto.GenerateClassicalKey()
	sig, _ := crypto.ClassicalSign(sk, []byte("msg"))
	if _, err := crypto.ClassicalVerify(pk[:len(pk)-1], []byte("msg"), sig); err == nil {
		t.Fatalf("expected a truncated public key to be rejected")
	}
	if _, err := crypto.ClassicalVerify(pk, []byte("msg"), sig[:len(sig)-1]); err == nil {
		t.Fatalf("expected a truncated signature to be rejected")
	}
}
