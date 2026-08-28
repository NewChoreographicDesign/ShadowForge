package validator

import (
	"testing"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/chain"
	"github.com/shadowforge/shadowforge-l1/pkg/consensus"
	"github.com/shadowforge/shadowforge-l1/pkg/crypto"
	shadownet "github.com/shadowforge/shadowforge-l1/pkg/net"
	"github.com/shadowforge/shadowforge-l1/pkg/state"
	"github.com/shadowforge/shadowforge-l1/pkg/tx"
	"github.com/shadowforge/shadowforge-l1/pkg/types"
	"github.com/shadowforge/shadowforge-l1/pkg/vault"
)

// These are white-box tests (package validator, not validator_test): they
// call unexported methods (handleBlockProposal, handleStageVote,
// tryFinalize, sweepTimeouts, recordOnline) directly to exercise the
// propose/vote/commit state machine deterministically, without depending
// on real network delivery timing. Every signature, every state root
// comparison, and every quorum check still goes through the exact same
// real Dilithium/Badger/chain.Append code a networked run would use — only
// the transport hop (shadownet.Node.Send/Broadcast) is bypassed, which is
// exactly what the genuine multi-process integration test (separate from
// this file) exists to cover.

func testLogf(t *testing.T) Logf {
	return func(format string, args ...interface{}) { t.Logf(format, args...) }
}

func openTestStore(t *testing.T) *state.Store {
	t.Helper()
	var key [32]byte
	copy(key[:], []byte("validator-test-key-32-bytes-pad!"))
	s, err := state.Open("", true, key)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newTestNode builds one real validator.Node: a real libp2p host, a real
// Badger-backed store, a real chain, and a real Dilithium identity keypair.
// genesisMs anchors both the chain's genesis block and the epoch clock's
// Genesis reference (consensus.CurrentEpoch) to the same moment, matching
// how a real deployment's genesis time is one coherent value, not two.
// Tests that construct more than one node meant to converge on the same
// chain must pass the same genesisMs to each, since the genesis block
// (and therefore every PrevHash-linked block after it) is
// timestamp-dependent.
func newTestNode(t *testing.T, roundTimeout time.Duration, genesisMs int64) *Node {
	t.Helper()
	h, err := shadownet.NewHost("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })

	store := openTestStore(t)
	tree := state.NewMerkleTree()
	chn, err := chain.Open(store, genesisMs)
	if err != nil {
		t.Fatalf("open chain: %v", err)
	}
	v := vault.New(vault.DefaultSplits())
	mempool := tx.NewMempool()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}

	cfg := Config{
		BatchInterval:     time.Second,
		RoundTimeout:      roundTimeout,
		HeartbeatInterval: consensus.HeartbeatInterval,
		OnlineTimeout:     time.Minute,
		Genesis:           consensus.GenesisTime(genesisMs),
	}
	return NewNode(cfg, h, nil, store, tree, chn, nil, v, nil, nil, mempool, pk, sk, false, testLogf(t))
}

type peerKey struct {
	id types.NFTID
	pk crypto.DilithiumPublicKey
	sk crypto.DilithiumPrivateKey
}

func genPeer(t *testing.T) peerKey {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate peer key: %v", err)
	}
	return peerKey{id: types.NFTID(types.SumHash(pk)), pk: pk, sk: sk}
}

// mustSignVote builds a real, correctly-signed, minimal-cost TxVote
// transaction: TxVote needs no ZK system (unlike TxTransfer), so it keeps
// these tests fast while still exercising the pipeline's real signature
// and stage checks. It also seeds a real, minted ValidatorNFT for the
// fresh signer key directly into n's store — real voter eligibility
// (pkg/tx's requireEligibleVoter) is unconditional, not something these
// pipeline-behavior tests are trying to exercise, so giving every
// generated voter a real NFT up front keeps them passing exactly as
// before that check existed.
func mustSignVote(t *testing.T, n *Node, proposalID string, commitment byte) types.ShieldedTx {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	owner := types.AddressFromPubkey(pk)
	if err := n.store.PutNFT(types.ValidatorNFT{ID: types.NFTID(types.SumHash(owner[:])), Owner: owner}); err != nil {
		t.Fatalf("seed voter nft: %v", err)
	}
	in := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID(proposalID),
			Commitment: types.Hash{commitment},
		},
		// TxID = Hash(proof || commitments || nullifier); none of those
		// vary with proposalID/commitment for a Vote tx, so every call
		// would otherwise collide on the same TxID (this is exactly the
		// bug this build found for real in cmd/walletsim). Nullifier
		// here is purely this call's uniqueifier, using the fresh
		// per-call pk as real, non-attacker-chosen entropy.
		Nullifier: types.SumHash([]byte(pk), []byte(proposalID), []byte{commitment}),
	}
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	return in
}

// registerOnline records n's own identity plus extra peers into n's online
// registry (as if each had already heartbeated), and returns the resulting
// deterministic committee for height.
func registerOnline(n *Node, height uint64, extra ...peerKey) []types.NFTID {
	now := time.Now()
	for _, p := range extra {
		n.recordOnline(p.id, p.pk, false, now)
	}
	online := n.onlineSet(now)
	return consensus.AssignCommittee(online, height, committeeSize(len(online)))
}

// TestFullRoundReachesQuorumAndCommits proves the entire real state
// machine end to end: a proposal is processed through the real pipeline,
// this node casts a real signed vote, two more real-keyed committee votes
// arrive, quorum is independently reverified by chain.Append (real
// signature checks against a real committee), and the chain head
// genuinely advances with a persisted block.
func TestFullRoundReachesQuorumAndCommits(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)
	if len(committee) != 4 {
		t.Fatalf("expected all 4 online validators in the committee, got %d", len(committee))
	}

	voteTx := mustSignVote(t, n, "proposal-1", 1)
	prop := shadownet.BlockProposalPayload{
		Height:    height,
		Epoch:     0,
		Proposer:  committee[0],
		Batch:     []types.ShieldedTx{voteTx},
		Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.roundMu.Lock()
	r, ok := n.rounds[height]
	n.roundMu.Unlock()
	if !ok {
		t.Fatalf("expected a tracked round at height %d after a valid proposal", height)
	}
	if len(r.votes) != 1 {
		t.Fatalf("expected the node's own vote to be recorded, got %d votes", len(r.votes))
	}

	// Two more real committee members vote for the same candidate: 3/4
	// clears BFTQuorumMet(4, 3) (3*2=6 > 4).
	byPeer := map[types.NFTID]peerKey{p1.id: p1, p2.id: p2, p3.id: p3}
	voted := 0
	for _, id := range committee {
		if id == n.identity {
			continue
		}
		peer, ok := byPeer[id]
		if !ok {
			continue
		}
		sig, err := crypto.DilithiumSign(peer.sk, r.candidate[:])
		if err != nil {
			t.Fatalf("sign vote: %v", err)
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
	if n.chn.HeadHash() != r.candidate {
		t.Fatalf("expected chain head hash to equal the finalized candidate")
	}
	n.roundMu.Lock()
	_, stillTracked := n.rounds[height]
	n.roundMu.Unlock()
	if stillTracked {
		t.Fatalf("expected the round to be cleaned up after finalization")
	}
}

// TestMaybeProposeRespectsMaxBatchSize proves the mempool drain a
// proposal is built from is genuinely bounded, not "drain everything":
// unbounded draining combined with rejected batches returning their valid
// transactions to the mempool (see the next test) lets one recurring bad
// entry force an ever-larger batch on every retry — this build hit
// Badger's real 1MB per-value limit doing exactly that under sustained
// traffic. A bounded drain keeps each attempt's cost, and worst-case
// serialized block size, bounded regardless of backlog size.
func TestMaybeProposeRespectsMaxBatchSize(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	n.cfg.MaxBatchSize = 3

	const submitted = 10
	for i := 0; i < submitted; i++ {
		if err := n.mempool.Submit(mustSignVote(t, n, "proposal-cap", byte(i)), time.Now()); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	// A lone online validator can no longer self-quorum (consensus.
	// MinCommitteeSize — real fork found live otherwise), so this test
	// needs a real second committee member whose vote actually reaches
	// quorum. Retry with a fresh peer until the real, key-derived
	// committee order puts self at committee[0] (this test is
	// specifically about maybePropose's own batch-building, which only
	// runs on this node's proposing turn).
	height := n.chn.NextHeight()
	var committee []types.NFTID
	var peer peerKey
	for attempt := 0; attempt < 100; attempt++ {
		n.mu.Lock()
		for id := range n.online {
			if id != n.identity {
				delete(n.online, id)
				delete(n.everSeen, id)
			}
		}
		n.mu.Unlock()
		// Refresh self's own online record too — it is stamped once at
		// construction and can otherwise silently age past OnlineTimeout
		// during a long-running retry loop, making committee[0] ==
		// n.identity permanently impossible for the rest of the attempts.
		n.recordOnline(n.identity, n.pk, n.isSentinel, time.Now())
		peer = genPeer(t)
		committee = registerOnline(n, height, peer)
		if committee[0] == n.identity {
			break
		}
	}
	if committee[0] != n.identity {
		t.Fatalf("failed to draw a committee with self as proposer after 100 attempts")
	}

	n.maybePropose()

	n.roundMu.Lock()
	r, tracked := n.rounds[height]
	n.roundMu.Unlock()
	if !tracked {
		t.Fatalf("expected maybePropose to start a round at height %d", height)
	}
	sig, err := crypto.DilithiumSign(peer.sk, r.candidate[:])
	if err != nil {
		t.Fatalf("sign vote: %v", err)
	}
	n.handleStageVote(shadownet.StageVotePayload{
		Height: height, Validator: peer.id, CandidateHash: r.candidate, Sig: types.DilithiumSig(sig),
	})

	if n.chn.HeadHeight() != height {
		t.Fatalf("expected maybePropose to commit height %d, still at %d", height, n.chn.HeadHeight())
	}
	block, found, err := n.store.GetBlock(height)
	if err != nil || !found {
		t.Fatalf("expected a persisted block: found=%v err=%v", found, err)
	}
	if len(block.Batch) != 3 {
		t.Fatalf("expected the committed batch to be capped at MaxBatchSize=3, got %d", len(block.Batch))
	}
	if got := n.mempool.Len(); got != submitted-3 {
		t.Fatalf("expected %d entries left in the mempool, got %d", submitted-3, got)
	}
}

// TestTryFinalizeReinsertsBatchOnChainAppendFailure proves that even
// when a round reaches local quorum but chain.Append itself rejects it
// (this build observed this for real: a stale round finalizing against
// an already-advanced head), the batch's transactions are still returned
// to the mempool — matching sweepTimeouts' timeout path — instead of
// silently vanishing along with the round.
func TestTryFinalizeReinsertsBatchOnChainAppendFailure(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)

	// Advance the head past `height` first, via a real, independent round
	// — so a second round still targeting `height` is now stale.
	firstTx := mustSignVote(t, n, "proposal-first", 1)
	n.handleBlockProposal(shadownet.BlockProposalPayload{
		Height: height, Proposer: committee[0], Batch: []types.ShieldedTx{firstTx}, Timestamp: time.Now().UnixMilli(),
	})
	n.roundMu.Lock()
	firstRound, ok := n.rounds[height]
	n.roundMu.Unlock()
	if !ok {
		t.Fatalf("expected the first proposal to start a round")
	}
	byPeerFirst := map[types.NFTID]peerKey{p1.id: p1, p2.id: p2, p3.id: p3}
	votedFirst := 0
	for _, id := range committee {
		if id == n.identity || votedFirst == 2 {
			continue
		}
		peer, ok := byPeerFirst[id]
		if !ok {
			continue
		}
		sig, err := crypto.DilithiumSign(peer.sk, firstRound.candidate[:])
		if err != nil {
			t.Fatalf("sign vote: %v", err)
		}
		n.handleStageVote(shadownet.StageVotePayload{
			Height: height, Validator: id, CandidateHash: firstRound.candidate, Sig: types.DilithiumSig(sig),
		})
		votedFirst++
	}
	if n.chn.HeadHeight() != height {
		t.Fatalf("expected the first round to commit height %d", height)
	}

	// Build a second, stale round by hand for the same (now-past) height,
	// with its own real, distinct, valid transaction, and drive it
	// straight to tryFinalizeLocked — bypassing handleBlockProposal's own
	// height check, since this test needs the round to exist so
	// tryFinalizeLocked's own chain.Append call is what fails.
	staleTx := mustSignVote(t, n, "proposal-stale", 2)
	txn := n.store.BeginTxn()
	entries := []tx.Entry{{Tx: staleTx, SubmittedAt: time.Now()}}
	pipeline := tx.NewPipeline(tx.Deps{Store: txn, StateTree: n.tree, Vault: n.vlt, Now: time.Now})
	if res := pipeline.ProcessBatch(entries); res[0].Error != nil {
		t.Fatalf("pipeline: %v", res[0].Error)
	}
	block := types.Block{Height: height, Epoch: 0, PrevHash: types.Hash{0xFF}, Batch: []types.ShieldedTx{staleTx}}
	candidate := types.HashBlock(block)
	sig, err := crypto.DilithiumSign(n.sk, candidate[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	r := &round{
		height: height, committee: committee, batch: []types.ShieldedTx{staleTx}, txn: txn,
		block: block, candidate: candidate,
		votes:    []types.Vote{{Validator: n.identity, StateRoot: candidate, Sig: types.DilithiumSig(sig)}},
		deadline: time.Now().Add(time.Minute),
	}
	byPeer := map[types.NFTID]peerKey{p1.id: p1, p2.id: p2, p3.id: p3}
	for _, id := range committee {
		if id == n.identity || len(r.votes) >= 3 {
			continue
		}
		peer, ok := byPeer[id]
		if !ok {
			continue
		}
		sig, err := crypto.DilithiumSign(peer.sk, candidate[:])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		r.votes = append(r.votes, types.Vote{Validator: id, StateRoot: candidate, Sig: types.DilithiumSig(sig)})
	}

	n.roundMu.Lock()
	n.rounds[height] = r
	n.tryFinalizeLocked(r)
	n.roundMu.Unlock()

	if n.mempool.Len() != 1 {
		t.Fatalf("expected the stale round's transaction to be recovered into the mempool, got %d entries", n.mempool.Len())
	}
	recovered := n.mempool.DrainBatch(0)
	if len(recovered) != 1 || recovered[0].Tx.TxID != staleTx.TxID {
		t.Fatalf("expected to recover exactly the stale round's tx, got %+v", recovered)
	}
}

// TestForgedVoteDoesNotCountTowardQuorum proves a vote "signed" by an
// attacker key while claiming a legitimate committee member's identity is
// dropped, not counted — chain.Append (and handleStageVote's own
// pre-check) independently re-verify every signature against the real
// public key on file, never trusting the claimed identity.
func TestForgedVoteDoesNotCountTowardQuorum(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)

	voteTx := mustSignVote(t, n, "proposal-2", 2)
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: committee[0], Batch: []types.ShieldedTx{voteTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.roundMu.Lock()
	r, ok := n.rounds[height]
	n.roundMu.Unlock()
	if !ok {
		t.Fatalf("expected a tracked round")
	}

	var target types.NFTID
	for _, id := range committee {
		if id != n.identity {
			target = id
			break
		}
	}

	attacker, _, err := crypto.GenerateDilithiumKey()
	_ = attacker
	_, attackerSK, err2 := crypto.GenerateDilithiumKey()
	if err != nil || err2 != nil {
		t.Fatalf("generate attacker key: %v / %v", err, err2)
	}
	forgedSig, err := crypto.DilithiumSign(attackerSK, r.candidate[:])
	if err != nil {
		t.Fatalf("sign forged vote: %v", err)
	}
	n.handleStageVote(shadownet.StageVotePayload{
		Height: height, Validator: target, CandidateHash: r.candidate, Sig: types.DilithiumSig(forgedSig),
	})

	n.roundMu.Lock()
	r2, ok := n.rounds[height]
	votes := len(r2.votes)
	n.roundMu.Unlock()
	if !ok {
		t.Fatalf("round should still be pending (quorum not reached)")
	}
	if votes != 1 {
		t.Fatalf("expected the forged vote to be dropped (still just the node's own vote), got %d votes", votes)
	}
	if n.chn.HeadHeight() != height-1 {
		t.Fatalf("chain must not advance on a forged vote")
	}
}

// TestRoundRollsBackOnTimeout proves an in-flight round that never reaches
// quorum is genuinely discarded: its tentative Badger writes (verified via
// the proposal record a TxVote's Stage 4 would otherwise persist) never
// become visible on the node's real store, and its transactions are
// returned to the mempool for retry.
func TestRoundRollsBackOnTimeout(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli()) // long timeout; we force expiry manually
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)

	const proposalID = "proposal-3"
	voteTx := mustSignVote(t, n, proposalID, 3)
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: committee[0], Batch: []types.ShieldedTx{voteTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.roundMu.Lock()
	r, ok := n.rounds[height]
	if !ok {
		n.roundMu.Unlock()
		t.Fatalf("expected a tracked round")
	}
	r.deadline = time.Now().Add(-time.Second) // force expiry
	n.roundMu.Unlock()

	// The tentative write must not be visible on the real store yet — it
	// only exists inside the round's still-open, uncommitted state.Txn.
	if _, found, err := n.store.GetProposal(proposalID); err != nil {
		t.Fatalf("get proposal: %v", err)
	} else if found {
		t.Fatalf("expected the tentative proposal write to not yet be visible on the committed store")
	}

	if n.mempool.Len() != 0 {
		t.Fatalf("mempool should be empty before rollback (tx came in via direct proposal, not mempool)")
	}

	n.sweepTimeouts()

	n.roundMu.Lock()
	_, stillTracked := n.rounds[height]
	n.roundMu.Unlock()
	if stillTracked {
		t.Fatalf("expected the timed-out round to be removed")
	}
	if _, found, err := n.store.GetProposal(proposalID); err != nil {
		t.Fatalf("get proposal: %v", err)
	} else if found {
		t.Fatalf("expected the rolled-back proposal write to remain absent from the store")
	}
	if n.mempool.Len() != 1 {
		t.Fatalf("expected the rolled-back tx to be resubmitted to the mempool, got %d entries", n.mempool.Len())
	}
	if n.chn.HeadHeight() != height-1 {
		t.Fatalf("chain must not advance when a round times out")
	}
}

// TestHandleBlockProposalRejectsInvalidTx proves a proposal containing a
// transaction that fails real pipeline validation (here: an unsigned
// TxVote, which fails Stage 2's real Dilithium signature check) never
// creates a round and leaves no trace in the state tree — the batch is
// rejected as a whole, not partially applied.
func TestHandleBlockProposalRejectsInvalidTx(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)

	preLen := n.tree.Len()
	badTx := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-4"),
			Commitment: types.Hash{4},
		},
		// No Sig / SignerPubKey: must fail Stage 2's real signature check.
	}
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: committee[0], Batch: []types.ShieldedTx{badTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.roundMu.Lock()
	_, ok := n.rounds[height]
	n.roundMu.Unlock()
	if ok {
		t.Fatalf("expected no round to be created for a proposal containing an invalid tx")
	}
	if n.tree.Len() != preLen {
		t.Fatalf("expected the state tree to be untouched after a rejected proposal, got %d want %d", n.tree.Len(), preLen)
	}
	if n.chn.HeadHeight() != height-1 {
		t.Fatalf("chain must not advance for a rejected proposal")
	}
}

// TestHandleBlockProposalRecoversValidTxsFromRejectedBatch proves that
// when one bad transaction sinks an entire proposal (quorum votes on one
// deterministic candidate root, so a batch can't be partially included),
// every OTHER, individually-valid transaction in that same batch is
// returned to the mempool rather than silently lost — a real network
// running real traffic hits exactly this (two nodes both admit the same
// legitimately-committed vote into their own separate mempools before
// gossip settles; whichever proposes second finds it already committed
// and must reject that one tx without losing the rest of its batch).
func TestHandleBlockProposalRecoversValidTxsFromRejectedBatch(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)

	goodTx := mustSignVote(t, n, "proposal-recover", 1)
	badTx := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID("proposal-bad"),
			Commitment: types.Hash{2},
		},
		// No Sig / SignerPubKey: fails Stage 2's real signature check.
	}
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: committee[0], Batch: []types.ShieldedTx{goodTx, badTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.roundMu.Lock()
	_, ok := n.rounds[height]
	n.roundMu.Unlock()
	if ok {
		t.Fatalf("expected no round to be created: the batch as a whole must still be rejected")
	}
	if n.chn.HeadHeight() != height-1 {
		t.Fatalf("chain must not advance for a rejected proposal")
	}
	if n.mempool.Len() != 1 {
		t.Fatalf("expected the valid transaction to be recovered into the mempool, got %d entries", n.mempool.Len())
	}
	recovered := n.mempool.DrainBatch(0)
	if len(recovered) != 1 || recovered[0].Tx.TxID != goodTx.TxID {
		t.Fatalf("expected to recover exactly the valid tx, got %+v", recovered)
	}
}

// TestHandleBlockProposalRejectsWrongProposer proves a proposal claiming to
// be from someone other than the deterministically-assigned proposer
// (committee[0]) is rejected outright, regardless of how well-formed its
// batch is.
func TestHandleBlockProposalRejectsWrongProposer(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)

	var impostor types.NFTID
	for _, id := range committee {
		if id != committee[0] {
			impostor = id
			break
		}
	}

	voteTx := mustSignVote(t, n, "proposal-5", 5)
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: impostor, Batch: []types.ShieldedTx{voteTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.roundMu.Lock()
	_, ok := n.rounds[height]
	n.roundMu.Unlock()
	if ok {
		t.Fatalf("expected the proposal from a non-assigned proposer to be rejected")
	}
}

// TestMaybeProposeOnlySelfProposesOnItsOwnTurn proves maybePropose (the
// self-initiated half of the state machine, as opposed to
// handleBlockProposal reacting to someone else's message) only starts a
// round when this node's own identity is the deterministically-assigned
// proposer for the next height.
func TestMaybeProposeOnlySelfProposesOnItsOwnTurn(t *testing.T) {
	n := newTestNode(t, time.Minute, time.Now().UnixMilli())
	p1, p2, p3 := genPeer(t), genPeer(t), genPeer(t)
	height := n.chn.NextHeight()
	committee := registerOnline(n, height, p1, p2, p3)

	voteTx := mustSignVote(t, n, "proposal-6", 6)
	if err := n.mempool.Submit(voteTx, time.Now()); err != nil {
		t.Fatalf("submit to mempool: %v", err)
	}

	n.maybePropose()

	n.roundMu.Lock()
	_, ok := n.rounds[height]
	n.roundMu.Unlock()

	if committee[0] == n.identity {
		if !ok {
			t.Fatalf("expected self to propose and start a round on its own turn")
		}
	} else {
		if ok {
			t.Fatalf("expected self NOT to propose when it is not the assigned proposer")
		}
		if n.mempool.Len() != 1 {
			t.Fatalf("expected the queued tx to remain in the mempool when this node doesn't propose")
		}
	}
}

// TestHandleBlockAnnounceAdoptsIndependentlyVerifiedBlock proves a node
// that never voted on a batch itself (it wasn't tracking any round for
// that height) can still adopt a block purely by replaying the batch
// through its own real pipeline and independently re-verifying quorum —
// it never simply trusts an announced StateRoot or vote set.
func TestHandleBlockAnnounceAdoptsIndependentlyVerifiedBlock(t *testing.T) {
	genesisMs := time.Now().UnixMilli()
	proposer := newTestNode(t, time.Minute, genesisMs)
	adopter := newTestNode(t, time.Minute, genesisMs)

	// Give both nodes an identical view of the online set (same committee)
	// so replay on the adopter recomputes the same committee as the
	// proposer used.
	height := proposer.chn.NextHeight()
	extra := []peerKey{
		{id: adopter.identity, pk: adopter.pk},
		genPeer(t), genPeer(t),
	}
	committeeProposer := registerOnline(proposer, height, extra...)
	// adopter must see the same online set (itself + proposer + the two extras)
	adopter.recordOnline(proposer.identity, proposer.pk, false, time.Now())
	for _, e := range extra[1:] {
		adopter.recordOnline(e.id, e.pk, false, time.Now())
	}

	// voteTx's real voter NFT must exist on BOTH nodes' stores: adopter
	// independently re-verifies this transaction (including real voter
	// eligibility) while replay-adopting the block below, exactly as
	// proposer does while first processing it.
	voteTx := mustSignVote(t, proposer, "proposal-7", 7)
	owner := types.AddressFromPubkey(voteTx.SignerPubKey)
	if err := adopter.store.PutNFT(types.ValidatorNFT{ID: types.NFTID(types.SumHash(owner[:])), Owner: owner}); err != nil {
		t.Fatalf("seed voter nft on adopter: %v", err)
	}
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: committeeProposer[0], Batch: []types.ShieldedTx{voteTx}, Timestamp: time.Now().UnixMilli(),
	}
	proposer.handleBlockProposal(prop)

	proposer.mu.Lock()
	r := proposer.rounds[height]
	proposer.mu.Unlock()
	if r == nil {
		t.Fatalf("expected proposer to track a round")
	}
	// Fabricate the remaining votes needed for quorum, from the extras'
	// real keys (the proposer node itself already contributed one).
	byID := map[types.NFTID]peerKey{}
	for _, e := range extra {
		if e.sk != nil {
			byID[e.id] = e
		}
	}
	votes := append([]types.Vote(nil), r.votes...)
	for _, id := range committeeProposer {
		if len(votes) >= 3 {
			break
		}
		if id == proposer.identity || id == adopter.identity {
			continue
		}
		p, ok := byID[id]
		if !ok {
			continue
		}
		sig, err := crypto.DilithiumSign(p.sk, r.candidate[:])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		votes = append(votes, types.Vote{Validator: id, StateRoot: r.candidate, Sig: types.DilithiumSig(sig)})
	}
	block := r.block
	block.Votes = votes

	adopter.handleBlockAnnounce(shadownet.BlockAnnouncePayload{Block: block})

	if adopter.chn.HeadHeight() != height {
		t.Fatalf("expected adopter to adopt the announced block, head still at %d", adopter.chn.HeadHeight())
	}
	if want := types.HashBlock(block); adopter.chn.HeadHash() != want {
		t.Fatalf("expected adopter's head hash to equal the announced block's own hash after independent replay: got %s want %s", adopter.chn.HeadHash(), want)
	}
}

// signVoteTx signs a fully-formed ShieldedTx with a caller-supplied
// keypair, for tests that need control over both the signer identity and
// the tx's Kind/public inputs (mustSignVote only ever builds a TxVote with
// a fresh random key).
func signVoteTx(t *testing.T, pk crypto.DilithiumPublicKey, sk crypto.DilithiumPrivateKey, in types.ShieldedTx) types.ShieldedTx {
	t.Helper()
	in.TxID = types.ComputeTxID(in.Proof, in.Commitments, in.Nullifier)
	sig, err := crypto.DilithiumSign(sk, in.TxID[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	in.Sig = types.DilithiumSig(sig)
	in.SignerPubKey = []byte(pk)
	return in
}

// TestEpochBoundaryTallyRunsOnRealProposal proves the epoch-boundary
// governance tally (spec 17.4) is wired for real into the block-commit
// path, driven by genuine wall-clock epoch progression (consensus.
// CurrentEpoch), not a fabricated or injectable clock: a ballot committed
// and revealed in epoch 0 is untallied until a block genuinely lands in
// epoch 1, at which point handleBlockProposal's real
// pipeline.TallyDueProposals call tallies it — and the independent Epoch
// reverification added alongside this (guarding against a proposer
// claiming an arbitrary epoch) does not itself block a genuinely
// epoch-appropriate proposal.
func TestEpochBoundaryTallyRunsOnRealProposal(t *testing.T) {
	// genesisMs is chosen so epoch 0 (a fixed 1 hour) has just a few
	// seconds left to run when the test starts, so the real epoch
	// boundary arrives quickly instead of requiring an actual hour-long
	// wait. The margin is generous (not e.g. 300ms) because this must
	// still hold under a loaded `go test ./...` run competing with other
	// packages' tests for CPU, not just in isolation.
	const epoch0Remaining = 3 * time.Second
	genesisMs := time.Now().Add(-(time.Hour - epoch0Remaining)).UnixMilli()
	n := newTestNode(t, time.Minute, genesisMs)

	if got := consensus.CurrentEpoch(n.cfg.Genesis, time.Now()); got != 0 {
		t.Fatalf("expected the test to start in epoch 0, got %d", got)
	}

	// A lone online validator can no longer self-quorum (consensus.
	// MinCommitteeSize — a real fork found live otherwise); register a
	// real second committee member whose vote both blocks below actually
	// need. Retry with a fresh peer until self lands at committee[0] for
	// height1 specifically (this test hardcodes its Proposer).
	height1 := n.chn.NextHeight()
	var committee1 []types.NFTID
	var peer peerKey
	for attempt := 0; attempt < 100; attempt++ {
		n.mu.Lock()
		for id := range n.online {
			if id != n.identity {
				delete(n.online, id)
				delete(n.everSeen, id)
			}
		}
		n.mu.Unlock()
		// Refresh self's own online record too — it is stamped once at
		// construction and can otherwise silently age past OnlineTimeout
		// during a long-running retry loop, making committee1[0] ==
		// n.identity permanently impossible for the rest of the attempts.
		n.recordOnline(n.identity, n.pk, n.isSentinel, time.Now())
		peer = genPeer(t)
		committee1 = registerOnline(n, height1, peer)
		if committee1[0] == n.identity {
			break
		}
	}
	if committee1[0] != n.identity {
		t.Fatalf("failed to draw a committee with self as proposer for height1 after 100 attempts")
	}

	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate voter key: %v", err)
	}
	owner := types.AddressFromPubkey(pk)
	if err := n.store.PutNFT(types.ValidatorNFT{ID: types.NFTID(types.SumHash(owner[:])), Owner: owner}); err != nil {
		t.Fatalf("seed voter nft: %v", err)
	}
	voter := types.NFTID(types.SumHash(pk))
	nonce := types.Hash{5}
	commitment := types.ComputeVoteCommitment(voter, true, nonce)
	commitTx := signVoteTx(t, pk, sk, types.ShieldedTx{
		Kind:             types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{ProposalID: types.ID("epoch-tally-proposal"), Commitment: commitment},
	})
	revealTx := signVoteTx(t, pk, sk, types.ShieldedTx{
		Kind: types.TxVoteReveal,
		VoteRevealPublicInputs: &types.VoteRevealPublicInputs{
			ProposalID: types.ID("epoch-tally-proposal"), Approve: true, Nonce: nonce,
		},
	})

	n.handleBlockProposal(shadownet.BlockProposalPayload{
		Height: height1, Epoch: 0, Proposer: n.identity,
		Batch: []types.ShieldedTx{commitTx, revealTx}, Timestamp: time.Now().UnixMilli(),
	})
	n.roundMu.Lock()
	r1, tracked1 := n.rounds[height1]
	n.roundMu.Unlock()
	if !tracked1 {
		t.Fatalf("expected a tracked round at height1")
	}
	sig1, err := crypto.DilithiumSign(peer.sk, r1.candidate[:])
	if err != nil {
		t.Fatalf("sign height1 vote: %v", err)
	}
	n.handleStageVote(shadownet.StageVotePayload{
		Height: height1, Validator: peer.id, CandidateHash: r1.candidate, Sig: types.DilithiumSig(sig1),
	})
	if n.chn.HeadHeight() != height1 {
		t.Fatalf("expected the epoch-0 commit+reveal block to commit, head at %d", n.chn.HeadHeight())
	}
	record, found, err := n.store.GetProposal("epoch-tally-proposal")
	if err != nil || !found {
		t.Fatalf("expected a persisted proposal record: found=%v err=%v", found, err)
	}
	if record.Tallied {
		t.Fatalf("must not be tallied yet: still in the proposal's own epoch")
	}

	// Wait for the real epoch boundary to actually pass.
	deadline := time.Now().Add(15 * time.Second)
	for consensus.CurrentEpoch(n.cfg.Genesis, time.Now()) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("epoch never advanced to 1 within the deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Any further block, now genuinely proposed in epoch 1, triggers the
	// tally for the epoch-0 proposal as a side effect of being committed.
	dummyPK, dummySK, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate second signer key: %v", err)
	}
	dummyOwner := types.AddressFromPubkey(dummyPK)
	if err := n.store.PutNFT(types.ValidatorNFT{ID: types.NFTID(types.SumHash(dummyOwner[:])), Owner: dummyOwner}); err != nil {
		t.Fatalf("seed dummy voter nft: %v", err)
	}
	dummyTx := signVoteTx(t, dummyPK, dummySK, types.ShieldedTx{
		Kind:             types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{ProposalID: types.ID("unrelated-proposal"), Commitment: types.Hash{9}},
	})
	nowEpoch := consensus.CurrentEpoch(n.cfg.Genesis, time.Now())
	height2 := n.chn.NextHeight()
	// Committee rotation guarantees a DIFFERENT committee[0] than height1
	// now that a real 2-member committee can't self-quorum (height1's
	// proposer can't also be height2's under real rotation) — use
	// whichever the real committee actually assigns rather than
	// hardcoding self, and have the other real member cast the second
	// vote quorum needs.
	onlineNow := n.onlineSet(time.Now())
	committee2 := consensus.AssignCommittee(onlineNow, height2, committeeSize(len(onlineNow)))
	if len(committee2) != 2 {
		t.Fatalf("expected a real 2-member committee at height2, got %v", committee2)
	}
	n.handleBlockProposal(shadownet.BlockProposalPayload{
		Height: height2, Epoch: nowEpoch, Proposer: committee2[0],
		Batch: []types.ShieldedTx{dummyTx}, Timestamp: time.Now().UnixMilli(),
	})
	n.roundMu.Lock()
	r2, tracked2 := n.rounds[height2]
	n.roundMu.Unlock()
	if !tracked2 {
		t.Fatalf("expected a tracked round at height2")
	}
	sig2, err := crypto.DilithiumSign(peer.sk, r2.candidate[:])
	if err != nil {
		t.Fatalf("sign height2 vote: %v", err)
	}
	n.handleStageVote(shadownet.StageVotePayload{
		Height: height2, Validator: peer.id, CandidateHash: r2.candidate, Sig: types.DilithiumSig(sig2),
	})
	if n.chn.HeadHeight() != height2 {
		t.Fatalf("expected the epoch-1 block to commit (epoch reverification must not block a genuinely current epoch), head at %d", n.chn.HeadHeight())
	}

	record, found, err = n.store.GetProposal("epoch-tally-proposal")
	if err != nil || !found {
		t.Fatalf("get proposal: found=%v err=%v", found, err)
	}
	if !record.Tallied {
		t.Fatalf("expected the epoch-0 proposal to be tallied once a block genuinely landed in epoch 1")
	}
	if record.Approve != 1 || record.Reject != 0 || !record.Passed {
		t.Fatalf("unexpected tally result: %+v", record)
	}
}
