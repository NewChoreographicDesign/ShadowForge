package vault_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
)

func dec(s string) decimal.Decimal { return decimal.MustFromString(s) }

func TestCollectFeeSplits20_10_10_60(t *testing.T) {
	v := vault.New(vault.DefaultSplits())
	v.CollectFee(dec("1000"))
	if v.EpochBonusPool.Cmp(dec("200")) != 0 {
		t.Fatalf("epoch bonus pool = %s, want 200", v.EpochBonusPool)
	}
	if v.BurnedTotal.Cmp(dec("100")) != 0 {
		t.Fatalf("burn = %s, want 100", v.BurnedTotal)
	}
	if v.AuditPool.Cmp(dec("100")) != 0 {
		t.Fatalf("audit pool = %s, want 100", v.AuditPool)
	}
	if v.RemainderPool.Cmp(dec("600")) != 0 {
		t.Fatalf("remainder pool = %s, want 600", v.RemainderPool)
	}
}

func TestCollectBankYieldSplits5050(t *testing.T) {
	v := vault.New(vault.DefaultSplits())
	v.CollectBankYield(dec("100"))
	if v.BankYieldBuybackPool.Cmp(dec("50")) != 0 {
		t.Fatalf("buyback pool = %s, want 50", v.BankYieldBuybackPool)
	}
	if v.BankYieldAirdropPool.Cmp(dec("50")) != 0 {
		t.Fatalf("airdrop pool = %s, want 50", v.BankYieldAirdropPool)
	}
}

func TestMultiTaskBonusMultiplier(t *testing.T) {
	cases := []struct {
		validate, mint bool
		want           string
	}{
		{true, true, "1.20"},
		{true, false, "0.90"},
		{false, true, "0.90"},
		{false, false, "1.0"},
	}
	for _, c := range cases {
		got := vault.MultiTaskBonusMultiplier(c.validate, c.mint)
		if got.Cmp(dec(c.want)) != 0 {
			t.Errorf("MultiTaskBonusMultiplier(%v,%v) = %s, want %s", c.validate, c.mint, got, c.want)
		}
	}
}

func TestEpochBonusFormula(t *testing.T) {
	got := vault.EpochBonus(dec("100"), dec("2"), true, true)
	// 100 * 1.20 * 2 = 240
	if got.Cmp(dec("240")) != 0 {
		t.Fatalf("epoch bonus = %s, want 240", got)
	}
}

func TestDrawEpochBonusPoolCapsAtAvailable(t *testing.T) {
	v := vault.New(vault.DefaultSplits())
	v.CollectFee(dec("100")) // epoch bonus pool = 20
	drawn := v.DrawEpochBonusPool(dec("1000"))
	if drawn.Cmp(dec("20")) != 0 {
		t.Fatalf("expected draw capped at available 20, got %s", drawn)
	}
	if v.EpochBonusPool.Sign() != 0 {
		t.Fatalf("expected pool to be drained, got %s", v.EpochBonusPool)
	}
}
