package zk_test

import (
	"bytes"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// TestSystemPersistenceEnablesCrossProcessVerification is a direct
// regression test for a real interoperability gap live CLI testing
// surfaced: two independent zk.Setup() calls produce mutually
// incompatible Groth16 keys, so a proof built by one process (e.g. this
// session's wallet CLI) never verifies against another's (a live
// validator node) — even though the transaction itself is entirely
// valid. Persisting and reloading a System's real keys is what lets
// separate processes agree on the same trusted parameters.
func TestSystemPersistenceEnablesCrossProcessVerification(t *testing.T) {
	original, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var buf bytes.Buffer
	if _, err := original.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	reloaded, err := zk.ReadSystem(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	input, _ := buildInput(t)
	pubWitness, err := zk.PublicWitness(input)
	if err != nil {
		t.Fatalf("public witness: %v", err)
	}

	// A proof built under the reloaded (real, persisted) keys verifies
	// under the original System — proving reloaded is genuinely the same
	// trusted setup, not merely a differently-shaped System that happens
	// to also work.
	proofFromReloaded, err := reloaded.Prove(input)
	if err != nil {
		t.Fatalf("prove with reloaded system: %v", err)
	}
	if err := original.Verify(proofFromReloaded, pubWitness); err != nil {
		t.Fatalf("expected the original system to verify a proof built under the reloaded (persisted) keys: %v", err)
	}

	// And the reverse: a proof built under the original System verifies
	// under the reloaded one.
	proofFromOriginal, err := original.Prove(input)
	if err != nil {
		t.Fatalf("prove with original system: %v", err)
	}
	if err := reloaded.Verify(proofFromOriginal, pubWitness); err != nil {
		t.Fatalf("expected the reloaded system to verify a proof built under the original keys: %v", err)
	}
}

// TestIndependentSetupsAreNotInterchangeable proves the gap
// TestSystemPersistenceEnablesCrossProcessVerification closes was real:
// two independently Setup() systems (never sharing serialized keys, the
// exact bug this session's live CLI testing hit) genuinely cannot verify
// each other's proofs.
func TestIndependentSetupsAreNotInterchangeable(t *testing.T) {
	sysA, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup A: %v", err)
	}
	sysB, err := zk.Setup()
	if err != nil {
		t.Fatalf("setup B: %v", err)
	}

	input, _ := buildInput(t)
	pubWitness, err := zk.PublicWitness(input)
	if err != nil {
		t.Fatalf("public witness: %v", err)
	}
	proof, err := sysA.Prove(input)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	if err := sysB.Verify(proof, pubWitness); err == nil {
		t.Fatalf("expected two independently-Setup() systems to be mutually incompatible")
	}
}
