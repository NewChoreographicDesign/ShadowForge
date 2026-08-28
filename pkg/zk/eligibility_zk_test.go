package zk_test

import (
	"bytes"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

func buildEligibilityInput(t *testing.T) (zk.EligibilityInput, *zk.Tree, zk.FieldElement) {
	t.Helper()
	tree := zk.NewTree()

	voterSK := zk.DeriveVoterSK([]byte("a real wallet's dilithium secret key bytes"))
	commitment := zk.VoterCommitment(voterSK)
	idx, err := tree.Insert(commitment)
	if err != nil {
		t.Fatalf("insert commitment: %v", err)
	}
	// pad the tree with a few other real voters so this isn't a
	// single-leaf tree.
	for i := 0; i < 3; i++ {
		other := zk.DeriveVoterSK([]byte{byte(i)})
		if _, err := tree.Insert(zk.VoterCommitment(other)); err != nil {
			t.Fatalf("insert other commitment: %v", err)
		}
	}

	proof, err := tree.Prove(idx)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	scope := zk.DeriveVoterSK([]byte("proposal-scope-1")) // any field element works as a scope for this test

	return zk.EligibilityInput{
		MerkleRoot:    proof.Root,
		ProposalScope: scope,
		VoterSK:       voterSK,
		Proof:         proof,
	}, tree, commitment
}

func TestEligibilityProofVerifiesForRealMember(t *testing.T) {
	in, _, _ := buildEligibilityInput(t)
	sys, err := zk.SetupEligibility()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	proof, err := sys.Prove(in)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	if err := sys.VerifyPublicProofBytes(proofBytes, in.Public()); err != nil {
		t.Fatalf("expected a real member's proof to verify, got: %v", err)
	}
}

func TestEligibilityProofRejectsNonMember(t *testing.T) {
	in, tree, _ := buildEligibilityInput(t)
	// Substitute a VoterSK that was never inserted into the tree, but
	// keep the honest member's Merkle path/root — a forged membership
	// claim, not merely a malformed one.
	forgedSK := zk.DeriveVoterSK([]byte("never minted an nft"))
	in.VoterSK = forgedSK
	_ = tree

	sys, err := zk.SetupEligibility()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := sys.Prove(in); err == nil {
		t.Fatalf("expected proving with a non-member secret to fail (the leaf pre-image constraint), got a proof")
	}
}

func TestEligibilityProofRejectsWrongScope(t *testing.T) {
	in, _, _ := buildEligibilityInput(t)
	sys, err := zk.SetupEligibility()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	proof, err := sys.Prove(in)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}

	pub := in.Public()
	pub.ProposalScope = zk.DeriveVoterSK([]byte("a different proposal"))
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err == nil {
		t.Fatalf("expected verification to fail when the claimed ProposalScope doesn't match what was proved (which nullifier binds to)")
	}
}

func TestEligibilityWriteToReadRoundTrips(t *testing.T) {
	sys, err := zk.SetupEligibility()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	in, _, _ := buildEligibilityInput(t)
	proof, err := sys.Prove(in)
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
	reloaded, err := zk.ReadEligibilitySystem(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := reloaded.VerifyPublicProofBytes(proofBytes, in.Public()); err != nil {
		t.Fatalf("expected a proof built under the original system to verify under a reloaded one: %v", err)
	}
}
