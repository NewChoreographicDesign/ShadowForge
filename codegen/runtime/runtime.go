// Package runtime is the hand-written surface that ShadowRust-generated Go
// code calls into. Concrete implementations live in pkg/tx (mempool +
// 5-stage pipeline), pkg/zk (circuit build/proof), pkg/bank, and
// pkg/consensus (queue insert). Generated code never talks to gnark,
// Badger, or libp2p directly — codegen output stays a thin, readable
// orchestration layer, with the heavy lifting done by hand-written Go, per
// spec 14.1 ("The L1 node software itself is that generated Go, plus a
// small amount of hand-written Go for networking and storage").
package runtime

import "math/big"

// Node is the interface every ShadowRust codegen artifact is compiled
// against.
type Node interface {
	// Transfer builds a shielded transfer circuit, derives/reuses the
	// session's ephemeral mirror, and submits the proof into the mempool.
	// feeTo is "" when the statement routed no fee.
	Transfer(from, to string, amount *big.Rat, feeTo string, feeAmount *big.Rat) (txid string, err error)

	// BankDeposit runs the ATR math (spec 11.1 / 19.3) and issues SFG.
	BankDeposit(holdName string, atrUSD *big.Rat) (holdID string, err error)

	// QueueInsert runs the unfair-position revolver insert (spec 5.4.1).
	QueueInsert(nftID string, positions []int) error

	// MintProposal submits an epoch mint proposal (spec 17.4).
	MintProposal(name string, amount *big.Rat, epoch *big.Rat) error

	// UpdateTrait applies a shielded NFTTrait transaction (spec 4.5, 16.3).
	UpdateTrait(target, key, op string, value *big.Rat) error
}

// --- big.Rat helpers used by generated code, kept short so generated
// source stays legible. ---

func N(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("runtime.N: invalid numeric literal " + s)
	}
	return r
}

func Add(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }
func Sub(a, b *big.Rat) *big.Rat { return new(big.Rat).Sub(a, b) }
func Mul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }

func Div(a, b *big.Rat) *big.Rat {
	if b.Sign() == 0 {
		panic("runtime.Div: division by zero")
	}
	return new(big.Rat).Quo(a, b)
}

func Truthy(r *big.Rat) bool { return r.Sign() != 0 }

func Bool(b bool) *big.Rat {
	if b {
		return big.NewRat(1, 1)
	}
	return new(big.Rat)
}

func Cmp(op string, a, b *big.Rat) *big.Rat {
	c := a.Cmp(b)
	switch op {
	case ">":
		return Bool(c > 0)
	case ">=":
		return Bool(c >= 0)
	case "<":
		return Bool(c < 0)
	case "<=":
		return Bool(c <= 0)
	case "==":
		return Bool(c == 0)
	case "!=":
		return Bool(c != 0)
	}
	panic("runtime.Cmp: unknown operator " + op)
}
