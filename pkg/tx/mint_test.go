package tx_test

import (
	"sync"
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// mintSystem is likewise expensive to Setup; build it once for the whole
// package's tests.
var (
	mintOnce sync.Once
	mintSys  *zk.MintSystem
	mintErr  error
)

func getMintSystem(t *testing.T) *zk.MintSystem {
	t.Helper()
	mintOnce.Do(func() { mintSys, mintErr = zk.SetupMint() })
	if mintErr != nil {
		t.Fatalf("mint zk setup: %v", mintErr)
	}
	return mintSys
}

// newDepsWithMint builds real pipeline Deps with the real canonical tree
// (newDepsWithCanonicalTree) plus a real, shared MintZK system — the
// configuration a live validator now always runs with.
func newDepsWithMint(t *testing.T) (tx.Deps, *zk.Tree) {
	t.Helper()
	deps, zkTree := newDepsWithCanonicalTree(t)
	deps.MintZK = getMintSystem(t)
	return deps, zkTree
}

// mustBuildMintVote builds a real, well-formed, correctly-proved TxVote
// binding proposalID to a real spec-17.4 mint claim for amount SFG,
// signed by pk/sk. Returns the tx and the real zk.NoteSecret opening the
// claimed output note — the caller needs the secret to later prove the
// note is real (e.g. spend it, or independently recompute its
// commitment).
func mustBuildMintVote(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, elig *types.VoteEligibilityProof, proposalID types.ID, approve bool, amount uint64, sys *zk.MintSystem) (types.ShieldedTx, zk.NoteSecret) {
	t.Helper()
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		t.Fatalf("owner sk: %v", err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatalf("rho: %v", err)
	}
	secret := zk.NoteSecret{Value: types.MintNetAmount(amount), OwnerSK: ownerSK, Rho: rho}

	proof, err := sys.Prove(secret)
	if err != nil {
		t.Fatalf("prove mint: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}

	nonce := types.Hash{9, 9}
	commitment := types.ComputeVoteCommitment(elig.Nullifier, approve, nonce)
	votetx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:    proposalID,
			Commitment:    commitment,
			MintAmount:    amount,
			MintOutCommit: types.Hash(zk.ToBytes32(secret.Commitment())),
			MintProof:     proofBytes,
		},
		VoteEligibility: elig,
	})
	return votetx, secret
}

// TestPipelineMintProposalPassesAndCreatesRealNote is the real,
// end-to-end proof this build's spec-17.4 epoch mint actually works: a
// real mint proposal, approved by real revealed ballots, results in a
// real spendable note landing in the same canonical tree a Transfer's
// own outputs live in, and a real Vault fee collection — not merely a
// persisted tally outcome.
func TestPipelineMintProposalPassesAndCreatesRealNote(t *testing.T) {
	deps, zkTree := newDepsWithMint(t)
	deps.Epoch = 1
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "mint-proposal-1")

	const amount = 1000
	commitTx, secret := mustBuildMintVote(t, pk, sk, elig, "mint-proposal-1", true, amount, getMintSystem(t))
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the mint-bound vote to be accepted: %v", r[0].Error)
	}

	revealTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: "mint-proposal-1", Approve: true, Nonce: types.Hash{9, 9},
		},
		VoteEligibility: elig,
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	remainingBefore := zkTree.Remaining()
	tallied, err := p.TallyDueProposals(2)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || !tallied[0].Passed {
		t.Fatalf("expected the proposal to pass 1-0, got %+v", tallied)
	}
	if !tallied[0].MintApplied {
		t.Fatalf("expected the real mint to be applied")
	}

	// The real proof: the note this test itself built (secret) really is
	// now part of the canonical tree — not just a recorded claim.
	if got := zkTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected exactly 1 new leaf inserted into the canonical tree, remaining went from %d to %d", remainingBefore, got)
	}
	wantCommit := types.Hash(zk.ToBytes32(secret.Commitment()))
	if tallied[0].MintOutCommit != wantCommit {
		t.Fatalf("persisted MintOutCommit %s does not match the real note's own commitment %s", tallied[0].MintOutCommit, wantCommit)
	}

	// Real Vault fee collection: 10% of 1000 = 100, split across the real
	// 20/10/10/60 default pools — summing them back up is the real proof
	// CollectFee actually ran with the real fee amount, not a fabricated
	// or missing one.
	collected := func() decimal.Decimal {
		return deps.Vault.EpochBonusPool.Add(deps.Vault.AuditPool).Add(deps.Vault.RemainderPool).Add(deps.Vault.BurnedTotal)
	}
	if collected().Cmp(decimal.FromInt(100)) != 0 {
		t.Fatalf("expected the Vault to have collected a real 100 SFG mint fee, got %s", collected())
	}

	// Idempotent: tallying again must not re-insert the note or re-collect the fee.
	if _, err := p.TallyDueProposals(3); err != nil {
		t.Fatalf("second tally: %v", err)
	}
	if got := zkTree.Remaining(); got != remainingBefore-1 {
		t.Fatalf("expected no further insertion on a second tally call, remaining=%d", got)
	}
	if collected().Cmp(decimal.FromInt(100)) != 0 {
		t.Fatalf("expected no further fee collection on a second tally call, got %s", collected())
	}
}

// TestPipelineMintProposalRejectsForgedProof proves the real Groth16
// check, not a bare-field-presence check, is what pkg/tx's Stage 4
// enforces: a MintProof that doesn't actually bind MintOutCommit to
// MintAmount — here, one built for a different, larger amount than
// claimed — is rejected outright, and no proposal record is created.
func TestPipelineMintProposalRejectsForgedProof(t *testing.T) {
	deps, _ := newDepsWithMint(t)
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "forged-mint-proposal")

	// Prove a real note of the CLAIMED net amount (900, i.e. claiming
	// amount=1000), but submit a wildly different claimed MintAmount
	// (10) so the pipeline's own recomputed NetAmount (9) doesn't match
	// what the proof actually proves (900).
	ownerSK, err := zk.NewSpendKey()
	if err != nil {
		t.Fatalf("owner sk: %v", err)
	}
	rho, err := zk.NewRho()
	if err != nil {
		t.Fatalf("rho: %v", err)
	}
	secret := zk.NoteSecret{Value: 900, OwnerSK: ownerSK, Rho: rho}
	proof, err := getMintSystem(t).Prove(secret)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	proofBytes, err := zk.ProofToBytes(proof)
	if err != nil {
		t.Fatalf("proof to bytes: %v", err)
	}

	forgedTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID:    "forged-mint-proposal",
			Commitment:    types.ComputeVoteCommitment(elig.Nullifier, true, types.Hash{1}),
			MintAmount:    10, // claims a tiny amount; proof is really for 900
			MintOutCommit: types.Hash(zk.ToBytes32(secret.Commitment())),
			MintProof:     proofBytes,
		},
		VoteEligibility: elig,
	})
	results := p.ProcessBatch([]tx.Entry{{Tx: forgedTx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a mint claim whose proof doesn't match its claimed amount to be rejected")
	}
	se, ok := results[0].Error.(*tx.StageError)
	if !ok || se.Stage != 4 {
		t.Fatalf("expected a stage-4 StageError (mint proof invalid), got %v", results[0].Error)
	}
	if _, found, err := deps.Store.GetProposal("forged-mint-proposal"); err != nil {
		t.Fatalf("get proposal: %v", err)
	} else if found {
		t.Fatalf("expected no proposal record to exist after a forged mint claim was rejected")
	}
}

// TestPipelineMintProposalRequiresMintZKConfigured proves the fail-closed
// posture: a mint-bound vote is rejected outright when no MintZK system
// is configured, not silently trusted.
func TestPipelineMintProposalRequiresMintZKConfigured(t *testing.T) {
	deps, _ := newDepsWithCanonicalTree(t) // MintZK left nil
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "unconfigured-mint-proposal")

	votetx, _ := mustBuildMintVote(t, pk, sk, elig, "unconfigured-mint-proposal", true, 500, getMintSystem(t))
	results := p.ProcessBatch([]tx.Entry{{Tx: votetx, SubmittedAt: time.Now()}})
	if results[0].Error == nil {
		t.Fatalf("expected a mint claim to be rejected when no MintZK system is configured")
	}
}

// TestPipelineMintProposalNotAppliedWhenRejected proves a mint bound to
// a proposal that fails its vote never executes: no note is inserted,
// no fee is collected, MintApplied stays false.
func TestPipelineMintProposalNotAppliedWhenRejected(t *testing.T) {
	deps, zkTree := newDepsWithMint(t)
	deps.Epoch = 1
	p := tx.NewPipeline(deps)

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	voter := seedVoter(t, deps, pk, sk)
	elig := voter.eligibilityFor(t, "rejected-mint-proposal")

	commitTx, _ := mustBuildMintVote(t, pk, sk, elig, "rejected-mint-proposal", false, 1000, getMintSystem(t))
	if r := p.ProcessBatch([]tx.Entry{{Tx: commitTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the mint-bound vote (rejecting) to be accepted as a real ballot: %v", r[0].Error)
	}
	revealTx := mustSignWithKey(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: "rejected-mint-proposal", Approve: false, Nonce: types.Hash{9, 9},
		},
		VoteEligibility: elig,
	})
	if r := p.ProcessBatch([]tx.Entry{{Tx: revealTx, SubmittedAt: time.Now()}}); r[0].Error != nil {
		t.Fatalf("expected the reveal to be accepted: %v", r[0].Error)
	}

	remainingBefore := zkTree.Remaining()
	tallied, err := p.TallyDueProposals(2)
	if err != nil {
		t.Fatalf("tally: %v", err)
	}
	if len(tallied) != 1 || tallied[0].Passed {
		t.Fatalf("expected the proposal to fail (sole voter rejected), got %+v", tallied)
	}
	if tallied[0].MintApplied {
		t.Fatalf("expected MintApplied to stay false for a failed proposal")
	}
	if got := zkTree.Remaining(); got != remainingBefore {
		t.Fatalf("expected no canonical-tree insertion for a failed mint proposal, remaining went from %d to %d", remainingBefore, got)
	}
	collected := deps.Vault.EpochBonusPool.Add(deps.Vault.AuditPool).Add(deps.Vault.RemainderPool).Add(deps.Vault.BurnedTotal)
	if collected.Sign() != 0 {
		t.Fatalf("expected no fee collection for a failed mint proposal, got %s", collected)
	}
}
