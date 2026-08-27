package txbuilder

import (
	"fmt"

	"github.com/shadowforge/shadowforge-l1/pkg/bank"
	"github.com/shadowforge/shadowforge-l1/pkg/oracle"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// bankPublicInputsFromQuote builds the exact BankPublicInputs shape
// pkg/tx's pipeline (Stage 4) recomputes and checks: BufferUSD must equal
// the kind's real ATR multiple times ATRUSD, and OraclePriceUSD/ATRUSD
// must sit within the node's own oracle tolerance of its real reading.
// This uses the same genesis-default multiples pkg/bank itself exports
// (bank.DepositATRMultiple / bank.WithdrawATRMultiple) rather than
// duplicating the constants.
//
// A real, disclosed limitation: if a live network's governance has voted
// to change these multiples away from the genesis default (pkg/tx.Deps.
// Governance, wired in this session's earlier governance work), a
// transaction built against the static default here would be rejected by
// a node checking against the new live value instead. Discovering a
// node's live governance parameters isn't something the read-only query
// API (pkg/query) exposes yet — this is the same category of boundary
// this codebase documents elsewhere rather than silently ignoring.
func bankPublicInputsFromQuote(kind types.TxKind, asset types.AssetID, quote oracle.Quote) *types.BankPublicInputs {
	multiple := bank.DepositATRMultiple
	if kind == types.TxBankWithdraw {
		multiple = bank.WithdrawATRMultiple
	}
	return &types.BankPublicInputs{
		Asset:          asset,
		OraclePriceUSD: quote.PriceUSD,
		ATRUSD:         quote.ATRUSD,
		BufferUSD:      multiple.Mul(quote.ATRUSD),
	}
}

// BankDepositFromQuote builds a real TxBankDeposit transaction bound to
// an already-fetched oracle.Quote — the pure, no-network-I/O core;
// BankDeposit below is the convenience wrapper that fetches a real quote
// first.
func (b *Builder) BankDepositFromQuote(asset types.AssetID, quote oracle.Quote) (types.ShieldedTx, error) {
	nullifier, err := randomHash()
	if err != nil {
		return types.ShieldedTx{}, err
	}
	t := types.ShieldedTx{
		Kind:             types.TxBankDeposit,
		BankPublicInputs: bankPublicInputsFromQuote(types.TxBankDeposit, asset, quote),
		Nullifier:        nullifier,
	}
	return b.finalize(t)
}

// BankWithdrawFromQuote is BankDepositFromQuote's withdraw counterpart.
func (b *Builder) BankWithdrawFromQuote(asset types.AssetID, quote oracle.Quote) (types.ShieldedTx, error) {
	nullifier, err := randomHash()
	if err != nil {
		return types.ShieldedTx{}, err
	}
	t := types.ShieldedTx{
		Kind:             types.TxBankWithdraw,
		BankPublicInputs: bankPublicInputsFromQuote(types.TxBankWithdraw, asset, quote),
		Nullifier:        nullifier,
	}
	return b.finalize(t)
}

// BankDeposit fetches a real, quorum-verified price/ATR quote for asset
// (the same pkg/oracle.Quorum a live node itself checks against) and
// builds a matching TxBankDeposit. quorum is the caller's own — typically
// the same real CoinGecko+Coinbase quorum construction cmd/node uses,
// so what this builder claims and what a node independently re-verifies
// come from the same real feeds, not a client-side guess.
func (b *Builder) BankDeposit(quorum *oracle.Quorum, asset types.AssetID) (types.ShieldedTx, error) {
	quote, err := quorum.Quote(asset)
	if err != nil {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: fetch oracle quote for deposit: %w", err)
	}
	return b.BankDepositFromQuote(asset, quote)
}

// BankWithdraw is BankDeposit's withdraw counterpart.
func (b *Builder) BankWithdraw(quorum *oracle.Quorum, asset types.AssetID) (types.ShieldedTx, error) {
	quote, err := quorum.Quote(asset)
	if err != nil {
		return types.ShieldedTx{}, fmt.Errorf("txbuilder: fetch oracle quote for withdraw: %w", err)
	}
	return b.BankWithdrawFromQuote(asset, quote)
}
