// Package chain owns block assembly, PrevHash-linked chain growth, and
// genesis — the piece that turns "a batch the pipeline validated" into "a
// block the network actually agrees happened" (spec 4.3, 5.7).
//
// The chain's head only ever advances through Append, and Append is the
// single safety gate regardless of who calls it: finalizing a round this
// node itself ran, or adopting a block another node announced. It
// independently re-verifies every vote's Dilithium signature and committee
// membership before counting it — a block is never trusted just because it
// arrived looking quorate.
package chain

import (
	"fmt"
	"sync"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// PubKeyLookup resolves a committee member's Dilithium public key, so
// Append can verify their vote signature independently rather than
// trusting an unverified Vote.Sig. Callers typically back this with a
// registry built from observed heartbeats (pkg/validator).
type PubKeyLookup func(types.NFTID) (crypto.DilithiumPublicKey, bool)

// Chain persists blocks via a state.Store and tracks the current head.
type Chain struct {
	store *state.Store

	mu         sync.Mutex
	headHeight uint64
	headHash   types.Hash
}

// Open loads the persisted chain head from store, or creates and persists
// a genesis block (height 0, all-zero roots and PrevHash) if none exists
// yet.
func Open(store *state.Store, genesisTimeMs int64) (*Chain, error) {
	c := &Chain{store: store}

	height, found, err := store.GetHead()
	if err != nil {
		return nil, fmt.Errorf("chain: load head: %w", err)
	}
	if found {
		b, blockFound, err := store.GetBlock(height)
		if err != nil {
			return nil, fmt.Errorf("chain: load head block: %w", err)
		}
		if !blockFound {
			return nil, fmt.Errorf("chain: head height %d recorded but its block is missing", height)
		}
		c.headHeight = height
		c.headHash = types.HashBlock(b)
		return c, nil
	}

	genesis := types.Block{Height: 0, Timestamp: genesisTimeMs}
	if err := store.PutBlock(genesis); err != nil {
		return nil, fmt.Errorf("chain: persist genesis block: %w", err)
	}
	if err := store.SetHead(0); err != nil {
		return nil, fmt.Errorf("chain: set genesis head: %w", err)
	}
	c.headHeight = 0
	c.headHash = types.HashBlock(genesis)
	return c, nil
}

func (c *Chain) HeadHeight() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.headHeight
}

func (c *Chain) HeadHash() types.Hash {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.headHash
}

// NextHeight is the height a new proposal must target to chain onto the
// current head.
func (c *Chain) NextHeight() uint64 { return c.HeadHeight() + 1 }

// NextBlock assembles a candidate (unvoted) block chained onto the current
// head. Its hash (types.HashBlock) is what committee members are expected
// to sign as their vote.
func (c *Chain) NextBlock(epoch uint64, batch []types.ShieldedTx, txRoot, stateRoot, daRoot types.Hash, proposer types.NFTID, timestampMs int64) types.Block {
	c.mu.Lock()
	defer c.mu.Unlock()
	return types.Block{
		Height:    c.headHeight + 1,
		Epoch:     epoch,
		PrevHash:  c.headHash,
		Timestamp: timestampMs,
		Batch:     batch,
		TxRoot:    txRoot,
		StateRoot: stateRoot,
		DARoot:    daRoot,
		Proposer:  proposer,
	}
}

// Append validates that b chains onto the current head (height and
// PrevHash), that b.Votes contains real, distinct, committee-member
// signatures over b's own header hash reaching BFT quorum (spec 5.7)
// against committee, and only then persists b and advances the head.
//
// Every vote is independently re-verified here — a Vote whose signature
// doesn't check out under lookup, or whose Validator isn't in committee,
// or that duplicates an already-counted validator, contributes nothing
// (see consensus.TallyVotes). A block that doesn't reach real quorum is
// rejected outright, regardless of who's asking.
func (c *Chain) Append(b types.Block, committee []types.NFTID, lookup PubKeyLookup) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if b.Height != c.headHeight+1 {
		return fmt.Errorf("chain: block height %d does not chain onto head %d", b.Height, c.headHeight)
	}
	if b.PrevHash != c.headHash {
		return fmt.Errorf("chain: block PrevHash %s does not match head hash %s", b.PrevHash, c.headHash)
	}

	candidate := types.HashBlock(b)
	verified := make([]types.Vote, 0, len(b.Votes))
	for _, v := range b.Votes {
		pk, ok := lookup(v.Validator)
		if !ok {
			continue // unknown identity: cannot verify, cannot count
		}
		ok, err := crypto.DilithiumVerify(pk, candidate[:], crypto.DilithiumSignature(v.Sig))
		if err != nil || !ok {
			continue // invalid signature: does not count as an endorsement
		}
		verified = append(verified, v)
	}

	endorsements, quorum := consensus.TallyVotes(committee, candidate, verified)
	if !quorum {
		return fmt.Errorf("chain: block at height %d has %d/%d verified votes, quorum not met", b.Height, endorsements, len(committee))
	}

	if err := c.store.PutBlock(b); err != nil {
		return fmt.Errorf("chain: persist block: %w", err)
	}
	if err := c.store.SetHead(b.Height); err != nil {
		return fmt.Errorf("chain: advance head: %w", err)
	}
	c.headHeight = b.Height
	c.headHash = candidate

	// Index every committed transaction by the height that just reached
	// real quorum, so pkg/query can answer "did this transaction land"
	// with a direct lookup instead of a linear scan back through the
	// chain. This runs only after the block above is durably the new
	// head — the index never records a transaction that didn't actually
	// commit.
	for _, t := range b.Batch {
		if err := c.store.IndexTx(t.TxID, b.Height); err != nil {
			return fmt.Errorf("chain: index tx %s at height %d: %w", t.TxID, b.Height, err)
		}
	}
	return nil
}
