// Package governance holds the genesis-default protocol parameters (spec
// section 22) and a minimal NFT-weighted proposal/vote/tally mechanism
// (spec 9.1: "governance weight (via NFT + optional stake)").
package governance

import (
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
)

// Params is the full set of governance-adjustable protocol parameters,
// initialized to the genesis defaults in spec section 22. Every field notes
// its spec default; "Who may change" is Governance for all of them except
// BankDailyCapUSD, which KYC verification may also raise per-wallet.
type Params struct {
	BatchInterval         time.Duration   // 1 second
	StageTimeout          time.Duration   // 4 seconds
	HeartbeatInterval     time.Duration   // 10 seconds, offline after 3 misses
	CooldownDuration      time.Duration   // 1 hour
	SentinelThreshold     int             // 10 online
	AdaptiveWidthAt200    int             // 2 validators/stage above 200 online
	AdaptiveWidthAt300    int             // 3 validators/stage above 300 online
	DepositATRMultiple    decimal.Decimal // 2.5
	WithdrawATRMultiple   decimal.Decimal // 1.5
	BankFeeRate           decimal.Decimal // 0.1% in and out
	BankDailyCapUSD       decimal.Decimal // $100,000 (governance + KYC raise)
	CycleSurchargeAfter   uint            // >3 / 30 days
	CycleSurchargeRate    decimal.Decimal // +1%
	DepositLock           time.Duration   // 24 hours
	SlippageMin           decimal.Decimal // 0.2%
	SlippageMax           decimal.Decimal // 0.5%
	InflationMin          decimal.Decimal // 2% / year
	InflationMax          decimal.Decimal // 5% / year
	VaultEpochBonusShare  decimal.Decimal // 20%
	VaultBurnShare        decimal.Decimal // 10%
	VaultAuditShare       decimal.Decimal // 10%
	VaultRemainderShare   decimal.Decimal // 60%
	KYCPromptThresholdUSD decimal.Decimal // $10,000 example
	SilentTxSpikePercent  decimal.Decimal // +20%
	SilentTxHoldDuration  time.Duration   // 7 days
	SilentTxVaultFee      decimal.Decimal // 10%
	ContainerHybridSplit  decimal.Decimal // 50/50, per-container envelope
	RefundCap             decimal.Decimal // 100%, may be voted to 80%
}

// Default returns the spec section 22 genesis defaults.
func Default() Params {
	return Params{
		BatchInterval:         time.Second,
		StageTimeout:          4 * time.Second,
		HeartbeatInterval:     10 * time.Second,
		CooldownDuration:      time.Hour,
		SentinelThreshold:     10,
		AdaptiveWidthAt200:    2,
		AdaptiveWidthAt300:    3,
		DepositATRMultiple:    decimal.MustFromString("2.5"),
		WithdrawATRMultiple:   decimal.MustFromString("1.5"),
		BankFeeRate:           decimal.MustFromString("0.001"),
		BankDailyCapUSD:       decimal.FromInt(100_000),
		CycleSurchargeAfter:   3,
		CycleSurchargeRate:    decimal.MustFromString("0.01"),
		DepositLock:           24 * time.Hour,
		SlippageMin:           decimal.MustFromString("0.002"),
		SlippageMax:           decimal.MustFromString("0.005"),
		InflationMin:          decimal.MustFromString("0.02"),
		InflationMax:          decimal.MustFromString("0.05"),
		VaultEpochBonusShare:  decimal.MustFromString("0.20"),
		VaultBurnShare:        decimal.MustFromString("0.10"),
		VaultAuditShare:       decimal.MustFromString("0.10"),
		VaultRemainderShare:   decimal.MustFromString("0.60"),
		KYCPromptThresholdUSD: decimal.FromInt(10_000),
		SilentTxSpikePercent:  decimal.MustFromString("0.20"),
		SilentTxHoldDuration:  7 * 24 * time.Hour,
		SilentTxVaultFee:      decimal.MustFromString("0.10"),
		ContainerHybridSplit:  decimal.MustFromString("0.50"),
		RefundCap:             decimal.MustFromString("1.0"),
	}
}
