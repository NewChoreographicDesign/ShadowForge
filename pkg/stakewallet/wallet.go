// Package stakewallet is a real, network-syncing client for spec 17.4's
// "staked 2 percent yield" epoch-mint proposer path. Unlike
// pkg/shieldedwallet/pkg/govwallet, which replay committed *blocks* to
// mirror their respective canonical trees, this package replays every
// known governance *proposal* (via pkg/queryclient's real /v1/proposals
// endpoint) — the real, canonical stake-commitment tree is populated
// purely by pkg/tx's TallyDueProposals, in ProposalID-sorted order,
// entirely independent of block height or ordinary transaction
// processing (see that function's own doc), so replaying proposals in
// that same sorted order reproduces the identical tree structure without
// needing block-level replay at all.
package stakewallet

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/shadowforge/shadowforge-l1/pkg/queryclient"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// Wallet tracks a real local mirror of the canonical stake-commitment
// tree, and the set of real staked positions this caller itself created
// (via pkg/txbuilder.Builder.ProposeMintStaked) and wants to later locate
// and redeem.
type Wallet struct {
	client *queryclient.Client

	mu         sync.Mutex
	tree       *zk.Tree
	synced     bool
	remembered map[types.Hash]zk.StakeSecret // commitment -> secret
	indices    map[types.Hash]int            // commitment -> tree index, once found
}

// Config configures a Wallet.
type Config struct {
	// QueryBase is a pkg/query API base URL (e.g. "http://127.0.0.1:8081").
	QueryBase string
}

// New wraps a fresh, empty local stake tree ready to Sync.
func New(cfg Config) (*Wallet, error) {
	if cfg.QueryBase == "" {
		return nil, fmt.Errorf("stakewallet: Config.QueryBase must not be empty")
	}
	return &Wallet{
		client:     queryclient.New(cfg.QueryBase),
		tree:       zk.NewTree(),
		remembered: map[types.Hash]zk.StakeSecret{},
		indices:    map[types.Hash]int{},
	}, nil
}

// Remember records a real staked position this caller itself created, so
// a later Sync can locate it once its proposal actually passes and is
// tallied. This package never persists anything (mirroring
// pkg/txbuilder's own stance — see that package's doc): the caller alone
// is responsible for calling Remember again after a process restart with
// whatever zk.StakeSecret it saved when the position was first created
// (e.g. 'wallet propose-mint's own printed output).
func (w *Wallet) Remember(secret zk.StakeSecret) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.remembered[types.Hash(zk.ToBytes32(secret.Commitment()))] = secret
}

// Sync fetches every known proposal via pkg/queryclient's real
// /v1/proposals endpoint and replays every real, passed, applied staked
// mint proposal's StakePositionCommit into this Wallet's local stake
// tree, in ProposalID-sorted order — the same real deterministic order
// pkg/tx's TallyDueProposals used when it built the real, canonical tree
// (see that function's own doc, and pkg/state.Store.ListProposals' own
// key-order guarantee). Real chain data, nothing simulated. Safe to call
// repeatedly — it always rebuilds its local tree from the complete,
// current proposal list rather than incrementally patching it, since
// (unlike block height) there is no cheap "only what's new since last
// time" cursor for proposal state that can change (Tallied/MintApplied
// flipping) without a new proposal ID ever appearing.
func (w *Wallet) Sync(ctx context.Context) error {
	proposals, err := w.client.Proposals(ctx)
	if err != nil {
		return fmt.Errorf("stakewallet: fetch proposals: %w", err)
	}
	sort.Slice(proposals, func(i, j int) bool { return proposals[i].ProposalID < proposals[j].ProposalID })

	w.mu.Lock()
	defer w.mu.Unlock()
	w.tree = zk.NewTree()
	w.indices = map[types.Hash]int{}
	for _, p := range proposals {
		if !p.MintStaked || !p.MintApplied || p.StakePositionCommit == "" {
			continue
		}
		commit, err := types.ParseHash(p.StakePositionCommit)
		if err != nil {
			// A real, sharp signal something is wrong (a malformed
			// response, not a real proposal this node ever produced) —
			// skipping it rather than aborting the whole sync keeps one
			// bad record from hiding every other real position.
			continue
		}
		idx, err := w.tree.Insert(zk.FieldElementFromBytes32(commit))
		if err != nil {
			continue
		}
		if _, mine := w.remembered[commit]; mine {
			w.indices[commit] = idx
		}
	}
	w.synced = true
	return nil
}

// Found reports whether secret's own real position commitment has been
// located in a synced tree — i.e. whether its proposal has actually
// passed and been tallied yet.
func (w *Wallet) Found(secret zk.StakeSecret) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, found := w.indices[types.Hash(zk.ToBytes32(secret.Commitment()))]
	return found
}

// CurrentRoot is this wallet's locally-synced view of the stake tree's
// current root — what a real Unstake proof anchors to.
func (w *Wallet) CurrentRoot() (zk.FieldElement, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.synced {
		return zk.FieldElement{}, fmt.Errorf("stakewallet: not yet synced")
	}
	return w.tree.Root()
}

// ProofFor builds a real Merkle membership proof for secret's own
// position — what pkg/txbuilder.Builder.Unstake needs to build a real
// unstake proof. Requires Found(secret); Sync must have located it first.
func (w *Wallet) ProofFor(secret zk.StakeSecret) (zk.Proof, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	commit := types.Hash(zk.ToBytes32(secret.Commitment()))
	idx, found := w.indices[commit]
	if !found {
		return zk.Proof{}, fmt.Errorf("stakewallet: position %s not found in any synced, passed, applied staked proposal — Sync again once it has tallied", commit)
	}
	return w.tree.Prove(idx)
}
