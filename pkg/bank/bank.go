// Package bank implements the ShadowForge Bank: the only supported way for
// volatile external assets to become in-network value (spec section 11).
// The exact deposit/withdraw math is taken from spec 19.3/19.4.
package bank

import (
	"errors"
	"fmt"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// Governance-default parameters (spec section 22 "genesis defaults").
var (
	DepositATRMultiple  = decimal.MustFromString("2.5")
	WithdrawATRMultiple = decimal.MustFromString("1.5")
	EntryFeeRate        = decimal.MustFromString("0.001") // 0.1% in
	ExitFeeRate         = decimal.MustFromString("0.001") // 0.1% out
	CycleSurchargeRate  = decimal.MustFromString("0.01")  // +1% after >3 cycles/30d
	RefundCapDefault    = decimal.MustFromString("1.0")   // 100%; governance may lower to 0.8
	SlippageMin         = decimal.MustFromString("0.002")
	SlippageMax         = decimal.MustFromString("0.005")
)

const (
	DepositLockDuration     = 24 * time.Hour
	DailyCapUSD             = 100_000
	CycleSurchargeThreshold = 3 // more than 3 cycles in 30 days triggers the surcharge
)

var (
	ErrBufferExceedsGross = errors.New("bank: volatility buffer exceeds gross deposit value")
	ErrDailyCapExceeded   = errors.New("bank: daily Bank cap exceeded for this wallet")
	ErrHoldStillLocked    = errors.New("bank: hold is still within its 24-hour lock")
	ErrHoldNotOpen        = errors.New("bank: hold is not open for withdrawal")
)

// DepositParams are the inputs to Deposit (spec 11.1).
type DepositParams struct {
	Owner          types.Address
	Asset          types.AssetID
	ExternalAmount decimal.Decimal // Q
	PriceUSD       decimal.Decimal // P, oracle snapshot
	ATRUSD         decimal.Decimal // current ATR, USD
	SFGUSDPrice    decimal.Decimal
	Now            int64 // unix ms

	// Daily-cap safeguard inputs (spec 11.3): the wallet's USD-equivalent
	// Bank usage so far today, the cap that applies (DailyCapUSD unless
	// KYC has raised it), and whether >3 cycles in the last 30 days apply
	// the surcharge to this deposit's fee.
	DailyUsedUSD  decimal.Decimal
	DailyCapUSD   decimal.Decimal
	CycleCount30d uint
}

// Deposit implements spec 11.1 / 19.3 exactly:
//
//	GrossUSD = Q * P
//	Buffer   = 2.5 * currentATR_USD
//	if Buffer >= GrossUSD { reject }
//	Net       = GrossUSD - Buffer
//	EntryFee  = 0.001 * Net
//	SFGIssued = (Net - EntryFee) / SFG_USD_price
//
// plus the 11.3 safeguards: daily cap and the >3-cycles/30d surcharge on
// EntryFee. The returned hold's Status is Locked24h (spec 11.1: "24-hour
// lock: Status = Locked24h").
func Deposit(p DepositParams) (types.BankHold, error) {
	grossUSD := p.ExternalAmount.Mul(p.PriceUSD)

	cap := p.DailyCapUSD
	if cap.Sign() == 0 {
		cap = decimal.FromInt(DailyCapUSD)
	}
	if p.DailyUsedUSD.Add(grossUSD).Cmp(cap) > 0 {
		return types.BankHold{}, ErrDailyCapExceeded
	}

	buffer := DepositATRMultiple.Mul(p.ATRUSD)
	if buffer.Cmp(grossUSD) >= 0 {
		return types.BankHold{}, ErrBufferExceedsGross
	}
	net := grossUSD.Sub(buffer)

	feeRate := EntryFeeRate
	if p.CycleCount30d > CycleSurchargeThreshold {
		feeRate = feeRate.Add(CycleSurchargeRate)
	}
	entryFee := net.Mul(feeRate)

	if p.SFGUSDPrice.Sign() <= 0 {
		return types.BankHold{}, fmt.Errorf("bank: invalid SFG/USD price %s", p.SFGUSDPrice)
	}
	sfgIssuedDec := net.Sub(entryFee).Div(p.SFGUSDPrice)
	if sfgIssuedDec.Sign() < 0 {
		sfgIssuedDec = decimal.Zero
	}

	holdID := types.SumHash(p.Owner[:], []byte(p.Asset), []byte(fmt.Sprintf("%d", p.Now)))
	return types.BankHold{
		HoldID:         holdID,
		Owner:          p.Owner,
		ExternalAsset:  p.Asset,
		ExternalAmount: p.ExternalAmount,
		EntryPriceUSD:  p.PriceUSD,
		EntryATR:       p.ATRUSD,
		EntryBuffer:    buffer,
		EntryFee:       entryFee,
		SFGIssued:      sfgIssuedDec.Uint64(),
		OpenedAt:       p.Now,
		DailySnapshots: []types.ATRPoint{{Timestamp: p.Now, ATRUSD: p.ATRUSD}},
		Status:         types.HoldLocked24h,
		CycleCount30d:  p.CycleCount30d,
	}, nil
}

// AverageATR returns the mean of a hold's DailySnapshots, per spec 11.2
// ("computes average ATR over daily snapshots of the hold").
func AverageATR(hold types.BankHold) decimal.Decimal {
	if len(hold.DailySnapshots) == 0 {
		return hold.EntryATR
	}
	sum := decimal.Zero
	for _, s := range hold.DailySnapshots {
		sum = sum.Add(s.ATRUSD)
	}
	return sum.Div(decimal.FromInt(int64(len(hold.DailySnapshots))))
}

// WithdrawParams are the inputs to Withdraw (spec 11.2).
type WithdrawParams struct {
	Hold         types.BankHold
	PriceNowUSD  decimal.Decimal // P_now
	SlippageRate decimal.Decimal // 0.002..0.005 from a liquidity oracle
	RefundCap    decimal.Decimal // fraction of EntryBuffer refundable; zero means use RefundCapDefault
	GasFeeSFG    decimal.Decimal // on-chain TX fee, in SFG
	Now          int64
}

// WithdrawResult is what the Bank owes/collects on close.
type WithdrawResult struct {
	Retention decimal.Decimal // 1.5 * AvgATR, USD
	RefundSFG decimal.Decimal // refunded buffer portion, in SFG
	RepaySFG  decimal.Decimal // total SFG the user must deliver to close the hold
}

// Withdraw implements spec 11.2 / 19.4:
//
//	retention := 1.5 * avgATR_USD
//	refund    := max(0, entryBuffer - retention)
//	// repaySFG includes asymmetry + slippage(0.002..0.005) + 0.001 exit + gas
//
// The refund is additionally capped at RefundCap fraction of EntryBuffer
// (spec 11.3: "an 80 percent-of-original-charge cap on refunds ... default
// 100 percent ... unless voted down to 80 percent").
//
// Asymmetry (spec 11.2: "if the external asset depreciated versus entry,
// the SFG the user must repay is increased so the Bank is not left short.
// If the asset appreciated, repayment is fixed at the entry-equivalent
// SFG"): 19.4's pseudocode leaves the exact factor as a comment. This
// implementation scales repayment by max(1, EntryPriceUSD/PriceNowUSD): a
// price drop since entry increases required repayment proportionally (the
// Bank's USD-denominated position is protected), while a price rise leaves
// repayment at the entry-equivalent amount (no windfall passed through).
func Withdraw(p WithdrawParams) (WithdrawResult, error) {
	hold := p.Hold
	if hold.Status != types.HoldLocked24h && hold.Status != types.HoldOpen {
		return WithdrawResult{}, ErrHoldNotOpen
	}
	if p.Now-hold.OpenedAt < int64(DepositLockDuration/time.Millisecond) {
		return WithdrawResult{}, ErrHoldStillLocked
	}

	avgATR := AverageATR(hold)
	retention := WithdrawATRMultiple.Mul(avgATR)
	refund := hold.EntryBuffer.Sub(retention).Max0()

	refundCap := p.RefundCap
	if refundCap.Sign() == 0 {
		refundCap = RefundCapDefault
	}
	capAmount := hold.EntryBuffer.Mul(refundCap)
	refund = decimal.Min(refund, capAmount)

	baseSFG := decimal.FromInt(int64(hold.SFGIssued))

	asymmetry := decimal.MustFromString("1")
	if p.PriceNowUSD.Sign() > 0 && hold.EntryPriceUSD.Sign() > 0 && p.PriceNowUSD.Cmp(hold.EntryPriceUSD) < 0 {
		asymmetry = hold.EntryPriceUSD.Div(p.PriceNowUSD)
	}
	repay := baseSFG.Mul(asymmetry)

	slippage := p.SlippageRate
	if slippage.Sign() == 0 {
		slippage = SlippageMax
	}
	repay = repay.Add(repay.Mul(slippage))

	exitFeeRate := ExitFeeRate
	if hold.CycleCount30d > CycleSurchargeThreshold {
		exitFeeRate = exitFeeRate.Add(CycleSurchargeRate)
	}
	repay = repay.Add(repay.Mul(exitFeeRate))
	repay = repay.Add(p.GasFeeSFG)

	return WithdrawResult{
		Retention: retention,
		RefundSFG: refund,
		RepaySFG:  repay,
	}, nil
}

// RecordDailySnapshot appends today's ATR reading to a hold, per spec 11.1
// ("starts daily ATR snapshots").
func RecordDailySnapshot(hold *types.BankHold, atrUSD decimal.Decimal, now int64) {
	hold.DailySnapshots = append(hold.DailySnapshots, types.ATRPoint{Timestamp: now, ATRUSD: atrUSD})
}
