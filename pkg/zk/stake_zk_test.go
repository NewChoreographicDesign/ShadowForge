package zk_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

var (
	stakeOnce sync.Once
	stakeSys  *zk.StakeSystem
	stakeErr  error

	unstakeOnce sync.Once
	unstakeSys  *zk.UnstakeSystem
	unstakeErr  error
)

func getStakeSystem(t *testing.T) *zk.StakeSystem {
	t.Helper()
	stakeOnce.Do(func() { stakeSys, stakeErr = zk.SetupStake() })
	if stakeErr != nil {
		t.Fatalf("stake zk setup: %v", stakeErr)
	}
	return stakeSys
}

func getUnstakeSystem(t *testing.T) *zk.UnstakeSystem {
	t.Helper()
	unstakeOnce.Do(func() { unstakeSys, unstakeErr = zk.SetupUnstake() })
	if unstakeErr != nil {
		t.Fatalf("unstake zk setup: %v", unstakeErr)
	}
	return unstakeSys
}

func TestStakeProofVerifiesForRealPosition(t *testing.T) {
	sys := getStakeSystem(t)
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		t.Fatal(err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatal(err)
	}
	secret := zk.StakeSecret{Principal: 5000, StartEpoch: 3, OwnerSK: ownerSK, Rho: rho}

	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	pub := zk.StakePublic{Principal: secret.Principal, StartEpoch: secret.StartEpoch, PositionCommit: secret.Commitment()}
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err != nil {
		t.Fatalf("expected a real stake proof to verify: %v", err)
	}
}

func TestStakeProofRejectsClaimedPrincipalMismatch(t *testing.T) {
	sys := getStakeSystem(t)
	ownerSK, _ := zk.NewSpendKey()
	rho, _ := zk.NewRho()
	secret := zk.StakeSecret{Principal: 5000, StartEpoch: 3, OwnerSK: ownerSK, Rho: rho}

	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	// Claim a different principal than the one actually proved.
	pub := zk.StakePublic{Principal: 9999, StartEpoch: secret.StartEpoch, PositionCommit: secret.Commitment()}
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err == nil {
		t.Fatalf("expected verification to fail for a claimed principal the proof was never built for")
	}
}

func TestStakeProofRejectsClaimedStartEpochMismatch(t *testing.T) {
	sys := getStakeSystem(t)
	ownerSK, _ := zk.NewSpendKey()
	rho, _ := zk.NewRho()
	secret := zk.StakeSecret{Principal: 5000, StartEpoch: 3, OwnerSK: ownerSK, Rho: rho}

	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	pub := zk.StakePublic{Principal: secret.Principal, StartEpoch: 999, PositionCommit: secret.Commitment()}
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err == nil {
		t.Fatalf("expected verification to fail for a claimed start epoch the proof was never built for")
	}
}

func TestStakeWriteToReadRoundTrips(t *testing.T) {
	sys := getStakeSystem(t)
	var buf bytes.Buffer
	if _, err := sys.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := zk.ReadStakeSystem(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	ownerSK, _ := zk.NewSpendKey()
	rho, _ := zk.NewRho()
	secret := zk.StakeSecret{Principal: 42, StartEpoch: 1, OwnerSK: ownerSK, Rho: rho}
	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove with original: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	pub := zk.StakePublic{Principal: secret.Principal, StartEpoch: secret.StartEpoch, PositionCommit: secret.Commitment()}
	if err := loaded.VerifyPublicProofBytes(proofBytes, pub); err != nil {
		t.Fatalf("expected the round-tripped system to verify a proof the original produced: %v", err)
	}
}

// buildRealUnstakeTree stakes a real position via sys into a fresh
// zk.Tree, returning the tree, the position secret, and the membership
// Proof — shared setup for the Unstake tests below.
func buildRealUnstakeTree(t *testing.T, principal, startEpoch uint64) (*zk.Tree, zk.StakeSecret, zk.Proof) {
	t.Helper()
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		t.Fatal(err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatal(err)
	}
	secret := zk.StakeSecret{Principal: principal, StartEpoch: startEpoch, OwnerSK: ownerSK, Rho: rho}

	tree := zk.NewTree()
	idx, err := tree.Insert(secret.Commitment())
	if err != nil {
		t.Fatalf("insert position: %v", err)
	}
	proof, err := tree.Prove(idx)
	if err != nil {
		t.Fatalf("build membership proof: %v", err)
	}
	return tree, secret, proof
}

func TestUnstakeProofVerifiesForRealPosition(t *testing.T) {
	sys := getUnstakeSystem(t)
	tree, secret, proof := buildRealUnstakeTree(t, 5000, 3)
	root, err := tree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}

	outOwnerSK, _ := zk.NewSpendKey()
	outRho, _ := zk.NewRho()
	const finalAmount = 5100 // e.g. principal + some real yield
	out := zk.NoteSecret{Value: finalAmount, OwnerSK: outOwnerSK, Rho: outRho}

	in := zk.UnstakeInput{MerkleRoot: root, Position: secret, Proof: proof, Out: out}
	proofBytes := mustProveUnstake(t, sys, in)

	if err := sys.VerifyPublicProofBytes(proofBytes, in.Public()); err != nil {
		t.Fatalf("expected a real unstake proof to verify: %v", err)
	}
}

func mustProveUnstake(t *testing.T, sys *zk.UnstakeSystem, in zk.UnstakeInput) []byte {
	t.Helper()
	p, err := sys.Prove(in)
	if err != nil {
		t.Fatalf("prove unstake: %v", err)
	}
	b, err := zk.ProofToBytes(p)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}
	return b
}

func TestUnstakeProofRejectsWrongPosition(t *testing.T) {
	sys := getUnstakeSystem(t)
	tree, _, proofA := buildRealUnstakeTree(t, 5000, 3)
	_, secretB, _ := buildRealUnstakeTree(t, 5000, 3) // a different, real position never inserted into tree
	root, err := tree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}

	outOwnerSK, _ := zk.NewSpendKey()
	outRho, _ := zk.NewRho()
	out := zk.NoteSecret{Value: 5100, OwnerSK: outOwnerSK, Rho: outRho}

	// Use secretB's opening but proofA's (secretA's own) membership path —
	// proving fails outright: Path[0] must equal the freshly computed
	// commitment, and secretB's commitment differs from secretA's.
	in := zk.UnstakeInput{MerkleRoot: root, Position: secretB, Proof: proofA, Out: out}
	if _, err := sys.Prove(in); err == nil {
		t.Fatalf("expected proving to fail for a position/membership-path mismatch")
	}
}

func TestUnstakeProofRejectsClaimedFinalAmountMismatch(t *testing.T) {
	sys := getUnstakeSystem(t)
	tree, secret, proof := buildRealUnstakeTree(t, 5000, 3)
	root, err := tree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	outOwnerSK, _ := zk.NewSpendKey()
	outRho, _ := zk.NewRho()
	out := zk.NoteSecret{Value: 5100, OwnerSK: outOwnerSK, Rho: outRho}
	in := zk.UnstakeInput{MerkleRoot: root, Position: secret, Proof: proof, Out: out}
	proofBytes := mustProveUnstake(t, sys, in)

	pub := in.Public()
	pub.FinalAmount = 999999 // claim a wildly different final amount than the proof was built for
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err == nil {
		t.Fatalf("expected verification to fail for a claimed final amount the proof was never built for")
	}
}

func TestUnstakeProofRejectsWrongMerkleRoot(t *testing.T) {
	sys := getUnstakeSystem(t)
	tree, secret, proof := buildRealUnstakeTree(t, 5000, 3)
	root, err := tree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	outOwnerSK, _ := zk.NewSpendKey()
	outRho, _ := zk.NewRho()
	out := zk.NoteSecret{Value: 5100, OwnerSK: outOwnerSK, Rho: outRho}
	in := zk.UnstakeInput{MerkleRoot: root, Position: secret, Proof: proof, Out: out}
	proofBytes := mustProveUnstake(t, sys, in)

	pub := in.Public()
	otherTree := zk.NewTree()
	otherRoot, err := otherTree.Root()
	if err != nil {
		t.Fatalf("other root: %v", err)
	}
	pub.MerkleRoot = otherRoot
	if err := sys.VerifyPublicProofBytes(proofBytes, pub); err == nil {
		t.Fatalf("expected verification to fail for a claimed root the proof was never anchored to")
	}
}

func TestUnstakeWriteToReadRoundTrips(t *testing.T) {
	sys := getUnstakeSystem(t)
	var buf bytes.Buffer
	if _, err := sys.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := zk.ReadUnstakeSystem(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	tree, secret, proof := buildRealUnstakeTree(t, 42, 1)
	root, err := tree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	outOwnerSK, _ := zk.NewSpendKey()
	outRho, _ := zk.NewRho()
	out := zk.NoteSecret{Value: 43, OwnerSK: outOwnerSK, Rho: outRho}
	in := zk.UnstakeInput{MerkleRoot: root, Position: secret, Proof: proof, Out: out}
	proofBytes := mustProveUnstake(t, sys, in)

	if err := loaded.VerifyPublicProofBytes(proofBytes, in.Public()); err != nil {
		t.Fatalf("expected the round-tripped system to verify a proof the original produced: %v", err)
	}
}
