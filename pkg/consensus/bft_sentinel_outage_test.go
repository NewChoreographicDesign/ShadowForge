package consensus_test

import (
	"testing"

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
	votes := []types.Vote{
		{StateRoot: root}, {StateRoot: root}, {StateRoot: other},
	}
	endorsements, quorum := consensus.TallyVotes(5, root, votes)
	if endorsements != 2 {
		t.Fatalf("expected 2 endorsements, got %d", endorsements)
	}
	if quorum {
		t.Fatalf("2 of 5 must not reach quorum")
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

	for i := 0; i < 25; i++ {
		o.Enqueue(types.ShieldedTx{TxID: types.Hash{byte(i)}})
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

func TestMegabatchRespectsMultiplierCap(t *testing.T) {
	o := consensus.NewOutageController(consensus.DefaultOutageThresholds())
	for i := 0; i < 100; i++ {
		o.Enqueue(types.ShieldedTx{TxID: types.Hash{byte(i)}})
	}
	batch := o.BuildMegabatch(5) // cap = 5*10 = 50
	if len(batch) != 50 {
		t.Fatalf("expected megabatch capped at 10x normal batch size (50), got %d", len(batch))
	}
	if o.BacklogDepth() != 50 {
		t.Fatalf("expected 50 remaining in backlog, got %d", o.BacklogDepth())
	}
}
