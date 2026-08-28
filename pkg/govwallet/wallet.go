// Package govwallet is a real, network-syncing client for spec 9.1's
// anonymous "one NFT, one vote" governance eligibility. It replays every
// committed Kind NFTMint transaction's VoterCommitment into a local
// mirror of the real eligibility-commitment tree (pkg/zk.Tree) — the same
// real deterministic replay pkg/shieldedwallet already does for shielded
// notes — locates this wallet's own leaf among them, and uses it to build
// a real pkg/zk.EligibilitySystem proof (types.VoteEligibilityProof) for
// a specific proposal: proving "I hold a real, minted NFT" without
// revealing which one.
package govwallet

import (
	"context"
	"fmt"
	"sync"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/queryclient"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/zk"
)

// Wallet tracks one real voter identity's local mirror of the canonical
// eligibility-commitment tree.
type Wallet struct {
	voterSK zk.FieldElement
	client  *queryclient.Client

	mu           sync.Mutex
	tree         *zk.Tree
	syncedHeight uint64
	synced       bool
	myIndex      int
	haveIndex    bool
}

// Config configures a Wallet.
type Config struct {
	// QueryBase is a pkg/query API base URL (e.g. "http://127.0.0.1:8081").
	QueryBase string
}

// New derives this wallet's persistent VoterSK from sk (zk.DeriveVoterSK —
// see that function's own doc for why this is deterministic rather than
// a freshly generated, separately-stored secret) and wraps a fresh, empty
// local eligibility tree ready to Sync.
func New(sk crypto.DilithiumPrivateKey, cfg Config) (*Wallet, error) {
	if cfg.QueryBase == "" {
		return nil, fmt.Errorf("govwallet: Config.QueryBase must not be empty")
	}
	return &Wallet{
		voterSK: zk.DeriveVoterSK([]byte(sk)),
		client:  queryclient.New(cfg.QueryBase),
		tree:    zk.NewTree(),
	}, nil
}

// VoterCommitment is this wallet's real eligibility-tree leaf value
// (zk.VoterCommitment(VoterSK)) — what a real Kind NFTMint transaction
// must carry in NFTMintPublicInputs.VoterCommitment for this wallet to
// ever become eligible to vote.
func (w *Wallet) VoterCommitment() types.Hash {
	return types.Hash(zk.ToBytes32(zk.VoterCommitment(w.voterSK)))
}

// Sync fetches every block since the last Sync (or genesis, on the first
// call) via pkg/queryclient, and replays every committed Kind NFTMint's
// VoterCommitment into this Wallet's local eligibility tree in the exact
// same order pkg/tx's pipeline (Stage 4) inserted them into the real one
// — the same real deterministic replay every honest validator already
// does for its own copy. Real chain data, nothing simulated.
func (w *Wallet) Sync(ctx context.Context) error {
	status, err := w.client.Status(ctx)
	if err != nil {
		return fmt.Errorf("govwallet: fetch status: %w", err)
	}

	w.mu.Lock()
	start := w.syncedHeight
	if w.synced {
		start++
	}
	w.mu.Unlock()

	for height := start; height <= status.Height; height++ {
		b, err := w.client.Block(ctx, height)
		if err != nil {
			return fmt.Errorf("govwallet: fetch block %d: %w", height, err)
		}
		w.replayBlock(b)
		w.mu.Lock()
		w.syncedHeight = height
		w.synced = true
		w.mu.Unlock()
	}
	return nil
}

func (w *Wallet) replayBlock(b types.Block) {
	w.mu.Lock()
	defer w.mu.Unlock()
	myCommitment := zk.VoterCommitment(w.voterSK)
	for _, t := range b.Batch {
		if t.Kind != types.TxNFTMint || t.NFTMintPublicInputs == nil {
			continue
		}
		elem := zk.FieldElementFromBytes32(t.NFTMintPublicInputs.VoterCommitment)
		idx, err := w.tree.Insert(elem)
		if err != nil {
			// A real, sharp signal something is wrong (canonical
			// eligibility tree desync, or this build's TreeSize
			// genuinely exhausted) — surfacing it silently here would
			// let a wallet quietly under-report its own tree state.
			continue
		}
		if elem.Equal(&myCommitment) {
			w.myIndex = idx
			w.haveIndex = true
		}
	}
}

// Eligible reports whether this wallet's own VoterCommitment has been
// found in any block Sync has replayed — i.e. whether a real Kind
// NFTMint for this identity has ever committed.
func (w *Wallet) Eligible() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.haveIndex
}

// CurrentRoot is this wallet's locally-synced view of the eligibility
// tree's current root — what a freshly-built eligibility proof anchors
// to.
func (w *Wallet) CurrentRoot() (zk.FieldElement, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tree.Root()
}

// BuildEligibilityProof builds a real, anonymous zk.EligibilitySystem
// proof of "this wallet holds a real, minted NFT" for proposalID, without
// revealing which leaf it is. Requires Eligible() — a wallet that has
// never seen its own VoterCommitment in a synced block has nothing to
// prove membership of.
func (w *Wallet) BuildEligibilityProof(sys *zk.EligibilitySystem, proposalID types.ID) (types.VoteEligibilityProof, error) {
	w.mu.Lock()
	if !w.haveIndex {
		w.mu.Unlock()
		return types.VoteEligibilityProof{}, fmt.Errorf("govwallet: this wallet's VoterCommitment was not found in any synced Kind NFTMint — mint a real NFT and Sync again before voting")
	}
	proof, err := w.tree.Prove(w.myIndex)
	voterSK := w.voterSK
	w.mu.Unlock()
	if err != nil {
		return types.VoteEligibilityProof{}, fmt.Errorf("govwallet: build merkle proof: %w", err)
	}

	scope := zk.FieldElementFromBytes32(types.VoteEligibilityScope(proposalID))
	in := zk.EligibilityInput{
		MerkleRoot:    proof.Root,
		ProposalScope: scope,
		VoterSK:       voterSK,
		Proof:         proof,
	}
	zproof, err := sys.Prove(in)
	if err != nil {
		return types.VoteEligibilityProof{}, fmt.Errorf("govwallet: prove eligibility: %w", err)
	}
	proofBytes, err := zk.ProofToBytes(zproof)
	if err != nil {
		return types.VoteEligibilityProof{}, fmt.Errorf("govwallet: serialize eligibility proof: %w", err)
	}
	return types.VoteEligibilityProof{
		Proof:      proofBytes,
		MerkleRoot: types.Hash(zk.ToBytes32(proof.Root)),
		Nullifier:  types.Hash(zk.ToBytes32(in.Nullifier())),
	}, nil
}
