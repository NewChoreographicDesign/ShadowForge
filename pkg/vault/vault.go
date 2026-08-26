// Package vault implements the ShadowVault: the fee treasury every
// transaction fee, privacy premium, Bank entry/exit fee, and container-sync
// fee lands in, and its fixed default allocation (spec section 9.2).
package vault

import (
	"github.com/shadowforge/shadowforge-l1/pkg/decimal"
	"github.com/shadowforge/shadowforge-l1/pkg/governance"
)

// Splits are the genesis-default Vault allocation shares (spec 9.2 table):
// 20% epoch bonuses, 10% burns, 10% audits/bounties, 60% remainder
// (grants, infrastructure, green offsets, SFG buybacks).
type Splits struct {
	EpochBonus decimal.Decimal
	Burn       decimal.Decimal
	Audit      decimal.Decimal
	Remainder  decimal.Decimal
}

// DefaultSplits returns the 20/10/10/60 genesis split from spec 9.2 /
// section 22's governance parameter table.
func DefaultSplits() Splits {
	return Splits{
		EpochBonus: decimal.MustFromString("0.20"),
		Burn:       decimal.MustFromString("0.10"),
		Audit:      decimal.MustFromString("0.10"),
		Remainder:  decimal.MustFromString("0.60"),
	}
}

// SplitsFromParams derives Splits from live governance parameters, so a
// governance vote that changes the Vault allocation is honored without a
// code change.
func SplitsFromParams(p governance.Params) Splits {
	return Splits{
		EpochBonus: p.VaultEpochBonusShare,
		Burn:       p.VaultBurnShare,
		Audit:      p.VaultAuditShare,
		Remainder:  p.VaultRemainderShare,
	}
}

// Vault is the fee-treasury ledger. All balances are in SFG.
type Vault struct {
	Splits Splits

	EpochBonusPool decimal.Decimal
	BurnedTotal    decimal.Decimal // permanent supply reduction, tracked here for reporting
	AuditPool      decimal.Decimal
	RemainderPool  decimal.Decimal // grants, infrastructure, green offsets, buybacks

	BankYieldBuybackPool decimal.Decimal
	BankYieldAirdropPool decimal.Decimal
}

func New(splits Splits) *Vault {
	return &Vault{Splits: splits}
}

// CollectFee routes a fee (transaction fee, privacy premium, Bank
// entry/exit fee, or container-sync fee — spec 9.2) into the four pools
// per the Vault's Splits. The Burn share is not literally burned by this
// call — pkg/tx / the node's supply accounting performs the actual token
// burn and calls RecordBurn — this only reserves the USD/SFG-equivalent
// share so downstream accounting balances.
func (v *Vault) CollectFee(amount decimal.Decimal) {
	v.EpochBonusPool = v.EpochBonusPool.Add(amount.Mul(v.Splits.EpochBonus))
	v.AuditPool = v.AuditPool.Add(amount.Mul(v.Splits.Audit))
	v.RemainderPool = v.RemainderPool.Add(amount.Mul(v.Splits.Remainder))
	v.BurnedTotal = v.BurnedTotal.Add(amount.Mul(v.Splits.Burn))
}

// CollectBankYield splits Bank buffer investment yield 50/50 between SFG
// buybacks and community airdrops (spec 9.2, 11.4).
func (v *Vault) CollectBankYield(amount decimal.Decimal) {
	half := amount.Div(decimal.FromInt(2))
	v.BankYieldBuybackPool = v.BankYieldBuybackPool.Add(half)
	v.BankYieldAirdropPool = v.BankYieldAirdropPool.Add(amount.Sub(half))
}

// DrawEpochBonusPool withdraws up to amount from the epoch bonus pool (for
// paying out EpochBonus below) and reports how much was actually available.
func (v *Vault) DrawEpochBonusPool(amount decimal.Decimal) decimal.Decimal {
	draw := decimal.Min(amount, v.EpochBonusPool)
	v.EpochBonusPool = v.EpochBonusPool.Sub(draw)
	return draw
}

// MultiTaskBonusMultiplier implements spec 19.7's toggle rule:
//
//	mult := 1.0
//	if validateOn && mintOn { mult = 1.20 }
//	if xor(validateOn, mintOn) { mult = 0.90 } // or badge instead of cash
func MultiTaskBonusMultiplier(validateOn, mintOn bool) decimal.Decimal {
	switch {
	case validateOn && mintOn:
		return decimal.MustFromString("1.20")
	case validateOn != mintOn:
		return decimal.MustFromString("0.90")
	default:
		return decimal.MustFromString("1.0")
	}
}

// EpochBonus implements spec 19.7's payout formula:
//
//	payout := baseBonus.Mul(mult).Mul(tpWeight)
func EpochBonus(baseBonus, tpWeight decimal.Decimal, validateOn, mintOn bool) decimal.Decimal {
	return baseBonus.Mul(MultiTaskBonusMultiplier(validateOn, mintOn)).Mul(tpWeight)
}
