// Package governance holds the genesis-default protocol parameters (spec
// section 22) and a minimal NFT-weighted proposal/vote/tally mechanism
// (spec 9.1: "governance weight (via NFT + optional stake)").
package governance

import (
	"fmt"
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

// ParamKeys are the Params fields ApplyParamChange knows how to set — the
// decimal.Decimal-typed subset with a real live consumer (pkg/tx Stage 4's
// ATR-buffer check, pkg/vault's fee split), so a passed
// ProposalParamChange vote actually changes running protocol behavior
// rather than only being tallied and forgotten. Every other Params field
// (durations, ints — spec-fixed protocol timings, not the "governance may
// tune this ratio" parameters spec 22's table calls out) is intentionally
// not settable this way; see this package's doc.
var ParamKeys = map[string]bool{
	"DepositATRMultiple":   true,
	"WithdrawATRMultiple":  true,
	"BankFeeRate":          true,
	"RefundCap":            true,
	"CycleSurchargeRate":   true,
	"VaultEpochBonusShare": true,
	"VaultBurnShare":       true,
	"VaultAuditShare":      true,
	"VaultRemainderShare":  true,
	"SilentTxSpikePercent": true,
}

// ApplyParamChange mutates p according to a passed ProposalParamChange
// proposal's key/rawValue — the real effect a passing governance vote has
// on the running protocol (spec 9.1's "governance weight" / spec 17.4's
// epoch-end tally), closing the gap where a proposal's outcome was
// tallied and persisted but never actually changed anything live. An
// unrecognized key or an unparseable value is rejected rather than
// silently ignored or applied partially.
func ApplyParamChange(p *Params, key, rawValue string) error {
	if !ParamKeys[key] {
		return fmt.Errorf("governance: unknown or unsupported param key %q", key)
	}
	v, err := decimal.FromString(rawValue)
	if err != nil {
		return fmt.Errorf("governance: invalid value %q for param %q: %w", rawValue, key, err)
	}
	switch key {
	case "DepositATRMultiple":
		p.DepositATRMultiple = v
	case "WithdrawATRMultiple":
		p.WithdrawATRMultiple = v
	case "BankFeeRate":
		p.BankFeeRate = v
	case "RefundCap":
		p.RefundCap = v
	case "CycleSurchargeRate":
		p.CycleSurchargeRate = v
	case "VaultEpochBonusShare":
		p.VaultEpochBonusShare = v
	case "VaultBurnShare":
		p.VaultBurnShare = v
	case "VaultAuditShare":
		p.VaultAuditShare = v
	case "VaultRemainderShare":
		p.VaultRemainderShare = v
	case "SilentTxSpikePercent":
		p.SilentTxSpikePercent = v
	}
	return nil
}

// IsVaultShareKey reports whether key is one of the four Vault allocation
// shares, so a caller that just applied a passing change (pkg/tx's
// TallyDueProposals) knows whether it also needs to resync a live
// *vault.Vault's Splits (vault.SplitsFromParams) to match.
func IsVaultShareKey(key string) bool {
	switch key {
	case "VaultEpochBonusShare", "VaultBurnShare", "VaultAuditShare", "VaultRemainderShare":
		return true
	default:
		return false
	}
}
