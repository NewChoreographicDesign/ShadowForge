package tx_test

import (
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/staking"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

var (
	stakeSysOnce   sync.Once
	stakeSysShared *zk.StakeSystem
	stakeSysErr    error

	unstakeSysOnce   sync.Once
	unstakeSysShared *zk.UnstakeSystem
	unstakeSysErr    error
)

func getStakeSystemForTx(t *testing.T) *zk.StakeSystem {
	t.Helper()
	stakeSysOnce.Do(func() { stakeSysShared, stakeSysErr = zk.SetupStake() })
	if stakeSysErr != nil {
		t.Fatalf("stake zk setup: %v", stakeSysErr)
	}
	return stakeSysShared
}

func getUnstakeSystemForTx(t *testing.T) *zk.UnstakeSystem {
	t.Helper()
	unstakeSysOnce.Do(func() { unstakeSysShared, unstakeSysErr = zk.SetupUnstake() })
	if unstakeSysErr != nil {
		t.Fatalf("unstake zk setup: %v", unstakeSysErr)
	}
	return unstakeSysShared
}

// newDepsWithStaking extends newDepsWithMint's real canonical-tree/MintZK
// configuration with a real, shared stake-commitment tree and its
// StakeZK/UnstakeZK systems — the full configuration a live validator now
// always runs with.
func newDepsWithStaking(t *testing.T) (tx.Deps, *zk.Tree, *zk.Tree) {
	t.Helper()
	deps, zkTree := newDepsWithMint(t)
	deps.StakeZK = getStakeSystemForTx(t)
	deps.UnstakeZK = getUnstakeSystemForTx(t)
	stakeTree := zk.NewTree()
	initialRoot, err := stakeTree.Root()
	if err != nil {
		t.Fatalf("initial stake tree root: %v", err)
	}
	deps.StakeTree = stakeTree
	deps.StakeRoots = zk.NewRootHistory(initialRoot)
	return deps, zkTree, stakeTree
}

// mustBuildStakedMintVote builds a real, well-formed, correctly-proved
// TxVote binding proposalID to a real spec-17.4 staked-yield mint claim
// for amount SFG, at startEpoch (must equal whatever Deps.Epoch the
// pipeline processing this vote is configured with). Returns the tx and
// the real zk.StakeSecret opening the resulting locked position.
func mustBuildStakedMintVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool, amount uint64, startEpoch types.EpochNumber, sys *zk.StakeSystem) (types.ShieldedTx, zk.StakeSecret) {
	t.Helper()
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		t.Fatalf("owner sk: %v", err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatalf("rho: %v", err)
	}
	secret := zk.StakeSecret{Principal: amount, StartEpoch: uint64(startEpoch), OwnerSK: ownerSK, Rho: rho}

	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove stake: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}

	nonce := types.Hash{7, 7}
	commitment := types.ComputeVoteCommitment(elig.Nullifier, approve, nonce)
	votetx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:          proposalID,
			Commitment:          commitment,
			MintAmount:          amount,
			MintStaked:          true,
			StakePositionCommit: types.Hash(zk.ToBytes32(secret.Commitment())),
			StakeProof:          proofBytes,
		},
		VoteEligibility: elig,
	})
	return votetx, secret
}

// TestPipelineStakedMintProposalPassesAndCreatesRealPosition proves the
// real, end-to-end staked-yield path: a real staked mint proposal,
// approved by real revealed ballots, results in a real locked position
// landing in the canonical stake-commitment tree — and, unlike the direct
// path, collects no Vault fee.
func TestPipelineStakedMintProposalPassesAndCreatesRealPosition(t *testing.T) {
	deps, _, stakeTree := newDepsWithStaking(t)
	deps.Epoch = 1
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "staked-proposal-1")

	const amount = 10000
	commitTx, secret := mustBuildStakedMintVote(t, pk, sk, elig, "staked-proposal-1", true, amount, deps.Epoch, getStakeSystemForTx(t))
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the staked mint-bound vote to be accepted: %v", r[0].Error)
	}

	revealTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: "staked-proposal-1", Approve: true, Nonce: types.Hash{7, 7},
		},
		VoteEligibility: elig,
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	remainingBefore := stakeTree.Remaining()
	tallied, err := p.TallyDueProposals(2)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].MintApplied {
		t.Fatalf("expected the proposal to pass and the real position to be applied, got %+v", tallied)
	}
	if got := stakeTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected exactly 1 new leaf inserted into the stake tree, remaining went from %d to %d", remainingBefore, got)
	}
	wantCommit := types.Hash(zk.ToBytes32(secret.Commitment()))
	if tallied[0].StakePositionCommit != wantCommit {
		t.Fatalf("persisted StakePositionCommit %s does not match the real position's own commitment %s", tallied[0].StakePositionCommit, wantCommit)
	}

	// No Vault fee on the staked path — see pkg/staking's own doc.
	collected := deps.Vault.EpochBonusPool.Add(deps.Vault.AuditPool).Add(deps.Vault.RemainderPool).Add(deps.Vault.BurnedTotal)
	if collected.Sign() != 0 {
		t.Fatalf("expected no Vault fee collection on the staked path, got %s", collected)
	}
}

// TestPipelineUnstakeRedeemsRealPositionForRealYield is the real,
// end-to-end proof the staked path actually pays out: after a real staked
// position lands (as above), a real Unstake transaction submitted at a
// later epoch redeems it for a real spendable note carrying principal
// plus real accrued yield, computed by the exact real formula
// pkg/staking.FinalAmount implements — and the position can never be
// redeemed twice.
func TestPipelineUnstakeRedeemsRealPositionForRealYield(t *testing.T) {
	deps, zkTree, stakeTree := newDepsWithStaking(t)
	deps.Epoch = 1
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "staked-proposal-2")

	const amount = 20000
	commitTx, secret := mustBuildStakedMintVote(t, pk, sk, elig, "staked-proposal-2", true, amount, deps.Epoch, getStakeSystemForTx(t))
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the staked mint-bound vote to be accepted: %v", r[0].Error)
	}
	revealTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: "staked-proposal-2", Approve: true, Nonce: types.Hash{7, 7},
		},
		VoteEligibility: elig,
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}
	if _, err := p.TallyDueProposals(2); err != nil {
		t.Fatalf("tally: %v", err)
	}

	// Real membership proof for the real position that landed.
	stakeRoot, err := stakeTree.Root()
	if err != nil {
		t.Fatalf("stake root: %v", err)
	}
	mp, err := stakeTree.Prove(0)
	if err != nil {
		t.Fatalf("build stake membership proof: %v", err)
	}

	// A later epoch — the pipeline processing Unstake runs at Epoch 50,
	// well after the position's own StartEpoch (1), so real yield accrues.
	deps.Epoch = 50
	p2 := tx.NewPipeline(deps)

	wantFinal := staking.FinalAmount(amount, 1, 50)
	if wantFinal <= amount {
		t.Fatalf("test setup: expected some real positive yield over 49 epochs, got final %d for principal %d", wantFinal, amount)
	}

	outOwnerSK, err := zk.NewSpendKey()
	if err != nil {
		t.Fatal(err)
	}
	outRho, err := zk.NewRho()
	if err != nil {
		t.Fatal(err)
	}
	out := zk.NoteSecret{Value: wantFinal, OwnerSK: outOwnerSK, Rho: outRho}
	in := zk.UnstakeInput{MerkleRoot: stakeRoot, Position: secret, Proof: mp, Out: out}
	proof, err := getUnstakeSystemForTx(t).Prove(in)
	if err != nil {
		t.Fatalf("prove unstake: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}

	unstakeTx := types.ShieldedTx{
		Kind:        types.TxUnstake,
		Proof:       proofBytes,
		Nullifier:   types.Hash(zk.ToBytes32(secret.Nullifier())),
		Commitments: []types.Hash{types.Hash(zk.ToBytes32(out.Commitment()))},
		UnstakePublicInputs: &types.UnstakePublicInputs{
			MerkleRoot:  types.Hash(zk.ToBytes32(stakeRoot)),
			Principal:   amount,
			StartEpoch:  1,
			FinalAmount: wantFinal,
		},
	}
	unstakeTx = mustSignWithKey(t, pk, sk, unstakeTx)

	remainingBefore := zkTree.Remaining()
	results := p2.ProcessBatch([]tx.Entry{{Tx: unstakeTx, SubmittedAt: time.Now()}})
	if results[0].Error != nil {
		t.Fatalf("expected a real, correctly-proved unstake to be accepted: %v", results[0].Error)
	}
	if got := zkTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected the real proceeds note to be inserted into the canonical note tree, remaining went from %d to %d", remainingBefore, got)
	}

	spent, err := deps.Store.IsNullifierSpent(unstakeTx.Nullifier)
	if err != nil {
		t.Fatalf("nullifier lookup: %v", err)
	}
	if !spent {
		t.Fatalf("expected the position's nullifier to be marked spent")
	}

	// The identical unstake tx submitted again must be rejected — the
	// position cannot be redeemed twice.
	replay := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind:        types.TxUnstake,
		Proof:       proofBytes,
		Nullifier:   unstakeTx.Nullifier,
		Commitments: unstakeTx.Commitments,
		UnstakePublicInputs: &types.UnstakePublicInputs{
			MerkleRoot:  unstakeTx.UnstakePublicInputs.MerkleRoot,
			Principal:   amount,
			StartEpoch:  1,
			FinalAmount: wantFinal,
		},
	})
	replayResults := p2.ProcessBatch([]tx.Entry{{Tx: replay, SubmittedAt: time.Now()}})
	if replayResults[0].Error == nil {
		t.Fatalf("expected a second unstake of the same position to be rejected (double-unstake)")
	}
}

// TestPipelineUnstakeRejectsForgedFinalAmount proves the real yield
// formula, not a bare proof-presence check, is what Stage 1 enforces: a
// claimed FinalAmount that doesn't match pkg/staking.FinalAmount for the
// real (principal, startEpoch, currentEpoch) is rejected outright, even
// though the proof itself is real and internally well-formed.
func TestPipelineUnstakeRejectsForgedFinalAmount(t *testing.T) {
	deps, _, stakeTree := newDepsWithStaking(t)
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		t.Fatal(err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatal(err)
	}
	secret := zk.StakeSecret{Principal: 1000, StartEpoch: 1, OwnerSK: ownerSK, Rho: rho}
	if _, err := stakeTree.Insert(secret.Commitment()); err != nil {
		t.Fatalf("insert position: %v", err)
	}
	root, err := stakeTree.Root()
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	deps.StakeRoots.Record(root)
	mp, err := stakeTree.Prove(0)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}

	deps.Epoch = 50
	p := tx.NewPipeline(deps)

	outOwnerSK, _ := zk.NewSpendKey()
	outRho, _ := zk.NewRho()
	// Claim a wildly inflated final amount that doesn't match the real
	// formula for this (principal, startEpoch, currentEpoch).
	const forgedFinal = 999999999
	out := zk.NoteSecret{Value: forgedFinal, OwnerSK: outOwnerSK, Rho: outRho}
	in := zk.UnstakeInput{MerkleRoot: root, Position: secret, Proof: mp, Out: out}
	proof, err := getUnstakeSystemForTx(t).Prove(in)
	if err != nil {
		t.Fatalf("prove unstake: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	unstakeTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind:        types.TxUnstake,
		Proof:       proofBytes,
		Nullifier:   types.Hash(zk.ToBytes32(secret.Nullifier())),
		Commitments: []types.Hash{types.Hash(zk.ToBytes32(out.Commitment()))},
		UnstakePublicInputs: &types.UnstakePublicInputs{
			MerkleRoot:  types.Hash(zk.ToBytes32(root)),
			Principal:   1000,
			StartEpoch:  1,
			FinalAmount: forgedFinal,
		},
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: unstakeTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a forged FinalAmount claim to be rejected")
	}
}

// TestPipelineUnstakeRequiresUnstakeZKConfigured proves the fail-closed
// posture: an Unstake transaction is rejected outright when no
// UnstakeZK system is configured, not silently trusted.
func TestPipelineUnstakeRequiresUnstakeZKConfigured(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t) // UnstakeZK/StakeZK/StakeTree left nil
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	unstakeTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind:        types.TxUnstake,
		Proof:       []byte{1, 2, 3},
		Nullifier:   types.Hash{9},
		Commitments: []types.Hash{{8}},
		UnstakePublicInputs: &types.UnstakePublicInputs{
			MerkleRoot:  types.Hash{1},
			Principal:   1000,
			StartEpoch:  1,
			FinalAmount: 1000,
		},
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: unstakeTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected an unstake tx to be rejected when no UnstakeZK system is configured")
	}
}
