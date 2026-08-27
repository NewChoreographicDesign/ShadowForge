package consensus_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestBFTQuorumOneValidatorPerStage(t *testing.T) {
	// spec 5.7: "With one validator per stage that is 3 of 5."
	assigned := 5
	if consensus.BFTQuorumMet(assigned, 2) {
		t.Fatalf("2 of 5 must not reach quorum")
	}
	if !consensus.BFTQuorumMet(assigned, 3) {
		t.Fatalf("3 of 5 must reach quorum")
	}
}

func TestBFTQuorumTwoValidatorsPerStage(t *testing.T) {
	// spec 5.7: "With two per stage that is 6 of 10."
	assigned := 10
	if consensus.BFTQuorumMet(assigned, 5) {
		t.Fatalf("5 of 10 must not reach quorum")
	}
	if !consensus.BFTQuorumMet(assigned, 6) {
		t.Fatalf("6 of 10 must reach quorum")
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
	if endorsements != 3 || !quorum {
		t.Fatalf("expected 3/5 to reach quorum, got endorsements=%d quorum=%v", endorsements, quorum)
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
