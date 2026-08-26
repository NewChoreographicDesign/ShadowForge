package container_test

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/container"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func TestRequiredValidatorsBasePlusPerDept(t *testing.T) {
	cfg := container.DefaultConfig("acme", []string{"finance", "hr", "ops"})
	if got := container.RequiredValidators(cfg); got != 20+2*3 {
		t.Fatalf("required validators = %d, want %d", got, 26)
	}
}

func TestHybridSplitCounts(t *testing.T) {
	cfg := container.DefaultConfig("acme", []string{"finance", "hr"}) // 20 + 4 = 24
	internal, public := container.HybridSplitCounts(cfg)
	if internal+public != 24 {
		t.Fatalf("split counts don't sum to total: %d + %d != 24", internal, public)
	}
	if internal != 12 || public != 12 {
		t.Fatalf("expected an even 50/50 split of 24, got internal=%d public=%d", internal, public)
	}
}

func TestShouldSyncOnTPSThreshold(t *testing.T) {
	cfg := container.DefaultConfig("acme", nil)
	s := container.NewSubspace(cfg)
	now := time.Now()
	if s.ShouldSync(now, 500) {
		t.Fatalf("500 TPS under the 1000 threshold and fresh interval should not sync")
	}
	if !s.ShouldSync(now, 1500) {
		t.Fatalf("1500 TPS over threshold should trigger sync")
	}
}

func TestShouldSyncOnIntervalElapsed(t *testing.T) {
	cfg := container.DefaultConfig("acme", nil)
	cfg.SyncInterval = time.Minute
	s := container.NewSubspace(cfg)
	past := time.Now().Add(-2 * time.Minute)
	s.MarkSynced(past)
	if !s.ShouldSync(time.Now(), 10) {
		t.Fatalf("expected sync to trigger once the interval has elapsed, even at low TPS")
	}
}

func TestInternalModeToggle(t *testing.T) {
	s := container.NewSubspace(container.DefaultConfig("acme", nil))
	if s.InternalMode() {
		t.Fatalf("expected internal mode to start false")
	}
	s.EnterInternalMode()
	if !s.InternalMode() {
		t.Fatalf("expected internal mode true after EnterInternalMode")
	}
	s.ExitInternalMode()
	if s.InternalMode() {
		t.Fatalf("expected internal mode false after ExitInternalMode")
	}
}

func TestWhitelistExemptsPayrollBursts(t *testing.T) {
	s := container.NewSubspace(container.DefaultConfig("acme", nil))
	if s.IsWhitelisted("payroll") {
		t.Fatalf("expected payroll not whitelisted by default")
	}
	s.Whitelist("payroll")
	if !s.IsWhitelisted("payroll") {
		t.Fatalf("expected payroll to be whitelisted after Whitelist()")
	}
}

func TestShadowVerifyMismatchBlocksCommit(t *testing.T) {
	a := types.Hash{1, 2, 3}
	b := types.Hash{1, 2, 3}
	c := types.Hash{9, 9, 9}
	if !container.ShadowVerify(a, b) {
		t.Fatalf("matching outputs should verify")
	}
	if container.ShadowVerify(a, c) {
		t.Fatalf("mismatched outputs must not verify")
	}
}
