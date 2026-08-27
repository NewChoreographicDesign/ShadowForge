package validator

import (
	"context"
	"encoding/json"
	"errors"
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
		n.recordOnline(hb.NFT, crypto.DilithiumPublicKey(hb.PubKey), hb.IsSentinel, time.Now())

	case shadownet.MsgTxOffer:
		var offer shadownet.TxOfferPayload
		if err := decode(env, &offer); err != nil {
			n.log("validator: bad tx offer from %s: %v", p, err)
			return
		}
		if n.outage.Active() {
			// Outage recovery (spec 5.6): live admission is paused, so
			// incoming transactions go to the backlog queue instead of
			// the mempool. They still need the same peer-forwarding a
			// live TxOffer gets below — otherwise a tx handed to a node
			// that never ends up building the recovery megabatch would
			// simply never be included — hence outage.Enqueue's own
			// duplicate check, mirroring mempool.Submit's, to keep that
			// forwarding from looping forever.
			switch err := n.outage.Enqueue(offer.Tx, time.Now()); {
			case err == nil:
				n.net.Broadcast(context.Background(), env)
			case errors.Is(err, consensus.ErrDuplicateBacklogTx):
				// Already backlogged — nothing to do.
			default:
				n.log("validator: tx offer from %s not backlogged: %v", p, err)
			}
			return
		}
		switch err := n.mempool.Submit(offer.Tx, time.Now()); {
		case err == nil:
			// Newly admitted: forward to our own peers so this
			// transaction propagates beyond whichever single node it
			// happened to be submitted to. Without this, a tx a wallet
			// hands to a non-proposer node would sit in that node's
			// mempool forever — proposals only ever come from whoever
			// is deterministically assigned proposer for a height, and
			// that node only ever drains its own local mempool.
			n.net.Broadcast(context.Background(), env)
		case errors.Is(err, tx.ErrDuplicateTx):
			// Already circulating (our own earlier forward looping back,
			// or two peers relaying the same offer) — nothing to do.
		default:
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
			n.evaluateOutage(time.Now())
			n.sweepTimeouts()
			n.maybePropose()
		}
	}
}

// evaluateSentinels applies spec 5.5's activation/withdrawal rule against
// this node's own real, locally-observed online-civilian count — every
// node evaluates it independently from its own view, the same way
// AssignCommittee is a pure function of each node's own online set rather
// than shared mutable state (see this package's doc for why).
func (n *Node) evaluateSentinels(now time.Time) {
	civilians := n.onlineCivilianCount(now)
	switch n.sentinels.Evaluate(civilians, now.UnixMilli()) {
	case consensus.ActionActivate:
		n.log("validator: sentinel manager activating (%d online civilians, below threshold %d)", civilians, consensus.SentinelThreshold)
	case consensus.ActionWithdraw:
		n.log("validator: sentinel manager withdrawing (%d online civilians, recovered)", civilians)
	}
}

// evaluateOutage applies spec 5.6's detection condition against this
// node's own real heartbeat history (outageBaseline) and, once triggered,
// declares the outage — pausing live tx admission (handleMessage's
// MsgTxOffer case) and switching maybePropose to dual-track recovery
// batches until MaybeClear (driven from tryFinalizeLocked/
// handleBlockAnnounce once a clean dual-track cycle commits) lifts it.
func (n *Node) evaluateOutage(now time.Time) {
	if n.outage.Active() {
		return // already declared; DetectOutage only matters for the initial trigger
	}
	lastKnown, missing := n.outageBaseline(now)
	if n.outage.DetectOutage(lastKnown, missing) {
		n.outage.Declare()
		n.log("validator: outage declared: %d/%d last-known-online validators missing heartbeats", missing, lastKnown)
	}
}

func (n *Node) sweepTimeouts() {
	now := time.Now()
	n.roundMu.Lock()
	var expired []*round
	for h, r := range n.rounds {
		if now.After(r.deadline) {
			expired = append(expired, r)
			delete(n.rounds, h)
		}
	}
	for _, r := range expired {
		n.log("validator: round at height %d timed out with %d/%d votes, rolling back", r.height, len(r.votes), len(r.committee))
		r.rollback()
		n.tree.TruncateTo(r.treeSnapshotLen)
	}
	n.roundMu.Unlock()

	// Mempool has its own internal locking and isn't touched by any other
	// roundMu-guarded path, so resubmission can happen after releasing
	// roundMu rather than extending its critical section.
	for _, r := range expired {
		for _, t := range r.batch {
			_ = n.mempool.Reinsert(t, time.Now()) // best-effort retry; a full mempool just drops it
		}
	}
}

func (n *Node) maybePropose() {
	nextHeight := n.chn.NextHeight()

	n.roundMu.Lock()
	_, inFlight := n.rounds[nextHeight]
	n.roundMu.Unlock()
	if inFlight {
		return // already proposed or voting on this height
	}

	online := n.onlineSet(time.Now())
	committee := consensus.AssignCommittee(online, nextHeight, committeeSize(len(online)))
	if len(committee) == 0 || committee[0] != n.identity {
		return // not our turn to propose
	}

	dualTrack := n.outage.Active()
	batch := n.buildProposalBatch(dualTrack)
	if len(batch) == 0 {
		return // nothing to propose; avoid empty-block spam
	}

	prop := shadownet.BlockProposalPayload{
		Height:    nextHeight,
		Epoch:     consensus.CurrentEpoch(n.cfg.Genesis, time.Now()),
		Proposer:  n.identity,
		Batch:     batch,
		Timestamp: time.Now().UnixMilli(),
		DualTrack: dualTrack,
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

// buildProposalBatch drains Track A (the live mempool, exactly as always)
// and, during an active outage, tops it up with Track B (drained backlog)
// — spec 5.6's dual-track recovery batch. The combined result stays
// bounded by the same MaxBatchBytes budget that protects every ordinary
// batch: OutageController.BuildMegabatch's own 10x-*count* cap alone isn't
// a safe bound on serialized *size* — this build already hit exactly that
// bug once for the live mempool (see Mempool.DrainBatchBytes's own doc: a
// real post-quantum Dilithium3 signature+pubkey alone is several KB, so a
// count-only cap can still blow past Badger's 1MB per-value limit).
// Anything drained from the backlog but trimmed for lack of room is
// re-enqueued rather than lost.
func (n *Node) buildProposalBatch(dualTrack bool) []types.ShieldedTx {
	entries := n.mempool.DrainBatchBytes(n.cfg.maxBatchSize(), n.cfg.maxBatchBytes())
	batch := make([]types.ShieldedTx, len(entries))
	for i, e := range entries {
		batch[i] = e.Tx
	}
	if !dualTrack {
		return batch
	}

	liveBytes, err := marshaledSize(batch)
	if err != nil {
		n.log("validator: measure live batch size: %v", err)
		return batch
	}
	remaining := n.cfg.maxBatchBytes() - liveBytes
	if remaining <= 0 {
		return batch // no room left for Track B this round; backlog stays queued
	}

	mega := n.outage.BuildMegabatch(n.cfg.maxBatchSize())
	fit, overflow, err := splitByByteBudget(mega, remaining)
	if err != nil {
		n.log("validator: measure megabatch size: %v", err)
		return batch
	}
	for _, t := range overflow {
		// Reinsert, not Enqueue: this is the backlog's own entry coming
		// back after BuildMegabatch drained it, not a new external
		// arrival — Enqueue's duplicate check (needed for gossip
		// forwarding, see handleMessage's MsgTxOffer case) would treat
		// this exact TxID as a dup of itself and silently drop it.
		n.outage.Reinsert(t)
	}
	return append(batch, fit...)
}

func marshaledSize(batch []types.ShieldedTx) (int, error) {
	b, err := json.Marshal(batch)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// splitByByteBudget greedily takes entries from txs (in order) whose
// cumulative JSON-marshaled size stays within budget, returning the taken
// prefix and the remainder — mirrors pkg/tx.Mempool.DrainBatchBytes' own
// greedy size-bounded selection.
func splitByByteBudget(txs []types.ShieldedTx, budget int) (fit, overflow []types.ShieldedTx, err error) {
	total := 0
	for i, t := range txs {
		size, serr := marshaledSize([]types.ShieldedTx{t})
		if serr != nil {
			return nil, nil, serr
		}
		if total+size > budget {
			return txs[:i], txs[i:], nil
		}
		total += size
	}
	return txs, nil, nil
}

// handleBlockProposal runs the pipeline against a proposed batch (whether
// received over the network or self-authored) and, if this node is an
// assigned committee member and every transaction validates, casts a real
// signed vote for the resulting candidate root.
func (n *Node) handleBlockProposal(prop shadownet.BlockProposalPayload) {
	if prop.Height != n.chn.NextHeight() {
		return // stale or premature: doesn't chain onto our current head
	}

	online := n.onlineSet(time.Now())
	committee := consensus.AssignCommittee(online, prop.Height, committeeSize(len(online)))
	if len(committee) == 0 || committee[0] != prop.Proposer {
		n.log("validator: rejecting proposal at height %d: %s is not the assigned proposer", prop.Height, prop.Proposer)
		return
	}
	if !containsID(committee, n.identity) {
		return // not our job to vote on this height
	}
	// Epoch is trusted nowhere else: it's what TallyDueProposals below
	// uses to decide a proposal is due, so a proposer who could claim any
	// Epoch could stall a proposal forever (always claim a past epoch) or
	// force a premature tally (jump it forward). Independently
	// recomputing and comparing closes that off — the same real-not-
	// trusted-caller standard every other consensus input in this
	// package already gets.
	if wantEpoch := consensus.CurrentEpoch(n.cfg.Genesis, time.Now()); prop.Epoch != wantEpoch {
		n.log("validator: rejecting proposal at height %d: claimed epoch %d does not match locally computed epoch %d", prop.Height, prop.Epoch, wantEpoch)
		return
	}

	// Every mutation of n.rounds, n.tree, or n.store made while processing
	// a round is serialized under roundMu — real concurrent network
	// delivery (a proposal and a vote, or two votes, arriving on
	// different libp2p streams at once) would otherwise race on the same
	// *state.MerkleTree.
	n.roundMu.Lock()
	defer n.roundMu.Unlock()

	if _, exists := n.rounds[prop.Height]; exists {
		return // already processing this height
	}

	treeSnapshot := n.tree.Len()
	txn := n.store.BeginTxn()

	entries := make([]tx.Entry, len(prop.Batch))
	now := time.Now()
	for i, t := range prop.Batch {
		entries[i] = tx.Entry{Tx: t, SubmittedAt: now}
	}
	pipeline := tx.NewPipeline(tx.Deps{Store: txn, StateTree: n.tree, ZK: n.zkSys, Vault: n.vlt, Silent: n.silentMon, Oracle: n.oracleQuorum, Epoch: types.EpochNumber(prop.Epoch), Now: func() time.Time { return now }})
	results := pipeline.ProcessBatch(entries)

	if failed := firstFailure(results); failed != nil {
		n.log("validator: rejecting proposal at height %d: tx %s failed the pipeline: %v", prop.Height, failed.Tx.TxID, failed.Error)
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		// The whole candidate is discarded — quorum votes on one
		// deterministic root, so a batch can't be partially included —
		// but every OTHER transaction in it was individually valid
		// (pipeline.ProcessBatch checks each independently) and doesn't
		// deserve to vanish along with the one that wasn't. Reinsert them
		// so the next round (this height retried, or wherever they end
		// up getting proposed) gets a chance to include them instead of
		// silently losing well-formed transactions to one bad apple.
		reinserted := 0
		for _, res := range results {
			if res.Error == nil {
				if err := n.mempool.Reinsert(res.Tx, time.Now()); err == nil {
					reinserted++
				}
			}
		}
		if reinserted > 0 {
			n.log("validator: returned %d otherwise-valid tx(es) from the rejected proposal to the mempool", reinserted)
		}
		return
	}

	// Real epoch-boundary governance tally (spec 17.4): deterministic
	// given this batch's own Epoch plus already-committed proposal state,
	// so every honest node reaches the same outcome — see
	// Pipeline.TallyDueProposals' own doc for why.
	if _, err := pipeline.TallyDueProposals(types.EpochNumber(prop.Epoch)); err != nil {
		n.log("validator: rejecting proposal at height %d: epoch tally failed: %v", prop.Height, err)
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return
	}

	stateRoot := n.tree.Root()
	txRoot := txRootOf(prop.Batch)
	daRoot := daRootOf(prop.Batch)
	block := n.chn.NextBlock(prop.Epoch, prop.Batch, txRoot, stateRoot, daRoot, prop.Proposer, prop.Timestamp)
	block.DualTrack = prop.DualTrack
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
	n.rounds[prop.Height] = r

	voteEnv, err := shadownet.NewEnvelope(shadownet.MsgStageVote, shadownet.StageVotePayload{
		Height: prop.Height, Validator: n.identity, CandidateHash: candidate, Sig: types.DilithiumSig(sig),
	})
	if err != nil {
		n.log("validator: build vote: %v", err)
		return
	}
	n.net.Broadcast(context.Background(), voteEnv)

	n.tryFinalizeLocked(r)
}

func (n *Node) handleStageVote(v shadownet.StageVotePayload) {
	n.roundMu.Lock()
	defer n.roundMu.Unlock()

	r, ok := n.rounds[v.Height]
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

	r.votes = append(r.votes, types.Vote{Validator: v.Validator, StateRoot: v.CandidateHash, Sig: v.Sig})

	n.tryFinalizeLocked(r)
}

// tryFinalizeLocked checks whether r's votes now meet BFT quorum and, if
// so, commits the tentatively-applied batch and grows the chain. Callers
// must already hold roundMu; this never locks it itself, since it's
// invoked from within both handleBlockProposal's and handleStageVote's
// own roundMu-held sections.
func (n *Node) tryFinalizeLocked(r *round) {
	if _, endorsed := n.rounds[r.height]; !endorsed {
		return // already finalized (or rolled back) by a concurrent path
	}
	votes := append([]types.Vote(nil), r.votes...)

	_, quorum := consensus.TallyVotes(r.committee, r.candidate, votes)
	if !quorum {
		return
	}

	r.block.Votes = votes
	if err := n.chn.Append(r.block, r.committee, n.pubKeyLookup); err != nil {
		n.log("validator: quorum reached locally but chain.Append rejected height %d: %v", r.height, err)
		r.rollback()
		n.tree.TruncateTo(r.treeSnapshotLen)
		delete(n.rounds, r.height)
		// Same reasoning as sweepTimeouts' timeout path: quorum was
		// reached, so every tx in this batch already passed the
		// pipeline — losing them here (rather than giving them another
		// round to be included in) would be a real, silent loss, not
		// just a rejected round.
		for _, t := range r.batch {
			_ = n.mempool.Reinsert(t, time.Now())
		}
		return
	}
	if err := r.txn.Commit(); err != nil {
		n.log("validator: FATAL-ish: chain accepted height %d but the state txn failed to commit: %v", r.height, err)
	}

	delete(n.rounds, r.height)

	n.log("validator: committed height %d (%d votes) state_root=%s", r.height, len(votes), r.block.StateRoot)
	n.noteCommittedBlock(r.block)

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

	online := n.onlineSet(time.Now())
	committee := consensus.AssignCommittee(online, height, committeeSize(len(online)))

	n.roundMu.Lock()
	defer n.roundMu.Unlock()

	// Re-check staleness now that roundMu is held: the check above ran
	// before acquiring it, so a concurrent announce for the same height
	// (a duplicate broadcast, or two committee members finalizing at
	// nearly the same time) can pass it before either has advanced the
	// head, then queue up here — this second check catches that case
	// before wasting a replay on a height already adopted.
	if height <= n.chn.HeadHeight() {
		return
	}

	if r, hadRound := n.rounds[height]; hadRound {
		// Whatever this node was tentatively tracking for this height is
		// superseded by the announced block; discard it before replaying.
		delete(n.rounds, height)
		r.rollback()
		n.tree.TruncateTo(r.treeSnapshotLen)
	}

	treeSnapshot := n.tree.Len()
	txn := n.store.BeginTxn()

	entries := make([]tx.Entry, len(ann.Block.Batch))
	now := time.Now()
	for i, t := range ann.Block.Batch {
		entries[i] = tx.Entry{Tx: t, SubmittedAt: now}
	}
	pipeline := tx.NewPipeline(tx.Deps{Store: txn, StateTree: n.tree, ZK: n.zkSys, Vault: n.vlt, Silent: n.silentMon, Oracle: n.oracleQuorum, Epoch: types.EpochNumber(ann.Block.Epoch), Now: func() time.Time { return now }})
	results := pipeline.ProcessBatch(entries)
	for _, res := range results {
		if res.Error != nil {
			n.log("validator: rejecting announced block at height %d: tx %s failed replay: %v", height, res.Tx.TxID, res.Error)
			txn.Discard()
			n.tree.TruncateTo(treeSnapshot)
			return
		}
	}

	// Same deterministic epoch tally as handleBlockProposal, replayed here
	// too — otherwise a node that only ever adopts blocks via announce
	// would silently disagree with the rest of the network about which
	// proposals have already been tallied.
	if _, err := pipeline.TallyDueProposals(types.EpochNumber(ann.Block.Epoch)); err != nil {
		n.log("validator: rejecting announced block at height %d: epoch tally failed: %v", height, err)
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return
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
	n.noteCommittedBlock(ann.Block)
}

// noteCommittedBlock updates outage-recovery bookkeeping for a block this
// node just committed — whether by reaching quorum itself
// (tryFinalizeLocked) or by adopting an announced one
// (handleBlockAnnounce), since either path can be how a node first learns
// a dual-track recovery batch actually made it onto the chain. spec 5.6:
// "clear OutageFlag once backlog is below threshold and one clean
// dual-track cycle has committed" — MaybeClear itself checks both
// conditions, so calling it here is safe even when this particular block
// wasn't the one that satisfies them.
func (n *Node) noteCommittedBlock(b types.Block) {
	if !b.DualTrack {
		return
	}
	n.outage.RecordCleanDualTrackCycle()
	if n.outage.MaybeClear() {
		n.log("validator: outage cleared at height %d", b.Height)
	}
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

// firstFailure returns a pointer to the first failing result, or nil if
// every entry succeeded.
func firstFailure(results []tx.Result) *tx.Result {
	for i := range results {
		if results[i].Error != nil {
			return &results[i]
		}
	}
	return nil
}
