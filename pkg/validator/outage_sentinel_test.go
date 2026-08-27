package validator

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// These tests prove pkg/consensus's SentinelManager and OutageController —
// real, already-unit-tested in their own package, but previously never
// invoked from anywhere in this package's round loop (see this package's
// doc's scope note) — are now genuinely wired into Node's real behavior:
// sentinel-flagged nodes actually stand down and reactivate off real
// heartbeat counts, incoming transactions actually get diverted to the
// backlog during a real outage, and a real dual-track recovery batch that
// reaches BFT quorum actually clears OutageFlag.

func TestOnlineCivilianCountExcludesSentinels(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	now := time.Now()
	civ1, civ2, sent1 := genPeer(t), genPeer(t), genPeer(t)
	n.recordOnline(civ1.id, civ1.pk, false, now)
	n.recordOnline(civ2.id, civ2.pk, false, now)
	n.recordOnline(sent1.id, sent1.pk, true, now)

	// newTestNode's own self-record (civilian, from NewNode) plus civ1/civ2
	// = 3 civilians; sent1 must not be counted.
	if got := n.onlineCivilianCount(now); got != 3 {
		t.Fatalf("expected 3 online civilians (self + civ1 + civ2), got %d", got)
	}
}

func TestEvaluateSentinelsActivatesAndWithdraws(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	now := time.Now()

	// Only the node's own self-record is online (1 civilian) — well below
	// SentinelThreshold (10) — so evaluating must activate.
	n.evaluateSentinels(now)
	if !n.sentinels.Active() {
		t.Fatalf("expected sentinels to activate with only 1 online civilian")
	}

	// Bring 10 more real civilians online: 11 total, at/above threshold.
	for i := 0; i < 10; i++ {
		p := genPeer(t)
		n.recordOnline(p.id, p.pk, false, now)
	}
	n.evaluateSentinels(now)
	if n.sentinels.Active() {
		t.Fatalf("expected sentinels to withdraw once online civilians reach the threshold")
	}
}

// newTestSentinelNode is newTestNode but with isSentinel=true and without
// pre-recording itself online at construction — matching NewNode's own
// real behavior (see NewNode's doc): a sentinel does not join the online
// set until sentinels are actually activated.
func newTestSentinelNode(t *testing.T, roundTimeout time.Duration, genesisMs int64) *Node {
	t.Helper()
	n := newTestNode(t, roundTimeout, genesisMs)
	n.isSentinel = true
	n.mu.Lock()
	delete(n.online, n.identity) // undo newTestNode's civilian self-record
	delete(n.everSeen, n.identity)
	n.mu.Unlock()
	return n
}

// TestSentinelNodeParticipationGatedOnActivation exercises the exact
// condition heartbeatLoop applies to a sentinel-flagged node
// (n.isSentinel && !n.sentinels.Active() => stand down) directly, since
// heartbeatLoop itself is timing/goroutine-driven — real multi-process
// integration territory per this file's own header comment — rather than
// something a deterministic unit test should drive via a live ticker.
func TestSentinelNodeParticipationGatedOnActivation(t *testing.T) {
	n := newTestSentinelNode(t, time.Minute, time.Now().UnixMilli())
	now := time.Now()

	// A lone sentinel evaluating itself sees 0 online civilians (it never
	// recorded itself, by design) — well below threshold, so it must
	// activate immediately.
	n.evaluateSentinels(now)
	if !n.sentinels.Active() {
		t.Fatalf("expected a lone sentinel to activate when 0 civilians are online")
	}
	if n.isSentinel && !n.sentinels.Active() {
		t.Fatalf("heartbeatLoop's stand-down condition must not hold once sentinels are active")
	}

	// Bring 10 real civilians online: sentinels should now withdraw, and
	// heartbeatLoop's stand-down condition must hold again.
	for i := 0; i < 10; i++ {
		p := genPeer(t)
		n.recordOnline(p.id, p.pk, false, now)
	}
	n.evaluateSentinels(now)
	if n.sentinels.Active() {
		t.Fatalf("expected sentinels to withdraw once 10 civilians are online")
	}
	if !n.isSentinel || n.sentinels.Active() {
		t.Fatalf("expected heartbeatLoop's stand-down condition to hold once sentinels withdraw")
	}
}

func TestOutageBaselineAndDetection(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	base := time.Now()

	// 3 more real peers heartbeat at t=0 (plus self = 4 last-known-online).
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	n.recordOnline(p1.id, p1.pk, false, base)
	n.recordOnline(p2.id, p2.pk, false, base)
	n.recordOnline(p3.id, p3.pk, false, base)

	// Advance well past OnlineTimeout (1 minute in newTestNode's Config)
	// for p1/p2/p3, but keep self fresh, and stay within
	// outageBaselineWindow (10 minutes) so they still count toward the
	// baseline as "missing" rather than aged out entirely.
	later := base.Add(2 * time.Minute)
	n.recordOnline(n.identity, n.pk, false, later) // self stays fresh

	lastKnown, missing := n.outageBaseline(later)
	if lastKnown != 4 {
		t.Fatalf("expected 4 last-known-online identities, got %d", lastKnown)
	}
	if missing != 3 {
		t.Fatalf("expected 3 missing (p1/p2/p3, all past OnlineTimeout), got %d", missing)
	}

	// 3/4 = 75% > 50% threshold: DetectOutage must trigger, and
	// evaluateOutage must actually declare it.
	if !n.outage.DetectOutage(lastKnown, missing) {
		t.Fatalf("expected DetectOutage to trigger at 75%% missing")
	}
	n.evaluateOutage(later)
	if !n.outage.Active() {
		t.Fatalf("expected evaluateOutage to declare the outage")
	}
}

func TestTxOfferRoutesToBacklogDuringOutage(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	n.outage.Declare()

	voteTx := mustSignVote(t, "outage-proposal", 1)
	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: voteTx})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	n.handleMessage(peer.ID("test-peer"), env)

	if n.mempool.Len() != 0 {
		t.Fatalf("expected the live mempool to stay empty during an active outage, got %d", n.mempool.Len())
	}
	if n.outage.BacklogDepth() != 1 {
		t.Fatalf("expected the tx to land in the outage backlog, depth=%d", n.outage.BacklogDepth())
	}
}

func TestTxOfferAdmitsToMempoolWhenNoOutage(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())

	voteTx := mustSignVote(t, "normal-proposal", 1)
	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: voteTx})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	n.handleMessage(peer.ID("test-peer"), env)

	if n.mempool.Len() != 1 {
		t.Fatalf("expected normal (non-outage) TxOffer admission to the live mempool, got len=%d", n.mempool.Len())
	}
	if n.outage.BacklogDepth() != 0 {
		t.Fatalf("expected the outage backlog to stay empty outside an outage, depth=%d", n.outage.BacklogDepth())
	}
}

// sizedVoteTx builds a real, individually-valid, distinctly-identified
// TxVote padded with a Memo so its JSON-marshaled size is deterministic
// and controllable — the same technique pkg/tx/mempool_test.go uses to
// test DrainBatchBytes' real size-based behavior.
func sizedVoteTx(t *testing.T, proposalID string, commitment byte, paddingLen int) types.ShieldedTx {
	t.Helper()
	tx := mustSignVote(t, proposalID, commitment)
	tx.Memo = make([]byte, paddingLen)
	return tx
}

func TestBuildProposalBatchDualTrackCombinesWithinByteBudget(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	n.cfg.MaxBatchSize = 100

	live := sizedVoteTx(t, "live-1", 1, 50)
	if err := n.mempool.Submit(live, time.Now()); err != nil {
		t.Fatalf("submit live tx: %v", err)
	}

	// Measure one real, fully-signed tx's actual marshaled size (a real
	// Dilithium3 signature+pubkey alone is several KB — see
	// Mempool.DrainBatchBytes's own doc — so a guessed byte budget would
	// either be meaninglessly huge or too small to fit even one tx) and
	// size the budget off it: room for the live tx plus exactly one
	// similar-sized backlog entry, not all three.
	oneTxSize, err := marshaledSize([]types.ShieldedTx{live})
	if err != nil {
		t.Fatalf("measure one tx size: %v", err)
	}
	n.cfg.MaxBatchBytes = oneTxSize*2 + 200

	now := time.Now()
	backlogTxs := []types.ShieldedTx{
		sizedVoteTx(t, "backlog-1", 2, 50),
		sizedVoteTx(t, "backlog-2", 3, 50),
		sizedVoteTx(t, "backlog-3", 4, 50), // should overflow the budget and get re-enqueued
	}
	for _, bt := range backlogTxs {
		if err := n.outage.Enqueue(bt, now); err != nil {
			t.Fatalf("enqueue backlog tx: %v", err)
		}
	}

	batch := n.buildProposalBatch(true)

	// The live tx must always be present.
	foundLive := false
	for _, bt := range batch {
		if bt.TxID == live.TxID {
			foundLive = true
		}
	}
	if !foundLive {
		t.Fatalf("expected Track A's live tx to be present in the combined batch")
	}
	if len(batch) <= 1 {
		t.Fatalf("expected at least some Track B backlog to be combined in, got only %d entries", len(batch))
	}

	size, err := marshaledSize(batch)
	if err != nil {
		t.Fatalf("measure batch size: %v", err)
	}
	if size > n.cfg.maxBatchBytes() {
		t.Fatalf("combined dual-track batch (%d bytes) exceeds MaxBatchBytes (%d) — the exact livelock class this build already hit once for the live mempool", size, n.cfg.maxBatchBytes())
	}

	// Whatever didn't fit must have been re-enqueued, not dropped.
	remaining := n.outage.BacklogDepth()
	includedFromBacklog := len(batch) - 1 // minus the live tx
	if remaining+includedFromBacklog != len(backlogTxs) {
		t.Fatalf("expected every backlog tx to be either included or re-enqueued (none dropped): included=%d remaining=%d want_total=%d", includedFromBacklog, remaining, len(backlogTxs))
	}
}

func TestNoteCommittedBlockRecordsCleanCycleAndClears(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	n.outage.Declare()
	if !n.outage.Active() {
		t.Fatalf("expected outage to be active after Declare")
	}

	// Backlog is already empty (DefaultOutageThresholds' BacklogClearThreshold
	// is 0) — noteCommittedBlock's one clean dual-track cycle is the only
	// remaining condition MaybeClear needs.
	n.noteCommittedBlock(types.Block{Height: 5, DualTrack: true})
	if n.outage.Active() {
		t.Fatalf("expected the outage to clear once a clean dual-track cycle committed with an empty backlog")
	}
}

func TestNoteCommittedBlockIgnoresNonDualTrackBlocks(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	n.outage.Declare()

	n.noteCommittedBlock(types.Block{Height: 5, DualTrack: false})
	if !n.outage.Active() {
		t.Fatalf("an ordinary (non-dual-track) commit must not count toward clearing the outage")
	}
}

// TestOutageEndToEndDualTrackRoundClearsOnQuorum drives the entire
// pipeline this task wires together, in the same white-box style as
// TestFullRoundReachesQuorumAndCommits: a real outage is declared, a real
// TxOffer is backlogged (not admitted live), a real dual-track proposal is
// built combining Track A (live mempool) and Track B (drained backlog),
// and once that proposal reaches real BFT quorum, the outage clears for
// real — not because any of these steps were individually mocked, but
// because tryFinalizeLocked's own noteCommittedBlock call fires on a
// genuinely DualTrack-flagged, genuinely quorum-committed block.
func TestOutageEndToEndDualTrackRoundClearsOnQuorum(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	height := n.chn.NextHeight()

	// AssignCommittee's proposer slot depends on the sorted-by-NFTID order
	// of whoever is online, which real key generation makes effectively
	// random relative to self — retry with fresh peers (bounded, real
	// keys each time) until a real draw puts self at committee[0], rather
	// than skip the test's main assertions on an unlucky draw.
	var p1, p2, p3 peerKey
	var committee []types.NFTID
	for attempt := 0; attempt < 100; attempt++ {
		// Reset to just self between attempts — registerOnline only adds,
		// so a stale peer from a previous attempt would otherwise linger
		// and skew both the committee size and its sorted order.
		n.mu.Lock()
		for id := range n.online {
			if id != n.identity {
				delete(n.online, id)
				delete(n.everSeen, id)
			}
		}
		n.mu.Unlock()

		p1, p2, p3 = genPeer(t), genPeer(t), genPeer(t)
		committee = registerOnline(n, height, p1, p2, p3)
		if committee[0] == n.identity {
			break
		}
	}
	if len(committee) != 4 {
		t.Fatalf("expected all 4 online validators in the committee, got %d", len(committee))
	}
	if committee[0] != n.identity {
		t.Fatalf("failed to draw a committee with self as proposer after 100 attempts")
	}

	n.outage.Declare()

	// A real TxOffer arrives while the outage is active: it must be
	// backlogged, not live-admitted.
	backlogged := mustSignVote(t, "backlog-proposal", 9)
	env, err := shadownet.NewEnvelope(shadownet.MsgTxOffer, shadownet.TxOfferPayload{Tx: backlogged})
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	n.handleMessage(peer.ID("wallet-peer"), env)
	if n.outage.BacklogDepth() != 1 {
		t.Fatalf("expected the tx to be backlogged, depth=%d", n.outage.BacklogDepth())
	}

	// maybePropose (via the real dual-track path) builds and self-processes
	// a proposal combining Track A (empty live mempool) and Track B (the
	// one backlogged tx).
	n.maybePropose()

	n.roundMu.Lock()
	r, ok := n.rounds[height]
	n.roundMu.Unlock()
	if !ok {
		t.Fatalf("expected a tracked round at height %d after maybePropose", height)
	}
	if !r.block.DualTrack {
		t.Fatalf("expected the self-proposed round's block to be flagged DualTrack during an active outage")
	}
	if len(r.batch) != 1 || r.batch[0].TxID != backlogged.TxID {
		t.Fatalf("expected the proposed batch to contain exactly the one backlogged tx, got %+v", r.batch)
	}

	// Two more real committee members vote for the same candidate: 3/4
	// clears BFTQuorumMet(4, 3).
	byPeer := map[types.NFTID]peerKey{p1.id: p1, p2.id: p2, p3.id: p3}
	voted := 0
	for _, id := range committee {
		if id == n.identity {
			continue
		}
		peerKey, ok := byPeer[id]
		if !ok {
			continue
		}
		sig, serr := crypto.DilithiumSign(peerKey.sk, r.candidate[:])
		if serr != nil {
			t.Fatalf("sign vote: %v", serr)
		}
		n.handleStageVote(shadownet.StageVotePayload{
			Height: height, Validator: id, CandidateHash: r.candidate, Sig: types.DilithiumSig(sig),
		})
		voted++
		if voted == 2 {
			break
		}
	}

	if n.chn.HeadHeight() != height {
		t.Fatalf("expected chain head to advance to %d, still at %d", height, n.chn.HeadHeight())
	}
	if n.outage.Active() {
		t.Fatalf("expected the outage to clear once the dual-track round reached quorum and committed")
	}
	if n.outage.BacklogDepth() != 0 {
		t.Fatalf("expected the backlog to be empty after the one backlogged tx was included and committed")
	}
}
