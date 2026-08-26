package bank_test

import (
	"testing"

	"github.com/shadowforge/shadowforge-l1/pkg/bank"
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

func dec(s string) decimal.Decimal { return decimal.MustFromString(s) }

// TestDepositExactMath walks a worked example through spec 19.3 by hand:
// Q=1 BTC, P=$60,000, ATR=$2,000.
//
//	GrossUSD = 60,000
//	Buffer   = 2.5 * 2,000 = 5,000
//	Net      = 55,000
//	EntryFee = 0.001 * 55,000 = 55
//	SFGIssued = (55,000 - 55) / 5  [SFG at $5]  = 10,989 exactly
func TestDepositExactMath(t *testing.T) {
	hold, err := bank.Deposit(bank.DepositParams{
		Asset:          types.AssetBTC,
		ExternalAmount: dec("1"),
		PriceUSD:       dec("60000"),
		ATRUSD:         dec("2000"),
		SFGUSDPrice:    dec("5"),
		Now:            1000,
	})
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	if hold.EntryBuffer.Cmp(dec("5000")) != 0 {
		t.Fatalf("buffer = %s, want 5000", hold.EntryBuffer)
	}
	if hold.EntryFee.Cmp(dec("55")) != 0 {
		t.Fatalf("fee = %s, want 55", hold.EntryFee)
	}
	// (55000 - 55) / 5 = 10989 exactly.
	if hold.SFGIssued != 10989 {
		t.Fatalf("SFGIssued = %d, want 10989", hold.SFGIssued)
	}
	if hold.Status != types.HoldLocked24h {
		t.Fatalf("expected Status=Locked24h immediately after deposit, got %s", hold.Status)
	}
}

func TestDepositRejectsWhenBufferExceedsGross(t *testing.T) {
	_, err := bank.Deposit(bank.DepositParams{
		ExternalAmount: dec("0.01"),
		PriceUSD:       dec("60000"), // gross = 600
		ATRUSD:         dec("2000"),  // buffer = 5000 >= 600
		SFGUSDPrice:    dec("5"),
		Now:            1000,
	})
	if err != bank.ErrBufferExceedsGross {
		t.Fatalf("expected ErrBufferExceedsGross, got %v", err)
	}
}

func TestDepositRejectsWhenDailyCapExceeded(t *testing.T) {
	_, err := bank.Deposit(bank.DepositParams{
		ExternalAmount: dec("2"),
		PriceUSD:       dec("60000"), // gross = 120,000
		ATRUSD:         dec("100"),
		SFGUSDPrice:    dec("5"),
		Now:            1000,
		DailyUsedUSD:   dec("0"),
	})
	if err != bank.ErrDailyCapExceeded {
		t.Fatalf("expected ErrDailyCapExceeded, got %v", err)
	}
}

func TestDepositSurchargeAfterThreeCyclesIn30Days(t *testing.T) {
	base, err := bank.Deposit(bank.DepositParams{
		ExternalAmount: dec("1"), PriceUSD: dec("60000"), ATRUSD: dec("100"),
		SFGUSDPrice: dec("5"), Now: 1000, CycleCount30d: 1,
	})
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	surcharged, err := bank.Deposit(bank.DepositParams{
		ExternalAmount: dec("1"), PriceUSD: dec("60000"), ATRUSD: dec("100"),
		SFGUSDPrice: dec("5"), Now: 1000, CycleCount30d: 4,
	})
	if err != nil {
		t.Fatalf("deposit: %v", err)
	}
	if surcharged.EntryFee.Cmp(base.EntryFee) <= 0 {
		t.Fatalf("expected surcharged fee (%s) > base fee (%s) after >3 cycles/30d", surcharged.EntryFee, base.EntryFee)
	}
}

func TestWithdrawRefundNeverNegative(t *testing.T) {
	hold := types.BankHold{
		EntryBuffer:    dec("100"),
		EntryPriceUSD:  dec("60000"),
		SFGIssued:      1000,
		OpenedAt:       0,
		DailySnapshots: []types.ATRPoint{{ATRUSD: dec("1000")}}, // huge retention
	}
	res, err := bank.Withdraw(bank.WithdrawParams{
		Hold:        hold,
		PriceNowUSD: dec("60000"),
		Now:         int64(bank.DepositLockDuration.Milliseconds()) + 1,
	})
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if res.RefundSFG.Sign() < 0 {
		t.Fatalf("refund must never be negative, got %s", res.RefundSFG)
	}
	if res.RefundSFG.Sign() != 0 {
		t.Fatalf("expected zero refund when retention exceeds buffer, got %s", res.RefundSFG)
	}
}

func TestWithdraw24HourLockEnforced(t *testing.T) {
	hold := types.BankHold{
		EntryBuffer: dec("100"), EntryPriceUSD: dec("60000"), SFGIssued: 1000,
		OpenedAt: 0, DailySnapshots: []types.ATRPoint{{ATRUSD: dec("10")}},
		Status: types.HoldLocked24h,
	}
	_, err := bank.Withdraw(bank.WithdrawParams{
		Hold: hold, PriceNowUSD: dec("60000"), Now: 1000, // 1 second later
	})
	if err != bank.ErrHoldStillLocked {
		t.Fatalf("expected ErrHoldStillLocked, got %v", err)
	}
}

func TestWithdrawAsymmetryDepreciationIncreasesRepay(t *testing.T) {
	hold := types.BankHold{
		EntryBuffer: dec("100"), EntryPriceUSD: dec("60000"), SFGIssued: 1000,
		OpenedAt: 0, DailySnapshots: []types.ATRPoint{{ATRUSD: dec("10")}},
		Status: types.HoldLocked24h,
	}
	after24h := int64(bank.DepositLockDuration.Milliseconds()) + 1

	same, err := bank.Withdraw(bank.WithdrawParams{Hold: hold, PriceNowUSD: dec("60000"), Now: after24h})
	if err != nil {
		t.Fatalf("withdraw (same price): %v", err)
	}
	depreciated, err := bank.Withdraw(bank.WithdrawParams{Hold: hold, PriceNowUSD: dec("30000"), Now: after24h})
	if err != nil {
		t.Fatalf("withdraw (depreciated): %v", err)
	}
	appreciated, err := bank.Withdraw(bank.WithdrawParams{Hold: hold, PriceNowUSD: dec("120000"), Now: after24h})
	if err != nil {
		t.Fatalf("withdraw (appreciated): %v", err)
	}
	if depreciated.RepaySFG.Cmp(same.RepaySFG) <= 0 {
		t.Fatalf("depreciation must increase repay: depreciated=%s same=%s", depreciated.RepaySFG, same.RepaySFG)
	}
	if appreciated.RepaySFG.Cmp(same.RepaySFG) != 0 {
		t.Fatalf("appreciation must not change repay below/above the entry-equivalent baseline: appreciated=%s same=%s", appreciated.RepaySFG, same.RepaySFG)
	}
}

func TestWithdrawRefundCapAt80Percent(t *testing.T) {
	// Retention is tiny, so the uncapped refund would be ~100 (~all of the
	// buffer). An 80% cap should limit it to 80.
	hold := types.BankHold{
		EntryBuffer: dec("100"), EntryPriceUSD: dec("60000"), SFGIssued: 1000,
		OpenedAt: 0, DailySnapshots: []types.ATRPoint{{ATRUSD: dec("0.001")}},
	}
	res, err := bank.Withdraw(bank.WithdrawParams{
		Hold: hold, PriceNowUSD: dec("60000"),
		Now:       int64(bank.DepositLockDuration.Milliseconds()) + 1,
		RefundCap: dec("0.8"),
	})
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if res.RefundSFG.Cmp(dec("80")) != 0 {
		t.Fatalf("expected refund capped at 80, got %s", res.RefundSFG)
	}
}

func TestAverageATRAcrossSnapshots(t *testing.T) {
	hold := types.BankHold{DailySnapshots: []types.ATRPoint{
		{ATRUSD: dec("10")}, {ATRUSD: dec("20")}, {ATRUSD: dec("30")},
	}}
	if got := bank.AverageATR(hold); got.Cmp(dec("20")) != 0 {
		t.Fatalf("average ATR = %s, want 20", got)
	}
}
