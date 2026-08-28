package txbuilder_test

import (
	"sync"
	"testing"

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

func realStakeSystem(t *testing.T) *zk.StakeSystem {
	t.Helper()
	stakeSysOnce.Do(func() { stakeSysShared, stakeSysErr = zk.SetupStake() })
	if stakeSysErr != nil {
		t.Fatalf("stake zk setup: %v", stakeSysErr)
	}
	return stakeSysShared
}

func realUnstakeSystem(t *testing.T) *zk.UnstakeSystem {
	t.Helper()
	unstakeSysOnce.Do(func() { unstakeSysShared, unstakeSysErr = zk.SetupUnstake() })
	if unstakeSysErr != nil {
		t.Fatalf("unstake zk setup: %v", unstakeSysErr)
	}
	return unstakeSysShared
}

// newStakePipeline is newMintPipeline plus a real, shared stake-commitment
// tree and StakeZK/UnstakeZK systems — what a passed staked-mint proposal
// actually inserts its new position into (pkg/tx's Stage 4/
// TallyDueProposals), and what a later Unstake redeems it against.
func newStakePipeline(t *testing.T) (*tx.Pipeline, tx.Deps, *zk.Tree, *zk.Tree) {
	t.Helper()
	_, deps, zkTree := newMintPipeline(t)
	deps.StakeZK = realStakeSystem(t)
	deps.UnstakeZK = realUnstakeSystem(t)
	stakeTree := zk.NewTree()
	initialRoot, err := stakeTree.Root()
	if err != nil {
		t.Fatalf("initial stake tree root: %v", err)
	}
	deps.StakeTree = stakeTree
	deps.StakeRoots = zk.NewRootHistory(initialRoot)
	p := tx.NewPipeline(deps)
	return p, deps, zkTree, stakeTree
}

func TestProposeMintStakedAndUnstakeEndToEnd(t *testing.T) {
	p, deps, zkTree, stakeTree := newStakePipeline(t)
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "builder-staked-1")

	const amount = 8000
	votetx, position, err := b.ProposeMintStaked("builder-staked-1", true, amount, deps.Epoch, realStakeSystem(t), elig)
	if err != nil {
		t.Fatalf("build propose-mint-staked: %v", err)
	}
	assertRealSignature(t, votetx)
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed staked mint proposal: %v", err)
	}
	if position.Principal != amount {
		t.Fatalf("expected position principal %d, got %d", amount, position.Principal)
	}

	revealtx, err := b.VoteReveal("builder-staked-1", true, elig)
	if err != nil {
		t.Fatalf("build reveal: %v", err)
	}
	if err := runOne(p, revealtx); err != nil {
		t.Fatalf("expected the real pipeline to accept the matching reveal: %v", err)
	}

	remainingBefore := stakeTree.Remaining()
	tallied, err := p.TallyDueProposals(deps.Epoch + 1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].MintApplied {
		t.Fatalf("expected the proposal to pass and the real position to be applied, got %+v", tallied)
	}
	if got := stakeTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected the real position to land in the stake tree, remaining went from %d to %d", remainingBefore, got)
	}

	// Real membership proof for the position that landed.
	stakeRoot, err := stakeTree.Root()
	if err != nil {
		t.Fatalf("stake root: %v", err)
	}
	mp, err := stakeTree.Prove(0)
	if err != nil {
		t.Fatalf("build stake membership proof: %v", err)
	}

	// A later real pipeline (a later epoch) is what actually processes the
	// unstake — mirrors how a live validator's own Deps.Epoch advances.
	deps.Epoch += 50
	p2 := tx.NewPipeline(deps)
	c := newVoterIdentity(t, deps)

	unstaketx, out, err := c.Unstake(position, mp, stakeRoot, deps.Epoch, realUnstakeSystem(t))
	if err != nil {
		t.Fatalf("build unstake: %v", err)
	}
	assertRealSignature(t, unstaketx)
	wantFinal := staking.FinalAmount(amount, 1, deps.Epoch)
	if out.Value != wantFinal {
		t.Fatalf("expected the real proceeds note to carry the real formula's amount %d, got %d", wantFinal, out.Value)
	}

	remainingNotesBefore := zkTree.Remaining()
	if err := runOne(p2, unstaketx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed unstake: %v", err)
	}
	if got := zkTree.Remaining(); got != remainingNotesBefore-1 {
		t.Fatalf("expected the real proceeds note to land in the canonical note tree, remaining went from %d to %d", remainingNotesBefore, got)
	}
}

func TestProposeMintStakedRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, _, err := b.ProposeMintStaked("", true, 1000, 1, realStakeSystem(t), types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}

func TestProposeMintStakedRejectsZeroAmount(t *testing.T) {
	b := newIdentity(t)
	if _, _, err := b.ProposeMintStaked("builder-staked-zero", true, 0, 1, realStakeSystem(t), types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected a zero amount to be rejected")
	}
}
