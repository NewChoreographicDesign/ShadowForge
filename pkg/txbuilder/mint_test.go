package txbuilder_test

import (
	"sync"
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

var (
	mintOnce sync.Once
	mintSys  *zk.MintSystem
	mintErr  error
)

func realMintSystem(t *testing.T) *zk.MintSystem {
	t.Helper()
	mintOnce.Do(func() { mintSys, mintErr = zk.SetupMint() })
	if mintErr != nil {
		t.Fatalf("mint zk setup: %v", mintErr)
	}
	return mintSys
}

// newMintPipeline is newRealPipeline plus the real canonical
// ZKTree/ZKRoots and a real, shared MintZK system — what a passed mint
// proposal actually inserts its new note into (pkg/tx's Stage 4/
// TallyDueProposals), mirroring pkg/tx's own newDepsWithMint test
// helper.
func newMintPipeline(t *testing.T) (*tx.Pipeline, tx.Deps, *zk.Tree) {
	t.Helper()
	_, deps := newRealPipeline(t, tx.Deps{})
	zkTree := zk.NewTree()
	initialRoot, err := zkTree.Root()
	if err != nil {
		t.Fatalf("initial zk tree root: %v", err)
	}
	deps.ZKTree = zkTree
	deps.ZKRoots = zk.NewRootHistory(initialRoot)
	deps.MintZK = realMintSystem(t)
	p := tx.NewPipeline(deps)
	return p, deps, zkTree
}

func TestProposeMintAcceptedByRealPipelineAndCreatesRealNote(t *testing.T) {
	p, deps, zkTree := newMintPipeline(t)
	b := newVoterIdentity(t, deps)
	elig := b.eligibilityFor(t, "builder-mint-1")

	votetx, secret, err := b.ProposeMint("builder-mint-1", true, 2000, realMintSystem(t), elig)
	if err != nil {
		t.Fatalf("build propose-mint: %v", err)
	}
	assertRealSignature(t, votetx)
	if err := runOne(p, votetx); err != nil {
		t.Fatalf("expected the real pipeline to accept a well-formed mint proposal: %v", err)
	}
	if votetx.VotePublicInputs.MintAmount != 2000 {
		t.Fatalf("expected MintAmount 2000, got %d", votetx.VotePublicInputs.MintAmount)
	}
	if secret.Value != types.MintNetAmount(2000) {
		t.Fatalf("expected the returned note secret's value to be the real net amount %d, got %d", types.MintNetAmount(2000), secret.Value)
	}
	wantCommit := types.Hash(zk.ToBytes32(secret.Commitment()))
	if votetx.VotePublicInputs.MintOutCommit != wantCommit {
		t.Fatalf("expected MintOutCommit to match the returned secret's own real commitment")
	}

	record, found, err := deps.Store.GetProposal("builder-mint-1")
	if err != nil || !found {
		t.Fatalf("expected a real proposal record: found=%v err=%v", found, err)
	}
	if record.MintAmount != 2000 || record.MintOutCommit != wantCommit {
		t.Fatalf("expected the proposal record to persist the real mint claim, got %+v", record)
	}

	// Real reveal + real tally executes the mint end to end.
	revealtx, err := b.VoteReveal("builder-mint-1", true, elig)
	if err != nil {
		t.Fatalf("build reveal: %v", err)
	}
	if err := runOne(p, revealtx); err != nil {
		t.Fatalf("expected the real pipeline to accept the matching reveal: %v", err)
	}
	remainingBefore := zkTree.Remaining()
	tallied, err := p.TallyDueProposals(deps.Epoch + 1)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed || !tallied[0].MintApplied {
		t.Fatalf("expected the proposal to pass and the real mint to be applied, got %+v", tallied)
	}
	if got := zkTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected the real note to be inserted into the canonical tree, remaining went from %d to %d", remainingBefore, got)
	}
}

func TestProposeMintRejectsEmptyProposalID(t *testing.T) {
	b := newIdentity(t)
	if _, _, err := b.ProposeMint("", true, 1000, realMintSystem(t), types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected an empty proposal id to be rejected")
	}
}

func TestProposeMintRejectsZeroAmount(t *testing.T) {
	b := newIdentity(t)
	if _, _, err := b.ProposeMint("builder-mint-zero", true, 0, realMintSystem(t), types.VoteEligibilityProof{}); err == nil {
		t.Fatalf("expected a zero amount to be rejected")
	}
}
