package validator

import (
	"context"
	"encoding/json"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// committeeSize implements spec 5.3.1/19.5's adaptive width, scaled to a
// whole committee (one slot per stage, spec 5.7: "with one validator per
// stage that is 3 of 5").
func committeeSize(online int) int {
	return consensus.ValidatorsPerStage(online) * 5
}

func (n *Node) handleMessage(p peer.ID, env shadownet.Envelope) {
	switch env.Type {
	case shadownet.MsgHeartbeat:
		var hb shadownet.HeartbeatPayload
		if err := decode(env, &hb); err != nil {
			n.log("validator: bad heartbeat from %s: %v", p, err)
			return
		}
		if len(hb.PubKey) == 0 {
			return
		}
		n.recordOnline(hb.NFT, crypto.DilithiumPublicKey(hb.PubKey), time.Now())

	case shadownet.MsgTxOffer:
		var offer shadownet.TxOfferPayload
		if err := decode(env, &offer); err != nil {
			n.log("validator: bad tx offer from %s: %v", p, err)
			return
		}
		if err := n.mempool.Submit(offer.Tx, time.Now()); err != nil {
			n.log("validator: tx offer from %s not admitted: %v", p, err)
		}

	case shadownet.MsgBlockProposal:
		var prop shadownet.BlockProposalPayload
		if err := decode(env, &prop); err != nil {
			n.log("validator: bad block proposal from %s: %v", p, err)
			return
		}
		n.handleBlockProposal(prop)

	case shadownet.MsgStageVote:
		var v shadownet.StageVotePayload
		if err := decode(env, &v); err != nil {
			n.log("validator: bad stage vote from %s: %v", p, err)
			return
		}
		n.handleStageVote(v)

	case shadownet.MsgBlockAnnounce:
		var ann shadownet.BlockAnnouncePayload
		if err := decode(env, &ann); err != nil {
			n.log("validator: bad block announce from %s: %v", p, err)
			return
		}
		n.handleBlockAnnounce(ann)

	default:
		// MegabatchPart, ContainerSync, SilentPad: accepted on the wire
		// (rate-limited like every other type) but not acted on by this
		// package — see the package doc's scope note.
	}
}

func decode(env shadownet.Envelope, v interface{}) error {
	return json.Unmarshal(env.Payload, v)
}

// roundLoop periodically checks whether this node must propose the next
// block, and sweeps timed-out rounds.
func (n *Node) roundLoop(ctx context.Context) {
	ticker := time.NewTicker(n.cfg.BatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.sweepTimeouts()
			n.maybePropose()
		}
	}
}

func (n *Node) sweepTimeouts() {
	now := time.Now()
	n.mu.Lock()
	var expired []*round
	for h, r := range n.rounds {
		if now.After(r.deadline) {
			expired = append(expired, r)
			delete(n.rounds, h)
		}
	}
	n.mu.Unlock()

	for _, r := range expired {
		n.log("validator: round at height %d timed out with %d/%d votes, rolling back", r.height, len(r.votes), len(r.committee))
		r.rollback()
		n.tree.TruncateTo(r.treeSnapshotLen)
		for _, t := range r.batch {
			_ = n.mempool.Submit(t, time.Now()) // best-effort retry; a full mempool just drops it
		}
	}
}

func (n *Node) maybePropose() {
	nextHeight := n.chn.NextHeight()

	n.mu.Lock()
	_, inFlight := n.rounds[nextHeight]
	n.mu.Unlock()
	if inFlight {
		return // already proposed or voting on this height
	}

	online := n.onlineSet(time.Now())
	committee := consensus.AssignCommittee(online, nextHeight, committeeSize(len(online)))
	if len(committee) == 0 || committee[0] != n.identity {
		return // not our turn to propose
	}

	entries := n.mempool.DrainBatch(0)
	if len(entries) == 0 {
		return // nothing to propose; avoid empty-block spam
	}
	batch := make([]types.ShieldedTx, len(entries))
	for i, e := range entries {
		batch[i] = e.Tx
	}

	prop := shadownet.BlockProposalPayload{
		Height:    nextHeight,
		Epoch:     consensus.CurrentEpoch(n.cfg.Genesis, time.Now()),
		Proposer:  n.identity,
		Batch:     batch,
		Timestamp: time.Now().UnixMilli(),
	}
	env, err := shadownet.NewEnvelope(shadownet.MsgBlockProposal, prop)
	if err != nil {
		n.log("validator: build proposal: %v", err)
		return
	}
	ctx := context.Background()
	n.net.Broadcast(ctx, env)

	// Broadcast excludes self; process our own proposal directly so the
	// proposer participates in its own round like any other committee
	// member.
	n.handleBlockProposal(prop)
}

// handleBlockProposal runs the pipeline against a proposed batch (whether
// received over the network or self-authored) and, if this node is an
// assigned committee member and every transaction validates, casts a real
// signed vote for the resulting candidate root.
func (n *Node) handleBlockProposal(prop shadownet.BlockProposalPayload) {
	if prop.Height != n.chn.NextHeight() {
		return // stale or premature: doesn't chain onto our current head
	}

	n.mu.Lock()
	if _, exists := n.rounds[prop.Height]; exists {
		n.mu.Unlock()
		return // already processing this height
	}
	n.mu.Unlock()

	online := n.onlineSet(time.Now())
	committee := consensus.AssignCommittee(online, prop.Height, committeeSize(len(online)))
	if len(committee) == 0 || committee[0] != prop.Proposer {
		n.log("validator: rejecting proposal at height %d: %s is not the assigned proposer", prop.Height, prop.Proposer)
		return
	}
	if !containsID(committee, n.identity) {
		return // not our job to vote on this height
	}

	treeSnapshot := n.tree.Len()
	txn := n.store.BeginTxn()

	entries := make([]tx.Entry, len(prop.Batch))
	now := time.Now()
	for i, t := range prop.Batch {
		entries[i] = tx.Entry{Tx: t, SubmittedAt: now}
	}
	pipeline := tx.NewPipeline(tx.Deps{Store: txn, StateTree: n.tree, ZK: n.zkSys, Vault: n.vlt, Now: func() time.Time { return now }})
	results := pipeline.ProcessBatch(entries)

	for _, res := range results {
		if res.Error != nil {
			n.log("validator: rejecting proposal at height %d: tx %s failed the pipeline: %v", prop.Height, res.Tx.TxID, res.Error)
			txn.Discard()
			n.tree.TruncateTo(treeSnapshot)
			return
		}
	}

	stateRoot := n.tree.Root()
	txRoot := txRootOf(prop.Batch)
	daRoot := daRootOf(prop.Batch)
	block := n.chn.NextBlock(prop.Epoch, prop.Batch, txRoot, stateRoot, daRoot, prop.Proposer, prop.Timestamp)
	candidate := types.HashBlock(block)

	sig, err := crypto.DilithiumSign(n.sk, candidate[:])
	if err != nil {
		n.log("validator: sign candidate: %v", err)
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return
	}
	ownVote := types.Vote{Validator: n.identity, StateRoot: candidate, Sig: types.DilithiumSig(sig)}

	r := &round{
		height:          prop.Height,
		committee:       committee,
		batch:           prop.Batch,
		txn:             txn,
		treeSnapshotLen: treeSnapshot,
		block:           block,
		candidate:       candidate,
		votes:           []types.Vote{ownVote},
		deadline:        time.Now().Add(n.cfg.RoundTimeout),
	}
	n.mu.Lock()
	n.rounds[prop.Height] = r
	n.mu.Unlock()

	voteEnv, err := shadownet.NewEnvelope(shadownet.MsgStageVote, shadownet.StageVotePayload{
		Height: prop.Height, Validator: n.identity, CandidateHash: candidate, Sig: types.DilithiumSig(sig),
	})
	if err != nil {
		n.log("validator: build vote: %v", err)
		return
	}
	n.net.Broadcast(context.Background(), voteEnv)

	n.tryFinalize(r)
}

func (n *Node) handleStageVote(v shadownet.StageVotePayload) {
	n.mu.Lock()
	r, ok := n.rounds[v.Height]
	n.mu.Unlock()
	if !ok || r.candidate != v.CandidateHash {
		return // not tracking this round, or vote is for a different candidate than ours
	}

	pk, ok := n.pubKeyLookup(v.Validator)
	if !ok {
		return
	}
	valid, err := crypto.DilithiumVerify(pk, v.CandidateHash[:], crypto.DilithiumSignature(v.Sig))
	if err != nil || !valid {
		n.log("validator: dropping vote from %s at height %d: does not verify", v.Validator, v.Height)
		return
	}

	n.mu.Lock()
	r.votes = append(r.votes, types.Vote{Validator: v.Validator, StateRoot: v.CandidateHash, Sig: v.Sig})
	n.mu.Unlock()

	n.tryFinalize(r)
}

// tryFinalize checks whether r's votes now meet BFT quorum and, if so,
// commits the tentatively-applied batch and grows the chain.
func (n *Node) tryFinalize(r *round) {
	n.mu.Lock()
	_, endorsed := n.rounds[r.height]
	votes := append([]types.Vote(nil), r.votes...)
	n.mu.Unlock()
	if !endorsed {
		return // already finalized (or rolled back) by a concurrent path
	}

	_, quorum := consensus.TallyVotes(r.committee, r.candidate, votes)
	if !quorum {
		return
	}

	r.block.Votes = votes
	if err := n.chn.Append(r.block, r.committee, n.pubKeyLookup); err != nil {
		n.log("validator: quorum reached locally but chain.Append rejected height %d: %v", r.height, err)
		r.rollback()
		n.tree.TruncateTo(r.treeSnapshotLen)
		n.mu.Lock()
		delete(n.rounds, r.height)
		n.mu.Unlock()
		return
	}
	if err := r.txn.Commit(); err != nil {
		n.log("validator: FATAL-ish: chain accepted height %d but the state txn failed to commit: %v", r.height, err)
	}

	n.mu.Lock()
	delete(n.rounds, r.height)
	n.mu.Unlock()

	n.log("validator: committed height %d (%d votes) state_root=%s", r.height, len(votes), r.block.StateRoot)

	env, err := shadownet.NewEnvelope(shadownet.MsgBlockAnnounce, shadownet.BlockAnnouncePayload{Block: r.block})
	if err != nil {
		n.log("validator: build block announce: %v", err)
		return
	}
	n.net.Broadcast(context.Background(), env)
}

// handleBlockAnnounce lets a node that did not itself reach quorum locally
// (it wasn't on the committee, or its votes arrived too late) adopt a
// finalized block by independently replaying it: the batch is run through
// the exact same pipeline this node would have used to vote, and the block
// is only adopted if the freshly recomputed state root agrees with what was
// announced and chain.Append's own quorum/signature reverification passes.
// Multi-block catch-up (this node more than one block behind) is out of
// scope, per the package doc.
func (n *Node) handleBlockAnnounce(ann shadownet.BlockAnnouncePayload) {
	height := ann.Block.Height
	if height <= n.chn.HeadHeight() || height != n.chn.NextHeight() {
		return // stale, or too far ahead for this build's single-block adoption
	}

	n.mu.Lock()
	r, hadRound := n.rounds[height]
	delete(n.rounds, height)
	n.mu.Unlock()
	if hadRound {
		// Whatever this node was tentatively tracking for this height is
		// superseded by the announced block; discard it before replaying.
		r.rollback()
		n.tree.TruncateTo(r.treeSnapshotLen)
	}

	online := n.onlineSet(time.Now())
	committee := consensus.AssignCommittee(online, height, committeeSize(len(online)))

	treeSnapshot := n.tree.Len()
	txn := n.store.BeginTxn()

	entries := make([]tx.Entry, len(ann.Block.Batch))
	now := time.Now()
	for i, t := range ann.Block.Batch {
		entries[i] = tx.Entry{Tx: t, SubmittedAt: now}
	}
	pipeline := tx.NewPipeline(tx.Deps{Store: txn, StateTree: n.tree, ZK: n.zkSys, Vault: n.vlt, Now: func() time.Time { return now }})
	results := pipeline.ProcessBatch(entries)
	for _, res := range results {
		if res.Error != nil {
			n.log("validator: rejecting announced block at height %d: tx %s failed replay: %v", height, res.Tx.TxID, res.Error)
			txn.Discard()
			n.tree.TruncateTo(treeSnapshot)
			return
		}
	}

	stateRoot := n.tree.Root()
	if stateRoot != ann.Block.StateRoot {
		n.log("validator: rejecting announced block at height %d: recomputed state root %s disagrees with announced %s", height, stateRoot, ann.Block.StateRoot)
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return
	}

	if err := n.chn.Append(ann.Block, committee, n.pubKeyLookup); err != nil {
		n.log("validator: rejecting announced block at height %d: chain.Append: %v", height, err)
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return
	}
	if err := txn.Commit(); err != nil {
		n.log("validator: FATAL-ish: adopted announced height %d but the state txn failed to commit: %v", height, err)
		return
	}

	n.log("validator: adopted announced height %d state_root=%s", height, ann.Block.StateRoot)
}

// txRootOf hashes the ordered TxIDs of a batch, giving every honest node
// that agrees on the batch's contents and order an identical root.
func txRootOf(batch []types.ShieldedTx) types.Hash {
	parts := make([][]byte, len(batch))
	for i, t := range batch {
		id := t.TxID
		parts[i] = id[:]
	}
	return types.SumHash(parts...)
}

// daRootOf hashes each tx's Proof and Memo blobs (the data-availability
// payload that must accompany the block for anyone to independently verify
// or decrypt it later), in batch order.
func daRootOf(batch []types.ShieldedTx) types.Hash {
	parts := make([][]byte, 0, len(batch)*2)
	for _, t := range batch {
		parts = append(parts, t.Proof, t.Memo)
	}
	return types.SumHash(parts...)
}

func containsID(ids []types.NFTID, id types.NFTID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
