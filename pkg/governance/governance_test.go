package governance_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/governance"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestDefaultParamsMatchSpecTable(t *testing.T) {
	p := governance.Default()
	if p.SentinelThreshold != 10 {
		t.Fatalf("sentinel threshold = %d, want 10", p.SentinelThreshold)
	}
	if p.DepositATRMultiple.Cmp(decimal.MustFromString("2.5")) != 0 {
		t.Fatalf("deposit ATR multiple = %s, want 2.5", p.DepositATRMultiple)
	}
	if p.VaultEpochBonusShare.Add(p.VaultBurnShare).Add(p.VaultAuditShare).Add(p.VaultRemainderShare).Cmp(decimal.FromInt(1)) != 0 {
		t.Fatalf("vault splits must sum to 1")
	}
}

func TestTallySimpleMajority(t *testing.T) {
	ballots := []governance.Ballot{
		{Voter: types.NFTID{1}, Approve: true},
		{Voter: types.NFTID{2}, Approve: true},
		{Voter: types.NFTID{3}, Approve: false},
	}
	res := governance.Tally(ballots, 10, decimal.Zero)
	if !res.Passed {
		t.Fatalf("expected proposal to pass with 2-1 approval")
	}
	if res.Turnout.Cmp(decimal.MustFromString("0.3")) != 0 {
		t.Fatalf("turnout = %s, want 0.3", res.Turnout)
	}
}

func TestTallyDeduplicatesVoter(t *testing.T) {
	ballots := []governance.Ballot{
		{Voter: types.NFTID{1}, Approve: true},
		{Voter: types.NFTID{1}, Approve: false}, // duplicate vote, first counts
	}
	res := governance.Tally(ballots, 10, decimal.Zero)
	if res.Approve != 1 || res.Reject != 0 {
		t.Fatalf("expected only the first ballot to count, got approve=%d reject=%d", res.Approve, res.Reject)
	}
}

func TestTallyFailsBelowMinTurnout(t *testing.T) {
	ballots := []governance.Ballot{{Voter: types.NFTID{1}, Approve: true}}
	res := governance.Tally(ballots, 100, decimal.MustFromString("0.20"))
	if res.Passed {
		t.Fatalf("1%% turnout must fail a 20%% minimum-turnout requirement")
	}
}
