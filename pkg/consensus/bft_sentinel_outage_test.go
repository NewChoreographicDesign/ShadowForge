package consensus_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestBFTQuorumOneValidatorPerStage(t *testing.T) {
	// spec 5.7's literal "3 of 5" is a simple majority, not a real
	// Byzantine-safe supermajority — see BFTQuorumMet's own doc and
	// TestBFTQuorumUnsafeAgainstClaimedFaultTolerance. The real, safe
	// threshold for a 5-validator committee (matching
	// BFTFaultTolerance(5) == 1) is 4 of 5.
	assigned := 5
	if consensus.BFTQuorumMet(assigned, 3) {
		t.Fatalf("3 of 5 must not reach quorum (unsafe against 1 equivocating validator)")
	}
	if !consensus.BFTQuorumMet(assigned, 4) {
		t.Fatalf("4 of 5 must reach quorum")
	}
}

func TestBFTQuorumTwoValidatorsPerStage(t *testing.T) {
	// spec 5.7's literal "6 of 10" is likewise unsafe (see
	// BFTQuorumMet's own doc) — the real, safe threshold for a
	// 10-validator committee (matching BFTFaultTolerance(10) == 3) is
	// 7 of 10.
	assigned := 10
	if consensus.BFTQuorumMet(assigned, 6) {
		t.Fatalf("6 of 10 must not reach quorum (unsafe against 3 equivocating validators)")
	}
	if !consensus.BFTQuorumMet(assigned, 7) {
		t.Fatalf("7 of 10 must reach quorum")
	}
}

func TestTallyVotesCountsOnlyMatchingRoot(t *testing.T) {
	root := types.Hash{1}
	other := types.Hash{2}
	committee := []types.NFTID{nftID(1), nftID(2), nftID(3), nftID(4), nftID(5)}
	votes := []types.Vote{
		{Validator: nftID(1), StateRoot: root},
		{Validator: nftID(2), StateRoot: root},
		{Validator: nftID(3), StateRoot: other},
	}
	endorsements, quorum := consensus.TallyVotes(committee, root, votes)
	if endorsements != 2 {
		t.Fatalf("expected 2 endorsements, got %d", endorsements)
	}
	if quorum {
		t.Fatalf("2 of 5 must not reach quorum")
	}
}

func TestTallyVotesIgnoresDuplicateVoteFromSameValidator(t *testing.T) {
	root := types.Hash{1}
	committee := []types.NFTID{nftID(1), nftID(2), nftID(3), nftID(4), nftID(5)}
	// nftID(1) votes three times for the same root — must still only
	// count once, or a single validator could manufacture quorum alone.
	votes := []types.Vote{
		{Validator: nftID(1), StateRoot: root},
		{Validator: nftID(1), StateRoot: root},
		{Validator: nftID(1), StateRoot: root},
	}
	endorsements, quorum := consensus.TallyVotes(committee, root, votes)
	if endorsements != 1 {
		t.Fatalf("expected exactly 1 endorsement despite 3 votes from the same validator, got %d", endorsements)
	}
	if quorum {
		t.Fatalf("1 of 5 must not reach quorum, even padded with duplicate votes")
	}
}

func TestTallyVotesIgnoresNonCommitteeVoter(t *testing.T) {
	root := types.Hash{1}
	committee := []types.NFTID{nftID(1), nftID(2), nftID(3)}
	outsider := nftID(99)
	votes := []types.Vote{
		{Validator: nftID(1), StateRoot: root},
		{Validator: outsider, StateRoot: root},
	}
	endorsements, quorum := consensus.TallyVotes(committee, root, votes)
	if endorsements != 1 {
		t.Fatalf("expected an outsider's vote to be ignored, got %d endorsements", endorsements)
	}
	if quorum {
		t.Fatalf("1 of 3 must not reach quorum")
	}
}

func TestTallyVotesReachesQuorum(t *testing.T) {
	root := types.Hash{1}
	committee := []types.NFTID{nftID(1), nftID(2), nftID(3), nftID(4), nftID(5)}
	votes := []types.Vote{
		{Validator: nftID(1), StateRoot: root},
		{Validator: nftID(2), StateRoot: root},
		{Validator: nftID(3), StateRoot: root},
	}
	endorsements, quorum := consensus.TallyVotes(committee, root, votes)
	if endorsements != 3 || quorum {
		t.Fatalf("expected 3/5 to NOT reach the real, safe quorum, got endorsements=%d quorum=%v", endorsements, quorum)
	}

	votes = append(votes, types.Vote{Validator: nftID(4), StateRoot: root})
	endorsements, quorum = consensus.TallyVotes(committee, root, votes)
	if endorsements != 4 || !quorum {
		t.Fatalf("expected 4/5 to reach quorum, got endorsements=%d quorum=%v", endorsements, quorum)
	}
}

// TestBFTQuorumUnsafeAgainstClaimedFaultTolerance is a real, independent
// audit finding (Phase 2): BFTFaultTolerance(assigned) claims the protocol
// "tolerates up to one third faulty nodes" (spec 5.1), but a simple-majority
// quorum (votes*2 > assigned, spec 5.7's literal "3 of 5" / "6 of 10") is
// not sufficient to guarantee that against Byzantine (not just crash-fault)
// validators — a validator that equivocates (signs two different candidate
// roots at the same height, exactly what "Byzantine" as opposed to
// "crash-fault" means) can be double-counted across two disjoint tallies.
//
// With a committee of 5 and BFTFaultTolerance(5) == 1 equivocating member:
// two honest validators vote for root A, two different honest validators
// vote for root B, and the one Byzantine validator signs both. That is
// exactly 1 Byzantine validator among 5 — the precise count this codebase's
// own BFTFaultTolerance claims to tolerate — yet under a simple-majority
// quorum both A and B independently reach "3 of 5", i.e. two conflicting
// blocks can both satisfy chain.Append's quorum check at the same height:
// a real safety (double-finalization) violation, not a liveness nitpick.
//
// A safe BFT quorum must instead require agreement to exceed (assigned+f)/2
// votes, which for f == BFTFaultTolerance(assigned) works out to the
// classic ">2/3" supermajority; this test asserts that real safety
// invariant directly (at most one of two disjoint-honest-voter candidates
// may reach quorum when the equivocator count is within BFTFaultTolerance),
// which the pre-fix majority rule fails.
func TestBFTQuorumUnsafeAgainstClaimedFaultTolerance(t *testing.T) {
	committee := []types.NFTID{nftID(1), nftID(2), nftID(3), nftID(4), nftID(5)}
	byzantine := nftID(5)
	if f := consensus.BFTFaultTolerance(len(committee)); f != 1 {
		t.Fatalf("test assumes BFTFaultTolerance(5) == 1, got %d", f)
	}

	rootA := types.Hash{0xAA}
	rootB := types.Hash{0xBB}

	votesForA := []types.Vote{
		{Validator: nftID(1), StateRoot: rootA},
		{Validator: nftID(2), StateRoot: rootA},
		{Validator: byzantine, StateRoot: rootA},
	}
	votesForB := []types.Vote{
		{Validator: nftID(3), StateRoot: rootB},
		{Validator: nftID(4), StateRoot: rootB},
		{Validator: byzantine, StateRoot: rootB},
	}

	_, quorumA := consensus.TallyVotes(committee, rootA, votesForA)
	_, quorumB := consensus.TallyVotes(committee, rootB, votesForB)
	if quorumA && quorumB {
		t.Fatalf("safety violation: two conflicting candidates (A and B) both reached quorum "+
			"using only %d equivocating validator(s), within BFTFaultTolerance(%d)=%d — "+
			"two different blocks could both be finalized at the same height",
			1, len(committee), consensus.BFTFaultTolerance(len(committee)))
	}
}

func TestSentinelManagerActivatesAndWithdraws(t *testing.T) {
	sm := consensus.NewSentinelManager()
	if act := sm.Evaluate(15, 1000); act != consensus.ActionNone {
		t.Fatalf("15 online civilians: expected ActionNone, got %v", act)
	}
	if act := sm.Evaluate(5, 2000); act != consensus.ActionActivate {
		t.Fatalf("5 online civilians: expected ActionActivate, got %v", act)
	}
	if !sm.Active() {
		t.Fatalf("expected sentinels to be active")
	}
	// Repeated low count should not re-activate (already active).
	if act := sm.Evaluate(3, 3000); act != consensus.ActionNone {
		t.Fatalf("still low: expected ActionNone (already active), got %v", act)
	}
	if act := sm.Evaluate(12, 4000); act != consensus.ActionWithdraw {
		t.Fatalf("recovered to 12: expected ActionWithdraw, got %v", act)
	}
	if sm.Active() {
		t.Fatalf("expected sentinels to be withdrawn")
	}
}

func TestSentinelActivationsInWindowMetric(t *testing.T) {
	sm := consensus.NewSentinelManager()
	sm.Evaluate(1, 1000)  // activate
	sm.Evaluate(20, 2000) // withdraw
	sm.Evaluate(1, 3000)  // activate again
	sm.Evaluate(20, 4000) // withdraw
	if n := sm.ActivationsInWindow(0, 5000); n != 2 {
		t.Fatalf("expected 2 activations in window, got %d", n)
	}
	if n := sm.ActivationsInWindow(1500, 5000); n != 1 {
		t.Fatalf("expected 1 activation in narrowed window, got %d", n)
	}
}

func TestOutageDetectionThreshold(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	if o.DetectOutage(100, 50) {
		t.Fatalf("exactly 50%% missing must not exceed the >50%% threshold")
	}
	if !o.DetectOutage(100, 51) {
		t.Fatalf("51%% missing must trigger outage detection")
	}
}

func TestOutageRecoveryPipeline(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	o.Declare()
	if !o.Active() {
		t.Fatalf("expected OutageFlag to be set after Declare")
	}

	now := time.Now()
	for i := 0; i < 25; i++ {
		if err := o.Enqueue(types.ShieldedTx{TxID: types.Hash{byte(i)}}, now); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	if o.BacklogDepth() != 25 {
		t.Fatalf("expected backlog depth 25, got %d", o.BacklogDepth())
	}

	// Clearing before any clean cycle must fail even with an empty backlog.
	batch := o.BuildMegabatch(5) // normalBatchSize=5 -> up to 50 drained
	if len(batch) != 25 {
		t.Fatalf("expected megabatch to drain all 25 backlogged txs (limit 50), got %d", len(batch))
	}
	if o.BacklogDepth() != 0 {
		t.Fatalf("expected empty backlog after full drain, got %d", o.BacklogDepth())
	}
	if o.MaybeClear() {
		t.Fatalf("must not clear before a clean dual-track cycle has committed")
	}

	o.RecordCleanDualTrackCycle()
	if !o.MaybeClear() {
		t.Fatalf("expected OutageFlag to clear once backlog is drained and a clean cycle committed")
	}
	if o.Active() {
		t.Fatalf("expected OutageFlag to be false after MaybeClear")
	}
}

// TestOutageEnqueueRejectsDuplicateWithinTTL proves Enqueue dedups the way
// pkg/tx.Mempool.Submit does — the property a real validator's gossip
// forwarding of backlogged transactions (pkg/validator's handleMessage,
// wired identically to how it already forwards live TxOffers) relies on to
// avoid re-enqueuing the same backlogged tx forever as it echoes between
// peers.
func TestOutageEnqueueRejectsDuplicateWithinTTL(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	now := time.Now()
	tx := types.ShieldedTx{TxID: types.Hash{7}}
	if err := o.Enqueue(tx, now); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := o.Enqueue(tx, now.Add(time.Second)); err != consensus.ErrDuplicateBacklogTx {
		t.Fatalf("expected ErrDuplicateBacklogTx for a resubmitted TxID, got %v", err)
	}
	if o.BacklogDepth() != 1 {
		t.Fatalf("a rejected duplicate must not grow the backlog, depth=%d", o.BacklogDepth())
	}
}

// TestOutageEnqueueRejectsOversizedTx is a real, independent pentest
// finding: before this fix, Enqueue had no per-transaction size check at
// all, unlike pkg/tx.Mempool.Submit's MaxTxSize bound — and Enqueue is
// reachable from any connected peer's TxOffer while an outage is active
// (pkg/validator's handleMessage routes there instead of the live mempool
// whenever OutageController.Active()), so this was a real, unauthenticated,
// remote memory-exhaustion vector, active during exactly the kind of real
// network stress an attacker might exploit.
func TestOutageEnqueueRejectsOversizedTx(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	now := time.Now()
	oversized := types.ShieldedTx{TxID: types.Hash{1}, Memo: make([]byte, 257*1024)}
	if err := o.Enqueue(oversized, now); err != consensus.ErrBacklogTxTooLarge {
		t.Fatalf("expected ErrBacklogTxTooLarge for an oversized transaction, got %v", err)
	}
	if o.BacklogDepth() != 0 {
		t.Fatalf("a rejected oversized transaction must not be backlogged, depth=%d", o.BacklogDepth())
	}
}

// TestOutageEnqueueRejectsWhenBacklogFull is the count-cap counterpart to
// TestOutageEnqueueRejectsOversizedTx — the same real pentest finding
// applies to total backlog depth, not just one transaction's size: before
// this fix, the backlog had no ceiling whatsoever, unlike pkg/tx.Mempool's
// MaxSize.
func TestOutageEnqueueRejectsWhenBacklogFull(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	now := time.Now()
	for i := 0; i < 100_000; i++ {
		id := types.Hash{byte(i), byte(i >> 8), byte(i >> 16)}
		if err := o.Enqueue(types.ShieldedTx{TxID: id}, now); err != nil {
			t.Fatalf("enqueue %d: unexpected error %v", i, err)
		}
	}
	if err := o.Enqueue(types.ShieldedTx{TxID: types.Hash{0xFF, 0xFF, 0xFF}}, now); err != consensus.ErrBacklogFull {
		t.Fatalf("expected ErrBacklogFull once the backlog is at capacity, got %v", err)
	}
	if o.BacklogDepth() != 100_000 {
		t.Fatalf("rejected submission past capacity must not grow the backlog, depth=%d", o.BacklogDepth())
	}
}

// TestOutageReinsertBypassesDuplicateCheck proves Reinsert (used by a real
// validator's dual-track proposal builder to return backlog entries that
// didn't fit a byte-bounded recovery batch) succeeds even though Enqueue
// would reject the same TxID as a duplicate — this is the backlog's own
// entry coming back, not an external resubmission. Mirrors
// pkg/tx.Mempool's identical Submit/Reinsert split.
func TestOutageReinsertBypassesDuplicateCheck(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	now := time.Now()
	tr := types.ShieldedTx{TxID: types.Hash{3}}
	if err := o.Enqueue(tr, now); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	drained := o.BuildMegabatch(1) // drains it out, simulating a proposer draining the backlog
	if len(drained) != 1 {
		t.Fatalf("expected the megabatch to drain the one entry, got %d", len(drained))
	}

	if err := o.Enqueue(tr, now); err != consensus.ErrDuplicateBacklogTx {
		t.Fatalf("expected Enqueue to still reject this TxID as a duplicate, got %v", err)
	}
	o.Reinsert(tr)
	if o.BacklogDepth() != 1 {
		t.Fatalf("expected the reinserted tx to be pending again, len=%d", o.BacklogDepth())
	}
}

// TestOutageRemoveDropsMatchingEntriesOnly proves Remove drops exactly
// the backlogged entries whose TxID matches, leaving everything else —
// the outage-backlog equivalent of pkg/tx.Mempool.Remove's own fix for
// the same real multi-node liveness bug (a node that receives a backlog
// tx via gossip but never itself drains it into a proposal keeps a stale
// copy after the tx is durably committed elsewhere).
func TestOutageRemoveDropsMatchingEntriesOnly(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	now := time.Now()
	keep := types.ShieldedTx{TxID: types.Hash{1}}
	drop1 := types.ShieldedTx{TxID: types.Hash{2}}
	drop2 := types.ShieldedTx{TxID: types.Hash{3}}
	for _, e := range []types.ShieldedTx{keep, drop1, drop2} {
		if err := o.Enqueue(e, now); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	o.Remove([]types.Hash{drop1.TxID, drop2.TxID})

	if o.BacklogDepth() != 1 {
		t.Fatalf("expected 1 remaining backlog entry, got %d", o.BacklogDepth())
	}
	batch := o.BuildMegabatch(10)
	if len(batch) != 1 || batch[0].TxID != keep.TxID {
		t.Fatalf("expected only the untouched entry to remain, got %+v", batch)
	}
}

// TestOutageRemoveDoesNotClearDedupRecord mirrors
// TestMempoolRemoveDoesNotClearDedupRecord: a late-arriving duplicate
// gossip echo for an already-committed TxID must still be recognized and
// rejected by Enqueue, not silently re-admitted as new.
func TestOutageRemoveDoesNotClearDedupRecord(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	now := time.Now()
	tr := types.ShieldedTx{TxID: types.Hash{9}}
	if err := o.Enqueue(tr, now); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	o.Remove([]types.Hash{tr.TxID})
	if err := o.Enqueue(tr, now.Add(time.Second)); err != consensus.ErrDuplicateBacklogTx {
		t.Fatalf("expected a late duplicate to still be rejected after Remove, got %v", err)
	}
}

func TestMegabatchRespectsMultiplierCap(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	now := time.Now()
	for i := 0; i < 100; i++ {
		if err := o.Enqueue(types.ShieldedTx{TxID: types.Hash{byte(i)}}, now); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	batch := o.BuildMegabatch(5) // cap = 5*10 = 50
	if len(batch) != 50 {
		t.Fatalf("expected megabatch capped at 10x normal batch size (50), got %d", len(batch))
	}
	if o.BacklogDepth() != 50 {
		t.Fatalf("expected 50 remaining in backlog, got %d", o.BacklogDepth())
	}
}
