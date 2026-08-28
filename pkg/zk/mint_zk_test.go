package zk_test

import (
	"bytes"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

func mkMintSecret(t *testing.T, value uint64) zk.NoteSecret {
	t.Helper()
	sk, err := zk.NewSpendKey()
	if err != nil {
		t.Fatalf("spend key: %v", err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatalf("rho: %v", err)
	}
	return zk.NoteSecret{Value: value, OwnerSK: sk, Rho: rho}
}

func TestMintProofVerifiesForRealCommitment(t *testing.T) {
	secret := mkMintSecret(t, 1000)
	sys, err := zk.SetupMint()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	pub := zk.MintPublic{Amount: secret.Value, OutCommit: secret.Commitment()}
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err != nil {
		t.Fatalf("expected a real mint proof to verify: %v", err)
	}
}

func TestMintProofRejectsClaimedAmountMismatch(t *testing.T) {
	secret := mkMintSecret(t, 1000)
	sys, err := zk.SetupMint()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	// A verifier is handed a claimed Amount that doesn't match what the
	// commitment actually opens to — must fail, not silently accept a
	// mismatched public claim.
	pub := zk.MintPublic{Amount: 999999, OutCommit: secret.Commitment()}
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err == nil {
		t.Fatalf("expected verification to fail when the claimed Amount doesn't match the real commitment's opening")
	}
}

func TestMintProofRejectsWrongOutCommit(t *testing.T) {
	secretA := mkMintSecret(t, 1000)
	secretB := mkMintSecret(t, 1000) // same amount, different real note
	sys, err := zk.SetupMint()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	proof, err := sys.Prove(secretA)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	// A proof of secretA's real opening must not also vouch for a
	// different note's commitment, even at the identical claimed amount.
	pub := zk.MintPublic{Amount: secretB.Value, OutCommit: secretB.Commitment()}
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err == nil {
		t.Fatalf("expected a proof for one note to fail verification against a different note's commitment")
	}
}

func TestMintWriteToReadRoundTrips(t *testing.T) {
	sys, err := zk.SetupMint()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	secret := mkMintSecret(t, 42)
	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}

	var buf bytes.Buffer
	if _, err := sys.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	reloaded, err := zk.ReadMintSystem(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	pub := zk.MintPublic{Amount: secret.Value, OutCommit: secret.Commitment()}
	if err := reloaded.VerifyPublicProofBytes(proofBytes, pub); err != nil {
		t.Fatalf("expected a proof built under the original system to verify under a reloaded one: %v", err)
	}
}
