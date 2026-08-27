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
// genesisMs is the chain's genesis timestamp; tests that construct more
// than one node meant to converge on the same chain must pass the same
// genesisMs to each, since the genesis block (and therefore every
// PrevHash-linked block after it) is timestamp-dependent.
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
		Genesis:           consensus.GenesisTime(time.Now().UnixMilli()),
	}
	return NewNode(cfg, h, nil, store, tree, chn, nil, v, mempool, pk, sk, testLogf(t))
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
// and stage checks.
func mustSignVote(t *testing.T, proposalID string, commitment byte) types.ShieldedTx {
	t.Helper()
	pk, sk, err := crypto.GenerateDilithiumKey()
	if err != nil {
		t.Fatalf("generate signer key: %v", err)
	}
	in := types.ShieldedTx{
		Kind: types.TxVote,
		VotePublicInputs: &types.VotePublicInputs{
			ProposalID: types.ID(proposalID),
			Commitment: types.Hash{commitment},
		},
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
		n.recordOnline(p.id, p.pk, now)
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

	voteTx := mustSignVote(t, "proposal-1", 1)
	prop := shadownet.BlockProposalPayload{
		Height:    height,
		Epoch:     0,
		Proposer:  committee[0],
		Batch:     []types.ShieldedTx{voteTx},
		Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.mu.Lock()
	r, ok := n.rounds[height]
	n.mu.Unlock()
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
	n.mu.Lock()
	_, stillTracked := n.rounds[height]
	n.mu.Unlock()
	if stillTracked {
		t.Fatalf("expected the round to be cleaned up after finalization")
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

	voteTx := mustSignVote(t, "proposal-2", 2)
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: committee[0], Batch: []types.ShieldedTx{voteTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.mu.Lock()
	r, ok := n.rounds[height]
	n.mu.Unlock()
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

	n.mu.Lock()
	r2, ok := n.rounds[height]
	votes := len(r2.votes)
	n.mu.Unlock()
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
	voteTx := mustSignVote(t, proposalID, 3)
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: committee[0], Batch: []types.ShieldedTx{voteTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.mu.Lock()
	r, ok := n.rounds[height]
	if !ok {
		n.mu.Unlock()
		t.Fatalf("expected a tracked round")
	}
	r.deadline = time.Now().Add(-time.Second) // force expiry
	n.mu.Unlock()

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

	n.mu.Lock()
	_, stillTracked := n.rounds[height]
	n.mu.Unlock()
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

	n.mu.Lock()
	_, ok := n.rounds[height]
	n.mu.Unlock()
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

	voteTx := mustSignVote(t, "proposal-5", 5)
	prop := shadownet.BlockProposalPayload{
		Height: height, Proposer: impostor, Batch: []types.ShieldedTx{voteTx}, Timestamp: time.Now().UnixMilli(),
	}
	n.handleBlockProposal(prop)

	n.mu.Lock()
	_, ok := n.rounds[height]
	n.mu.Unlock()
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

	voteTx := mustSignVote(t, "proposal-6", 6)
	if err := n.mempool.Submit(voteTx, time.Now()); err != nil {
		t.Fatalf("submit to mempool: %v", err)
	}

	n.maybePropose()

	n.mu.Lock()
	_, ok := n.rounds[height]
	n.mu.Unlock()

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
	adopter.recordOnline(proposer.identity, proposer.pk, time.Now())
	for _, e := range extra[1:] {
		adopter.recordOnline(e.id, e.pk, time.Now())
	}

	voteTx := mustSignVote(t, "proposal-7", 7)
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
