package validator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
		// Phase 2 independent audit finding (critical): a heartbeat's NFT
		// must be the real, self-consistent hash of the PubKey it also
		// carries — exactly how every node derives its own identity (see
		// NewNode's identity: types.NFTID(types.SumHash(pk))). Without
		// this check, recordOnline below would blindly overwrite
		// n.online[hb.NFT].pubKey with whatever key the sender claims,
		// letting any peer hijack an already-online validator's identity
		// (send a heartbeat with NFT = <victim's real ID>, PubKey =
		// <attacker's own key>) and have every later StageVote/block
		// signature "verified" against pubKeyLookup(victim) actually check
		// against the attacker's key instead — full forged-vote/quorum
		// impersonation of that validator, not just an unregistered-Sybil
		// nuisance. This is a self-consistency check only; it does not by
		// itself confirm hb.NFT is a real, minted, on-chain NFT (see
		// HeartbeatPayload's own doc on that separate, already-disclosed
		// gap), but it closes the much sharper hole of hijacking an
		// identity that *is* real.
		if hb.NFT != types.NFTID(types.SumHash(hb.PubKey)) {
			n.log("validator: dropping heartbeat from %s: claimed NFT %s is not Hash(PubKey)", p, hb.NFT)
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
		n.handleBlockAnnounce(p, ann)

	case shadownet.MsgBlockRequest:
		var req shadownet.BlockRequestPayload
		if err := decode(env, &req); err != nil {
			n.log("validator: bad block request from %s: %v", p, err)
			return
		}
		n.handleBlockRequest(p, req)

	case shadownet.MsgBlockResponse:
		var resp shadownet.BlockResponsePayload
		if err := decode(env, &resp); err != nil {
			n.log("validator: bad block response from %s: %v", p, err)
			return
		}
		n.handleBlockResponse(resp)

	case shadownet.MsgMegabatchPart:
		var part shadownet.MegabatchPartPayload
		if err := decode(env, &part); err != nil {
			n.log("validator: bad megabatch part from %s: %v", p, err)
			return
		}
		n.handleMegabatchPart(p, part)

	default:
		// ContainerSync, SilentPad: accepted on the wire (rate-limited
		// like every other type) but not acted on by this package — see
		// the package doc's scope note.
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

		// A round timing out without local quorum doesn't mean the
		// network as a whole failed to finalize this height — another
		// node may have reached quorum and broadcast a real
		// BlockAnnounce (tryFinalizeLocked) that this node's link simply
		// never delivered, since that broadcast is a real, one-shot
		// send with no retry of its own (spec 18.6 hardening: consensus
		// must still converge under real jitter/packet loss, not just a
		// clean network). Proactively asking every currently connected
		// peer for this exact height closes that gap through the same
		// real, independently-reverified catch-up path
		// (requestCatchUp/handleBlockRequest/handleBlockResponse) a
		// multi-block-behind node already relies on. A peer that doesn't
		// have it either just answers with nothing
		// (handleBlockRequest), so this is always safe to repeat on
		// every timeout, and handleBlockResponse's own re-verification
		// means a stale or duplicate reply can never cause anything
		// beyond a no-op.
		for _, p := range n.net.Host.Network().Peers() {
			n.requestCatchUp(p, r.height, r.height)
		}
	}

	// Phase 2 independent audit finding: a node can fall behind a height
	// it has already seen referenced by a real BlockAnnounce without ever
	// having (or keeping) a tracked round for it — tryAdoptBlockLocked
	// unconditionally deletes any existing round before it even knows
	// whether adoption will succeed, so a rejected announce (e.g. a
	// transient local committee-view mismatch under real jitter/packet
	// loss — this was observed causing a permanent stall in
	// TestFourNodesConvergeUnderJitterAndPacketLoss) previously left
	// nothing behind for the retry loop above to ever fire from. This
	// piggybacks on the same periodic sweep to keep asking for a height
	// this node knows exists, independent of the per-round expiry loop,
	// until its own view catches up enough for tryAdoptBlockLocked to
	// actually succeed — using the exact same requestCatchUp/
	// handleBlockResponse path above, which independently re-verifies
	// everything regardless of how many times it's asked.
	n.roundMu.Lock()
	nextHeight := n.chn.NextHeight()
	behind := n.highestAnnounced >= nextHeight
	target := n.highestAnnounced
	n.roundMu.Unlock()
	if behind {
		for _, p := range n.net.Host.Network().Peers() {
			n.requestCatchUp(p, nextHeight, target)
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
// re-enqueued rather than lost. filterReadyReveals also holds back any
// TxVoteReveal whose commit hasn't caught up yet — see its own doc for
// the real network-liveness bug this closes.
func (n *Node) buildProposalBatch(dualTrack bool) []types.ShieldedTx {
	entries := n.mempool.DrainBatchBytes(n.cfg.maxBatchSize(), n.cfg.maxBatchBytes())
	drained := make([]types.ShieldedTx, len(entries))
	for i, e := range entries {
		drained[i] = e.Tx
	}
	batch, deferred := n.filterReadyReveals(drained)
	if len(deferred) > 0 {
		now := time.Now()
		for _, t := range deferred {
			if err := n.mempool.Reinsert(t, now); err != nil {
				n.log("validator: re-enqueue not-yet-ready reveal: %v", err)
			}
		}
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

	megaDrained := n.outage.BuildMegabatch(n.cfg.maxBatchSize())
	if len(megaDrained) > 0 {
		// Real transparency broadcast (shadownet.MegabatchPartPayload's
		// own doc): the FULL pre-trim megabatch, not just whatever ends
		// up fitting the committee's own MaxBatchBytes-bounded proposal
		// below — best-effort, never fatal to proposing.
		n.broadcastMegabatchPart(n.chn.NextHeight(), megaDrained)
	}
	mega, megaDeferred := n.filterReadyReveals(megaDrained)
	for _, t := range megaDeferred {
		n.outage.Reinsert(t)
	}
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

// filterReadyReveals splits txs into ready (safe to include in a proposal
// now) and deferred (a TxVoteReveal whose corresponding TxVote commit
// isn't visible yet — neither durably committed in n.store nor earlier in
// this same candidate batch). Everything else passes through as ready
// unconditionally.
//
// This closes a real network-liveness bug found live under sustained
// 3-node TxVote/TxVoteReveal traffic (not hypothetical): gossip delivery
// over independent libp2p connections gives no ordering guarantee, so a
// reveal a wallet (or cmd/walletsim) sends moments after its own commit,
// without waiting for confirmation, can genuinely arrive at a node before
// that node has admitted or committed the matching commit. Stage 4 then
// rejects the reveal outright ("has not cast a ballot" / "no such
// proposal"), and spec 5.3's whole-batch atomicity rule means the ENTIRE
// proposal — every other, individually valid tx bundled alongside it —
// gets discarded too; under continuous traffic this can repeat every
// single round indefinitely; a live 3-node cluster stalled completely
// (multiple minutes, zero committed heights) hitting exactly this.
// Deferring the not-yet-ready reveal back to the mempool (via Reinsert,
// tried again in a later round once its commit has hopefully caught up)
// fixes it without weakening Stage 4's real cryptographic check for a
// reveal that's genuinely wrong (bad nonce/approve, already revealed,
// already tallied) — those still fail hard, exactly as before.
func (n *Node) filterReadyReveals(txs []types.ShieldedTx) (ready, deferred []types.ShieldedTx) {
	// voter correlates a same-batch TxVote/TxVoteReveal pair by the real
	// anonymous eligibility proof's Nullifier — not by SignerPubKey
	// (types.VoteEligibilityProof's own doc: a wallet should sign each
	// vote with a fresh, unlinked key, so SumHash(SignerPubKey) can no
	// longer be assumed to match between a commit and its own reveal).
	// Nullifier is deterministic given the same (VoterSK, ProposalID)
	// pair, so it still reliably identifies "the same real voter, same
	// proposal" here exactly as SumHash(SignerPubKey) used to.
	seenThisBatch := map[types.ID]map[types.Hash]bool{}
	for _, t := range txs {
		switch t.Kind {
		case types.TxVote:
			if t.VotePublicInputs != nil && t.VoteEligibility != nil {
				voter := t.VoteEligibility.Nullifier
				pid := t.VotePublicInputs.ProposalID
				if seenThisBatch[pid] == nil {
					seenThisBatch[pid] = map[types.Hash]bool{}
				}
				seenThisBatch[pid][voter] = true
			}
			ready = append(ready, t)

		case types.TxVoteReveal:
			if t.VoteRevealPublicInputs == nil || t.VoteEligibility == nil {
				ready = append(ready, t) // malformed; let Stage 2 reject it normally
				continue
			}
			voter := t.VoteEligibility.Nullifier
			pid := t.VoteRevealPublicInputs.ProposalID
			if seenThisBatch[pid][voter] {
				ready = append(ready, t) // its commit is earlier in this very batch
				continue
			}
			if record, found, err := n.store.GetProposal(string(pid)); err == nil && found {
				if _, committed := record.Commitments[voter]; committed {
					ready = append(ready, t) // its commit is already durably committed
					continue
				}
			}
			deferred = append(deferred, t)

		default:
			ready = append(ready, t)
		}
	}
	return ready, deferred
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
	pipeline := tx.NewPipeline(tx.Deps{Store: txn, StateTree: n.tree, ZK: n.zkSys, ZKTree: n.zkTree, ZKRoots: n.zkRoots, Vault: n.vlt, Silent: n.silentMon, Oracle: n.oracleQuorum, Governance: n.governanceParams, Epoch: types.EpochNumber(prop.Epoch), Height: prop.Height, TrustedPoHAttestors: n.trustedPoHAttestors, EligibilityZK: n.eligibilityZK, EligibilityTree: n.eligibilityTree, EligibilityRoots: n.eligibilityRoots, MintZK: n.mintZK, StakeZK: n.stakeZK, UnstakeZK: n.unstakeZK, StakeTree: n.stakeTree, StakeRoots: n.stakeRoots, Now: func() time.Time { return now }})
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
	// prop.Batch is now spoken for by this in-flight round, whether this
	// node proposed it or only voted on it (see
	// pruneCommittedFromLocalQueues' own doc for the real multi-node
	// liveness bug this closes); if the round later rolls back, the
	// existing sweepTimeouts/tryFinalizeLocked failure paths already
	// reinsert the whole batch, so pruning here now is safe.
	n.pruneCommittedFromLocalQueues(prop.Batch)

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

	// Phase 2 independent audit finding (medium): StageVote is not one of
	// pkg/net's rate-limited message types (only Heartbeat/TxOffer are, per
	// spec 6), so nothing previously stopped a peer from replaying the same
	// already-valid, already-broadcast vote envelope indefinitely for a
	// round's whole lifetime — each replay still passes the real signature
	// check above and was unconditionally appended to r.votes, growing it
	// (and the cost of every subsequent tryFinalizeLocked re-tally) without
	// bound. consensus.TallyVotes already dedupes by validator for the
	// purpose of *counting* quorum, but that doesn't stop the underlying
	// slice, and the CPU spent re-verifying and re-tallying every replay,
	// from growing unbounded. Dropping an already-recorded validator's
	// vote here, before it's ever appended, closes that memory/CPU
	// amplification directly, independent of TallyVotes' own protection.
	for _, existing := range r.votes {
		if existing.Validator == v.Validator {
			return
		}
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

// recordHighestAnnounced remembers the highest height this node has ever
// seen referenced by a real BlockAnnounce, so sweepTimeouts can keep
// retrying catch-up for it even when adopting it never establishes (or
// itself destroys) a tracked round to time out — see highestAnnounced's
// own doc on Node.
func (n *Node) recordHighestAnnounced(height uint64) {
	n.roundMu.Lock()
	if height > n.highestAnnounced {
		n.highestAnnounced = height
	}
	n.roundMu.Unlock()
}

// handleBlockAnnounce lets a node that did not itself reach quorum locally
// (it wasn't on the committee, or its votes arrived too late) adopt a
// finalized block by independently replaying it, or — if this node has
// fallen more than one block behind — first triggers real multi-block
// catch-up (requestCatchUp) against whoever sent the announce, previously
// out of scope per this package's own doc.
func (n *Node) handleBlockAnnounce(sender peer.ID, ann shadownet.BlockAnnouncePayload) {
	height := ann.Block.Height
	n.recordHighestAnnounced(height)
	if height <= n.chn.HeadHeight() {
		return // stale
	}
	if height > n.chn.NextHeight() {
		// More than one block behind: this node cannot verify ann.Block
		// yet (chain.Append requires the immediately preceding block to
		// already be canonical), so request everything it's missing,
		// this announced block included, from the peer that sent it.
		// The eventual BlockResponse (handleBlockResponse) replays and
		// adopts each real block in order via the exact same tryAdoptBlockLocked
		// this function itself uses below — no separate re-adoption of
		// ann.Block is needed once that catches this node up.
		n.requestCatchUp(sender, n.chn.NextHeight(), height)
		return
	}

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
	if err := n.tryAdoptBlockLocked(ann.Block, false); err != nil {
		n.log("validator: rejecting announced block at height %d: %v", height, err)
		return
	}
	n.log("validator: adopted announced height %d state_root=%s", height, ann.Block.StateRoot)
}

// tryAdoptBlockLocked independently replays and, if the freshly
// recomputed state root agrees with b's own and chain.Append's own
// quorum/signature reverification passes, adopts b — the real
// verification core both handleBlockAnnounce's single-block path and
// handleBlockResponse's multi-block catch-up path share. Callers must
// already hold roundMu, and b.Height must equal n.chn.NextHeight().
//
// excludeSelfFromCommittee is a Phase 2 independent audit finding: every
// node counts itself in its own onlineSet from the moment it's
// constructed (see NewNode), so recomputing "the committee" from this
// node's own current online view always includes self — correct for
// handleBlockAnnounce's single-block path (b.Height is this node's own
// very next height, so self was almost certainly online and a real
// candidate committee member for this exact round, even if its vote
// simply arrived too late to reach local quorum itself), but wrong for
// handleBlockResponse's multi-block catch-up path: there, b can be an
// arbitrarily old height from before this node ever joined the network,
// so self provably cast no vote in that historical round, and including
// it anyway inflates the recomputed committee's denominator with an
// identity that couldn't have contributed to b's real quorum — compounding
// with every other node met since, this can push BFTQuorumMet's real
// 2/3+ supermajority threshold on the already-quorate vote set b actually
// carries out of reach, wrongly rejecting a legitimately finalized block
// (see TestNodeCatchesUpAcrossMultipleBlocks). Callers pass true only for
// that multi-block catch-up case; excluding self there only ever shrinks
// the denominator toward the committee's true historical size, never
// below the votes b actually carries, so it cannot turn an illegitimate
// quorum into a legitimate one.
func (n *Node) tryAdoptBlockLocked(b types.Block, excludeSelfFromCommittee bool) error {
	height := b.Height
	if height != n.chn.NextHeight() {
		return fmt.Errorf("block at height %d is not the expected next height %d", height, n.chn.NextHeight())
	}

	online := n.onlineSet(time.Now())
	if excludeSelfFromCommittee {
		online = excludeSelf(online, n.identity)
	}
	committee := consensus.AssignCommittee(online, height, committeeSize(len(online)))

	if r, hadRound := n.rounds[height]; hadRound {
		// Whatever this node was tentatively tracking for this height is
		// superseded by the real block being adopted; discard it before
		// replaying.
		delete(n.rounds, height)
		r.rollback()
		n.tree.TruncateTo(r.treeSnapshotLen)
	}

	treeSnapshot := n.tree.Len()
	txn := n.store.BeginTxn()

	entries := make([]tx.Entry, len(b.Batch))
	now := time.Now()
	for i, t := range b.Batch {
		entries[i] = tx.Entry{Tx: t, SubmittedAt: now}
	}
	pipeline := tx.NewPipeline(tx.Deps{Store: txn, StateTree: n.tree, ZK: n.zkSys, ZKTree: n.zkTree, ZKRoots: n.zkRoots, Vault: n.vlt, Silent: n.silentMon, Oracle: n.oracleQuorum, Governance: n.governanceParams, Epoch: types.EpochNumber(b.Epoch), Height: b.Height, TrustedPoHAttestors: n.trustedPoHAttestors, EligibilityZK: n.eligibilityZK, EligibilityTree: n.eligibilityTree, EligibilityRoots: n.eligibilityRoots, MintZK: n.mintZK, StakeZK: n.stakeZK, UnstakeZK: n.unstakeZK, StakeTree: n.stakeTree, StakeRoots: n.stakeRoots, Now: func() time.Time { return now }})
	results := pipeline.ProcessBatch(entries)
	for _, res := range results {
		if res.Error != nil {
			txn.Discard()
			n.tree.TruncateTo(treeSnapshot)
			return fmt.Errorf("tx %s failed replay: %w", res.Tx.TxID, res.Error)
		}
	}

	// Same deterministic epoch tally as handleBlockProposal, replayed here
	// too — otherwise a node that only ever adopts blocks via announce or
	// catch-up would silently disagree with the rest of the network about
	// which proposals have already been tallied.
	if _, err := pipeline.TallyDueProposals(types.EpochNumber(b.Epoch)); err != nil {
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return fmt.Errorf("epoch tally failed: %w", err)
	}

	stateRoot := n.tree.Root()
	if stateRoot != b.StateRoot {
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return fmt.Errorf("recomputed state root %s disagrees with announced %s", stateRoot, b.StateRoot)
	}

	if err := n.chn.Append(b, committee, n.pubKeyLookup); err != nil {
		txn.Discard()
		n.tree.TruncateTo(treeSnapshot)
		return fmt.Errorf("chain.Append: %w", err)
	}
	if err := txn.Commit(); err != nil {
		return fmt.Errorf("FATAL-ish: adopted height %d but the state txn failed to commit: %w", height, err)
	}
	// b.Batch is now durably committed regardless of whether this node
	// ever locally drained it — see pruneCommittedFromLocalQueues' own
	// doc.
	n.pruneCommittedFromLocalQueues(b.Batch)
	n.noteCommittedBlock(b)
	return nil
}

// requestCatchUp asks sender (net.Node.Send, unicast — not a broadcast,
// since only the peer that just proved it has a block this node lacks is
// asked) for every real, already-committed block in [from, to], capped
// at shadownet.MaxCatchUpBlocks. handleBlockResponse independently
// re-verifies and replays whatever comes back — this call only ever
// asks; it never trusts.
func (n *Node) requestCatchUp(sender peer.ID, from, to uint64) {
	if to < from {
		return
	}
	if to-from+1 > shadownet.MaxCatchUpBlocks {
		to = from + shadownet.MaxCatchUpBlocks - 1
	}
	env, err := shadownet.NewEnvelope(shadownet.MsgBlockRequest, shadownet.BlockRequestPayload{FromHeight: from, ToHeight: to})
	if err != nil {
		n.log("validator: build block request: %v", err)
		return
	}
	if err := n.net.Send(context.Background(), sender, env); err != nil {
		n.log("validator: send block request to %s: %v", sender, err)
	}
}

// handleBlockRequest answers a peer's real catch-up request with every
// real block this node actually has stored in the requested range (still
// capped at shadownet.MaxCatchUpBlocks even if the requester's own claim
// was larger) — never fabricated to fill a gap this node doesn't
// actually have; it stops at the first missing height, which is either
// this node's own head or a real hole neither side can do anything
// about right now.
func (n *Node) handleBlockRequest(sender peer.ID, req shadownet.BlockRequestPayload) {
	if req.ToHeight < req.FromHeight {
		return
	}
	to := req.ToHeight
	if to-req.FromHeight+1 > shadownet.MaxCatchUpBlocks {
		to = req.FromHeight + shadownet.MaxCatchUpBlocks - 1
	}
	var blocks []types.Block
	for h := req.FromHeight; h <= to; h++ {
		b, found, err := n.store.GetBlock(h)
		if err != nil {
			n.log("validator: get block %d for catch-up response to %s: %v", h, sender, err)
			break
		}
		if !found {
			break
		}
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return
	}
	env, err := shadownet.NewEnvelope(shadownet.MsgBlockResponse, shadownet.BlockResponsePayload{Blocks: blocks})
	if err != nil {
		n.log("validator: build block response: %v", err)
		return
	}
	if err := n.net.Send(context.Background(), sender, env); err != nil {
		n.log("validator: send block response to %s: %v", sender, err)
	}
}

// handleBlockResponse replays every block a real catch-up response
// carries, in ascending height order, through the identical
// independent-reverification path (tryAdoptBlockLocked) a single
// BlockAnnounce gets — nothing here is trusted just because it arrived
// as a response to this node's own request. Stops at the first height
// that isn't exactly this node's own next height (a gap, an
// out-of-order/duplicate response, or a failed replay), leaving the
// remainder for a later announce or request rather than skipping ahead
// unsafely.
func (n *Node) handleBlockResponse(resp shadownet.BlockResponsePayload) {
	blocks := append([]types.Block(nil), resp.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].Height < blocks[j].Height })

	// Phase 2 independent audit finding: more than one block in a single
	// response means this node was genuinely more than one block behind
	// when it asked — real multi-block catch-up, where at least the
	// earlier heights in this batch plausibly predate this node ever
	// being online (see tryAdoptBlockLocked's own doc on
	// excludeSelfFromCommittee). A single-block response, by contrast, is
	// indistinguishable from an ordinary "missed the one announce, retry
	// requested it" case — the same live, very-next-height situation
	// handleBlockAnnounce's direct path already treats as "self was
	// plausibly a real committee member", so self is not excluded there
	// either.
	excludeSelfFromCommittee := len(blocks) > 1

	n.roundMu.Lock()
	defer n.roundMu.Unlock()

	for _, b := range blocks {
		if b.Height <= n.chn.HeadHeight() {
			continue // already have it
		}
		if b.Height != n.chn.NextHeight() {
			break // a gap, or out of order; stop rather than skip ahead
		}
		if err := n.tryAdoptBlockLocked(b, excludeSelfFromCommittee); err != nil {
			n.log("validator: catch-up replay failed at height %d: %v", b.Height, err)
			break
		}
		n.log("validator: caught up to height %d via block response", b.Height)
	}
}

// broadcastMegabatchPart announces the real, full pre-trim megabatch a
// dual-track proposal round is draining, chunked over the real wire — see
// shadownet.MegabatchPartPayload's own doc for why this exists as a real,
// disclosed transparency channel alongside (never instead of) the actual
// committee-voted proposal buildProposalBatch still assembles separately.
// Best-effort: a marshal or send failure here never blocks or fails
// proposing, only the broadcast.
func (n *Node) broadcastMegabatchPart(height uint64, batch []types.ShieldedTx) {
	data, err := json.Marshal(batch)
	if err != nil {
		n.log("validator: marshal megabatch for broadcast: %v", err)
		return
	}
	partCount := (len(data) + shadownet.MaxMegabatchPartBytes - 1) / shadownet.MaxMegabatchPartBytes
	if partCount == 0 {
		partCount = 1
	}
	ctx := context.Background()
	for i := 0; i < partCount; i++ {
		start := i * shadownet.MaxMegabatchPartBytes
		end := start + shadownet.MaxMegabatchPartBytes
		if end > len(data) {
			end = len(data)
		}
		env, err := shadownet.NewEnvelope(shadownet.MsgMegabatchPart, shadownet.MegabatchPartPayload{
			Height: height, PartIndex: i, PartCount: partCount, Data: data[start:end],
		})
		if err != nil {
			n.log("validator: build megabatch part %d/%d: %v", i+1, partCount, err)
			return
		}
		n.net.Broadcast(ctx, env)
	}
}

// handleMegabatchPart buffers one real chunk of a peer's megabatch
// announcement and, once every chunk has arrived, reassembles and
// records it — see shadownet.MegabatchPartPayload's own doc. Never
// trusted for anything consensus-affecting: a malformed, incomplete, or
// dishonest announcement can only ever fail to reassemble, or produce a
// recorded result a later observer compares against reality — it never
// touches real chain state.
func (n *Node) handleMegabatchPart(sender peer.ID, part shadownet.MegabatchPartPayload) {
	if part.PartCount <= 0 || part.PartCount > shadownet.MaxMegabatchParts || part.PartIndex < 0 || part.PartIndex >= part.PartCount {
		n.log("validator: bad megabatch part index/count from %s: %d/%d", sender, part.PartIndex, part.PartCount)
		return
	}
	if len(part.Data) > shadownet.MaxMegabatchPartBytes {
		n.log("validator: oversized megabatch part from %s: %d bytes", sender, len(part.Data))
		return
	}

	n.megabatchMu.Lock()
	key := megabatchKey{sender: sender, height: part.Height}
	asm, ok := n.megabatchRecv[key]
	if !ok || part.PartCount != len(asm.parts) {
		// Either the first part of a new announcement, or a peer
		// restarting mid-stream with a different part count — either way,
		// start a fresh assembly rather than reassembling a mismatched
		// mix of two different broadcasts.
		asm = &megabatchAssembly{parts: make([][]byte, part.PartCount)}
		n.megabatchRecv[key] = asm
	}
	if asm.parts[part.PartIndex] == nil {
		asm.seen++
	}
	asm.parts[part.PartIndex] = part.Data
	complete := asm.seen == part.PartCount
	if complete {
		delete(n.megabatchRecv, key)
	}
	n.megabatchMu.Unlock()

	if !complete {
		return
	}

	var data []byte
	for _, p := range asm.parts {
		data = append(data, p...)
	}
	var batch []types.ShieldedTx
	if err := json.Unmarshal(data, &batch); err != nil {
		n.log("validator: reassembled megabatch from %s at height %d failed to decode: %v", sender, part.Height, err)
		return
	}
	n.log("validator: reassembled real megabatch from %s at height %d: %d tx(es)", sender, part.Height, len(batch))
	n.recordCompletedMegabatch(part.Height, batch)
}

// recordCompletedMegabatch stores batch under height in megabatchDone,
// evicting the oldest entry once megabatchDoneCap is exceeded — see that
// field's own doc.
func (n *Node) recordCompletedMegabatch(height uint64, batch []types.ShieldedTx) {
	n.megabatchMu.Lock()
	defer n.megabatchMu.Unlock()
	if _, exists := n.megabatchDone[height]; !exists {
		n.megabatchDoneOrder = append(n.megabatchDoneOrder, height)
	}
	n.megabatchDone[height] = batch
	for len(n.megabatchDoneOrder) > megabatchDoneCap {
		oldest := n.megabatchDoneOrder[0]
		n.megabatchDoneOrder = n.megabatchDoneOrder[1:]
		delete(n.megabatchDone, oldest)
	}
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

// pruneCommittedFromLocalQueues removes every tx in batch from this
// node's own local mempool and outage backlog. batch has just become
// spoken for by an in-flight or already-committed round; without this, a
// node that only ever votes (never proposes) keeps a stale local copy of
// every gossiped tx even after it's durably committed via someone else's
// proposal, and later drags that stale copy into its own proposal once it
// *is* the proposer — Stage 4 rejects it outright (its effect is already
// applied), and the whole-batch-atomicity rule (spec 5.3) then discards
// every other, individually-valid tx bundled alongside it too. This is a
// real failure mode this build hit live under sustained multi-node
// traffic (three real OS processes, real gossip, a rejected-and-reinserted
// batch count climbing round after round) — not a hypothetical one. If
// the round this batch belongs to later rolls back (timeout,
// chain.Append failure), the existing rollback paths (sweepTimeouts,
// tryFinalizeLocked) already reinsert the whole batch via Mempool.Reinsert/
// OutageController.Reinsert, so pruning proactively here is safe.
func (n *Node) pruneCommittedFromLocalQueues(batch []types.ShieldedTx) {
	ids := make([]types.Hash, len(batch))
	for i, t := range batch {
		ids[i] = t.TxID
	}
	n.mempool.Remove(ids)
	n.outage.Remove(ids)
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

// excludeSelf returns ids with self removed, preserving order, without
// mutating the input slice — used by tryAdoptBlockLocked (see its own
// doc) to keep a node's own just-joined identity from inflating the
// committee denominator it recomputes for a round it never voted in.
func excludeSelf(ids []types.NFTID, self types.NFTID) []types.NFTID {
	out := make([]types.NFTID, 0, len(ids))
	for _, id := range ids {
		if id != self {
			out = append(out, id)
		}
	}
	return out
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
